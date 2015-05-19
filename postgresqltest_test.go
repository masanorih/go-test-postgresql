package postgresqltest

import (
	"database/sql"
	"strings"
	"testing"
	"time"
	"runtime"
	"os"

	_ "github.com/lib/pq"
)

func TestBasic(t *testing.T) {
	if runtime.GOOS == "freebsd" {
		os.Setenv("LANG", "C")
		os.Setenv("LC_CTYPE", "C")
	}

	postgresql, err := NewPostgreSQL(NewConfig())
	if err != nil {
		t.Errorf("Failed to start postgresql: %s", err)
	}
	defer postgresql.Stop()

	dsn := postgresql.Datasource("test", "", "", 15432, "", "")

	wantdsn := "sslmode=disable port=15432 dbname=test"

	if dsn != wantdsn {
		t.Errorf("DSN does not match expected (got '%s', want '%s')", dsn, wantdsn)
	}

	_, err = sql.Open("postgres", dsn)
	if err != nil {
		t.Errorf("Failed to connect to database: %s", err)
	}

	// Got to wait for a bit till the log gets anything in it
	time.Sleep(2 * time.Second)

	buf, err := postgresql.ReadLog()
	if err != nil {
		t.Errorf("Failed to read log: %s", err)
	}
	var readystr string
	if runtime.GOOS == "freebsd" {
		readystr = "ending log output to stderr"
	} else {
		readystr = "ready to accept connections"
	}
	if strings.Index(string(buf), readystr) < 0 {
		t.Errorf("Could not find 'ready to accept connections' in log: %s", buf)
	}
}

var FindProgram = findProgram

func TestFindProgram(t *testing.T) {
	cases := []string{"initdb", "postmaster"}
	for _, c := range cases {
		path, err := FindProgram(c)
		if err != nil {
			t.Errorf("FindProgram(%q) got error %q", c, err)
		}
		if path == "" {
			t.Errorf("FindProgram(%q) got empty path", c)
		}
	}
}

var IsDir = isDir

func TestIsDir(t *testing.T) {
	cases := []string{"/", "."}
	for _, c := range cases {
		if !IsDir(c) {
			t.Errorf("IsDir(%q) returned false", c)
		}
	}
}
