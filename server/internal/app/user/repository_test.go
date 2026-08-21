package user

import (
	"context"
	"testing"
	"time"

	"database/sql"
	_ "github.com/mattn/go-sqlite3"
)

func TestUpsertByOpenIDIsIdempotent(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(`CREATE TABLE users (
		id TEXT PRIMARY KEY,
		openid TEXT NOT NULL UNIQUE,
		unionid TEXT,
		nickname TEXT,
		avatar_asset_id TEXT,
		last_login_at TIMESTAMP NOT NULL,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL
	)`)
	if err != nil {
		t.Fatal(err)
	}
	repository, err := NewRepository(db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	first, err := repository.UpsertByOpenID(context.Background(), "openid-1", nil, time.Unix(100, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	second, err := repository.UpsertByOpenID(context.Background(), "openid-1", nil, time.Unix(200, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID {
		t.Fatalf("user id changed: %q != %q", first.ID, second.ID)
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM users WHERE openid = ?", "openid-1").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("user count = %d, want 1", count)
	}
	if !second.LastLoginAt.Equal(time.Unix(200, 0).UTC()) {
		t.Fatalf("last login = %v, want updated timestamp", second.LastLoginAt)
	}
}
