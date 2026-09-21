package main

import (
	"database/sql"
	"log"

	_ "github.com/jackc/pgx/v5/stdlib"
	migrate "github.com/rubenv/sql-migrate"

	"github.com/johnquangdev/laverte-core/config"
	"github.com/johnquangdev/laverte-core/migrations"
)

func main() {
	cfg := config.GetConfig()
	db, err := sql.Open("pgx", cfg.DatabaseURL())
	if err != nil {
		log.Fatalf("migrate: open db: %v", err)
	}
	defer func() {
		if cerr := db.Close(); cerr != nil {
			log.Printf("migrate: close db: %v", cerr)
		}
	}()

	src := &migrate.EmbedFileSystemMigrationSource{FileSystem: migrations.FS, Root: "."}
	n, err := migrate.Exec(db, "postgres", src, migrate.Up)
	if err != nil {
		log.Fatalf("migrate: exec: %v", err)
	}
	log.Printf("migrations applied: %d", n)
}
