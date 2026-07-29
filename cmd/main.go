package main

import (
	"database/sql"

	_ "github.com/jackc/pgx/v5/stdlib"
	migrate "github.com/rubenv/sql-migrate"
	"go.uber.org/zap"

	"github.com/johnquangdev/laverte-home/config"
	httpserver "github.com/johnquangdev/laverte-home/delivery/http"
	"github.com/johnquangdev/laverte-home/migrations"
)

func main() {
	cfg := config.GetConfig()

	log, _ := zap.NewProduction()
	defer log.Sync()

	if err := runMigrations(cfg, log); err != nil {
		log.Fatal("auto-migration failed", zap.Error(err))
	}

	srv := httpserver.NewServer(*cfg, log)
	log.Info("starting server", zap.String("port", cfg.Port))
	if err := srv.Start(); err != nil {
		log.Fatal("server error", zap.Error(err))
	}
}

func runMigrations(cfg *config.Config, log *zap.Logger) error {
	db, err := sql.Open("pgx", cfg.DatabaseURL())
	if err != nil {
		return err
	}
	defer func() {
		if cerr := db.Close(); cerr != nil {
			log.Warn("close migration db connection", zap.Error(cerr))
		}
	}()

	src := &migrate.EmbedFileSystemMigrationSource{FileSystem: migrations.FS, Root: "."}
	n, err := migrate.Exec(db, "postgres", src, migrate.Up)
	if err != nil {
		return err
	}
	log.Info("migrations applied", zap.Int("count", n))
	return nil
}
