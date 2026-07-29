package main

import (
	"go.uber.org/zap"

	"github.com/johnquangdev/laverte-home/config"
	httpserver "github.com/johnquangdev/laverte-home/delivery/http"
)

func main() {
	cfg := config.GetConfig()

	log, _ := zap.NewProduction()
	defer log.Sync()

	srv := httpserver.NewServer(*cfg, log)
	log.Info("starting server", zap.String("port", cfg.Port))
	if err := srv.Start(); err != nil {
		log.Fatal("server error", zap.Error(err))
	}
}
