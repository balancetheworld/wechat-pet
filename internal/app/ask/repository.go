package ask

import (
	"context"
	"database/sql"
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
)

type Repository interface {
	CreateSessionRun(context.Context, Session, Turn, Run, Event) error
	GetSession(context.Context, string, string) (Session, error)
	GetRun(context.Context, string, string) (Run, error)
	GetTurn(context.Context, string, string) (Turn, error)
	CreateTurnRun(context.Context, Turn, Run, Event) error
	CreateFollowUpTurnRun(context.Context, Session, Turn, Run, Event) error
	TransitionRun(context.Context, string, RunStatus, RunStatus, RiskLevel, string, time.Time, Event) error
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
	var startedAt, completedAt sql.NullTime
	err := r.db.QueryRowContext(ctx, r.query("SELECT id, session_id, turn_id, run_index, status, risk_level, rule_version, prompt_version, created_at, started_at, completed_at, error_code FROM ask_runs WHERE id = ? AND session_id = ?"), runID, sessionID).Scan(&value.ID, &value.SessionID, &value.TurnID, &value.RunIndex, &value.Status, &value.RiskLevel, &value.RuleVersion, &value.PromptVersion, &value.CreatedAt, &startedAt, &completedAt, &value.ErrorCode)
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
	return value, nil
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

func (r *SQLRepository) TransitionRun(ctx context.Context, runID string, from, to RunStatus, riskLevel RiskLevel, errorCode string, at time.Time, event Event) error {
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
	if to == RunWaitingInput || to == RunCompleted || to == RunEscalated || to == RunFailed || to == RunCanceled || to == RunInterrupted {
		completedAt = at
	}
	if to == RunCompleted || to == RunEscalated || to == RunCanceled {
		sessionCompletedAt = at
	}
	result, err := tx.ExecContext(ctx, r.query("UPDATE ask_runs SET status = ?, risk_level = ?, started_at = COALESCE(started_at, ?), completed_at = COALESCE(?, completed_at), error_code = ? WHERE id = ? AND status = ?"), to, riskLevel, startedAt, completedAt, errorCode, runID, from)
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
	if r.driver != "postgres" && r.driver != "postgresql" && r.driver != "pgx" {
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
