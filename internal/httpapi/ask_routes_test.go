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

// askRouteModel 是 HTTP 路由端到端测试的 v2 决策循环模型替身：始终返回
// request_input，使 ProcessRun 降级为 assistant.question（waiting_input），便于驱动 Reply 流程。
type askRouteModel struct{}

func (askRouteModel) Step(context.Context, askapp.StepInput) (askapp.ModelStepResult, error) {
	return askapp.ModelStepResult{Records: askRouteRequestInputRecords(), Usage: askapp.ModelUsage{Complete: true}}, nil
}

func askRouteRequestInputRecords() []askapp.ProtocolRecord {
	return []askapp.ProtocolRecord{
		{Type: askapp.RecordHeader, Header: &askapp.HeaderRecord{
			Type:          askapp.RecordHeader,
			SchemaVersion: askapp.RecordArrayV1,
			Action:        askapp.ActionRequestInput,
			TaskUpdates:   []askapp.TaskUpdate{{TaskKey: "t1", Goal: "查旺仔呕吐记录"}},
		}},
		{Type: askapp.RecordQuestion, Question: &askapp.QuestionRecord{
			Type:        askapp.RecordQuestion,
			QuestionKey: "q1",
			TaskKeys:    []string{"t1"},
			Text:        "请问是哪只宠物？",
			MissingFields: []askapp.MissingField{
				{TaskKey: "t1", Field: "pet_id", Necessity: "blocking"},
			},
		}},
		{Type: askapp.RecordCoverage, Coverage: &askapp.CoverageRecord{
			Type:  askapp.RecordCoverage,
			Tasks: []askapp.TaskCoverage{{TaskKey: "t1", QuestionKeys: []string{"q1"}}},
		}},
		{Type: askapp.RecordEnd},
	}
}

// injectAskRouteV2Dependencies 给 ask Service 注入 v2 决策循环依赖（目录、业务读取、模型），
// 使 ProcessRun 走 v2 路径而非降级。
func injectAskRouteV2Dependencies(t *testing.T, service *askapp.Service, calendarRepository *calendarapp.SQLRepository, petRepository *petapp.SQLRepository, askRepository *askapp.SQLRepository) {
	t.Helper()
	catalog, err := askapp.DefaultCatalog(askapp.DefaultToolVersion)
	if err != nil {
		t.Fatal(err)
	}
	service.SetCalendarRepository(calendarRepository)
	service.SetToolCatalog(catalog)
	service.SetBusinessReadRepository(askapp.NewBusinessReadRepository(petRepository, calendarRepository, askRepository))
	service.SetAgentModel(askRouteModel{})
}

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
	askService, err := askapp.NewService(askRepository, petRepository)
	if err != nil {
		t.Fatal(err)
	}
	calendarRepository, err := calendarapp.NewRepository(db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	injectAskRouteV2Dependencies(t, askService, calendarRepository, petRepository, askRepository)
	signer, err := jwtpkg.NewSigner("test-secret", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	runQueue := &recordingRunQueue{service: askService}
	router := NewWithDependencies(Dependencies{AskService: askService, AskRunQueue: runQueue, FamilyRepository: familyRepository, TokenSigner: signer})
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
	if len(runQueue.jobs) != 1 || runQueue.jobs[0].FamilyID != "family-1" || runQueue.jobs[0].SessionID != sessionID || runQueue.jobs[0].RunID != runID || runQueue.jobs[0].RowVersion != 1 {
		t.Fatalf("created run jobs = %+v", runQueue.jobs)
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
	if snapshot.Code != http.StatusOK || !strings.Contains(snapshot.Body.String(), `"pets":[{"pet_id":"pet-1","pet_name":"团子"`) || !strings.Contains(snapshot.Body.String(), `"turn_index":0`) || !strings.Contains(snapshot.Body.String(), `"row_version":3`) || !strings.Contains(snapshot.Body.String(), `"runs":[{"run":{"id":"`+runID+`"`) || !strings.Contains(snapshot.Body.String(), `"messages":[{"role":"user","content":"最近没精神"`) || !strings.Contains(snapshot.Body.String(), `"type":"run.progress"`) || !strings.Contains(snapshot.Body.String(), `"event_cursors":[{"run_id":"`+runID+`","sequence":7}]`) {
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
	if reply.Code != http.StatusOK || !strings.Contains(reply.Body.String(), `"turn_count":2`) || !strings.Contains(reply.Body.String(), `"id":"`+runID+`"`) || !strings.Contains(reply.Body.String(), `"row_version":4`) || !strings.Contains(reply.Body.String(), `"clarification_count":1`) || !strings.Contains(reply.Body.String(), `"sequence":8`) || !strings.Contains(reply.Body.String(), `"status":"queued"`) {
		t.Fatalf("reply status = %d, body = %s", reply.Code, reply.Body.String())
	}
	if queued := runQueue.jobs[len(runQueue.jobs)-1]; queued.RunID != runID || queued.RowVersion != 4 {
		t.Fatalf("replied run job = %+v", queued)
	}
	replayedReply := askRouteRequest(t, router, token, http.MethodPost, replyPath, replyBody)
	if replayedReply.Code != http.StatusOK || !strings.Contains(replayedReply.Body.String(), `"row_version":4`) || !strings.Contains(replayedReply.Body.String(), `"sequence":8`) {
		t.Fatalf("replayed reply status = %d, body = %s", replayedReply.Code, replayedReply.Body.String())
	}
	conflictingReply := askRouteRequestWithKey(t, router, token, http.MethodPost, replyPath, `{"input":"其他回答","expected_version":3}`, askRouteIdempotencyKey(http.MethodPost, replyPath, replyBody))
	if conflictingReply.Code != http.StatusConflict {
		t.Fatalf("conflicting reply status = %d, body = %s", conflictingReply.Code, conflictingReply.Body.String())
	}
	resumed := askRouteRequest(t, router, token, http.MethodPost, "/api/v1/ask/sessions/"+sessionID+"/runs/"+runID+"/process", "")
	if resumed.Code != http.StatusOK || !strings.Contains(resumed.Body.String(), `"status":"completed"`) || !strings.Contains(resumed.Body.String(), `"risk_level":"red"`) || !strings.Contains(resumed.Body.String(), `"sequence":13`) {
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
	if events.Code != http.StatusOK || strings.Contains(events.Body.String(), `"sequence":1,`) || !strings.Contains(events.Body.String(), `"sequence":2`) {
		t.Fatalf("events status = %d, body = %s", events.Code, events.Body.String())
	}
	missing := askRouteRequest(t, router, token, http.MethodGet, "/api/v1/ask/sessions/missing", "")
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing status = %d, body = %s", missing.Code, missing.Body.String())
	}
}

func askRouteRequest(t *testing.T, router http.Handler, token, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	return askRouteRequestWithKey(t, router, token, method, path, body, askRouteIdempotencyKey(method, path, body))
}

type recordingRunQueue struct {
	service *askapp.Service
	jobs    []askapp.RunJob
}

func (q *recordingRunQueue) Enqueue(ctx context.Context, job askapp.RunJob) error {
	q.jobs = append(q.jobs, job)
	if q.service == nil {
		return nil
	}
	_, err := q.service.ProcessRunVersion(ctx, job.FamilyID, job.SessionID, job.RunID, job.RowVersion, job.ExecutionEpoch)
	return err
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
		`CREATE TABLE pets (id TEXT PRIMARY KEY, family_id TEXT NOT NULL, name TEXT NOT NULL, breed TEXT, gender TEXT, sterilized INTEGER, birthday TEXT, home_date TEXT, created_by TEXT NOT NULL, updated_by TEXT NOT NULL, created_at TIMESTAMP NOT NULL, updated_at TIMESTAMP NOT NULL, deleted_at TIMESTAMP)`,
	}
	for _, statement := range statements {
		if _, err := db.ExecContext(context.Background(), statement); err != nil {
			t.Fatal(err)
		}
	}
	askStatements := []string{
		// v2：会话（对齐 000015/000016）
		`CREATE TABLE ask_sessions (id TEXT PRIMARY KEY, family_id TEXT NOT NULL, pet_id TEXT NOT NULL, resolved_pet_id TEXT, created_by TEXT NOT NULL, status TEXT NOT NULL, risk_level TEXT NOT NULL, turn_count INTEGER NOT NULL, input_sequence INTEGER NOT NULL DEFAULT 0, event_sequence INTEGER NOT NULL DEFAULT 0, launch_instance TEXT NOT NULL DEFAULT '', generation INTEGER NOT NULL DEFAULT 1, prompt_version TEXT NOT NULL, rule_version TEXT NOT NULL, knowledge_version TEXT NOT NULL, created_at TIMESTAMP NOT NULL, updated_at TIMESTAMP NOT NULL, completed_at TIMESTAMP, deleted_at TIMESTAMP)`,
		`CREATE TABLE ask_session_pets (id TEXT PRIMARY KEY, session_id TEXT NOT NULL, pet_id TEXT NOT NULL, mention TEXT NOT NULL, sort_order INTEGER NOT NULL, UNIQUE(session_id, pet_id), UNIQUE(session_id, sort_order))`,
		`CREATE TABLE ask_turns (id TEXT PRIMARY KEY, session_id TEXT NOT NULL, turn_index INTEGER NOT NULL, input_sequence INTEGER NOT NULL DEFAULT 0, status TEXT NOT NULL, input TEXT NOT NULL, asset_refs TEXT NOT NULL DEFAULT '[]', selected_run_id TEXT NOT NULL, superseded_by TEXT, created_at TIMESTAMP NOT NULL, deleted_at TIMESTAMP, UNIQUE(session_id, turn_index))`,
		`CREATE TABLE ask_runs (id TEXT PRIMARY KEY, session_id TEXT NOT NULL, turn_id TEXT NOT NULL, origin_turn_id TEXT NOT NULL DEFAULT '', run_index INTEGER NOT NULL, row_version INTEGER NOT NULL DEFAULT 1, clarification_count INTEGER NOT NULL DEFAULT 0, status TEXT NOT NULL, risk_level TEXT NOT NULL, input_revision INTEGER NOT NULL DEFAULT 0, execution_epoch INTEGER NOT NULL DEFAULT 0, termination_reason TEXT NOT NULL DEFAULT '', checkpoint TEXT NOT NULL DEFAULT '', rule_version TEXT NOT NULL, prompt_version TEXT NOT NULL, created_at TIMESTAMP NOT NULL, started_at TIMESTAMP, completed_at TIMESTAMP, error_code TEXT NOT NULL DEFAULT '', lease_owner TEXT NOT NULL DEFAULT '', lease_expires_at TIMESTAMP, attempt_count INTEGER NOT NULL DEFAULT 0, next_attempt_at TIMESTAMP, deleted_at TIMESTAMP, UNIQUE(turn_id, run_index))`,
		`CREATE TABLE ask_events (id TEXT PRIMARY KEY, session_id TEXT NOT NULL, turn_id TEXT NOT NULL, run_id TEXT NOT NULL, sequence INTEGER NOT NULL, type TEXT NOT NULL, data TEXT NOT NULL, created_at TIMESTAMP NOT NULL, deleted_at TIMESTAMP, UNIQUE(run_id, sequence))`,
		`CREATE TABLE ask_messages (id TEXT PRIMARY KEY, session_id TEXT NOT NULL, turn_id TEXT NOT NULL, run_id TEXT, role TEXT NOT NULL, content TEXT NOT NULL, created_at TIMESTAMP NOT NULL, deleted_at TIMESTAMP)`,
		`CREATE TABLE ask_task_items (id TEXT PRIMARY KEY, run_id TEXT NOT NULL, origin_turn_id TEXT NOT NULL DEFAULT '', item_revision INTEGER NOT NULL DEFAULT 1, goal TEXT NOT NULL, source_turn_ids TEXT NOT NULL DEFAULT '[]', subjects TEXT NOT NULL DEFAULT '[]', outcome TEXT NOT NULL DEFAULT 'pending', missing_fields TEXT NOT NULL DEFAULT '[]', incomplete_reason TEXT NOT NULL DEFAULT '', result_ref TEXT NOT NULL DEFAULT '', supersedes TEXT NOT NULL DEFAULT '', superseded_by TEXT NOT NULL DEFAULT '', withdrawn_reason TEXT NOT NULL DEFAULT '', created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, deleted_at TIMESTAMP)`,
		`CREATE TABLE ask_budgets (id TEXT PRIMARY KEY, scope TEXT NOT NULL, scope_id TEXT NOT NULL, max_model_calls INTEGER NOT NULL DEFAULT 0, max_tool_calls INTEGER NOT NULL DEFAULT 0, max_tokens INTEGER NOT NULL DEFAULT 0, max_cost_micros BIGINT NOT NULL DEFAULT 0, max_duration_millis BIGINT NOT NULL DEFAULT 0, max_concurrency INTEGER NOT NULL DEFAULT 0, model_calls_used INTEGER NOT NULL DEFAULT 0, tool_calls_used INTEGER NOT NULL DEFAULT 0, tokens_used INTEGER NOT NULL DEFAULT 0, cost_used_micros BIGINT NOT NULL DEFAULT 0, duration_used_millis BIGINT NOT NULL DEFAULT 0, model_calls_reserved INTEGER NOT NULL DEFAULT 0, tool_calls_reserved INTEGER NOT NULL DEFAULT 0, tokens_reserved INTEGER NOT NULL DEFAULT 0, cost_reserved_micros BIGINT NOT NULL DEFAULT 0, duration_reserved_millis BIGINT NOT NULL DEFAULT 0, concurrency_reserved INTEGER NOT NULL DEFAULT 0, created_at TIMESTAMP NOT NULL, updated_at TIMESTAMP NOT NULL, UNIQUE(scope, scope_id))`,
		`CREATE TABLE ask_reservations (id TEXT PRIMARY KEY, budget_id TEXT NOT NULL REFERENCES ask_budgets(id), model_calls INTEGER NOT NULL DEFAULT 0, tool_calls INTEGER NOT NULL DEFAULT 0, tokens INTEGER NOT NULL DEFAULT 0, cost_micros BIGINT NOT NULL DEFAULT 0, duration_millis BIGINT NOT NULL DEFAULT 0, concurrency INTEGER NOT NULL DEFAULT 0, status TEXT NOT NULL DEFAULT 'reserved', created_at TIMESTAMP NOT NULL, settled_at TIMESTAMP, released_at TIMESTAMP)`,
		`CREATE TABLE ask_idempotency_keys (id TEXT PRIMARY KEY, family_id TEXT NOT NULL, user_id TEXT NOT NULL, operation TEXT NOT NULL, idempotency_key TEXT NOT NULL, request_hash TEXT NOT NULL, response_data TEXT NOT NULL, session_id TEXT NOT NULL, turn_id TEXT NOT NULL, run_id TEXT NOT NULL, created_at TIMESTAMP NOT NULL, deleted_at TIMESTAMP, UNIQUE(user_id, operation, idempotency_key))`,
	}
	for _, statement := range askStatements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
}
