package httpapi

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	familyapp "github.com/balancetheworld/wechat-pet/server/internal/app/family"
	jwtpkg "github.com/balancetheworld/wechat-pet/server/internal/pkg/jwt"
	_ "github.com/mattn/go-sqlite3"
)

func TestFamilyRoutesEnforceRoleAndFamilyScope(t *testing.T) {
	repository, db := newFamilyRouteRepository(t)
	service, err := familyapp.NewService(repository)
	if err != nil {
		t.Fatal(err)
	}
	for _, user := range []struct {
		id       string
		nickname string
	}{
		{id: "owner-a", nickname: "拥有者 A"},
		{id: "member-a", nickname: "成员 A"},
		{id: "owner-b", nickname: "拥有者 B"},
		{id: "guest-b", nickname: "申请人 B"},
	} {
		seedFamilyRouteUser(t, db, user.id, user.nickname)
	}
	familyA, err := service.Create(context.Background(), "owner-a", familyapp.CreateFamilyRequest{Name: "家庭 A"})
	if err != nil {
		t.Fatal(err)
	}
	familyB, err := service.Create(context.Background(), "owner-b", familyapp.CreateFamilyRequest{Name: "家庭 B"})
	if err != nil {
		t.Fatal(err)
	}
	applicationA, err := service.ApplyJoin(context.Background(), "member-a", familyapp.JoinFamilyRequest{Code: familyA.JoinCode})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.ApproveApplication(context.Background(), familyA.ID, applicationA.ID); err != nil {
		t.Fatal(err)
	}
	applicationB, err := service.ApplyJoin(context.Background(), "guest-b", familyapp.JoinFamilyRequest{Code: familyB.JoinCode})
	if err != nil {
		t.Fatal(err)
	}
	signer, err := jwtpkg.NewSigner("test-secret", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	router := NewWithDependencies(Dependencies{FamilyService: service, FamilyRepository: repository, TokenSigner: signer})

	memberPending := familyRequest(t, router, signer, http.MethodGet, "/api/v1/families/join-applications", "member-a")
	if memberPending.Code != http.StatusForbidden || !strings.Contains(memberPending.Body.String(), `"code":40302`) {
		t.Fatalf("member pending response: status=%d body=%s", memberPending.Code, memberPending.Body.String())
	}

	memberList := familyRequest(t, router, signer, http.MethodGet, "/api/v1/families/members", "member-a")
	if memberList.Code != http.StatusOK || !strings.Contains(memberList.Body.String(), "成员 A") || strings.Contains(memberList.Body.String(), "拥有者 B") {
		t.Fatalf("member list response: status=%d body=%s", memberList.Code, memberList.Body.String())
	}

	ownerAPending := familyRequest(t, router, signer, http.MethodGet, "/api/v1/families/join-applications", "owner-a")
	if ownerAPending.Code != http.StatusOK || strings.Contains(ownerAPending.Body.String(), applicationB.ID) {
		t.Fatalf("owner A pending response: status=%d body=%s", ownerAPending.Code, ownerAPending.Body.String())
	}

	wrongOwnerApprove := familyRequest(t, router, signer, http.MethodPost, "/api/v1/families/join-applications/"+applicationB.ID+"/approve", "owner-a")
	if wrongOwnerApprove.Code != http.StatusNotFound || !strings.Contains(wrongOwnerApprove.Body.String(), `"code":40401`) {
		t.Fatalf("wrong owner approval response: status=%d body=%s", wrongOwnerApprove.Code, wrongOwnerApprove.Body.String())
	}

	ownerMembers, err := service.Members(context.Background(), familyA.ID)
	if err != nil {
		t.Fatal(err)
	}
	ownerMemberID := ""
	for _, member := range ownerMembers {
		if member.UserID == "owner-a" {
			ownerMemberID = member.ID
			break
		}
	}
	if ownerMemberID == "" {
		t.Fatal("owner membership not found")
	}
	selfRemove := familyRequest(t, router, signer, http.MethodPost, "/api/v1/families/members/"+ownerMemberID+"/remove", "owner-a")
	if selfRemove.Code != http.StatusConflict || !strings.Contains(selfRemove.Body.String(), `"code":40901`) {
		t.Fatalf("self removal response: status=%d body=%s", selfRemove.Code, selfRemove.Body.String())
	}

	approve := familyRequest(t, router, signer, http.MethodPost, "/api/v1/families/join-applications/"+applicationB.ID+"/approve", "owner-b")
	if approve.Code != http.StatusOK {
		t.Fatalf("approval response: status=%d body=%s", approve.Code, approve.Body.String())
	}
	current := familyRequest(t, router, signer, http.MethodGet, "/api/v1/families/current", "guest-b")
	if current.Code != http.StatusOK || !strings.Contains(current.Body.String(), familyB.ID) || !strings.Contains(current.Body.String(), `"role":"member"`) {
		t.Fatalf("current family response: status=%d body=%s", current.Code, current.Body.String())
	}
}

func familyRequest(t *testing.T, router http.Handler, signer *jwtpkg.Signer, method string, path string, userID string) *httptest.ResponseRecorder {
	t.Helper()
	token, err := signer.Sign(userID)
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, nil)
	request.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(recorder, request)
	return recorder
}

func newFamilyRouteRepository(t *testing.T) (*familyapp.SQLRepository, *sql.DB) {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	schema := `CREATE TABLE users (id TEXT PRIMARY KEY, openid TEXT NOT NULL UNIQUE, unionid TEXT, nickname TEXT, avatar_asset_id TEXT, last_login_at TIMESTAMP NOT NULL, created_at TIMESTAMP NOT NULL, updated_at TIMESTAMP NOT NULL);
CREATE TABLE families (id TEXT PRIMARY KEY, name TEXT NOT NULL, code TEXT, created_at TIMESTAMP NOT NULL, updated_at TIMESTAMP NOT NULL);
CREATE UNIQUE INDEX idx_families_code ON families (code);
CREATE TABLE family_members (id TEXT PRIMARY KEY, family_id TEXT NOT NULL, user_id TEXT NOT NULL, role TEXT NOT NULL CHECK (role IN ('member', 'owner')), status TEXT NOT NULL, created_at TIMESTAMP NOT NULL, updated_at TIMESTAMP NOT NULL, UNIQUE (family_id, user_id));
CREATE UNIQUE INDEX idx_family_members_active_user ON family_members (user_id) WHERE status = 'active';
CREATE TABLE family_join_applications (id TEXT PRIMARY KEY, family_id TEXT NOT NULL, user_id TEXT NOT NULL, status TEXT NOT NULL, created_at TIMESTAMP NOT NULL, updated_at TIMESTAMP NOT NULL, UNIQUE (family_id, user_id));`
	if _, err := db.Exec("CREATE TABLE pets (id TEXT PRIMARY KEY, family_id TEXT NOT NULL, name TEXT NOT NULL, created_by TEXT NOT NULL, updated_by TEXT NOT NULL, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, deleted_at TIMESTAMP)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(schema); err != nil {
		t.Fatal(err)
	}
	repository, err := familyapp.NewRepository(db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	return repository, db
}

func seedFamilyRouteUser(t *testing.T, db *sql.DB, id string, nickname string) {
	t.Helper()
	if _, err := db.Exec("INSERT INTO users (id, openid, nickname, last_login_at, created_at, updated_at) VALUES (?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)", id, "openid-"+id, nickname); err != nil {
		t.Fatal(err)
	}
}
