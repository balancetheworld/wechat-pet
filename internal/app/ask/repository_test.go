package ask

import (
	"context"
	"database/sql"
	"errors"
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
	turn := Turn{ID: "turn-1", SessionID: session.ID, TurnIndex: 0, Status: RunQueued, Input: "最近没有精神", SelectedRunID: "run-1", CreatedAt: now}
	run := Run{ID: "run-1", SessionID: session.ID, TurnID: turn.ID, RunIndex: 0, Status: RunQueued, RiskLevel: RiskUnknown, RuleVersion: "rule-v1", PromptVersion: "prompt-v1", CreatedAt: now}
	queuedEvent := Event{ID: "event-1", SessionID: session.ID, TurnID: turn.ID, RunID: run.ID, Sequence: 1, Type: "run.queued", Data: `{}`, CreatedAt: now}
	if err := repository.CreateSessionRun(context.Background(), session, turn, run, queuedEvent); err != nil {
		t.Fatal(err)
	}
	startedEvent := Event{ID: "event-2", SessionID: session.ID, TurnID: turn.ID, RunID: run.ID, Sequence: 2, Type: "run.started", Data: `{}`, CreatedAt: now.Add(time.Minute)}
	if err := repository.TransitionRun(context.Background(), run.ID, RunQueued, RunRunning, RiskUnknown, "", now.Add(time.Minute), startedEvent); err != nil {
		t.Fatal(err)
	}
	completedEvent := Event{ID: "event-3", SessionID: session.ID, TurnID: turn.ID, RunID: run.ID, Sequence: 3, Type: "run.completed", Data: `{"risk_level":"green"}`, CreatedAt: now.Add(2 * time.Minute)}
	if err := repository.TransitionRun(context.Background(), run.ID, RunQueued, RunRunning, RiskGreen, "", now.Add(2*time.Minute), completedEvent); !errors.Is(err, ErrRunStateConflict) {
		t.Fatalf("conflict error = %v, want %v", err, ErrRunStateConflict)
	}
	if err := repository.TransitionRun(context.Background(), run.ID, RunRunning, RunCompleted, RiskGreen, "", now.Add(2*time.Minute), completedEvent); err != nil {
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
	if loaded.ID != session.ID || loaded.Status != SessionCompleted || loaded.RiskLevel != RiskGreen {
		t.Fatalf("loaded session = %+v", loaded)
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
	turn := Turn{ID: "turn-1", SessionID: session.ID, TurnIndex: 0, Status: RunQueued, Input: "最近没有精神", SelectedRunID: "run-1", CreatedAt: now}
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
	turn := Turn{ID: "turn-1", SessionID: session.ID, TurnIndex: 0, Status: RunQueued, Input: "问题", SelectedRunID: "run-1", CreatedAt: now}
	run := Run{ID: "run-1", SessionID: session.ID, TurnID: turn.ID, RunIndex: 0, Status: RunQueued, RiskLevel: RiskUnknown, RuleVersion: "rule-v1", PromptVersion: "prompt-v1", CreatedAt: now}
	event := Event{ID: "event-1", SessionID: session.ID, TurnID: turn.ID, RunID: run.ID, Sequence: 1, Type: "run.queued", Data: `{}`, CreatedAt: now}
	if err := repository.CreateSessionRun(context.Background(), session, turn, run, event); err != nil {
		t.Fatal(err)
	}
	followTurn := Turn{ID: "turn-2", SessionID: session.ID, TurnIndex: 1, Status: RunQueued, Input: "回答", SelectedRunID: "run-2", CreatedAt: now.Add(time.Minute)}
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
		`CREATE TABLE ask_sessions (id TEXT PRIMARY KEY, family_id TEXT NOT NULL, pet_id TEXT NOT NULL, created_by TEXT NOT NULL, status TEXT NOT NULL, risk_level TEXT NOT NULL, turn_count INTEGER NOT NULL, prompt_version TEXT NOT NULL, rule_version TEXT NOT NULL, knowledge_version TEXT NOT NULL, created_at TIMESTAMP NOT NULL, updated_at TIMESTAMP NOT NULL, completed_at TIMESTAMP)`,
		`CREATE TABLE ask_session_pets (id TEXT PRIMARY KEY, session_id TEXT NOT NULL, pet_id TEXT NOT NULL, mention TEXT NOT NULL, sort_order INTEGER NOT NULL, UNIQUE(session_id, pet_id), UNIQUE(session_id, sort_order))`,
		`CREATE TABLE ask_turns (id TEXT PRIMARY KEY, session_id TEXT NOT NULL, turn_index INTEGER NOT NULL, status TEXT NOT NULL, input TEXT NOT NULL, selected_run_id TEXT NOT NULL, created_at TIMESTAMP NOT NULL, UNIQUE(session_id, turn_index))`,
		`CREATE TABLE ask_runs (id TEXT PRIMARY KEY, session_id TEXT NOT NULL, turn_id TEXT NOT NULL REFERENCES ask_turns(id), run_index INTEGER NOT NULL, status TEXT NOT NULL, risk_level TEXT NOT NULL, rule_version TEXT NOT NULL, prompt_version TEXT NOT NULL, created_at TIMESTAMP NOT NULL, started_at TIMESTAMP, completed_at TIMESTAMP, error_code TEXT NOT NULL, UNIQUE(turn_id, run_index))`,
		`CREATE TABLE ask_events (id TEXT PRIMARY KEY, session_id TEXT NOT NULL, turn_id TEXT NOT NULL, run_id TEXT NOT NULL, sequence INTEGER NOT NULL, type TEXT NOT NULL, data TEXT NOT NULL, created_at TIMESTAMP NOT NULL, UNIQUE(run_id, sequence))`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
}
