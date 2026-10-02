package ask

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func TestSQLRepositoryPersistsRunAndEvents(t *testing.T) {
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
	now := time.Unix(100, 0).UTC()
	session := Session{ID: "session-1", FamilyID: "family-1", PetID: "pet-1", CreatedBy: "user-1", Status: SessionActive, RiskLevel: RiskUnknown, PromptVersion: "prompt-v1", RuleVersion: "rule-v1", KnowledgeVersion: "knowledge-v1", CreatedAt: now, UpdatedAt: now}
	turn := Turn{ID: "turn-1", SessionID: session.ID, TurnIndex: 0, Status: TurnReceived, Input: "最近没有精神", SelectedRunID: "run-1", CreatedAt: now}
	run := Run{ID: "run-1", SessionID: session.ID, TurnID: turn.ID, RunIndex: 0, Status: RunQueued, RiskLevel: RiskUnknown, RuleVersion: "rule-v1", PromptVersion: "prompt-v1", CreatedAt: now}
	queuedEvent := Event{ID: "event-1", SessionID: session.ID, TurnID: turn.ID, RunID: run.ID, Sequence: 1, Type: "run.queued", Data: `{}`, CreatedAt: now}
	if err := repository.CreateSessionRun(context.Background(), session, turn, run, queuedEvent); err != nil {
		t.Fatal(err)
	}
	startedEvent := Event{ID: "event-2", SessionID: session.ID, TurnID: turn.ID, RunID: run.ID, Sequence: 2, Type: "run.started", Data: `{}`, CreatedAt: now.Add(time.Minute)}
	if err := repository.TransitionRun(context.Background(), run.ID, 1, RunQueued, RunRunning, RiskUnknown, "", now.Add(time.Minute), startedEvent, Message{}); err != nil {
		t.Fatal(err)
	}
	completedEvent := Event{ID: "event-3", SessionID: session.ID, TurnID: turn.ID, RunID: run.ID, Sequence: 3, Type: "run.completed", Data: `{"risk_level":"green"}`, CreatedAt: now.Add(2 * time.Minute)}
	if err := repository.TransitionRun(context.Background(), run.ID, 1, RunRunning, RunCompleted, RiskGreen, "", now.Add(2*time.Minute), completedEvent, Message{}); !errors.Is(err, ErrRunStateConflict) {
		t.Fatalf("conflict error = %v, want %v", err, ErrRunStateConflict)
	}
	if err := repository.TransitionRun(context.Background(), run.ID, 2, RunRunning, RunCompleted, RiskGreen, "", now.Add(2*time.Minute), completedEvent, Message{}); err != nil {
		t.Fatal(err)
	}
	result, err := repository.ListEvents(context.Background(), session.ID, run.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 3 || result[2].Sequence != 3 {
		t.Fatalf("events = %+v, want three ordered events", result)
	}
	loaded, err := repository.GetSession(context.Background(), session.FamilyID, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ID != session.ID || loaded.Status != SessionActive || loaded.RiskLevel != RiskGreen {
		t.Fatalf("loaded session = %+v", loaded)
	}
	loadedRun, err := repository.GetRun(context.Background(), session.ID, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loadedRun.RowVersion != 3 || loadedRun.ClarificationCount != 0 {
		t.Fatalf("loaded run = %+v", loadedRun)
	}
	if _, err := db.Exec("INSERT INTO ask_messages (id, session_id, turn_id, run_id, role, content, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)", "message-1", session.ID, turn.ID, run.ID, "user", turn.Input, now); err != nil {
		t.Fatal(err)
	}
	snapshotTurns, err := repository.GetSnapshotTurns(context.Background(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshotTurns) != 1 || snapshotTurns[0].Turn.ID != turn.ID || snapshotTurns[0].Run.RowVersion != 3 || len(snapshotTurns[0].Events) != 3 || snapshotTurns[0].Events[2].Sequence != 3 || len(snapshotTurns[0].Messages) != 1 || snapshotTurns[0].Messages[0].Content != turn.Input {
		t.Fatalf("snapshot turns = %+v", snapshotTurns)
	}
}

func TestSQLRepositoryCreateSessionRunRollsBack(t *testing.T) {
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
	now := time.Unix(100, 0).UTC()
	session := Session{ID: "session-1", FamilyID: "family-1", PetID: "pet-1", CreatedBy: "user-1", Status: SessionActive, RiskLevel: RiskUnknown, PromptVersion: "prompt-v1", RuleVersion: "rule-v1", KnowledgeVersion: "knowledge-v1", CreatedAt: now, UpdatedAt: now}
	turn := Turn{ID: "turn-1", SessionID: session.ID, TurnIndex: 0, Status: TurnReceived, Input: "最近没有精神", SelectedRunID: "run-1", CreatedAt: now}
	run := Run{ID: "run-1", SessionID: session.ID, TurnID: "missing-turn", RunIndex: 0, Status: RunQueued, RiskLevel: RiskUnknown, RuleVersion: "rule-v1", PromptVersion: "prompt-v1", CreatedAt: now}
	event := Event{ID: "event-1", SessionID: session.ID, TurnID: turn.ID, RunID: run.ID, Sequence: 1, Type: "run.queued", Data: `{}`, CreatedAt: now}
	if err := repository.CreateSessionRun(context.Background(), session, turn, run, event); err == nil {
		t.Fatal("CreateSessionRun() error = nil")
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM ask_sessions").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("session count = %d, want 0", count)
	}
}

func TestSQLRepositoryClaimsAndRecoversRunLease(t *testing.T) {
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
	now := time.Date(2026, 9, 10, 10, 0, 0, 0, time.UTC)
	session := Session{ID: "session-lease", FamilyID: "family-1", PetID: "pet-1", CreatedBy: "user-1", Status: SessionActive, RiskLevel: RiskUnknown, TurnCount: 1, PromptVersion: "prompt", RuleVersion: "rule", KnowledgeVersion: "knowledge", CreatedAt: now, UpdatedAt: now}
	turn := Turn{ID: "turn-lease", SessionID: session.ID, Status: TurnReceived, SelectedRunID: "run-lease", Input: "问题", CreatedAt: now}
	run := Run{ID: "run-lease", SessionID: session.ID, TurnID: turn.ID, RowVersion: 1, Status: RunQueued, RiskLevel: RiskUnknown, RuleVersion: "rule", PromptVersion: "prompt", CreatedAt: now}
	event := Event{ID: "event-lease", SessionID: session.ID, TurnID: turn.ID, RunID: run.ID, Sequence: 1, Type: "run.queued", Data: `{}`, CreatedAt: now}
	if err := repository.CreateSessionRun(context.Background(), session, turn, run, event); err != nil {
		t.Fatal(err)
	}
	jobs, err := repository.ListRunnableRunJobs(context.Background(), now, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || jobs[0].RunID != run.ID {
		t.Fatalf("runnable jobs = %+v", jobs)
	}
	claimed, ok, err := repository.ClaimRunJob(context.Background(), jobs[0], "worker-1", now, time.Minute, 3)
	if err != nil || !ok || claimed.AttemptCount != 1 || claimed.RowVersion != 2 {
		t.Fatalf("first claim = %+v, ok = %t, error = %v", claimed, ok, err)
	}
	if _, ok, err := repository.ClaimRunJob(context.Background(), jobs[0], "worker-2", now, time.Minute, 3); err != nil || ok {
		t.Fatalf("competing claim ok = %t, error = %v", ok, err)
	}
	if err := repository.TransitionRun(context.Background(), run.ID, claimed.RowVersion, RunQueued, RunRunning, RiskUnknown, "", now, Event{ID: "event-started", SessionID: session.ID, TurnID: turn.ID, RunID: run.ID, Sequence: 2, Type: "run.started", Data: `{}`, CreatedAt: now}, Message{}); err != nil {
		t.Fatal(err)
	}
	if err := repository.RenewRunLease(context.Background(), run.ID, "worker-1", now.Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	recovered, ok, err := repository.ClaimRunJob(context.Background(), RunJob{FamilyID: session.FamilyID, SessionID: session.ID, RunID: run.ID, RowVersion: 3}, "worker-2", now, time.Minute, 3)
	if err != nil || !ok || recovered.RowVersion != 4 || recovered.AttemptCount != 2 {
		t.Fatalf("recovered claim = %+v, ok = %t, error = %v", recovered, ok, err)
	}
	if err := repository.TransitionRun(context.Background(), run.ID, 3, RunRunning, RunCompleted, RiskGreen, "", now, Event{ID: "event-late", SessionID: session.ID, TurnID: turn.ID, RunID: run.ID, Sequence: 3, Type: "run.completed", Data: `{}`, CreatedAt: now}, Message{}); !errors.Is(err, ErrRunStateConflict) {
		t.Fatalf("late transition error = %v", err)
	}
	loaded, err := repository.GetRun(context.Background(), session.ID, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Status != RunQueued || loaded.RowVersion != 4 || loaded.LeaseOwner != "worker-2" || loaded.AttemptCount != 2 {
		t.Fatalf("recovered run = %+v", loaded)
	}
	events, err := repository.ListEvents(context.Background(), session.ID, run.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 || events[2].Type != "run.recovered" {
		t.Fatalf("recovery events = %+v", events)
	}
}

func TestSQLRepositoryReclaimingQueuedLeaseRejectsLateWorker(t *testing.T) {
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
	now := time.Date(2026, 9, 10, 10, 0, 0, 0, time.UTC)
	session := Session{ID: "session-queued-lease", FamilyID: "family-1", PetID: "pet-1", CreatedBy: "user-1", Status: SessionActive, RiskLevel: RiskUnknown, TurnCount: 1, PromptVersion: "prompt", RuleVersion: "rule", KnowledgeVersion: "knowledge", CreatedAt: now, UpdatedAt: now}
	turn := Turn{ID: "turn-queued-lease", SessionID: session.ID, Status: TurnReceived, SelectedRunID: "run-queued-lease", Input: "问题", CreatedAt: now}
	run := Run{ID: "run-queued-lease", SessionID: session.ID, TurnID: turn.ID, RowVersion: 1, Status: RunQueued, RiskLevel: RiskUnknown, RuleVersion: "rule", PromptVersion: "prompt", CreatedAt: now}
	event := Event{ID: "event-queued-lease", SessionID: session.ID, TurnID: turn.ID, RunID: run.ID, Sequence: 1, Type: "run.queued", Data: `{}`, CreatedAt: now}
	if err := repository.CreateSessionRun(context.Background(), session, turn, run, event); err != nil {
		t.Fatal(err)
	}
	first, ok, err := repository.ClaimRunJob(context.Background(), RunJob{FamilyID: session.FamilyID, SessionID: session.ID, RunID: run.ID, RowVersion: 1}, "worker-1", now, time.Minute, 3)
	if err != nil || !ok || first.RowVersion != 2 {
		t.Fatalf("first claim = %+v, ok = %t, error = %v", first, ok, err)
	}
	second, ok, err := repository.ClaimRunJob(context.Background(), first, "worker-2", now.Add(time.Minute), time.Minute, 3)
	if err != nil || !ok || second.RowVersion != 3 {
		t.Fatalf("second claim = %+v, ok = %t, error = %v", second, ok, err)
	}
	if err := repository.TransitionRun(context.Background(), run.ID, first.RowVersion, RunQueued, RunRunning, RiskUnknown, "", now.Add(time.Minute), Event{ID: "event-late-queued", SessionID: session.ID, TurnID: turn.ID, RunID: run.ID, Sequence: 2, Type: "run.started", Data: `{}`, CreatedAt: now.Add(time.Minute)}, Message{}); !errors.Is(err, ErrRunStateConflict) {
		t.Fatalf("late transition error = %v", err)
	}
}

func TestSQLRepositoryPersistsRetryAndFailsAfterAttemptLimit(t *testing.T) {
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
	now := time.Date(2026, 9, 10, 10, 0, 0, 0, time.UTC)
	session := Session{ID: "session-retry", FamilyID: "family-1", PetID: "pet-1", CreatedBy: "user-1", Status: SessionActive, RiskLevel: RiskUnknown, TurnCount: 1, PromptVersion: "prompt", RuleVersion: "rule", KnowledgeVersion: "knowledge", CreatedAt: now, UpdatedAt: now}
	turn := Turn{ID: "turn-retry", SessionID: session.ID, Status: TurnReceived, SelectedRunID: "run-retry", Input: "问题", CreatedAt: now}
	run := Run{ID: "run-retry", SessionID: session.ID, TurnID: turn.ID, RowVersion: 1, Status: RunQueued, RiskLevel: RiskUnknown, RuleVersion: "rule", PromptVersion: "prompt", CreatedAt: now}
	event := Event{ID: "event-retry", SessionID: session.ID, TurnID: turn.ID, RunID: run.ID, Sequence: 1, Type: "run.queued", Data: `{}`, CreatedAt: now}
	if err := repository.CreateSessionRun(context.Background(), session, turn, run, event); err != nil {
		t.Fatal(err)
	}
	job := RunJob{FamilyID: session.FamilyID, SessionID: session.ID, RunID: run.ID, RowVersion: 1}
	claimed, ok, err := repository.ClaimRunJob(context.Background(), job, "worker-1", now, time.Minute, 2)
	if err != nil || !ok {
		t.Fatalf("claim ok = %t, error = %v", ok, err)
	}
	nextAttempt := now.Add(time.Minute)
	failed, err := repository.RetryRunJob(context.Background(), claimed, "worker-1", now, nextAttempt, 2)
	if err != nil || failed {
		t.Fatalf("first retry failed = %t, error = %v", failed, err)
	}
	if jobs, err := repository.ListRunnableRunJobs(context.Background(), now, 10); err != nil || len(jobs) != 0 {
		t.Fatalf("early retry jobs = %+v, error = %v", jobs, err)
	}
	jobs, err := repository.ListRunnableRunJobs(context.Background(), nextAttempt, 10)
	if err != nil || len(jobs) != 1 || jobs[0].RowVersion != 3 {
		t.Fatalf("due retry jobs = %+v, error = %v", jobs, err)
	}
	claimed, ok, err = repository.ClaimRunJob(context.Background(), jobs[0], "worker-2", nextAttempt, time.Minute, 2)
	if err != nil || !ok || claimed.AttemptCount != 2 {
		t.Fatalf("second claim = %+v, ok = %t, error = %v", claimed, ok, err)
	}
	failed, err = repository.RetryRunJob(context.Background(), claimed, "worker-2", nextAttempt, nextAttempt.Add(time.Minute), 2)
	if err != nil || !failed {
		t.Fatalf("exhausted retry failed = %t, error = %v", failed, err)
	}
	loaded, err := repository.GetRun(context.Background(), session.ID, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Status != RunFailed || loaded.ErrorCode != "worker_attempts_exhausted" || loaded.LeaseOwner != "" || loaded.RowVersion != 5 {
		t.Fatalf("failed run = %+v", loaded)
	}
	events, err := repository.ListEvents(context.Background(), session.ID, run.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 || events[1].Type != "run.retry_scheduled" || events[2].Type != "run.failed" || !strings.Contains(events[2].Data, `"error_code":"worker_attempts_exhausted"`) || !strings.Contains(events[2].Data, "多次调用 AI 服务仍未成功") {
		t.Fatalf("retry events = %+v", events)
	}
}

func TestSQLRepositoryCreateFollowUpTurnRunRollsBack(t *testing.T) {
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
	now := time.Unix(100, 0).UTC()
	session := Session{ID: "session-1", FamilyID: "family-1", PetID: "pet-1", CreatedBy: "user-1", Status: SessionActive, RiskLevel: RiskUnknown, TurnCount: 1, PromptVersion: "prompt-v1", RuleVersion: "rule-v1", KnowledgeVersion: "knowledge-v1", CreatedAt: now, UpdatedAt: now}
	turn := Turn{ID: "turn-1", SessionID: session.ID, TurnIndex: 0, Status: TurnReceived, Input: "问题", SelectedRunID: "run-1", CreatedAt: now}
	run := Run{ID: "run-1", SessionID: session.ID, TurnID: turn.ID, RunIndex: 0, Status: RunQueued, RiskLevel: RiskUnknown, RuleVersion: "rule-v1", PromptVersion: "prompt-v1", CreatedAt: now}
	event := Event{ID: "event-1", SessionID: session.ID, TurnID: turn.ID, RunID: run.ID, Sequence: 1, Type: "run.queued", Data: `{}`, CreatedAt: now}
	if err := repository.CreateSessionRun(context.Background(), session, turn, run, event); err != nil {
		t.Fatal(err)
	}
	followTurn := Turn{ID: "turn-2", SessionID: session.ID, TurnIndex: 1, Status: TurnReceived, Input: "回答", SelectedRunID: "run-2", CreatedAt: now.Add(time.Minute)}
	followRun := Run{ID: "run-2", SessionID: session.ID, TurnID: followTurn.ID, RunIndex: 0, Status: RunQueued, RiskLevel: RiskUnknown, RuleVersion: "rule-v1", PromptVersion: "prompt-v1", CreatedAt: followTurn.CreatedAt}
	badEvent := Event{ID: "event-1", SessionID: session.ID, TurnID: followTurn.ID, RunID: followRun.ID, Sequence: 1, Type: "run.queued", Data: `{}`, CreatedAt: followTurn.CreatedAt}
	if err := repository.CreateFollowUpTurnRun(context.Background(), session, followTurn, followRun, badEvent); err == nil {
		t.Fatal("CreateFollowUpTurnRun() error = nil")
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM ask_turns").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("turn count = %d, want 1", count)
	}
}

func TestSQLRepositoryGetSessionNotFound(t *testing.T) {
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
	if _, err := repository.GetSession(context.Background(), "family-1", "missing"); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("error = %v, want %v", err, ErrSessionNotFound)
	}
}

func createAskSchema(t *testing.T, db *sql.DB) {
	t.Helper()
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		t.Fatal(err)
	}
	statements := []string{
		// v2：会话（Session 仅 active/closed；新增 input_sequence/event_sequence/launch_instance/generation + deleted_at）
		`CREATE TABLE ask_sessions (id TEXT PRIMARY KEY, family_id TEXT NOT NULL, pet_id TEXT NOT NULL, resolved_pet_id TEXT, created_by TEXT NOT NULL, status TEXT NOT NULL, risk_level TEXT NOT NULL, turn_count INTEGER NOT NULL, input_sequence INTEGER NOT NULL DEFAULT 0, event_sequence INTEGER NOT NULL DEFAULT 0, launch_instance TEXT NOT NULL DEFAULT '', generation INTEGER NOT NULL DEFAULT 1, prompt_version TEXT NOT NULL, rule_version TEXT NOT NULL, knowledge_version TEXT NOT NULL, created_at TIMESTAMP NOT NULL, updated_at TIMESTAMP NOT NULL, completed_at TIMESTAMP, deleted_at TIMESTAMP)`,
		`CREATE TABLE ask_session_pets (id TEXT PRIMARY KEY, session_id TEXT NOT NULL, pet_id TEXT NOT NULL, mention TEXT NOT NULL, sort_order INTEGER NOT NULL, UNIQUE(session_id, pet_id), UNIQUE(session_id, sort_order))`,
		// v2：Turn 独立状态 received/attached/superseded；新增 input_sequence/superseded_by + deleted_at
		`CREATE TABLE ask_turns (id TEXT PRIMARY KEY, session_id TEXT NOT NULL, turn_index INTEGER NOT NULL, input_sequence INTEGER NOT NULL DEFAULT 0, status TEXT NOT NULL, input TEXT NOT NULL, asset_refs TEXT NOT NULL DEFAULT '[]', selected_run_id TEXT NOT NULL, superseded_by TEXT, created_at TIMESTAMP NOT NULL, deleted_at TIMESTAMP, UNIQUE(session_id, turn_index))`,
		// v2：Run 新增 origin_turn_id/input_revision/execution_epoch/termination_reason/checkpoint + deleted_at
		`CREATE TABLE ask_runs (id TEXT PRIMARY KEY, session_id TEXT NOT NULL, turn_id TEXT NOT NULL REFERENCES ask_turns(id), origin_turn_id TEXT NOT NULL DEFAULT '', run_index INTEGER NOT NULL, row_version INTEGER NOT NULL DEFAULT 1, clarification_count INTEGER NOT NULL DEFAULT 0, status TEXT NOT NULL, risk_level TEXT NOT NULL, input_revision INTEGER NOT NULL DEFAULT 0, execution_epoch INTEGER NOT NULL DEFAULT 0, termination_reason TEXT NOT NULL DEFAULT '', checkpoint TEXT NOT NULL DEFAULT '', rule_version TEXT NOT NULL, prompt_version TEXT NOT NULL, created_at TIMESTAMP NOT NULL, started_at TIMESTAMP, completed_at TIMESTAMP, error_code TEXT NOT NULL DEFAULT '', lease_owner TEXT NOT NULL DEFAULT '', lease_expires_at TIMESTAMP, attempt_count INTEGER NOT NULL DEFAULT 0, next_attempt_at TIMESTAMP, deleted_at TIMESTAMP, UNIQUE(turn_id, run_index))`,
		`CREATE TABLE ask_events (id TEXT PRIMARY KEY, session_id TEXT NOT NULL, turn_id TEXT NOT NULL, run_id TEXT NOT NULL, sequence INTEGER NOT NULL, type TEXT NOT NULL, data TEXT NOT NULL, created_at TIMESTAMP NOT NULL, deleted_at TIMESTAMP, UNIQUE(run_id, sequence))`,
		`CREATE TABLE ask_messages (id TEXT PRIMARY KEY, session_id TEXT NOT NULL, turn_id TEXT NOT NULL, run_id TEXT, role TEXT NOT NULL, content TEXT NOT NULL, created_at TIMESTAMP NOT NULL, deleted_at TIMESTAMP)`,
		`CREATE TABLE ask_idempotency_keys (id TEXT PRIMARY KEY, family_id TEXT NOT NULL, user_id TEXT NOT NULL, operation TEXT NOT NULL, idempotency_key TEXT NOT NULL, request_hash TEXT NOT NULL, response_data TEXT NOT NULL, session_id TEXT NOT NULL, turn_id TEXT NOT NULL, run_id TEXT NOT NULL, created_at TIMESTAMP NOT NULL, deleted_at TIMESTAMP, UNIQUE(user_id, operation, idempotency_key))`,
		// v2 新增：至多一次模型请求
		`CREATE TABLE ask_attempts (id TEXT PRIMARY KEY, session_id TEXT NOT NULL, run_id TEXT NOT NULL, sequence INTEGER NOT NULL, status TEXT NOT NULL DEFAULT 'queued', purpose TEXT NOT NULL DEFAULT '', input_snapshot TEXT NOT NULL DEFAULT '', request_id TEXT NOT NULL DEFAULT '', error_code TEXT NOT NULL DEFAULT '', usage TEXT NOT NULL DEFAULT '', started_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, completed_at TIMESTAMP, deleted_at TIMESTAMP, UNIQUE(run_id, sequence))`,
		// v2 新增：任务项
		`CREATE TABLE ask_task_items (id TEXT PRIMARY KEY, run_id TEXT NOT NULL, origin_turn_id TEXT NOT NULL DEFAULT '', item_revision INTEGER NOT NULL DEFAULT 1, goal TEXT NOT NULL, source_turn_ids TEXT NOT NULL DEFAULT '[]', subjects TEXT NOT NULL DEFAULT '[]', outcome TEXT NOT NULL DEFAULT 'pending', missing_fields TEXT NOT NULL DEFAULT '[]', incomplete_reason TEXT NOT NULL DEFAULT '', result_ref TEXT NOT NULL DEFAULT '', supersedes TEXT NOT NULL DEFAULT '', superseded_by TEXT NOT NULL DEFAULT '', withdrawn_reason TEXT NOT NULL DEFAULT '', created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, deleted_at TIMESTAMP)`,
		// v2 新增：确认写入操作
		`CREATE TABLE ask_operations (id TEXT PRIMARY KEY, session_id TEXT NOT NULL, run_id TEXT NOT NULL, created_by TEXT NOT NULL, status TEXT NOT NULL DEFAULT 'pending', preview TEXT NOT NULL DEFAULT '', target TEXT NOT NULL DEFAULT '', payload TEXT NOT NULL DEFAULT '', result TEXT NOT NULL DEFAULT '', version INTEGER NOT NULL DEFAULT 0, confirmed_at TIMESTAMP, expires_at TIMESTAMP, verify_until TIMESTAMP, verify_count INTEGER NOT NULL DEFAULT 0, created_at TIMESTAMP NOT NULL, updated_at TIMESTAMP NOT NULL, deleted_at TIMESTAMP)`,
		// v2 新增：长期记忆
		`CREATE TABLE ask_memories (id TEXT PRIMARY KEY, family_id TEXT NOT NULL, pet_id TEXT NOT NULL DEFAULT '', kind TEXT NOT NULL, content TEXT NOT NULL, source_type TEXT NOT NULL DEFAULT '', source_id TEXT NOT NULL DEFAULT '', source_version TEXT NOT NULL DEFAULT '', active INTEGER NOT NULL DEFAULT 1, created_at TIMESTAMP NOT NULL, updated_at TIMESTAMP NOT NULL, deleted_at TIMESTAMP)`,
		// v2 新增：来源集合版本
		`CREATE TABLE ask_source_versions (source_type TEXT NOT NULL, source_id TEXT NOT NULL, version TEXT NOT NULL, versioned_at TIMESTAMP NOT NULL, PRIMARY KEY (source_type, source_id))`,
		// v2 新增：工具结果持久化与复用（7.7）
		`CREATE TABLE ask_tool_results (id TEXT PRIMARY KEY, session_id TEXT NOT NULL, run_id TEXT NOT NULL, tool_call_id TEXT NOT NULL, tool_name TEXT NOT NULL, tool_version TEXT NOT NULL, fingerprint TEXT NOT NULL, task_keys TEXT NOT NULL DEFAULT '[]', status TEXT NOT NULL, completeness TEXT NOT NULL DEFAULT '', has_more INTEGER NOT NULL DEFAULT 0, next_cursor TEXT NOT NULL DEFAULT '', text TEXT NOT NULL DEFAULT '', evidence TEXT NOT NULL DEFAULT '[]', source_type TEXT NOT NULL DEFAULT '', source_id TEXT NOT NULL DEFAULT '', source_version TEXT NOT NULL DEFAULT '', created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, deleted_at TIMESTAMP)`,
		// v2 新增：四类预算账本（9.3）
		`CREATE TABLE ask_budgets (id TEXT PRIMARY KEY, scope TEXT NOT NULL, scope_id TEXT NOT NULL, max_model_calls INTEGER NOT NULL DEFAULT 0, max_tool_calls INTEGER NOT NULL DEFAULT 0, max_tokens INTEGER NOT NULL DEFAULT 0, max_cost_micros BIGINT NOT NULL DEFAULT 0, max_duration_millis BIGINT NOT NULL DEFAULT 0, max_concurrency INTEGER NOT NULL DEFAULT 0, model_calls_used INTEGER NOT NULL DEFAULT 0, tool_calls_used INTEGER NOT NULL DEFAULT 0, tokens_used INTEGER NOT NULL DEFAULT 0, cost_used_micros BIGINT NOT NULL DEFAULT 0, duration_used_millis BIGINT NOT NULL DEFAULT 0, model_calls_reserved INTEGER NOT NULL DEFAULT 0, tool_calls_reserved INTEGER NOT NULL DEFAULT 0, tokens_reserved INTEGER NOT NULL DEFAULT 0, cost_reserved_micros BIGINT NOT NULL DEFAULT 0, duration_reserved_millis BIGINT NOT NULL DEFAULT 0, concurrency_reserved INTEGER NOT NULL DEFAULT 0, created_at TIMESTAMP NOT NULL, updated_at TIMESTAMP NOT NULL, UNIQUE(scope, scope_id))`,
		// v2 新增：原子预留记录（9.3）
		`CREATE TABLE ask_reservations (id TEXT PRIMARY KEY, budget_id TEXT NOT NULL REFERENCES ask_budgets(id), model_calls INTEGER NOT NULL DEFAULT 0, tool_calls INTEGER NOT NULL DEFAULT 0, tokens INTEGER NOT NULL DEFAULT 0, cost_micros BIGINT NOT NULL DEFAULT 0, duration_millis BIGINT NOT NULL DEFAULT 0, concurrency INTEGER NOT NULL DEFAULT 0, status TEXT NOT NULL DEFAULT 'reserved', created_at TIMESTAMP NOT NULL, settled_at TIMESTAMP, released_at TIMESTAMP)`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	// 9.2：同一用户 + 家庭至多一个当前 Session
	if _, err := db.Exec(`CREATE UNIQUE INDEX idx_ask_sessions_current_unique ON ask_sessions (family_id, created_by) WHERE status = 'active' AND deleted_at IS NULL`); err != nil {
		t.Fatal(err)
	}
}
