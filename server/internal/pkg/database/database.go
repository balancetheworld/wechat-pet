package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/balancetheworld/wechat-pet/server/internal/pkg/config"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "github.com/mattn/go-sqlite3"
)

const (
	maxOpenConns    = 10
	maxIdleConns    = 5
	connMaxLifetime = 30 * time.Minute
	pingTimeout     = 10 * time.Second
)

func Open(ctx context.Context, cfg config.Config) (*sql.DB, error) {
	driver, dsn, err := driverAndDSN(cfg)
	if err != nil {
		return nil, err
	}

	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, fmt.Errorf("open %s database: %w", cfg.DatabaseDriver, err)
	}
	db.SetMaxOpenConns(maxOpenConns)
	db.SetMaxIdleConns(maxIdleConns)
	db.SetConnMaxLifetime(connMaxLifetime)

	pingCtx, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping %s database: %w", cfg.DatabaseDriver, err)
	}
	return db, nil
}

func driverAndDSN(cfg config.Config) (string, string, error) {
	switch strings.ToLower(cfg.DatabaseDriver) {
	case "postgres", "postgresql", "pgx":
		return "pgx", cfg.DatabaseDSN, nil
	case "sqlite", "sqlite3":
		return "sqlite3", cfg.DatabaseDSN, nil
	default:
		return "", "", fmt.Errorf("unsupported database driver: %s", cfg.DatabaseDriver)
	}
}
