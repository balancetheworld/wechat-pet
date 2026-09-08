package httpapi

import (
	"bytes"
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	askapp "github.com/balancetheworld/wechat-pet/internal/app/ask"
	familyapp "github.com/balancetheworld/wechat-pet/internal/app/family"
	petapp "github.com/balancetheworld/wechat-pet/internal/app/pet"
	jwtpkg "github.com/balancetheworld/wechat-pet/internal/pkg/jwt"
	_ "github.com/mattn/go-sqlite3"
)

func TestAskRoutesLifecycle(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	setupAskRouteSchema(t, db)
	if _, err := db.Exec("INSERT INTO users (id, openid, nickname, last_login_at, created_at, updated_at) VALUES ('user-1', 'openid-1', '用户', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP), ('user-2', 'openid-2', '其他用户', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO families (id, name, created_at, updated_at) VALUES ('family-1', '家庭', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO family_members (id, family_id, user_id, role, status, created_at, updated_at) VALUES ('member-1', 'family-1', 'user-1', 'owner', 'active', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO pets (id, family_id, name, created_by, updated_by, created_at, updated_at) VALUES ('pet-1', 'family-1', '团子', 'user-1', 'user-1', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)"); err != nil {
		t.Fatal(err)
	}
	familyRepository, err := familyapp.NewRepository(db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	petRepository, err := petapp.NewRepository(db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	askRepository, err := askapp.NewRepository(db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	askService, err := askapp.NewService(askRepository, petRepository, askapp.DeterministicExecutor{})
	if err != nil {
		t.Fatal(err)
	}
	signer, err := jwtpkg.NewSigner("test-secret", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	router := NewWithDependencies(Dependencies{AskService: askService, FamilyRepository: familyRepository, TokenSigner: signer})
	unauthorized := httptest.NewRecorder()
	router.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodPost, "/api/v1/pets/pet-1/ask/sessions", strings.NewReader(`{"input":"最近没精神"}`)))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d, body = %s", unauthorized.Code, unauthorized.Body.String())
	}
	token, err := signer.Sign("user-1")
	if err != nil {
		t.Fatal(err)
	}
	create := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/pets/pet-1/ask/sessions", bytes.NewBufferString(`{"input":"最近没精神"}`))
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(create, request)
	if create.Code != http.StatusOK || !strings.Contains(create.Body.String(), `"status":"queued"`) || !strings.Contains(create.Body.String(), `"pet_id":"pet-1"`) {
		t.Fatalf("create status = %d, body = %s", create.Code, create.Body.String())
	}
	sessionID := extractAskRouteID(create.Body.String(), "session")
	runID := extractAskRouteID(create.Body.String(), "run")
	if sessionID == "" || runID == "" {
		t.Fatalf("missing IDs: %s", create.Body.String())
	}
	process := askRouteRequest(t, router, token, http.MethodPost, "/api/v1/ask/sessions/"+sessionID+"/runs/"+runID+"/process", "")
	if process.Code != http.StatusOK || !strings.Contains(process.Body.String(), `"status":"waiting_input"`) || !strings.Contains(process.Body.String(), `"assistant.question"`) {
		t.Fatalf("process status = %d, body = %s", process.Code, process.Body.String())
	}
	reply := askRouteRequest(t, router, token, http.MethodPost, "/api/v1/ask/sessions/"+sessionID+"/runs/"+runID+"/reply", `{"input":"已经吐了两次"}`)
	if reply.Code != http.StatusOK || !strings.Contains(reply.Body.String(), `"turn_count":2`) || !strings.Contains(reply.Body.String(), `"status":"queued"`) {
		t.Fatalf("reply status = %d, body = %s", reply.Code, reply.Body.String())
	}
	events := askRouteRequest(t, router, token, http.MethodGet, "/api/v1/ask/sessions/"+sessionID+"/runs/"+runID+"/events?after=1", "")
	if events.Code != http.StatusOK || strings.Contains(events.Body.String(), `"sequence":1`) || !strings.Contains(events.Body.String(), `"sequence":2`) {
		t.Fatalf("events status = %d, body = %s", events.Code, events.Body.String())
	}
	missing := askRouteRequest(t, router, token, http.MethodGet, "/api/v1/ask/sessions/missing", "")
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing status = %d, body = %s", missing.Code, missing.Body.String())
	}
}

func askRouteRequest(t *testing.T, router http.Handler, token, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+token)
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	router.ServeHTTP(recorder, request)
	return recorder
}

func extractAskRouteID(body, field string) string {
	marker := `"` + field + `":{"id":"`
	start := strings.Index(body, marker)
	if start < 0 {
		return ""
	}
	start += len(marker)
	end := strings.IndexByte(body[start:], '"')
	if end < 0 {
		return ""
	}
	return body[start : start+end]
}

func setupAskRouteSchema(t *testing.T, db *sql.DB) {
	t.Helper()
	statements := []string{
		`CREATE TABLE users (id TEXT PRIMARY KEY, openid TEXT NOT NULL UNIQUE, unionid TEXT, nickname TEXT, avatar_asset_id TEXT, last_login_at TIMESTAMP NOT NULL, created_at TIMESTAMP NOT NULL, updated_at TIMESTAMP NOT NULL)`,
		`CREATE TABLE families (id TEXT PRIMARY KEY, name TEXT NOT NULL, code TEXT, created_at TIMESTAMP NOT NULL, updated_at TIMESTAMP NOT NULL)`,
		`CREATE TABLE family_members (id TEXT PRIMARY KEY, family_id TEXT NOT NULL, user_id TEXT NOT NULL, role TEXT NOT NULL, status TEXT NOT NULL, created_at TIMESTAMP NOT NULL, updated_at TIMESTAMP NOT NULL)`,
		`CREATE TABLE pets (id TEXT PRIMARY KEY, family_id TEXT NOT NULL, name TEXT NOT NULL, created_by TEXT NOT NULL, updated_by TEXT NOT NULL, created_at TIMESTAMP NOT NULL, updated_at TIMESTAMP NOT NULL, deleted_at TIMESTAMP)`,
	}
	for _, statement := range statements {
		if _, err := db.ExecContext(context.Background(), statement); err != nil {
			t.Fatal(err)
		}
	}
	askStatements := []string{
		`CREATE TABLE ask_sessions (id TEXT PRIMARY KEY, family_id TEXT NOT NULL, pet_id TEXT NOT NULL, created_by TEXT NOT NULL, status TEXT NOT NULL, risk_level TEXT NOT NULL, turn_count INTEGER NOT NULL, prompt_version TEXT NOT NULL, rule_version TEXT NOT NULL, knowledge_version TEXT NOT NULL, created_at TIMESTAMP NOT NULL, updated_at TIMESTAMP NOT NULL, completed_at TIMESTAMP)`,
		`CREATE TABLE ask_turns (id TEXT PRIMARY KEY, session_id TEXT NOT NULL, turn_index INTEGER NOT NULL, status TEXT NOT NULL, input TEXT NOT NULL, selected_run_id TEXT NOT NULL, created_at TIMESTAMP NOT NULL, UNIQUE(session_id, turn_index))`,
		`CREATE TABLE ask_runs (id TEXT PRIMARY KEY, session_id TEXT NOT NULL, turn_id TEXT NOT NULL, run_index INTEGER NOT NULL, status TEXT NOT NULL, risk_level TEXT NOT NULL, rule_version TEXT NOT NULL, prompt_version TEXT NOT NULL, created_at TIMESTAMP NOT NULL, started_at TIMESTAMP, completed_at TIMESTAMP, error_code TEXT NOT NULL, UNIQUE(turn_id, run_index))`,
		`CREATE TABLE ask_events (id TEXT PRIMARY KEY, session_id TEXT NOT NULL, turn_id TEXT NOT NULL, run_id TEXT NOT NULL, sequence INTEGER NOT NULL, type TEXT NOT NULL, data TEXT NOT NULL, created_at TIMESTAMP NOT NULL, UNIQUE(run_id, sequence))`,
		`CREATE TABLE ask_messages (id TEXT PRIMARY KEY, session_id TEXT NOT NULL, turn_id TEXT NOT NULL, run_id TEXT, role TEXT NOT NULL, content TEXT NOT NULL, created_at TIMESTAMP NOT NULL)`,
	}
	for _, statement := range askStatements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
}
