package postgres

import (
	"context"
	"log"
	"sync"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/johnquangdev/laverte-home/config"
)

var (
	once sync.Once
	db   *gorm.DB
)

// GetClient lazily opens the shared *gorm.DB on first call.
func GetClient(ctx context.Context) *gorm.DB {
	once.Do(func() {
		cfg := config.GetConfig()
		conn, err := gorm.Open(postgres.Open(cfg.DatabaseURL()), &gorm.Config{})
		if err != nil {
			log.Fatalf("postgres: connect: %v", err)
		}
		db = conn
	})
	return db.WithContext(ctx)
}
