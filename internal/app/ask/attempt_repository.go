package ask

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

var (
	// ErrAttemptNotFound 表示模型请求记录不存在。
	ErrAttemptNotFound = errors.New("ask attempt not found")
	// ErrAttemptStateConflict 表示 Attempt 当前状态与预期不符（含终态不可回开）。
	ErrAttemptStateConflict = errors.New("ask attempt state conflict")
)

// AttemptRepository 是模型请求（Attempt）的独立持久化契约。
// Attempt 与 Worker 领取次数（ask_runs.attempt_count）、工具执行次数分别记录，
// 不能共用一个计数表达（v2 文档 3.3）。
type AttemptRepository interface {
	CreateAttempt(context.Context, Attempt) error
	GetAttempt(context.Context, string) (Attempt, error)
	ListAttempts(context.Context, string) ([]Attempt, error)
	TransitionAttempt(context.Context, string, AttemptStatus, AttemptStatus, string, string, string, time.Time) error
}

// CreateAttempt 持久化一次新的模型请求。Sequence 为空时按 Run 内最大值 +1 分配；
// 状态强制为 queued（同一 Attempt 至多发出一次请求，发出前由 TransitionAttempt 推进）。
func (r *SQLRepository) CreateAttempt(ctx context.Context, attempt Attempt) error {
	if attempt.ID == "" || attempt.SessionID == "" || attempt.RunID == "" {
		return errors.New("ask attempt identity is required")
	}
	if attempt.StartedAt.IsZero() {
		attempt.StartedAt = time.Now().UTC()
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if r.isPostgres() {
		var locked any
		if err := tx.QueryRowContext(ctx, r.query("SELECT pg_advisory_xact_lock(hashtext(?))"), attempt.RunID).Scan(&locked); err != nil {
			return err
		}
	}
	sequence := attempt.Sequence
	if sequence <= 0 {
		if err := tx.QueryRowContext(ctx, r.query("SELECT COALESCE(MAX(sequence), 0) + 1 FROM ask_attempts WHERE run_id = ?"), attempt.RunID).Scan(&sequence); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, r.query("INSERT INTO ask_attempts (id, session_id, run_id, sequence, status, purpose, input_snapshot, request_id, error_code, usage, started_at, completed_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)"),
		attempt.ID, attempt.SessionID, attempt.RunID, sequence, AttemptQueued, attempt.Purpose, attempt.InputSnapshot, attempt.RequestID, "", "", attempt.StartedAt, attempt.CompletedAt); err != nil {
		return err
	}
	return tx.Commit()
}

// GetAttempt 按 id 读取一次模型请求记录。
func (r *SQLRepository) GetAttempt(ctx context.Context, attemptID string) (Attempt, error) {
	row := r.db.QueryRowContext(ctx, r.query("SELECT id, session_id, run_id, sequence, status, purpose, input_snapshot, request_id, error_code, usage, started_at, completed_at FROM ask_attempts WHERE id = ?"), attemptID)
	return scanAttempt(row)
}

// ListAttempts 按 sequence 升序列出一个 Run 的全部模型请求。
func (r *SQLRepository) ListAttempts(ctx context.Context, runID string) ([]Attempt, error) {
	rows, err := r.db.QueryContext(ctx, r.query("SELECT id, session_id, run_id, sequence, status, purpose, input_snapshot, request_id, error_code, usage, started_at, completed_at FROM ask_attempts WHERE run_id = ? ORDER BY sequence"), runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Attempt, 0)
	for rows.Next() {
		var value Attempt
		var completedAt sql.NullTime
		if err := rows.Scan(&value.ID, &value.SessionID, &value.RunID, &value.Sequence, &value.Status, &value.Purpose, &value.InputSnapshot, &value.RequestID, &value.ErrorCode, &value.Usage, &value.StartedAt, &completedAt); err != nil {
			return nil, err
		}
		if completedAt.Valid {
			value.CompletedAt = &completedAt.Time
		}
		result = append(result, value)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// TransitionAttempt 按 3.3 状态机条件更新 Attempt 状态。终态（succeeded/failed/canceled/unknown）
// 写入 completed_at；非法转移返回 ErrInvalidAttemptTransition，状态不符返回 ErrAttemptStateConflict。
func (r *SQLRepository) TransitionAttempt(ctx context.Context, attemptID string, from, to AttemptStatus, errorCode, requestID, usage string, at time.Time) error {
	if !CanTransitionAttempt(from, to) {
		return ErrInvalidAttemptTransition
	}
	var result sql.Result
	var err error
	if isAttemptTerminal(to) {
		result, err = r.db.ExecContext(ctx, r.query("UPDATE ask_attempts SET status = ?, error_code = ?, request_id = ?, usage = ?, completed_at = ? WHERE id = ? AND status = ?"), to, errorCode, requestID, usage, at, attemptID, from)
	} else {
		result, err = r.db.ExecContext(ctx, r.query("UPDATE ask_attempts SET status = ?, error_code = ?, request_id = ?, usage = ? WHERE id = ? AND status = ?"), to, errorCode, requestID, usage, attemptID, from)
	}
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrAttemptStateConflict
	}
	return nil
}

func isAttemptTerminal(status AttemptStatus) bool {
	return status == AttemptSucceeded || status == AttemptFailed || status == AttemptCanceled || status == AttemptUnknown
}

func scanAttempt(row *sql.Row) (Attempt, error) {
	var value Attempt
	var completedAt sql.NullTime
	err := row.Scan(&value.ID, &value.SessionID, &value.RunID, &value.Sequence, &value.Status, &value.Purpose, &value.InputSnapshot, &value.RequestID, &value.ErrorCode, &value.Usage, &value.StartedAt, &completedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Attempt{}, ErrAttemptNotFound
	}
	if err != nil {
		return Attempt{}, err
	}
	if completedAt.Valid {
		value.CompletedAt = &completedAt.Time
	}
	return value, nil
}
