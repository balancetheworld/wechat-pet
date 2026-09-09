package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	askapp "github.com/balancetheworld/wechat-pet/internal/app/ask"
	calendarapp "github.com/balancetheworld/wechat-pet/internal/app/calendar"
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
	missingIdempotency := httptest.NewRecorder()
	missingIdempotencyRequest := httptest.NewRequest(http.MethodPost, "/api/v1/pets/pet-1/ask/sessions", bytes.NewBufferString(`{"input":"最近没精神"}`))
	missingIdempotencyRequest.Header.Set("Authorization", "Bearer "+token)
	missingIdempotencyRequest.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(missingIdempotency, missingIdempotencyRequest)
	if missingIdempotency.Code != http.StatusBadRequest {
		t.Fatalf("missing idempotency status = %d, body = %s", missingIdempotency.Code, missingIdempotency.Body.String())
	}
	create := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/pets/pet-1/ask/sessions", bytes.NewBufferString(`{"input":"最近没精神"}`))
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "create-lifecycle")
	router.ServeHTTP(create, request)
	if create.Code != http.StatusOK || !strings.Contains(create.Body.String(), `"status":"queued"`) || !strings.Contains(create.Body.String(), `"pet_id":"pet-1"`) {
		t.Fatalf("create status = %d, body = %s", create.Code, create.Body.String())
	}
	sessionID := extractAskRouteID(create.Body.String(), "session")
	runID := extractAskRouteID(create.Body.String(), "run")
	if sessionID == "" || runID == "" {
		t.Fatalf("missing IDs: %s", create.Body.String())
	}
	replayedCreate := askRouteRequestWithKey(t, router, token, http.MethodPost, "/api/v1/pets/pet-1/ask/sessions", `{"input":"最近没精神"}`, "create-lifecycle")
	if replayedCreate.Code != http.StatusOK || !strings.Contains(replayedCreate.Body.String(), `"id":"`+sessionID+`"`) || !strings.Contains(replayedCreate.Body.String(), `"id":"`+runID+`"`) {
		t.Fatalf("replayed create status = %d, body = %s", replayedCreate.Code, replayedCreate.Body.String())
	}
	conflictingCreate := askRouteRequestWithKey(t, router, token, http.MethodPost, "/api/v1/pets/pet-1/ask/sessions", `{"input":"另一条问题"}`, "create-lifecycle")
	if conflictingCreate.Code != http.StatusConflict {
		t.Fatalf("conflicting create status = %d, body = %s", conflictingCreate.Code, conflictingCreate.Body.String())
	}
	process := askRouteRequest(t, router, token, http.MethodPost, "/api/v1/ask/sessions/"+sessionID+"/runs/"+runID+"/process", "")
	if process.Code != http.StatusOK || !strings.Contains(process.Body.String(), `"status":"waiting_input"`) || !strings.Contains(process.Body.String(), `"assistant.question"`) {
		t.Fatalf("process status = %d, body = %s", process.Code, process.Body.String())
	}
	if !strings.Contains(process.Body.String(), `"row_version":3`) || !strings.Contains(process.Body.String(), `"clarification_count":0`) || !strings.Contains(process.Body.String(), `"completed_at":null`) {
		t.Fatalf("process version fields missing: %s", process.Body.String())
	}
	snapshot := askRouteRequest(t, router, token, http.MethodGet, "/api/v1/ask/sessions/"+sessionID+"/snapshot", "")
	if snapshot.Code != http.StatusOK || !strings.Contains(snapshot.Body.String(), `"pets":[{"pet_id":"pet-1","pet_name":"团子"`) || !strings.Contains(snapshot.Body.String(), `"turn_index":0`) || !strings.Contains(snapshot.Body.String(), `"row_version":3`) || !strings.Contains(snapshot.Body.String(), `"event_cursors":[{"run_id":"`+runID+`","sequence":3}]`) {
		t.Fatalf("snapshot status = %d, body = %s", snapshot.Code, snapshot.Body.String())
	}
	stream := askRouteRequest(t, router, token, http.MethodGet, "/api/v1/ask/sessions/"+sessionID+"/runs/"+runID+"/events/stream?after=1", "")
	if stream.Code != http.StatusOK || !strings.HasPrefix(stream.Header().Get("Content-Type"), "application/x-ndjson") || !strings.Contains(stream.Body.String(), `"run_id":"`+runID+`"`) || !strings.Contains(stream.Body.String(), `"sequence":2`) || !strings.Contains(stream.Body.String(), `"assistant.question"`) || strings.Contains(stream.Body.String(), `"code":0`) {
		t.Fatalf("stream status = %d, content-type = %s, body = %s", stream.Code, stream.Header().Get("Content-Type"), stream.Body.String())
	}
	replyPath := "/api/v1/ask/sessions/" + sessionID + "/runs/" + runID + "/reply"
	missingVersion := askRouteRequest(t, router, token, http.MethodPost, replyPath, `{"input":"现在呼吸困难"}`)
	if missingVersion.Code != http.StatusBadRequest {
		t.Fatalf("missing version status = %d, body = %s", missingVersion.Code, missingVersion.Body.String())
	}
	replyBody := `{"input":"现在呼吸困难","expected_version":3}`
	reply := askRouteRequest(t, router, token, http.MethodPost, replyPath, replyBody)
	if reply.Code != http.StatusOK || !strings.Contains(reply.Body.String(), `"turn_count":1`) || !strings.Contains(reply.Body.String(), `"id":"`+runID+`"`) || !strings.Contains(reply.Body.String(), `"row_version":4`) || !strings.Contains(reply.Body.String(), `"clarification_count":1`) || !strings.Contains(reply.Body.String(), `"sequence":4`) || !strings.Contains(reply.Body.String(), `"status":"queued"`) {
		t.Fatalf("reply status = %d, body = %s", reply.Code, reply.Body.String())
	}
	replayedReply := askRouteRequest(t, router, token, http.MethodPost, replyPath, replyBody)
	if replayedReply.Code != http.StatusOK || !strings.Contains(replayedReply.Body.String(), `"row_version":4`) || !strings.Contains(replayedReply.Body.String(), `"sequence":4`) {
		t.Fatalf("replayed reply status = %d, body = %s", replayedReply.Code, replayedReply.Body.String())
	}
	conflictingReply := askRouteRequestWithKey(t, router, token, http.MethodPost, replyPath, `{"input":"其他回答","expected_version":3}`, askRouteIdempotencyKey(http.MethodPost, replyPath, replyBody))
	if conflictingReply.Code != http.StatusConflict {
		t.Fatalf("conflicting reply status = %d, body = %s", conflictingReply.Code, conflictingReply.Body.String())
	}
	resumed := askRouteRequest(t, router, token, http.MethodPost, "/api/v1/ask/sessions/"+sessionID+"/runs/"+runID+"/process", "")
	if resumed.Code != http.StatusOK || !strings.Contains(resumed.Body.String(), `"status":"escalated"`) || !strings.Contains(resumed.Body.String(), `"risk_level":"red"`) || !strings.Contains(resumed.Body.String(), `"sequence":6`) {
		t.Fatalf("resumed process status = %d, body = %s", resumed.Code, resumed.Body.String())
	}
	var messageCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM ask_messages WHERE run_id = ?", runID).Scan(&messageCount); err != nil {
		t.Fatal(err)
	}
	if messageCount != 3 {
		t.Fatalf("message count = %d, want 3", messageCount)
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

func TestAskRoutesMultiPetFactContract(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	setupAskRouteSchema(t, db)
	if _, err := db.Exec("INSERT INTO users (id, openid, nickname, last_login_at, created_at, updated_at) VALUES ('user-1', 'openid-1', '用户', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO families (id, name, created_at, updated_at) VALUES ('family-1', '家庭', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO family_members (id, family_id, user_id, role, status, created_at, updated_at) VALUES ('member-1', 'family-1', 'user-1', 'owner', 'active', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO pets (id, family_id, name, created_by, updated_by, created_at, updated_at) VALUES ('pet-1', 'family-1', '旺仔', 'user-1', 'user-1', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP), ('pet-2', 'family-1', '球球', 'user-1', 'user-1', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE calendar_records (id TEXT PRIMARY KEY, family_id TEXT NOT NULL, pet_id TEXT NOT NULL, category TEXT NOT NULL, medical_type TEXT, custom_medical_type TEXT, content TEXT NOT NULL, occurred_at TIMESTAMP NOT NULL, deleted_at TIMESTAMP)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO calendar_records (id, family_id, pet_id, category, medical_type, custom_medical_type, content, occurred_at, deleted_at) VALUES ('record-1', 'family-1', 'pet-1', 'daily', '', '', '旺仔洗澡', '2026-08-20 10:00:00', NULL)"); err != nil {
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
	calendarRepository, err := calendarapp.NewRepository(db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	askService, err := askapp.NewService(askRepository, petRepository, askapp.DeterministicExecutor{})
	if err != nil {
		t.Fatal(err)
	}
	askService.SetCalendarRepository(calendarRepository)
	signer, err := jwtpkg.NewSigner("test-secret", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	router := NewWithDependencies(Dependencies{AskService: askService, FamilyRepository: familyRepository, TokenSigner: signer})
	token, err := signer.Sign("user-1")
	if err != nil {
		t.Fatal(err)
	}
	create := askRouteRequest(t, router, token, http.MethodPost, "/api/v1/ask/sessions", `{"input":"旺仔和球球上次洗澡分别是什么时候"}`)
	if create.Code != http.StatusOK || !strings.Contains(create.Body.String(), `"pet_id":"pet-1"`) || !strings.Contains(create.Body.String(), `"pet_id":"pet-2"`) {
		t.Fatalf("create status = %d, body = %s", create.Code, create.Body.String())
	}
	sessionID := extractAskRouteID(create.Body.String(), "session")
	runID := extractAskRouteID(create.Body.String(), "run")
	if sessionID == "" || runID == "" {
		t.Fatalf("missing IDs: %s", create.Body.String())
	}
	process := askRouteRequest(t, router, token, http.MethodPost, "/api/v1/ask/sessions/"+sessionID+"/runs/"+runID+"/process", "")
	body := process.Body.String()
	if process.Code != http.StatusOK || !strings.Contains(body, `"status":"completed"`) || !strings.Contains(body, `"type":"fact.completed"`) || !strings.Contains(body, `"fact_type":"bath"`) || !strings.Contains(body, `"pet_id":"pet-1"`) || !strings.Contains(body, `"pet_id":"pet-2"`) || !strings.Contains(body, `"found":false`) {
		t.Fatalf("process status = %d, body = %s", process.Code, body)
	}
	if strings.Contains(body, `"PetID"`) || strings.Contains(body, `"PetName"`) {
		t.Fatalf("fact fields are not snake_case: %s", body)
	}
	events := askRouteRequest(t, router, token, http.MethodGet, "/api/v1/ask/sessions/"+sessionID+"/runs/"+runID+"/events?after=1", "")
	if events.Code != http.StatusOK || !strings.Contains(events.Body.String(), `"type":"fact.completed"`) || !strings.Contains(events.Body.String(), `"fact_type":"bath"`) {
		t.Fatalf("events status = %d, body = %s", events.Code, events.Body.String())
	}
}

func askRouteRequest(t *testing.T, router http.Handler, token, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	return askRouteRequestWithKey(t, router, token, method, path, body, askRouteIdempotencyKey(method, path, body))
}

func askRouteRequestWithKey(t *testing.T, router http.Handler, token, method, path, body, idempotencyKey string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Idempotency-Key", idempotencyKey)
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	router.ServeHTTP(recorder, request)
	return recorder
}

func askRouteIdempotencyKey(method, path, body string) string {
	value := sha256.Sum256([]byte(method + "\x00" + path + "\x00" + body))
	return hex.EncodeToString(value[:])
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
		`CREATE TABLE ask_session_pets (id TEXT PRIMARY KEY, session_id TEXT NOT NULL, pet_id TEXT NOT NULL, mention TEXT NOT NULL, sort_order INTEGER NOT NULL, UNIQUE(session_id, pet_id), UNIQUE(session_id, sort_order))`,
		`CREATE TABLE ask_turns (id TEXT PRIMARY KEY, session_id TEXT NOT NULL, turn_index INTEGER NOT NULL, status TEXT NOT NULL, input TEXT NOT NULL, selected_run_id TEXT NOT NULL, created_at TIMESTAMP NOT NULL, UNIQUE(session_id, turn_index))`,
		`CREATE TABLE ask_runs (id TEXT PRIMARY KEY, session_id TEXT NOT NULL, turn_id TEXT NOT NULL, run_index INTEGER NOT NULL, row_version INTEGER NOT NULL DEFAULT 1, clarification_count INTEGER NOT NULL DEFAULT 0, status TEXT NOT NULL, risk_level TEXT NOT NULL, rule_version TEXT NOT NULL, prompt_version TEXT NOT NULL, created_at TIMESTAMP NOT NULL, started_at TIMESTAMP, completed_at TIMESTAMP, error_code TEXT NOT NULL, UNIQUE(turn_id, run_index))`,
		`CREATE TABLE ask_events (id TEXT PRIMARY KEY, session_id TEXT NOT NULL, turn_id TEXT NOT NULL, run_id TEXT NOT NULL, sequence INTEGER NOT NULL, type TEXT NOT NULL, data TEXT NOT NULL, created_at TIMESTAMP NOT NULL, UNIQUE(run_id, sequence))`,
		`CREATE TABLE ask_messages (id TEXT PRIMARY KEY, session_id TEXT NOT NULL, turn_id TEXT NOT NULL, run_id TEXT, role TEXT NOT NULL, content TEXT NOT NULL, created_at TIMESTAMP NOT NULL)`,
		`CREATE TABLE ask_idempotency_keys (id TEXT PRIMARY KEY, family_id TEXT NOT NULL, user_id TEXT NOT NULL, operation TEXT NOT NULL, idempotency_key TEXT NOT NULL, request_hash TEXT NOT NULL, response_data TEXT NOT NULL, session_id TEXT NOT NULL, turn_id TEXT NOT NULL, run_id TEXT NOT NULL, created_at TIMESTAMP NOT NULL, UNIQUE(user_id, operation, idempotency_key))`,
	}
	for _, statement := range askStatements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
}
