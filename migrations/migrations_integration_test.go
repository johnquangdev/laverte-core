package migrations

import (
	"database/sql"
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
	migrate "github.com/rubenv/sql-migrate"
)

// requires `docker compose -f docker-compose.dev.yml up -d` and
// TEST_DATABASE_URL, e.g.
// postgres://laverte:laverte@localhost:55432/laverte?sslmode=disable
func TestMigrationsApplyCleanly(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer func() {
		if cerr := db.Close(); cerr != nil {
			t.Errorf("close db: %v", cerr)
		}
	}()

	src := &migrate.EmbedFileSystemMigrationSource{FileSystem: FS, Root: "."}
	if _, err := migrate.Exec(db, "postgres", src, migrate.Up); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	if _, err := migrate.Exec(db, "postgres", src, migrate.Down); err != nil {
		t.Fatalf("migrate down: %v", err)
	}
}
