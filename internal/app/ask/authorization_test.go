package ask

import (
	"context"
	"errors"
	"net/http"
	"testing"

	appErrors "github.com/balancetheworld/wechat-pet/internal/pkg/errors"
)

// 本文件验证会话越权访问修复：同家庭成员（知道 Session UUID）不能读取、续写或
// 回答他人会话。所有读写入口必须校验 session.created_by == userID，越权与会话
// 不存在统一返回 404（不泄露资源存在性）。

func isNotFoundError(err error) bool {
	var appErr *appErrors.AppError
	if errors.As(err, &appErr) {
		return appErr.HTTPStatus == http.StatusNotFound
	}
	return false
}

// TestSameFamilyOtherUserCannotReadOrAct 用同家庭 A/B 用户覆盖所有读写接口。
func TestSameFamilyOtherUserCannotReadOrAct(t *testing.T) {
	model := &scriptedModel{responses: [][]ProtocolRecord{requestInputRecords()}}
	service, _ := newV2Service(t, model, &fakeBusinessRead{})

	// A 创建会话与 Run。
	created, err := service.CreateSession(context.Background(), "family-1", "user-1", "pet-1", "最近没精神", "auth-create")
	if err != nil {
		t.Fatal(err)
	}

	// B（同家庭）尝试各种读接口，均应返回 404 不泄露存在性。
	if _, err := service.GetSession(context.Background(), "family-1", created.Session.ID, "user-2"); !isNotFoundError(err) {
		t.Fatalf("GetSession cross-user = %v, want 404", err)
	}
	if _, err := service.GetSnapshot(context.Background(), "family-1", created.Session.ID, "user-2"); !isNotFoundError(err) {
		t.Fatalf("GetSnapshot cross-user = %v, want 404", err)
	}
	if _, err := service.GetEvents(context.Background(), "family-1", created.Session.ID, created.Run.ID, 0, "user-2"); !isNotFoundError(err) {
		t.Fatalf("GetEvents cross-user = %v, want 404", err)
	}
	if _, err := service.GetExecution(context.Background(), "family-1", created.Session.ID, created.Run.ID, "user-2"); !isNotFoundError(err) {
		t.Fatalf("GetExecution cross-user = %v, want 404", err)
	}

	// B 续写会话。
	if _, err := service.ContinueSession(context.Background(), "family-1", "user-2", created.Session.ID, "补充一下", "auth-continue"); !isNotFoundError(err) {
		t.Fatalf("ContinueSession cross-user = %v, want 404", err)
	}

	// A 把 Run 推进到 waiting_input，然后 B 回答追问。
	waiting, err := service.ProcessRun(context.Background(), "family-1", created.Session.ID, created.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Reply(context.Background(), "family-1", "user-2", created.Session.ID, created.Run.ID, "现在呼吸困难", waiting.Run.RowVersion, "auth-reply"); !isNotFoundError(err) {
		t.Fatalf("Reply cross-user = %v, want 404", err)
	}
}

// TestOwnerCanReadOwnSession 验证会话创建者本人读取不受影响（修复不误伤正常路径）。
func TestOwnerCanReadOwnSession(t *testing.T) {
	model := &scriptedModel{responses: [][]ProtocolRecord{requestInputRecords()}}
	service, _ := newV2Service(t, model, &fakeBusinessRead{})

	created, err := service.CreateSession(context.Background(), "family-1", "user-1", "pet-1", "最近没精神", "auth-owner")
	if err != nil {
		t.Fatal(err)
	}
	session, err := service.GetSession(context.Background(), "family-1", created.Session.ID, "user-1")
	if err != nil {
		t.Fatalf("owner GetSession = %v", err)
	}
	if session.ID != created.Session.ID {
		t.Fatalf("session id mismatch")
	}
}
