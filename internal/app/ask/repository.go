package ask

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"
)

var (
	ErrSessionNotFound      = errors.New("ask session not found")
	ErrRunNotFound          = errors.New("ask run not found")
	ErrTurnNotFound         = errors.New("ask turn not found")
	ErrRunStateConflict     = errors.New("ask run state conflict")
	ErrSessionStateConflict = errors.New("ask session state conflict")
	ErrEventConflict        = errors.New("ask event conflict")
	ErrIdempotencyNotFound  = errors.New("ask idempotency key not found")
	ErrIdempotencyConflict  = errors.New("ask idempotency key conflict")
	ErrRunLeaseLost         = errors.New("ask run lease lost")
)

type RunLeaseRepository interface {
	ListRunnableRunJobs(context.Context, time.Time, int) ([]RunJob, error)
	ClaimRunJob(context.Context, RunJob, string, time.Time, time.Duration, int) (RunJob, bool, error)
	RenewRunLease(context.Context, string, string, time.Time) error
	RetryRunJob(context.Context, RunJob, string, time.Time, time.Time, int) (bool, error)
}

type Repository interface {
	CreateSessionRun(context.Context, Session, Turn, Run, Event) error
	CreateSessionRunIdempotent(context.Context, Session, []SessionPet, Turn, Run, Event, Message, IdempotencyRecord) error
	GetSession(context.Context, string, string) (Session, error)
	GetRun(context.Context, string, string) (Run, error)
	GetTurn(context.Context, string, string) (Turn, error)
	GetIdempotency(context.Context, string, string, string) (IdempotencyRecord, error)
	GetSnapshotTurns(context.Context, string) ([]SnapshotTurn, error)
	ListMessages(context.Context, string, string) ([]Message, error)
	CreateTurnRun(context.Context, Turn, Run, Event) error
	CreateFollowUpTurnRun(context.Context, Session, Turn, Run, Event) error
	TransitionRun(context.Context, string, int, RunStatus, RunStatus, RiskLevel, string, time.Time, Event, Message) error
	AppendRunEvent(context.Context, Event) (Event, error)
	ResumeRun(context.Context, Run, time.Time, Event, Message, IdempotencyRecord) error
	ListEvents(context.Context, string, string, int) ([]Event, error)
}

type SQLRepository struct {
	db     *sql.DB
	driver string
}

func NewRepository(db *sql.DB, driver string) (*SQLRepository, error) {
	if db == nil {
		return nil, errors.New("database is required")
	}
	return &SQLRepository{db: db, driver: strings.ToLower(driver)}, nil
}

func (r *SQLRepository) CreateSessionRun(ctx context.Context, session Session, turn Turn, run Run, event Event) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, r.query("INSERT INTO ask_sessions (id, family_id, pet_id, created_by, status, risk_level, turn_count, prompt_version, rule_version, knowledge_version, created_at, updated_at, completed_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)"), session.ID, session.FamilyID, session.PetID, session.CreatedBy, session.Status, session.RiskLevel, session.TurnCount, session.PromptVersion, session.RuleVersion, session.KnowledgeVersion, session.CreatedAt, session.UpdatedAt, session.CompletedAt); err != nil {
		return err
	}
	if err := r.createTurnRun(ctx, tx, turn, run); err != nil {
		return err
	}
	if err := r.appendEvent(ctx, tx, event); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *SQLRepository) CreateSessionRunWithPets(ctx context.Context, session Session, pets []SessionPet, turn Turn, run Run, event Event) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, r.query("INSERT INTO ask_sessions (id, family_id, pet_id, created_by, status, risk_level, turn_count, prompt_version, rule_version, knowledge_version, created_at, updated_at, completed_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)"), session.ID, session.FamilyID, session.PetID, session.CreatedBy, session.Status, session.RiskLevel, session.TurnCount, session.PromptVersion, session.RuleVersion, session.KnowledgeVersion, session.CreatedAt, session.UpdatedAt, session.CompletedAt); err != nil {
		return err
	}
	for _, pet := range pets {
		if _, err := tx.ExecContext(ctx, r.query("INSERT INTO ask_session_pets (id, session_id, pet_id, mention, sort_order) VALUES (?, ?, ?, ?, ?)"), petRecordID(session.ID, pet.SortOrder), session.ID, pet.PetID, pet.Mention, pet.SortOrder); err != nil {
			return err
		}
	}
	if err := r.createTurnRun(ctx, tx, turn, run); err != nil {
		return err
	}
	if err := r.appendEvent(ctx, tx, event); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *SQLRepository) CreateSessionRunIdempotent(ctx context.Context, session Session, pets []SessionPet, turn Turn, run Run, event Event, message Message, idempotency IdempotencyRecord) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, r.query("INSERT INTO ask_sessions (id, family_id, pet_id, created_by, status, risk_level, turn_count, prompt_version, rule_version, knowledge_version, created_at, updated_at, completed_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)"), session.ID, session.FamilyID, session.PetID, session.CreatedBy, session.Status, session.RiskLevel, session.TurnCount, session.PromptVersion, session.RuleVersion, session.KnowledgeVersion, session.CreatedAt, session.UpdatedAt, session.CompletedAt); err != nil {
		return err
	}
	for _, pet := range pets {
		if _, err := tx.ExecContext(ctx, r.query("INSERT INTO ask_session_pets (id, session_id, pet_id, mention, sort_order) VALUES (?, ?, ?, ?, ?)"), petRecordID(session.ID, pet.SortOrder), session.ID, pet.PetID, pet.Mention, pet.SortOrder); err != nil {
			return err
		}
	}
	if err := r.createTurnRun(ctx, tx, turn, run); err != nil {
		return err
	}
	if err := r.appendEvent(ctx, tx, event); err != nil {
		return err
	}
	if err := r.appendMessage(ctx, tx, message); err != nil {
		return err
	}
	if err := r.appendIdempotency(ctx, tx, idempotency); err != nil {
		return err
	}
	return tx.Commit()
}

func petRecordID(sessionID string, sortOrder int) string {
	return sessionID + "-pet-" + strconv.Itoa(sortOrder)
}

func (r *SQLRepository) GetSession(ctx context.Context, familyID, sessionID string) (Session, error) {
	var value Session
	var completedAt sql.NullTime
	err := r.db.QueryRowContext(ctx, r.query("SELECT id, family_id, pet_id, created_by, status, risk_level, turn_count, prompt_version, rule_version, knowledge_version, created_at, updated_at, completed_at FROM ask_sessions WHERE id = ? AND family_id = ?"), sessionID, familyID).Scan(&value.ID, &value.FamilyID, &value.PetID, &value.CreatedBy, &value.Status, &value.RiskLevel, &value.TurnCount, &value.PromptVersion, &value.RuleVersion, &value.KnowledgeVersion, &value.CreatedAt, &value.UpdatedAt, &completedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, ErrSessionNotFound
	}
	if err != nil {
		return Session{}, err
	}
	if completedAt.Valid {
		value.CompletedAt = &completedAt.Time
	}
	return value, nil
}

func (r *SQLRepository) GetIdempotency(ctx context.Context, userID, operation, key string) (IdempotencyRecord, error) {
	var value IdempotencyRecord
	err := r.db.QueryRowContext(ctx, r.query("SELECT id, family_id, user_id, operation, idempotency_key, request_hash, response_data, session_id, turn_id, run_id, created_at FROM ask_idempotency_keys WHERE user_id = ? AND operation = ? AND idempotency_key = ?"), userID, operation, key).Scan(&value.ID, &value.FamilyID, &value.UserID, &value.Operation, &value.IdempotencyKey, &value.RequestHash, &value.ResponseData, &value.SessionID, &value.TurnID, &value.RunID, &value.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return IdempotencyRecord{}, ErrIdempotencyNotFound
	}
	if err != nil {
		return IdempotencyRecord{}, err
	}
	return value, nil
}

func (r *SQLRepository) ListSessionPets(ctx context.Context, sessionID string) ([]SessionPet, error) {
	rows, err := r.db.QueryContext(ctx, r.query("SELECT pet_id, mention, sort_order FROM ask_session_pets WHERE session_id = ? ORDER BY sort_order"), sessionID)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no such table") || strings.Contains(strings.ToLower(err.Error()), "does not exist") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	result := make([]SessionPet, 0)
	for rows.Next() {
		var value SessionPet
		if err := rows.Scan(&value.PetID, &value.Mention, &value.SortOrder); err != nil {
			return nil, err
		}
		value.PetName = value.Mention
		result = append(result, value)
	}
	return result, rows.Err()
}

func (r *SQLRepository) GetRun(ctx context.Context, sessionID, runID string) (Run, error) {
	var value Run
	var startedAt, completedAt, leaseExpiresAt, nextAttemptAt sql.NullTime
	err := r.db.QueryRowContext(ctx, r.query("SELECT id, session_id, turn_id, run_index, row_version, clarification_count, status, risk_level, rule_version, prompt_version, created_at, started_at, completed_at, error_code, lease_owner, lease_expires_at, attempt_count, next_attempt_at FROM ask_runs WHERE id = ? AND session_id = ?"), runID, sessionID).Scan(&value.ID, &value.SessionID, &value.TurnID, &value.RunIndex, &value.RowVersion, &value.ClarificationCount, &value.Status, &value.RiskLevel, &value.RuleVersion, &value.PromptVersion, &value.CreatedAt, &startedAt, &completedAt, &value.ErrorCode, &value.LeaseOwner, &leaseExpiresAt, &value.AttemptCount, &nextAttemptAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Run{}, ErrRunNotFound
	}
	if err != nil {
		return Run{}, err
	}
	if startedAt.Valid {
		value.StartedAt = &startedAt.Time
	}
	if completedAt.Valid {
		value.CompletedAt = &completedAt.Time
	}
	if leaseExpiresAt.Valid {
		value.LeaseExpiresAt = &leaseExpiresAt.Time
	}
	if nextAttemptAt.Valid {
		value.NextAttemptAt = &nextAttemptAt.Time
	}
	return value, nil
}

func (r *SQLRepository) ListRunnableRunJobs(ctx context.Context, now time.Time, limit int) ([]RunJob, error) {
	if limit < 1 {
		return nil, errors.New("ask runnable run limit must be positive")
	}
	query := "SELECT s.family_id, r.session_id, r.id, r.row_version FROM ask_runs r INNER JOIN ask_sessions s ON s.id = r.session_id WHERE (r.status = ? AND (r.next_attempt_at IS NULL OR r.next_attempt_at <= ?) AND (r.lease_expires_at IS NULL OR r.lease_expires_at <= ?)) OR (r.status = ? AND r.lease_expires_at IS NOT NULL AND r.lease_expires_at <= ?) ORDER BY r.created_at, r.id LIMIT ?"
	rows, err := r.db.QueryContext(ctx, r.query(query), RunQueued, now, now, RunRunning, now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]RunJob, 0)
	for rows.Next() {
		var value RunJob
		if err := rows.Scan(&value.FamilyID, &value.SessionID, &value.RunID, &value.RowVersion); err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (r *SQLRepository) ClaimRunJob(ctx context.Context, job RunJob, owner string, now time.Time, leaseDuration time.Duration, maxAttempts int) (RunJob, bool, error) {
	if owner == "" || leaseDuration <= 0 || maxAttempts < 1 {
		return RunJob{}, false, errors.New("ask run lease parameters are invalid")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return RunJob{}, false, err
	}
	defer tx.Rollback()
	query := "SELECT s.family_id, r.session_id, r.turn_id, r.row_version, r.status, r.attempt_count, r.lease_owner, r.lease_expires_at, r.next_attempt_at FROM ask_runs r INNER JOIN ask_sessions s ON s.id = r.session_id WHERE r.id = ? AND r.session_id = ?"
	if r.isPostgres() {
		query += " FOR UPDATE"
	}
	var familyID, sessionID, turnID, leaseOwner string
	var rowVersion, attemptCount int
	var status RunStatus
	var leaseExpiresAt, nextAttemptAt sql.NullTime
	err = tx.QueryRowContext(ctx, r.query(query), job.RunID, job.SessionID).Scan(&familyID, &sessionID, &turnID, &rowVersion, &status, &attemptCount, &leaseOwner, &leaseExpiresAt, &nextAttemptAt)
	if errors.Is(err, sql.ErrNoRows) {
		return RunJob{}, false, nil
	}
	if err != nil {
		return RunJob{}, false, err
	}
	if familyID != job.FamilyID || rowVersion != job.RowVersion || !runLeaseEligible(status, leaseExpiresAt, nextAttemptAt, now) {
		return RunJob{}, false, nil
	}
	if attemptCount >= maxAttempts {
		if err := r.failRunAttemptTx(ctx, tx, sessionID, turnID, job.RunID, rowVersion, status, now); err != nil {
			return RunJob{}, false, err
		}
		return RunJob{}, false, tx.Commit()
	}
	leaseExpires := now.Add(leaseDuration)
	claimedVersion := rowVersion + 1
	var result sql.Result
	if status == RunQueued {
		result, err = tx.ExecContext(ctx, r.query("UPDATE ask_runs SET row_version = row_version + 1, lease_owner = ?, lease_expires_at = ?, attempt_count = attempt_count + 1, next_attempt_at = NULL WHERE id = ? AND session_id = ? AND status = ? AND row_version = ? AND (lease_expires_at IS NULL OR lease_expires_at <= ?) AND (next_attempt_at IS NULL OR next_attempt_at <= ?)"), owner, leaseExpires, job.RunID, sessionID, RunQueued, rowVersion, now, now)
	} else {
		result, err = tx.ExecContext(ctx, r.query("UPDATE ask_runs SET status = ?, row_version = row_version + 1, lease_owner = ?, lease_expires_at = ?, attempt_count = attempt_count + 1, next_attempt_at = NULL, completed_at = NULL, error_code = '' WHERE id = ? AND session_id = ? AND status = ? AND row_version = ? AND lease_expires_at IS NOT NULL AND lease_expires_at <= ?"), RunQueued, owner, leaseExpires, job.RunID, sessionID, RunRunning, rowVersion, now)
	}
	if err != nil {
		return RunJob{}, false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return RunJob{}, false, err
	}
	if affected == 0 {
		return RunJob{}, false, nil
	}
	if status == RunRunning {
		if err := r.updateRecoveredRunTx(ctx, tx, sessionID, turnID, job.RunID, leaseOwner, now); err != nil {
			return RunJob{}, false, err
		}
	}
	if err := tx.Commit(); err != nil {
		return RunJob{}, false, err
	}
	return RunJob{FamilyID: familyID, SessionID: sessionID, RunID: job.RunID, RowVersion: claimedVersion, AttemptCount: attemptCount + 1}, true, nil
}

func (r *SQLRepository) RenewRunLease(ctx context.Context, runID, owner string, expiresAt time.Time) error {
	result, err := r.db.ExecContext(ctx, r.query("UPDATE ask_runs SET lease_expires_at = ? WHERE id = ? AND lease_owner = ? AND status IN (?, ?)"), expiresAt, runID, owner, RunQueued, RunRunning)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrRunLeaseLost
	}
	return nil
}

func (r *SQLRepository) RetryRunJob(ctx context.Context, job RunJob, owner string, now, nextAttemptAt time.Time, maxAttempts int) (bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	query := "SELECT turn_id, row_version, status, attempt_count FROM ask_runs WHERE id = ? AND session_id = ? AND lease_owner = ?"
	if r.isPostgres() {
		query += " FOR UPDATE"
	}
	var turnID string
	var rowVersion, attemptCount int
	var status RunStatus
	err = tx.QueryRowContext(ctx, r.query(query), job.RunID, job.SessionID, owner).Scan(&turnID, &rowVersion, &status, &attemptCount)
	if errors.Is(err, sql.ErrNoRows) {
		return false, ErrRunLeaseLost
	}
	if err != nil {
		return false, err
	}
	if status != RunQueued && status != RunRunning {
		return false, ErrRunLeaseLost
	}
	if attemptCount >= maxAttempts {
		if err := r.failRunAttemptTx(ctx, tx, job.SessionID, turnID, job.RunID, rowVersion, status, now); err != nil {
			return false, err
		}
		return true, tx.Commit()
	}
	result, err := tx.ExecContext(ctx, r.query("UPDATE ask_runs SET status = ?, row_version = row_version + 1, lease_owner = '', lease_expires_at = NULL, next_attempt_at = ?, completed_at = NULL, error_code = '' WHERE id = ? AND session_id = ? AND status = ? AND row_version = ? AND lease_owner = ?"), RunQueued, nextAttemptAt, job.RunID, job.SessionID, status, rowVersion, owner)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if affected == 0 {
		return false, ErrRunLeaseLost
	}
	if _, err := tx.ExecContext(ctx, r.query("UPDATE ask_turns SET status = ? WHERE id = ? AND selected_run_id = ?"), RunQueued, turnID, job.RunID); err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, r.query("UPDATE ask_sessions SET status = ?, updated_at = ?, completed_at = NULL WHERE id = ?"), SessionActive, now, job.SessionID); err != nil {
		return false, err
	}
	data, err := json.Marshal(map[string]any{"attempt_count": attemptCount, "next_attempt_at": nextAttemptAt})
	if err != nil {
		return false, err
	}
	if err := r.appendRunEventTx(ctx, tx, job.SessionID, turnID, job.RunID, "run.retry_scheduled", string(data), now); err != nil {
		return false, err
	}
	return false, tx.Commit()
}

func runLeaseEligible(status RunStatus, leaseExpiresAt, nextAttemptAt sql.NullTime, now time.Time) bool {
	if status == RunQueued {
		return (!leaseExpiresAt.Valid || !leaseExpiresAt.Time.After(now)) && (!nextAttemptAt.Valid || !nextAttemptAt.Time.After(now))
	}
	return status == RunRunning && leaseExpiresAt.Valid && !leaseExpiresAt.Time.After(now)
}

func (r *SQLRepository) updateRecoveredRunTx(ctx context.Context, tx *sql.Tx, sessionID, turnID, runID, previousOwner string, now time.Time) error {
	if _, err := tx.ExecContext(ctx, r.query("UPDATE ask_turns SET status = ? WHERE id = ? AND selected_run_id = ?"), RunQueued, turnID, runID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, r.query("UPDATE ask_sessions SET status = ?, updated_at = ?, completed_at = NULL WHERE id = ?"), SessionActive, now, sessionID); err != nil {
		return err
	}
	data, err := json.Marshal(map[string]any{"previous_lease_owner": previousOwner})
	if err != nil {
		return err
	}
	return r.appendRunEventTx(ctx, tx, sessionID, turnID, runID, "run.recovered", string(data), now)
}

func (r *SQLRepository) failRunAttemptTx(ctx context.Context, tx *sql.Tx, sessionID, turnID, runID string, rowVersion int, status RunStatus, now time.Time) error {
	result, err := tx.ExecContext(ctx, r.query("UPDATE ask_runs SET status = ?, row_version = row_version + 1, risk_level = ?, lease_owner = '', lease_expires_at = NULL, next_attempt_at = NULL, completed_at = ?, error_code = ? WHERE id = ? AND session_id = ? AND status = ? AND row_version = ?"), RunFailed, RiskUnknown, now, "worker_attempts_exhausted", runID, sessionID, status, rowVersion)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrRunStateConflict
	}
	if _, err := tx.ExecContext(ctx, r.query("UPDATE ask_turns SET status = ? WHERE id = ? AND selected_run_id = ?"), RunFailed, turnID, runID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, r.query("UPDATE ask_sessions SET status = ?, risk_level = ?, updated_at = ? WHERE id = ?"), SessionActive, RiskUnknown, now, sessionID); err != nil {
		return err
	}
	return r.appendRunEventTx(ctx, tx, sessionID, turnID, runID, "run.failed", `{"message":"任务重试次数已达上限"}`, now)
}

func (r *SQLRepository) appendRunEventTx(ctx context.Context, tx *sql.Tx, sessionID, turnID, runID, eventType, data string, now time.Time) error {
	var sequence int
	if err := tx.QueryRowContext(ctx, r.query("SELECT COALESCE(MAX(sequence), 0) + 1 FROM ask_events WHERE run_id = ?"), runID).Scan(&sequence); err != nil {
		return err
	}
	id, err := newID()
	if err != nil {
		return err
	}
	return r.appendEvent(ctx, tx, Event{ID: id, SessionID: sessionID, TurnID: turnID, RunID: runID, Sequence: sequence, Type: eventType, Data: data, CreatedAt: now})
}

func (r *SQLRepository) GetSnapshotTurns(ctx context.Context, sessionID string) ([]SnapshotTurn, error) {
	rows, err := r.db.QueryContext(ctx, r.query("SELECT t.id, t.session_id, t.turn_index, t.status, t.input, t.selected_run_id, t.created_at, r.id, r.session_id, r.turn_id, r.run_index, r.row_version, r.clarification_count, r.status, r.risk_level, r.rule_version, r.prompt_version, r.created_at, r.started_at, r.completed_at, r.error_code, r.lease_owner, r.lease_expires_at, r.attempt_count, r.next_attempt_at FROM ask_turns t INNER JOIN ask_runs r ON r.turn_id = t.id WHERE t.session_id = ? ORDER BY t.turn_index, r.run_index"), sessionID)
	if err != nil {
		return nil, err
	}
	result := make([]SnapshotTurn, 0)
	runIndexes := make(map[string]struct{ turnIndex, runIndex int })
	turnIndexes := make(map[string]int)
	for rows.Next() {
		var value SnapshotTurn
		var run SnapshotRun
		var startedAt, completedAt, leaseExpiresAt, nextAttemptAt sql.NullTime
		if err := rows.Scan(&value.Turn.ID, &value.Turn.SessionID, &value.Turn.TurnIndex, &value.Turn.Status, &value.Turn.Input, &value.Turn.SelectedRunID, &value.Turn.CreatedAt, &run.Run.ID, &run.Run.SessionID, &run.Run.TurnID, &run.Run.RunIndex, &run.Run.RowVersion, &run.Run.ClarificationCount, &run.Run.Status, &run.Run.RiskLevel, &run.Run.RuleVersion, &run.Run.PromptVersion, &run.Run.CreatedAt, &startedAt, &completedAt, &run.Run.ErrorCode, &run.Run.LeaseOwner, &leaseExpiresAt, &run.Run.AttemptCount, &nextAttemptAt); err != nil {
			rows.Close()
			return nil, err
		}
		if startedAt.Valid {
			run.Run.StartedAt = &startedAt.Time
		}
		if completedAt.Valid {
			run.Run.CompletedAt = &completedAt.Time
		}
		if leaseExpiresAt.Valid {
			run.Run.LeaseExpiresAt = &leaseExpiresAt.Time
		}
		if nextAttemptAt.Valid {
			run.Run.NextAttemptAt = &nextAttemptAt.Time
		}
		turnIndex, ok := turnIndexes[value.Turn.ID]
		if !ok {
			value.Events = make([]Event, 0)
			value.Messages = make([]Message, 0)
			value.Runs = make([]SnapshotRun, 0)
			turnIndex = len(result)
			turnIndexes[value.Turn.ID] = turnIndex
			result = append(result, value)
		}
		runIndex := len(result[turnIndex].Runs)
		result[turnIndex].Runs = append(result[turnIndex].Runs, run)
		runIndexes[run.Run.ID] = struct{ turnIndex, runIndex int }{turnIndex: turnIndex, runIndex: runIndex}
		if run.Run.ID == value.Turn.SelectedRunID {
			result[turnIndex].Run = run.Run
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	eventRows, err := r.db.QueryContext(ctx, r.query("SELECT id, session_id, turn_id, run_id, sequence, type, data, created_at FROM ask_events WHERE session_id = ? ORDER BY run_id, sequence"), sessionID)
	if err != nil {
		return nil, err
	}
	defer eventRows.Close()
	for eventRows.Next() {
		var value Event
		if err := eventRows.Scan(&value.ID, &value.SessionID, &value.TurnID, &value.RunID, &value.Sequence, &value.Type, &value.Data, &value.CreatedAt); err != nil {
			return nil, err
		}
		if index, ok := runIndexes[value.RunID]; ok {
			result[index.turnIndex].Runs[index.runIndex].Events = append(result[index.turnIndex].Runs[index.runIndex].Events, value)
			if result[index.turnIndex].Runs[index.runIndex].Run.ID == result[index.turnIndex].Turn.SelectedRunID {
				result[index.turnIndex].Events = append(result[index.turnIndex].Events, value)
			}
		}
	}
	if err := eventRows.Err(); err != nil {
		return nil, err
	}
	messageRows, err := r.db.QueryContext(ctx, r.query("SELECT id, session_id, turn_id, run_id, role, content, created_at FROM ask_messages WHERE session_id = ? ORDER BY created_at, id"), sessionID)
	if err != nil {
		return nil, err
	}
	defer messageRows.Close()
	for messageRows.Next() {
		var value Message
		if err := messageRows.Scan(&value.ID, &value.SessionID, &value.TurnID, &value.RunID, &value.Role, &value.Content, &value.CreatedAt); err != nil {
			return nil, err
		}
		if index, ok := runIndexes[value.RunID]; ok {
			result[index.turnIndex].Runs[index.runIndex].Messages = append(result[index.turnIndex].Runs[index.runIndex].Messages, value)
			if result[index.turnIndex].Runs[index.runIndex].Run.ID == result[index.turnIndex].Turn.SelectedRunID {
				result[index.turnIndex].Messages = append(result[index.turnIndex].Messages, value)
			}
		}
	}
	if err := messageRows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func (r *SQLRepository) GetTurn(ctx context.Context, sessionID, turnID string) (Turn, error) {
	var value Turn
	err := r.db.QueryRowContext(ctx, r.query("SELECT id, session_id, turn_index, status, input, selected_run_id, created_at FROM ask_turns WHERE id = ? AND session_id = ?"), turnID, sessionID).Scan(&value.ID, &value.SessionID, &value.TurnIndex, &value.Status, &value.Input, &value.SelectedRunID, &value.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Turn{}, ErrTurnNotFound
	}
	if err != nil {
		return Turn{}, err
	}
	return value, nil
}

func (r *SQLRepository) ListContextTurns(ctx context.Context, sessionID string, limit int) ([]ContextTurn, error) {
	rows, err := r.db.QueryContext(ctx, r.query("SELECT turn_index, input, status, created_at FROM ask_turns WHERE session_id = ? ORDER BY turn_index DESC LIMIT ?"), sessionID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]ContextTurn, 0)
	for rows.Next() {
		var value ContextTurn
		if err := rows.Scan(&value.TurnIndex, &value.Input, &value.Status, &value.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for left, right := 0, len(result)-1; left < right; left, right = left+1, right-1 {
		result[left], result[right] = result[right], result[left]
	}
	return result, nil
}

func (r *SQLRepository) ListMessages(ctx context.Context, sessionID, runID string) ([]Message, error) {
	rows, err := r.db.QueryContext(ctx, r.query("SELECT id, session_id, turn_id, run_id, role, content, created_at FROM ask_messages WHERE session_id = ? AND run_id = ? ORDER BY created_at, id"), sessionID, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Message, 0)
	for rows.Next() {
		var value Message
		if err := rows.Scan(&value.ID, &value.SessionID, &value.TurnID, &value.RunID, &value.Role, &value.Content, &value.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func (r *SQLRepository) CreateTurnRun(ctx context.Context, turn Turn, run Run, event Event) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := r.createTurnRun(ctx, tx, turn, run); err != nil {
		return err
	}
	if err := r.appendEvent(ctx, tx, event); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *SQLRepository) CreateFollowUpTurnRun(ctx context.Context, session Session, turn Turn, run Run, event Event) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := r.createTurnRun(ctx, tx, turn, run); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, r.query("UPDATE ask_sessions SET turn_count = ?, updated_at = ? WHERE id = ? AND status = ? AND turn_count = ?"), turn.TurnIndex+1, turn.CreatedAt, session.ID, SessionActive, session.TurnCount)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrSessionStateConflict
	}
	if err := r.appendEvent(ctx, tx, event); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *SQLRepository) createTurnRun(ctx context.Context, tx *sql.Tx, turn Turn, run Run) error {
	if _, err := tx.ExecContext(ctx, r.query("INSERT INTO ask_turns (id, session_id, turn_index, status, input, selected_run_id, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)"), turn.ID, turn.SessionID, turn.TurnIndex, turn.Status, turn.Input, turn.SelectedRunID, turn.CreatedAt); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, r.query("INSERT INTO ask_runs (id, session_id, turn_id, run_index, status, risk_level, rule_version, prompt_version, created_at, started_at, completed_at, error_code) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)"), run.ID, run.SessionID, run.TurnID, run.RunIndex, run.Status, run.RiskLevel, run.RuleVersion, run.PromptVersion, run.CreatedAt, run.StartedAt, run.CompletedAt, run.ErrorCode); err != nil {
		return err
	}
	return nil
}

func (r *SQLRepository) TransitionRun(ctx context.Context, runID string, expectedVersion int, from, to RunStatus, riskLevel RiskLevel, errorCode string, at time.Time, event Event, message Message) error {
	if !CanTransitionRun(from, to) {
		return ErrInvalidRunTransition
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	startedAt, completedAt := any(nil), any(nil)
	sessionCompletedAt := any(nil)
	if to == RunRunning {
		startedAt = at
	}
	if to == RunCompleted || to == RunEscalated || to == RunFailed || to == RunCanceled || to == RunInterrupted {
		completedAt = at
	}
	if to == RunCompleted || to == RunEscalated || to == RunCanceled {
		sessionCompletedAt = at
	}
	query := "UPDATE ask_runs SET status = ?, risk_level = ?, row_version = row_version + 1, started_at = COALESCE(started_at, ?), completed_at = ?, error_code = ? WHERE id = ? AND status = ? AND row_version = ?"
	if to != RunRunning {
		query = "UPDATE ask_runs SET status = ?, risk_level = ?, row_version = row_version + 1, started_at = COALESCE(started_at, ?), completed_at = ?, error_code = ?, lease_owner = '', lease_expires_at = NULL, next_attempt_at = NULL WHERE id = ? AND status = ? AND row_version = ?"
	}
	result, err := tx.ExecContext(ctx, r.query(query), to, riskLevel, startedAt, completedAt, errorCode, runID, from, expectedVersion)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrRunStateConflict
	}
	if _, err := tx.ExecContext(ctx, r.query("UPDATE ask_turns SET status = ? WHERE selected_run_id = ?"), to, runID); err != nil {
		return err
	}
	sessionStatus := SessionActive
	if to == RunCompleted {
		sessionStatus = SessionCompleted
	}
	if to == RunEscalated {
		sessionStatus = SessionEscalated
	}
	if to == RunCanceled {
		sessionStatus = SessionCanceled
	}
	if _, err := tx.ExecContext(ctx, r.query("UPDATE ask_sessions SET status = ?, risk_level = ?, updated_at = ?, completed_at = COALESCE(?, completed_at) WHERE id = (SELECT session_id FROM ask_runs WHERE id = ?)"), sessionStatus, riskLevel, at, sessionCompletedAt, runID); err != nil {
		return err
	}
	if err := r.appendEvent(ctx, tx, event); err != nil {
		return err
	}
	if err := r.appendMessage(ctx, tx, message); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *SQLRepository) AppendRunEvent(ctx context.Context, value Event) (Event, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return Event{}, err
	}
	defer tx.Rollback()
	if err := tx.QueryRowContext(ctx, r.query("SELECT COALESCE(MAX(sequence), 0) + 1 FROM ask_events WHERE run_id = ?"), value.RunID).Scan(&value.Sequence); err != nil {
		return Event{}, err
	}
	if err := r.appendEvent(ctx, tx, value); err != nil {
		return Event{}, err
	}
	if err := tx.Commit(); err != nil {
		return Event{}, err
	}
	return value, nil
}

func (r *SQLRepository) ResumeRun(ctx context.Context, run Run, at time.Time, event Event, message Message, idempotency IdempotencyRecord) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, r.query("UPDATE ask_runs SET status = ?, risk_level = ?, row_version = row_version + 1, clarification_count = clarification_count + 1, completed_at = NULL, error_code = '', lease_owner = '', lease_expires_at = NULL, attempt_count = 0, next_attempt_at = NULL WHERE id = ? AND status = ? AND row_version = ?"), RunQueued, RiskUnknown, run.ID, RunWaitingInput, run.RowVersion)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrRunStateConflict
	}
	if _, err := tx.ExecContext(ctx, r.query("UPDATE ask_turns SET status = ? WHERE id = ? AND selected_run_id = ?"), RunQueued, run.TurnID, run.ID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, r.query("UPDATE ask_sessions SET status = ?, risk_level = ?, updated_at = ?, completed_at = NULL WHERE id = ?"), SessionActive, RiskUnknown, at, run.SessionID); err != nil {
		return err
	}
	if err := r.appendMessage(ctx, tx, message); err != nil {
		return err
	}
	if err := r.appendEvent(ctx, tx, event); err != nil {
		return err
	}
	if err := r.appendIdempotency(ctx, tx, idempotency); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *SQLRepository) appendEvent(ctx context.Context, tx *sql.Tx, value Event) error {
	_, err := tx.ExecContext(ctx, r.query("INSERT INTO ask_events (id, session_id, turn_id, run_id, sequence, type, data, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)"), value.ID, value.SessionID, value.TurnID, value.RunID, value.Sequence, value.Type, value.Data, value.CreatedAt)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return ErrEventConflict
		}
		return err
	}
	return nil
}

func (r *SQLRepository) appendMessage(ctx context.Context, tx *sql.Tx, value Message) error {
	if value.ID == "" {
		return nil
	}
	_, err := tx.ExecContext(ctx, r.query("INSERT INTO ask_messages (id, session_id, turn_id, run_id, role, content, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)"), value.ID, value.SessionID, value.TurnID, value.RunID, value.Role, value.Content, value.CreatedAt)
	return err
}

func (r *SQLRepository) appendIdempotency(ctx context.Context, tx *sql.Tx, value IdempotencyRecord) error {
	_, err := tx.ExecContext(ctx, r.query("INSERT INTO ask_idempotency_keys (id, family_id, user_id, operation, idempotency_key, request_hash, response_data, session_id, turn_id, run_id, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)"), value.ID, value.FamilyID, value.UserID, value.Operation, value.IdempotencyKey, value.RequestHash, value.ResponseData, value.SessionID, value.TurnID, value.RunID, value.CreatedAt)
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "unique") {
		return ErrIdempotencyConflict
	}
	return err
}

func (r *SQLRepository) ListEvents(ctx context.Context, sessionID, runID string, after int) ([]Event, error) {
	rows, err := r.db.QueryContext(ctx, r.query("SELECT id, session_id, turn_id, run_id, sequence, type, data, created_at FROM ask_events WHERE session_id = ? AND run_id = ? AND sequence > ? ORDER BY sequence"), sessionID, runID, after)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Event, 0)
	for rows.Next() {
		var value Event
		if err := rows.Scan(&value.ID, &value.SessionID, &value.TurnID, &value.RunID, &value.Sequence, &value.Type, &value.Data, &value.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func (r *SQLRepository) query(value string) string {
	if !r.isPostgres() {
		return value
	}
	var result strings.Builder
	index := 0
	for _, character := range value {
		if character != '?' {
			result.WriteRune(character)
			continue
		}
		index++
		result.WriteByte('$')
		result.WriteString(strconv.Itoa(index))
	}
	return result.String()
}

func (r *SQLRepository) isPostgres() bool {
	return r.driver == "postgres" || r.driver == "postgresql" || r.driver == "pgx"
}
