package migrations

import (
	"database/sql"
	"net/url"
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
	migrate "github.com/rubenv/sql-migrate"
)

// migrationTestDB is created and dropped by this test alone.
const migrationTestDB = "laverte_migrationtest"

// swapDatabase points a DSN at a different database on the same server.
func swapDatabase(dsn, name string) (string, error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return "", err
	}
	u.Path = "/" + name
	return u.String(), nil
}

// TestMigrationsApplyCleanly runs against its OWN throwaway database, never the
// shared dev one. Proving Down works means dropping every table, and `go test`
// runs packages in parallel — against a shared database that pulls the schema out
// from under repository/booking's and repository/payment's integration tests
// mid-run, which fails as `relation "bookings" does not exist`.
//
// Requires `docker compose -f docker-compose.dev.yml up -d` and TEST_DATABASE_URL,
// e.g. postgres://laverte:laverte@localhost:55432/laverte?sslmode=disable
func TestMigrationsApplyCleanly(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}

	// CREATE/DROP DATABASE cannot run while connected to the target, so issue them
	// from the database the DSN already names.
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open admin db: %v", err)
	}
	// Registered before the db-drop cleanup below so LIFO ordering closes admin
	// last: a plain defer would run at function return, before t.Cleanup fires,
	// closing admin while the drop-test-db cleanup still needs it.
	t.Cleanup(func() {
		if cerr := admin.Close(); cerr != nil {
			t.Errorf("close admin db: %v", cerr)
		}
	})

	if _, err = admin.Exec("DROP DATABASE IF EXISTS " + migrationTestDB); err != nil {
		t.Fatalf("drop stale test db: %v", err)
	}
	if _, err = admin.Exec("CREATE DATABASE " + migrationTestDB); err != nil {
		t.Fatalf("create test db: %v", err)
	}

	tmpDSN, err := swapDatabase(dsn, migrationTestDB)
	if err != nil {
		t.Fatalf("build test dsn: %v", err)
	}
	db, err := sql.Open("pgx", tmpDSN)
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() {
		// The database cannot be dropped while this connection is open.
		if cerr := db.Close(); cerr != nil {
			t.Errorf("close test db: %v", cerr)
		}
		if _, derr := admin.Exec("DROP DATABASE IF EXISTS " + migrationTestDB); derr != nil {
			t.Errorf("drop test db: %v", derr)
		}
	})

	src := &migrate.EmbedFileSystemMigrationSource{FileSystem: FS, Root: "."}
	if _, err = migrate.Exec(db, "postgres", src, migrate.Up); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	if _, err = migrate.Exec(db, "postgres", src, migrate.Down); err != nil {
		t.Fatalf("migrate down: %v", err)
	}
}
