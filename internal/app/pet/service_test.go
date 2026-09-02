package pet

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	appErrors "github.com/balancetheworld/wechat-pet/internal/pkg/errors"
	_ "github.com/mattn/go-sqlite3"
)

func TestPetCRUDAndSoftDelete(t *testing.T) {
	repository, db := newTestRepository(t)
	service, err := NewService(repository)
	if err != nil {
		t.Fatal(err)
	}
	pet, err := service.Create(context.Background(), "family-a", "member-a", CreatePetRequest{Name: "小白"})
	if err != nil {
		t.Fatal(err)
	}
	if pet.ID == "" || pet.Name != "小白" {
		t.Fatalf("pet=%+v", pet)
	}
	updated, err := service.Update(context.Background(), "family-a", pet.ID, "member-a", UpdatePetRequest{Name: "小黑"})
	if err != nil || updated.Name != "小黑" {
		t.Fatalf("updated=%+v err=%v", updated, err)
	}
	list, err := service.List(context.Background(), "family-a")
	if err != nil || len(list) != 1 || list[0].Name != "小黑" {
		t.Fatalf("list=%+v err=%v", list, err)
	}
	if err := service.Delete(context.Background(), "family-a", pet.ID, "member-a"); err != nil {
		t.Fatal(err)
	}
	list, err = service.List(context.Background(), "family-a")
	if err != nil || len(list) != 0 {
		t.Fatalf("list after delete=%+v err=%v", list, err)
	}
	_, err = service.Get(context.Background(), "family-a", pet.ID)
	var appError *appErrors.AppError
	if !errors.As(err, &appError) || appError.HTTPStatus != 404 {
		t.Fatalf("get deleted error=%v", err)
	}
	var deletedAt sql.NullTime
	if err := db.QueryRow("SELECT deleted_at FROM pets WHERE id = ?", pet.ID).Scan(&deletedAt); err != nil {
		t.Fatal(err)
	}
	if !deletedAt.Valid {
		t.Fatal("deleted_at is not set")
	}
}

func TestPetQueriesAreScopedToFamily(t *testing.T) {
	repository, _ := newTestRepository(t)
	service, err := NewService(repository)
	if err != nil {
		t.Fatal(err)
	}
	pet, err := service.Create(context.Background(), "family-a", "member-a", CreatePetRequest{Name: "小白"})
	if err != nil {
		t.Fatal(err)
	}
	list, err := service.List(context.Background(), "family-b")
	if err != nil || len(list) != 0 {
		t.Fatalf("other family list=%+v err=%v", list, err)
	}
	_, err = service.Get(context.Background(), "family-b", pet.ID)
	var appError *appErrors.AppError
	if !errors.As(err, &appError) || appError.HTTPStatus != 404 {
		t.Fatalf("other family get error=%v", err)
	}
	if _, err := service.Update(context.Background(), "family-b", pet.ID, "member-b", UpdatePetRequest{Name: "越权"}); !errors.As(err, &appError) || appError.HTTPStatus != 404 {
		t.Fatalf("other family update error=%v", err)
	}
}

func newTestRepository(t *testing.T) (*SQLRepository, *sql.DB) {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	schema := `CREATE TABLE users (id TEXT PRIMARY KEY);
CREATE TABLE families (id TEXT PRIMARY KEY);
CREATE TABLE pets (id TEXT PRIMARY KEY, family_id TEXT NOT NULL, name TEXT NOT NULL, created_by TEXT NOT NULL, updated_by TEXT NOT NULL, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, deleted_at TIMESTAMP);`
	if _, err := db.Exec(schema); err != nil {
		t.Fatal(err)
	}
	return &SQLRepository{db: db, driver: "sqlite"}, db
}
