package ask

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	petapp "github.com/balancetheworld/wechat-pet/internal/app/pet"
	_ "github.com/mattn/go-sqlite3"
)

// TestClaimRunJobIncrementsExecutionEpoch 验证每次领取（含恢复领取）执行代次 +1，
// 且与状态版本（row_version）是独立计数。
func TestClaimRunJobIncrementsExecutionEpoch(t *testing.T) {
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
	session := Session{ID: "session-epoch", FamilyID: "family-1", PetID: "pet-1", CreatedBy: "user-1", Status: SessionActive, RiskLevel: RiskUnknown, TurnCount: 1, PromptVersion: "prompt", RuleVersion: "rule", KnowledgeVersion: "knowledge", CreatedAt: now, UpdatedAt: now}
	turn := Turn{ID: "turn-epoch", SessionID: session.ID, Status: TurnReceived, SelectedRunID: "run-epoch", Input: "问题", CreatedAt: now}
	run := Run{ID: "run-epoch", SessionID: session.ID, TurnID: turn.ID, RowVersion: 1, Status: RunQueued, RiskLevel: RiskUnknown, RuleVersion: "rule", PromptVersion: "prompt", CreatedAt: now}
	event := Event{ID: "event-epoch", SessionID: session.ID, TurnID: turn.ID, RunID: run.ID, Sequence: 1, Type: "run.queued", Data: `{}`, CreatedAt: now}
	if err := repository.CreateSessionRun(context.Background(), session, turn, run, event); err != nil {
		t.Fatal(err)
	}
	first, ok, err := repository.ClaimRunJob(context.Background(), RunJob{FamilyID: session.FamilyID, SessionID: session.ID, RunID: run.ID, RowVersion: 1}, "worker-1", now, time.Minute, 3)
	if err != nil || !ok {
		t.Fatalf("first claim ok = %t, error = %v", ok, err)
	}
	if first.ExecutionEpoch != 1 {
		t.Fatalf("first claim execution epoch = %d, want 1", first.ExecutionEpoch)
	}
	if err := repository.TransitionRun(context.Background(), run.ID, first.RowVersion, RunQueued, RunRunning, RiskUnknown, "", now, Event{ID: "event-epoch-started", SessionID: session.ID, TurnID: turn.ID, RunID: run.ID, Sequence: 2, Type: "run.started", Data: `{}`, CreatedAt: now}, Message{}); err != nil {
		t.Fatal(err)
	}
	if err := repository.RenewRunLease(context.Background(), run.ID, "worker-1", now.Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	second, ok, err := repository.ClaimRunJob(context.Background(), RunJob{FamilyID: session.FamilyID, SessionID: session.ID, RunID: run.ID, RowVersion: 3}, "worker-2", now, time.Minute, 3)
	if err != nil || !ok {
		t.Fatalf("second claim ok = %t, error = %v", ok, err)
	}
	if second.ExecutionEpoch != 2 {
		t.Fatalf("second claim execution epoch = %d, want 2", second.ExecutionEpoch)
	}
	if second.RowVersion != 4 {
		t.Fatalf("second claim row version = %d, want 4", second.RowVersion)
	}
}

// TestProcessRunVersionRejectsStaleEpoch 验证提交时执行代次不匹配被拒绝：
// 即使状态版本匹配，旧 Worker 携带的执行代次过期也不能执行。
func TestProcessRunVersionRejectsStaleEpoch(t *testing.T) {
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
	service, err := NewService(repository, servicePetRepository{pet: petapp.Pet{ID: "pet-1", FamilyID: "family-1"}})
	if err != nil {
		t.Fatal(err)
	}
	created, err := service.CreateSession(context.Background(), "family-1", "user-1", "pet-1", "问题", "create-epoch")
	if err != nil {
		t.Fatal(err)
	}
	// 新 Run execution_epoch=0、row_version=1。传入匹配的版本但过期的代次（1），
	// 应被 epoch 校验拒绝，Run 保持 queued 不执行。
	result, err := service.ProcessRunVersion(context.Background(), "family-1", created.Session.ID, created.Run.ID, created.Run.RowVersion, 1)
	if err != nil {
		t.Fatal(err)
	}
	if result.Run.Status != RunQueued {
		t.Fatalf("stale epoch run status = %s, want queued", result.Run.Status)
	}
}

// TestRetryRunJobPersistsCheckpointAndReason 验证退避重试时保存检查点与恢复原因。
func TestRetryRunJobPersistsCheckpointAndReason(t *testing.T) {
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
	session := Session{ID: "session-retry-cp", FamilyID: "family-1", PetID: "pet-1", CreatedBy: "user-1", Status: SessionActive, RiskLevel: RiskUnknown, TurnCount: 1, PromptVersion: "prompt", RuleVersion: "rule", KnowledgeVersion: "knowledge", CreatedAt: now, UpdatedAt: now}
	turn := Turn{ID: "turn-retry-cp", SessionID: session.ID, Status: TurnReceived, SelectedRunID: "run-retry-cp", Input: "问题", CreatedAt: now}
	run := Run{ID: "run-retry-cp", SessionID: session.ID, TurnID: turn.ID, RowVersion: 1, Status: RunQueued, RiskLevel: RiskUnknown, RuleVersion: "rule", PromptVersion: "prompt", CreatedAt: now}
	event := Event{ID: "event-retry-cp", SessionID: session.ID, TurnID: turn.ID, RunID: run.ID, Sequence: 1, Type: "run.queued", Data: `{}`, CreatedAt: now}
	if err := repository.CreateSessionRun(context.Background(), session, turn, run, event); err != nil {
		t.Fatal(err)
	}
	claimed, ok, err := repository.ClaimRunJob(context.Background(), RunJob{FamilyID: session.FamilyID, SessionID: session.ID, RunID: run.ID, RowVersion: 1}, "worker-1", now, time.Minute, 3)
	if err != nil || !ok {
		t.Fatalf("claim ok = %t, error = %v", ok, err)
	}
	nextAttempt := now.Add(time.Minute)
	failed, err := repository.RetryRunJob(context.Background(), claimed, "worker-1", now, nextAttempt, 3)
	if err != nil || failed {
		t.Fatalf("retry failed = %t, error = %v", failed, err)
	}
	loaded, err := repository.GetRun(context.Background(), session.ID, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.TerminationReason != TerminationReasonRetryBackoff {
		t.Fatalf("termination reason = %q, want %q", loaded.TerminationReason, TerminationReasonRetryBackoff)
	}
	var checkpoint RunCheckpoint
	if err := json.Unmarshal([]byte(loaded.Checkpoint), &checkpoint); err != nil {
		t.Fatalf("checkpoint = %q, unmarshal error = %v", loaded.Checkpoint, err)
	}
	if checkpoint.Reason != TerminationReasonRetryBackoff {
		t.Fatalf("checkpoint reason = %q, want %q", checkpoint.Reason, TerminationReasonRetryBackoff)
	}
	if checkpoint.NextAttemptAt == nil || !checkpoint.NextAttemptAt.Equal(nextAttempt) {
		t.Fatalf("checkpoint next attempt = %v, want %v", checkpoint.NextAttemptAt, nextAttempt)
	}
}

// TestClaimRunJobRecoveryPersistsCheckpoint 验证崩溃恢复时保存检查点与 lease_recovered 原因。
func TestClaimRunJobRecoveryPersistsCheckpoint(t *testing.T) {
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
	session := Session{ID: "session-recover-cp", FamilyID: "family-1", PetID: "pet-1", CreatedBy: "user-1", Status: SessionActive, RiskLevel: RiskUnknown, TurnCount: 1, PromptVersion: "prompt", RuleVersion: "rule", KnowledgeVersion: "knowledge", CreatedAt: now, UpdatedAt: now}
	turn := Turn{ID: "turn-recover-cp", SessionID: session.ID, Status: TurnReceived, SelectedRunID: "run-recover-cp", Input: "问题", CreatedAt: now}
	run := Run{ID: "run-recover-cp", SessionID: session.ID, TurnID: turn.ID, RowVersion: 1, Status: RunQueued, RiskLevel: RiskUnknown, RuleVersion: "rule", PromptVersion: "prompt", CreatedAt: now}
	event := Event{ID: "event-recover-cp", SessionID: session.ID, TurnID: turn.ID, RunID: run.ID, Sequence: 1, Type: "run.queued", Data: `{}`, CreatedAt: now}
	if err := repository.CreateSessionRun(context.Background(), session, turn, run, event); err != nil {
		t.Fatal(err)
	}
	claimed, ok, err := repository.ClaimRunJob(context.Background(), RunJob{FamilyID: session.FamilyID, SessionID: session.ID, RunID: run.ID, RowVersion: 1}, "worker-1", now, time.Minute, 3)
	if err != nil || !ok {
		t.Fatalf("claim ok = %t, error = %v", ok, err)
	}
	if err := repository.TransitionRun(context.Background(), run.ID, claimed.RowVersion, RunQueued, RunRunning, RiskUnknown, "", now, Event{ID: "event-recover-cp-started", SessionID: session.ID, TurnID: turn.ID, RunID: run.ID, Sequence: 2, Type: "run.started", Data: `{}`, CreatedAt: now}, Message{}); err != nil {
		t.Fatal(err)
	}
	if err := repository.RenewRunLease(context.Background(), run.ID, "worker-1", now.Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := repository.ClaimRunJob(context.Background(), RunJob{FamilyID: session.FamilyID, SessionID: session.ID, RunID: run.ID, RowVersion: 3}, "worker-2", now, time.Minute, 3); err != nil || !ok {
		t.Fatalf("recover claim ok = %t, error = %v", ok, err)
	}
	loaded, err := repository.GetRun(context.Background(), session.ID, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.TerminationReason != TerminationReasonLeaseRecovered {
		t.Fatalf("termination reason = %q, want %q", loaded.TerminationReason, TerminationReasonLeaseRecovered)
	}
	var checkpoint RunCheckpoint
	if err := json.Unmarshal([]byte(loaded.Checkpoint), &checkpoint); err != nil {
		t.Fatalf("checkpoint = %q, unmarshal error = %v", loaded.Checkpoint, err)
	}
	if checkpoint.Reason != TerminationReasonLeaseRecovered {
		t.Fatalf("checkpoint reason = %q, want %q", checkpoint.Reason, TerminationReasonLeaseRecovered)
	}
}

// TestCanAutoRetry 验证模型自动重试的累计上限边界。
func TestCanAutoRetry(t *testing.T) {
	if !canAutoRetry(0) || !canAutoRetry(1) {
		t.Fatalf("auto retry should be allowed below %d", MaxModelAutoRetry)
	}
	if canAutoRetry(MaxModelAutoRetry) {
		t.Fatalf("auto retry should be blocked at %d", MaxModelAutoRetry)
	}
	if canAutoRetry(-1) {
		t.Fatalf("negative auto retry count should be blocked")
	}
}
