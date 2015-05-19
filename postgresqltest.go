package postgresqltest

/*
 this script simply does

 % cd /tmp
 % mkdir -p _pgsql/tmp
 % /usr/lib/postgresql/9.3/bin/initdb -A trust -D _pgsql/data
 % /usr/lib/postgresql/9.3/bin/postmaster -p 15432
     -D /tmp/_pgsql/data -k /tmp/_pgsql/tmp
 # then you can connect via unix domain socket
 % psql --port 15432 --host /tmp/_pgsql/tmp template1
*/

import (
	"database/sql"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	_ "github.com/lib/pq"
)

var SearchPaths = []string{
	// popular installtion dir?
	"/usr/local/pgsql",
	// ubuntu (maybe debian as well, find the newest version)
	"/usr/lib/postgresql/*",
	// macport
	"/usr/local/lib/postgresql-*",
}

// PostgreSQLConfig is used to configure the new postgresql instance
type PostgreSQLConfig struct {
	BaseDir        string
	DataDir        string
	PidFile        string
	Port           int
	TmpDir         string
	InitDB         string
	InitDBArgs     string
	Postmaster     string
	PostmasterArgs string
	AutoStart      int
}

// TestPostgreSQL is the main struct that handles the execution of postgresql
type TestPostgreSQL struct {
	Config  *PostgreSQLConfig
	Command *exec.Cmd
	Guards  []func()
	LogFile string
}

// NewConfig creates a new PostgreSQLConfig struct with default values
func NewConfig() *PostgreSQLConfig {
	return &PostgreSQLConfig{
		AutoStart:      2,
		InitDBArgs:     "-A trust",
		PostmasterArgs: "-h 127.0.0.1",
	}
}

// NewPostgreSQL creates a new TestPostgreSQL instance
func NewPostgreSQL(config *PostgreSQLConfig) (*TestPostgreSQL, error) {
	guards := []func(){}
	if config == nil {
		config = NewConfig()
	}

	if config.BaseDir != "" {
		// BaseDir provided, make sure it's an absolute path
		abspath, err := filepath.Abs(config.BaseDir)
		if err != nil {
			return nil, err
		}
		config.BaseDir = abspath
	} else {
		preserve, err := strconv.ParseBool(os.Getenv("TEST_POSTGRESQL_PRESERVE"))
		if err != nil {
			preserve = false // just to make sure
		}

		tempdir, err := ioutil.TempDir("", "postgresqltest")
		if err != nil {
			return nil, fmt.Errorf("error: Failed to create temporary directory: %s", err)
		}

		config.BaseDir = tempdir

		if !preserve {
			guards = append(guards, func() {
				os.RemoveAll(config.BaseDir)
			})
		}
	}

	if config.DataDir == "" {
		config.DataDir = filepath.Join(config.BaseDir, "var")
	}

	fi, err := os.Stat(config.BaseDir)
	if err != nil && fi.Mode()&os.ModeSymlink == os.ModeSymlink {
		resolved, err := os.Readlink(config.BaseDir)
		if err != nil {
			return nil, err
		}
		config.BaseDir = resolved
	}

	if config.TmpDir == "" {
		config.TmpDir = filepath.Join(config.BaseDir, "tmp")
	}

	if config.InitDB == "" {
		prog, err := findProgram("initdb")
		if err != nil {
			return nil, fmt.Errorf("error: Could not find initdb: %s", err)
		}
		config.InitDB = prog
	}

	if config.Postmaster == "" {
		prog, err := findProgram("postmaster")
		if err != nil {
			return nil, fmt.Errorf("error: Could not find postmaster: %s", err)
		}
		config.Postmaster = prog
	}

	postgresql := &TestPostgreSQL{
		config,
		nil,
		guards,
		"",
	}

	if config.AutoStart > 0 {
		if config.AutoStart > 1 {
			if err := postgresql.Setup(); err != nil {
				return nil, err
			}
		}

		if err := postgresql.Start(); err != nil {
			return nil, err
		}
	}

	return postgresql, nil
}

// AssertNotRunning returns nil if postgresql is not running
func (m *TestPostgreSQL) AssertNotRunning() error {
	if pidfile := m.Config.PidFile; pidfile != "" {
		_, err := os.Stat(pidfile)
		if err == nil {
			return fmt.Errorf("postgresql is already running (%s)", pidfile)
		}
		if !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

// Setup sets up all the files and directories needed to start postgresql
func (m *TestPostgreSQL) Setup() error {
	config := m.Config
	// (re)create directory structure
	if err := os.MkdirAll(config.TmpDir, 0755); err != nil {
		return err
	}

	// initdb
	datadir := filepath.Join(config.BaseDir, "data")
	if !isDir(datadir) {
		cmd := exec.Command(
			config.InitDB,
			config.InitDBArgs,
			"-D",
			datadir,
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("error: *** initdb failed ***\n%s\n", out)
		}
	}
	return nil
}

// Start starts the postgresql process
func (m *TestPostgreSQL) Start() error {
	if err := m.AssertNotRunning(); err != nil {
		return err
	}
	config := m.Config
	if config.Port != 0 {
		err := m.tryStart(config.Port)
		if err != nil {
			return err
		}
	} else {
		BasePort := 15432
		for port := BasePort; port < BasePort+100; port++ {
			err := m.tryStart(port)
			if err == nil {
				config.Port = port
				return nil
			}
		}
	}
	return nil
}

func (m *TestPostgreSQL) tryStart(port int) error {
	config := m.Config
	logname := filepath.Join(config.TmpDir, "postgresql.log")
	file, err := os.OpenFile(logname, os.O_CREATE|os.O_WRONLY, 0755)
	if err != nil {
		return err
	}
	m.LogFile = logname

	cmd := exec.Command(
		config.Postmaster,
		config.PostmasterArgs,
		"-p",
		strconv.Itoa(port),
		"-D",
		filepath.Join(config.BaseDir, "data"),
		"-k",
		config.TmpDir,
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	stdoutpipe, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderrpipe, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	m.Command = cmd

	go io.Copy(file, stdoutpipe)
	go io.Copy(file, stderrpipe)

	c := make(chan bool)
	go func() {
		cmd.Run()
		c <- true
	}()

	for {
		if cmd.Process != nil {
			if _, err = os.FindProcess(cmd.Process.Pid); err == nil {
				break
			}
		}

		select {
		case <-c:
			// Fuck, we exited
			return errors.New("error: Failed to launch postgresql")
		default:
			time.Sleep(100 * time.Millisecond)
		}
	}

	// Wait until we can connect to the database
	timeout := time.Now().Add(30 * time.Second)
	var db *sql.DB
	for time.Now().Before(timeout) {
		dsn := m.Datasource("template1", "", "", port, config.TmpDir, "")
		db, err = sql.Open("postgres", dsn)
		if err == nil {
			var id int
			row := db.QueryRow("SELECT 1")
			err = row.Scan(&id)
			if err == nil {
				break
			}
		}
		time.Sleep(1 * time.Second)
	}

	if db == nil {
		return errors.New("error: Could not connect to database. Server failed to start?")
	}

	return nil
}

// Datasource creates the appropriate Datasource string that can be passed
func (m *TestPostgreSQL) Datasource(dbname string, user string, pass string, port int, host string, sslmode string) string {
	if dbname == "" {
		dbname = "postgres"
	}

	if sslmode == "" {
		sslmode = "disable"
	}

	config := m.Config
	if port <= 0 {
		port = config.Port
	}

	dsn := fmt.Sprintf("sslmode=%s ", sslmode)
	dsn += fmt.Sprintf("port=%d ", port)
	if host != "" {
		dsn += fmt.Sprintf("host=%s ", host)
	}
	if user != "" {
		dsn += fmt.Sprintf("user=%s ", user)
	}
	dsn += fmt.Sprintf("dbname=%s", dbname)
	return dsn
}

// Stop explicitly stops the execution of mysqld
func (m *TestPostgreSQL) Stop() {
	if cmd := m.Command; cmd != nil {
		if process := cmd.Process; process != nil {
			process.Kill()
		}
	}

	// Run any guards that are registered
	for _, g := range m.Guards {
		g()
	}
}

// ReadLog reads the output log file specified by LogFile and returns its content
func (m *TestPostgreSQL) ReadLog() ([]byte, error) {
	filename := m.LogFile
	fi, err := os.Lstat(filename)
	if err != nil {
		return nil, err
	}

	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}

	buf := make([]byte, fi.Size())
	_, err = io.ReadFull(file, buf)
	if err != nil {
		return nil, err
	}
	return buf, nil
}

func findProgram(prog string) (string, error) {
	path, err := exec.LookPath(prog)
	if err == nil {
		return path, nil
	}
	for _, v := range SearchPaths {
		pattern := filepath.Join(v, "bin", prog)
		files, err := filepath.Glob(pattern)
		if err != nil {
			panic(err)
		}
		for _, file := range files {
			if _, err := os.Stat(file); err == nil {
				return file, nil
			}
		}
	}
	return "", fmt.Errorf("error: *** could not find file ***\n%s\n", prog)
}

func isDir(path string) bool {
	fi, err := os.Stat(path)
	if err == nil {
		mode := fi.Mode()
		if mode.IsDir() {
			return true
		}
	}
	return false
}
