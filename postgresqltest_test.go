package postgresqltest

import (
	"database/sql"
	"fmt"
	"os"
	"runtime"
	"strings"
	"testing"

	_ "github.com/lib/pq"
)

func TestBasic(t *testing.T) {
	if runtime.GOOS == "freebsd" {
		os.Setenv("LANG", "C")
		os.Setenv("LC_CTYPE", "C")
	}

	postgresql, err := NewPostgreSQL(NewConfig())
	if err != nil {
		t.Fatalf("Failed to start postgresql: %s", err)
	}
	defer postgresql.Stop()

	config := postgresql.Config
	dsn := postgresql.Datasource("test", "", "", config.Port, config.TmpDir, "")
	wantdsn := fmt.Sprintf("sslmode=disable port=%d host=%s dbname=test",
		config.Port, config.TmpDir)
	if dsn != wantdsn {
		t.Fatalf("DSN does not match expected (got '%s', want '%s')",
			dsn, wantdsn)
	}
	err = postgresql.CreateDB("test")
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("Failed to connect to database: %v", err)
	}
	dbResult := 0
	err = db.QueryRow("select 1").Scan(&dbResult)
	if err != nil {
		t.Errorf("Failed to 'select 1': %v", err)
	}
	if dbResult != 1 {
		t.Errorf("DBResult is not 1. %v", err)
	}

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
