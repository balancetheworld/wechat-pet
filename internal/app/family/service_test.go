package family

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	appErrors "github.com/balancetheworld/wechat-pet/internal/pkg/errors"
	_ "github.com/mattn/go-sqlite3"
)

func TestCreateFamilyCreatesOwnerMembership(t *testing.T) {
	repository, db := newTestRepository(t, false)
	service, err := NewService(repository)
	if err != nil {
		t.Fatal(err)
	}
	seedUser(t, db, "owner")

	result, err := service.Create(context.Background(), "owner", CreateFamilyRequest{Name: "家庭"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Role != "owner" || result.JoinCode == "" {
		t.Fatalf("result=%+v", result)
	}
	summary, err := repository.GetActiveFamilySummary(context.Background(), "owner")
	if err != nil || summary == nil || summary.ID != result.ID || summary.Role != "owner" {
		t.Fatalf("summary=%+v err=%v", summary, err)
	}
	if _, err := service.Create(context.Background(), "owner", CreateFamilyRequest{Name: "另一个家庭"}); err == nil {
		t.Fatal("Create() error = nil")
	}
}

func TestCreateFamilyRollsBackWhenOwnerMembershipFails(t *testing.T) {
	repository, db := newTestRepository(t, true)
	seedUser(t, db, "owner")
	if _, err := repository.Create(context.Background(), "owner", "家庭"); err == nil {
		t.Fatal("Create() error = nil")
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM families").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("family count = %d, want 0", count)
	}
}

func TestJoinApplicationAndApproval(t *testing.T) {
	repository, db := newTestRepository(t, false)
	service, err := NewService(repository)
	if err != nil {
		t.Fatal(err)
	}
	seedUser(t, db, "owner")
	seedUser(t, db, "guest")
	family, err := service.Create(context.Background(), "owner", CreateFamilyRequest{Name: "家庭"})
	if err != nil {
		t.Fatal(err)
	}
	application, err := service.ApplyJoin(context.Background(), "guest", JoinFamilyRequest{Code: family.JoinCode})
	if err != nil || application.Status != "pending" {
		t.Fatalf("application=%+v err=%v", application, err)
	}
	if err := service.ApproveApplication(context.Background(), family.ID, application.ID); err != nil {
		t.Fatal(err)
	}
	summary, err := repository.GetActiveFamilySummary(context.Background(), "guest")
	if err != nil || summary == nil || summary.ID != family.ID || summary.Role != "member" {
		t.Fatalf("summary=%+v err=%v", summary, err)
	}
	if _, err := service.ApplyJoin(context.Background(), "guest", JoinFamilyRequest{Code: family.JoinCode}); err == nil {
		t.Fatal("ApplyJoin() error = nil")
	}
}

func TestJoinApplicationRejectsInvalidCode(t *testing.T) {
	repository, db := newTestRepository(t, false)
	service, err := NewService(repository)
	if err != nil {
		t.Fatal(err)
	}
	seedUser(t, db, "guest")
	_, err = service.ApplyJoin(context.Background(), "guest", JoinFamilyRequest{Code: "INVALID"})
	var appError *appErrors.AppError
	if !errors.As(err, &appError) || appError.HTTPStatus != 404 {
		t.Fatalf("error=%v", err)
	}
}

func TestApproveApplicationKeepsOnlyOneActiveFamily(t *testing.T) {
	repository, db := newTestRepository(t, false)
	service, err := NewService(repository)
	if err != nil {
		t.Fatal(err)
	}
	seedUser(t, db, "owner-a")
	seedUser(t, db, "owner-b")
	seedUser(t, db, "guest")
	familyA, err := service.Create(context.Background(), "owner-a", CreateFamilyRequest{Name: "家庭 A"})
	if err != nil {
		t.Fatal(err)
	}
	familyB, err := service.Create(context.Background(), "owner-b", CreateFamilyRequest{Name: "家庭 B"})
	if err != nil {
		t.Fatal(err)
	}
	applicationA, err := service.ApplyJoin(context.Background(), "guest", JoinFamilyRequest{Code: familyA.JoinCode})
	if err != nil {
		t.Fatal(err)
	}
	applicationB, err := service.ApplyJoin(context.Background(), "guest", JoinFamilyRequest{Code: familyB.JoinCode})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.ApproveApplication(context.Background(), familyA.ID, applicationA.ID); err != nil {
		t.Fatal(err)
	}
	if err := service.ApproveApplication(context.Background(), familyB.ID, applicationB.ID); err == nil {
		t.Fatal("ApproveApplication() error = nil")
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM family_members WHERE user_id = ? AND status = 'active'", "guest").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("active family count = %d, want 1", count)
	}
}

func TestRemoveOwnerSelfIsRejected(t *testing.T) {
	repository, db := newTestRepository(t, false)
	service, err := NewService(repository)
	if err != nil {
		t.Fatal(err)
	}
	seedUser(t, db, "owner")
	family, err := service.Create(context.Background(), "owner", CreateFamilyRequest{Name: "家庭"})
	if err != nil {
		t.Fatal(err)
	}
	members, err := service.Members(context.Background(), family.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 1 {
		t.Fatalf("members = %d, want 1", len(members))
	}
	err = service.RemoveMember(context.Background(), family.ID, members[0].ID, "owner")
	var appError *appErrors.AppError
	if !errors.As(err, &appError) || appError.HTTPStatus != 409 {
		t.Fatalf("error=%v", err)
	}
}

func newTestRepository(t *testing.T, rejectOwner bool) (*SQLRepository, *sql.DB) {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	memberRole := "CHECK (role IN ('member', 'owner'))"
	if rejectOwner {
		memberRole = "CHECK (role = 'member')"
	}
	schema := `CREATE TABLE users (id TEXT PRIMARY KEY, openid TEXT NOT NULL UNIQUE, unionid TEXT, nickname TEXT, avatar_asset_id TEXT, last_login_at TIMESTAMP NOT NULL, created_at TIMESTAMP NOT NULL, updated_at TIMESTAMP NOT NULL);
CREATE TABLE families (id TEXT PRIMARY KEY, name TEXT NOT NULL, code TEXT, created_at TIMESTAMP NOT NULL, updated_at TIMESTAMP NOT NULL);
CREATE UNIQUE INDEX idx_families_code ON families (code);
CREATE TABLE family_members (id TEXT PRIMARY KEY, family_id TEXT NOT NULL, user_id TEXT NOT NULL, role TEXT NOT NULL ` + memberRole + `, status TEXT NOT NULL, created_at TIMESTAMP NOT NULL, updated_at TIMESTAMP NOT NULL, UNIQUE (family_id, user_id));
CREATE UNIQUE INDEX idx_family_members_active_user ON family_members (user_id) WHERE status = 'active';
CREATE TABLE family_join_applications (id TEXT PRIMARY KEY, family_id TEXT NOT NULL, user_id TEXT NOT NULL, status TEXT NOT NULL, created_at TIMESTAMP NOT NULL, updated_at TIMESTAMP NOT NULL, UNIQUE (family_id, user_id));`
	if _, err := db.Exec(schema); err != nil {
		t.Fatal(err)
	}
	repository, err := NewRepository(db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	return repository, db
}

func seedUser(t *testing.T, db *sql.DB, id string) {
	t.Helper()
	if _, err := db.Exec("INSERT INTO users (id, openid, last_login_at, created_at, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)", id, "openid-"+id); err != nil {
		t.Fatal(err)
	}
}
