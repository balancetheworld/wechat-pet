package database

import (
	"context"
	"testing"

	"github.com/balancetheworld/wechat-pet/server/internal/pkg/config"
)

func TestOpenSQLite(t *testing.T) {
	db, err := Open(context.Background(), config.Config{DatabaseDriver: "sqlite", DatabaseDSN: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		t.Fatal(err)
	}
}

func TestOpenRejectsUnsupportedDriver(t *testing.T) {
	_, err := Open(context.Background(), config.Config{DatabaseDriver: "mysql", DatabaseDSN: "example"})
	if err == nil {
		t.Fatal("Open() error = nil, want unsupported driver error")
	}
}
