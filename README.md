go-test-postgresql
==============

Create real PostgreSQL server instance for testing

To install, simply issue a `go get`:

```
go get github.com/masanorih/go-test-postgresql
```

By default importing `github.com/masanorih/go-test-postgresql` will import package
`postgresqltest`

```go
import (
    "database/sql"
    "log"
    "github.com/masanorih/go-test-postgresql"
)

postgresql, err := postgresqltest.NewPostgreSQL(nil)
if err != nil {
   log.Fatalf("Failed to start postgresql: %s", err)
}
defer postgresql.Stop()

db, err := sql.Open("postgres", postgresql.Datasource("test", "", "", 0))
// Now use db, which is connected to a postgresql db
```

`go-test-postgresql` is a port of [Test::postgresql](https://metacpan.org/release/Test-postgresql)

When you create a new struct via `NewPostgreSQL()` a new postgresql instance is
automatically setup and launched. Don't forget to call `Stop()` on this
struct to stop the launched postgresql

If you want to customize the configuration, create a new config and set each
field on the struct:

```go

config := postgresqltest.NewConfig()
config.Port = 15432

// Starts postgresql listening on port 15432
postgresql, _ := postgresqltest.NewPostgreSQL(config)
```

TODO
====
