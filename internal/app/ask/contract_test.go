package ask

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"testing"

	petapp "github.com/balancetheworld/wechat-pet/internal/app/pet"
	appErrors "github.com/balancetheworld/wechat-pet/internal/pkg/errors"
	_ "github.com/mattn/go-sqlite3"
)

// TestCommandTypeClassification 验证命令类型分类与合法性（contract.go）。
func TestCommandTypeClassification(t *testing.T) {
	all := []CommandType{
		CommandNewTask, CommandSupplement, CommandReply,
		CommandStop, CommandRetry,
		CommandConfirm, CommandWithdraw, CommandAbandon,
	}
	for _, command := range all {
		if !command.Valid() {
			t.Fatalf("command %q should be valid", command)
		}
	}
	if CommandType("bogus").Valid() {
		t.Fatal("unknown command should be invalid")
	}

	if !CommandNewTask.IsInputCommand() || !CommandSupplement.IsInputCommand() || !CommandReply.IsInputCommand() {
		t.Fatal("new_task/supplement/reply should be input commands")
	}
	if CommandStop.IsInputCommand() || CommandConfirm.IsInputCommand() {
		t.Fatal("stop/confirm should not be input commands")
	}
	if !CommandStop.IsRunControl() || !CommandRetry.IsRunControl() {
		t.Fatal("stop/retry should be run controls")
	}
	if !CommandConfirm.IsOperationControl() || !CommandWithdraw.IsOperationControl() || !CommandAbandon.IsOperationControl() {
		t.Fatal("confirm/withdraw/abandon should be operation controls")
	}
}

// TestSourceVersionUpsertAndGet 验证来源集合版本的最小读写。
func TestSourceVersionUpsertAndGet(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	createAskSchema(t, db)
	repository, err := NewRepository(db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}

	if err := repository.UpsertSourceVersion(context.Background(), "calendar_record", "rec-1", "v1"); err != nil {
		t.Fatal(err)
	}
	value, err := repository.GetSourceVersion(context.Background(), "calendar_record", "rec-1")
	if err != nil {
		t.Fatal(err)
	}
	if value.Version != "v1" || value.SourceType != "calendar_record" || value.SourceID != "rec-1" {
		t.Fatalf("source version = %+v", value)
	}

	// 覆盖更新
	if err := repository.UpsertSourceVersion(context.Background(), "calendar_record", "rec-1", "v2"); err != nil {
		t.Fatal(err)
	}
	updated, err := repository.GetSourceVersion(context.Background(), "calendar_record", "rec-1")
	if err != nil {
		t.Fatal(err)
	}
	if updated.Version != "v2" {
		t.Fatalf("version = %q, want v2", updated.Version)
	}
}

// TestSourceVersionNotFound 验证来源版本不存在时返回哨兵错误。
func TestSourceVersionNotFound(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	createAskSchema(t, db)
	repository, err := NewRepository(db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := repository.GetSourceVersion(context.Background(), "calendar_record", "missing"); !errors.Is(err, ErrSourceVersionNotFound) {
		t.Fatalf("error = %v, want ErrSourceVersionNotFound", err)
	}
}

// TestGetSessionRejectsWrongFamily 验证跨家庭访问会话被拒绝且不泄露存在性（3.6、9.4）。
func TestGetSessionRejectsWrongFamily(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	createAskSchema(t, db)
	repository, err := NewRepository(db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewService(repository, servicePetRepository{pet: petapp.Pet{ID: "pet-1", FamilyID: "family-1", Name: "团子"}})
	if err != nil {
		t.Fatal(err)
	}

	created, err := service.CreateSession(context.Background(), "family-1", "user-1", "pet-1", "最近没精神", "k-wrong-family")
	if err != nil {
		t.Fatal(err)
	}

	_, err = service.GetSession(context.Background(), "family-2", created.Session.ID)
	var appErr *appErrors.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("error = %v, want AppError", err)
	}
	if appErr.HTTPStatus != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (不泄露目标存在性)", appErr.HTTPStatus)
	}
}

// TestCurrentSessionUniqueConstraint 验证 9.2「同一用户 + 家庭至多一个当前 Session」。
func TestCurrentSessionUniqueConstraint(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	createAskSchema(t, db)

	insert := func(id string) error {
		_, err := db.Exec(`INSERT INTO ask_sessions (id, family_id, pet_id, created_by, status, risk_level, turn_count, prompt_version, rule_version, knowledge_version, created_at, updated_at) VALUES (?, 'family-1', 'pet-1', 'user-1', 'active', 'unknown', 0, 'pv', 'rv', 'kv', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, id)
		return err
	}

	if err := insert("s1"); err != nil {
		t.Fatal(err)
	}
	// 同一用户 + 家庭再建一个 active Session 应被唯一约束拒绝
	if err := insert("s2"); err == nil {
		t.Fatal("expected unique constraint violation for duplicate active session")
	}

	// 软删除第一个后释放唯一位置，可再建 active Session
	if _, err := db.Exec(`UPDATE ask_sessions SET deleted_at = CURRENT_TIMESTAMP WHERE id = 's1'`); err != nil {
		t.Fatal(err)
	}
	if err := insert("s2"); err != nil {
		t.Fatalf("expected insert after soft-delete to succeed, got %v", err)
	}

	// 关闭后（closed）同样释放唯一位置
	if _, err := db.Exec(`UPDATE ask_sessions SET status = 'closed', deleted_at = NULL WHERE id = 's2'`); err != nil {
		t.Fatal(err)
	}
	if err := insert("s3"); err != nil {
		t.Fatalf("expected insert after close to succeed, got %v", err)
	}
}
