// Package testdb gives each repository integration test package its own
// throwaway Postgres database, migrated to head. go test runs packages in
// parallel: a database shared across packages means one package's cleanup
// (TRUNCATE or DROP) wipes out another package's fixtures mid-run, which is
// what made repository/booking and repository/blockedslot's integration
// tests fail nondeterministically when run together under plain `go test`.
package testdb

import (
	"database/sql"
	"net/url"
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
	migrate "github.com/rubenv/sql-migrate"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/johnquangdev/laverte-home/migrations"
)

// New skips the calling test when TEST_DATABASE_URL is unset. Otherwise it
// drops and recreates "laverte_test_<suffix>", migrates it to head, and
// returns a GORM handle to it. The database is dropped via t.Cleanup, so
// callers do not TRUNCATE anything themselves — there is nothing left to
// clean up per-row once the whole database goes away.
func New(t testing.TB, suffix string) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}

	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("testdb: open admin db: %v", err)
	}
	// Registered before the drop-database cleanup below so LIFO ordering closes
	// admin last: a plain defer would run at function return, before t.Cleanup
	// fires, closing admin while the drop-test-db cleanup still needs it.
	t.Cleanup(func() {
		if cerr := admin.Close(); cerr != nil {
			t.Errorf("testdb: close admin db: %v", cerr)
		}
	})

	dbName := "laverte_test_" + suffix
	if _, err = admin.Exec("DROP DATABASE IF EXISTS " + dbName); err != nil {
		t.Fatalf("testdb: drop stale db %s: %v", dbName, err)
	}
	if _, err = admin.Exec("CREATE DATABASE " + dbName); err != nil {
		t.Fatalf("testdb: create db %s: %v", dbName, err)
	}

	testDSN, err := swapDatabase(dsn, dbName)
	if err != nil {
		t.Fatalf("testdb: build dsn for %s: %v", dbName, err)
	}

	migrateConn, err := sql.Open("pgx", testDSN)
	if err != nil {
		t.Fatalf("testdb: open %s for migration: %v", dbName, err)
	}
	src := &migrate.EmbedFileSystemMigrationSource{FileSystem: migrations.FS, Root: "."}
	if _, err = migrate.Exec(migrateConn, "postgres", src, migrate.Up); err != nil {
		t.Fatalf("testdb: migrate up %s: %v", dbName, err)
	}
	if err = migrateConn.Close(); err != nil {
		t.Fatalf("testdb: close migration handle for %s: %v", dbName, err)
	}

	gormDB, err := gorm.Open(postgres.Open(testDSN), &gorm.Config{})
	if err != nil {
		t.Fatalf("testdb: gorm open %s: %v", dbName, err)
	}
	// Registered after the admin-close cleanup so LIFO runs this one first,
	// dropping dbName while admin — the connection that issues the DROP — is
	// still open.
	t.Cleanup(func() {
		sqlDB, derr := gormDB.DB()
		if derr != nil {
			t.Errorf("testdb: get sql.DB for %s: %v", dbName, derr)
			return
		}
		if cerr := sqlDB.Close(); cerr != nil {
			t.Errorf("testdb: close %s: %v", dbName, cerr)
		}
		if _, derr = admin.Exec("DROP DATABASE IF EXISTS " + dbName); derr != nil {
			t.Errorf("testdb: drop %s: %v", dbName, derr)
		}
	})

	return gormDB
}

// swapDatabase points a DSN at a different database on the same server.
func swapDatabase(dsn, name string) (string, error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return "", err
	}
	u.Path = "/" + name
	return u.String(), nil
}
