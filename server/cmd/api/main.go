package main

import (
	"context"
	"os"
	"time"

	"github.com/balancetheworld/wechat-pet/server/internal/httpapi"
	"github.com/balancetheworld/wechat-pet/server/internal/logging"
	"github.com/balancetheworld/wechat-pet/server/internal/pkg/config"
	"github.com/balancetheworld/wechat-pet/server/internal/pkg/database"
)

func main() {
	logger := logging.New()

	cfg, err := config.Load()
	if err != nil {
		logger.Error("load config", "error", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	db, err := database.Open(ctx, cfg)
	if err != nil {
		logger.Error("open database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	server := httpapi.New(logger)
	logger.Info("api server starting", "addr", cfg.HTTPAddr)
	if err := server.Run(cfg.HTTPAddr); err != nil {
		logger.Error("api server stopped", "error", err)
		os.Exit(1)
	}
}
