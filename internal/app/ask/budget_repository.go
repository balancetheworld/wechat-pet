package ask

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// BudgetRepository 是预算账本与原子预留的持久化契约（v2 文档 9.3）。
type BudgetRepository interface {
	EnsureBudget(context.Context, BudgetScope, string, BudgetLimits) error
	GetBudget(context.Context, BudgetScope, string) (BudgetLedger, error)
	ReserveBudget(context.Context, BudgetScope, string, BudgetAmount) (BudgetReservation, error)
	SettleBudget(context.Context, string, BudgetAmount) error
	ReleaseBudget(context.Context, string) error
}

// budgetQuerier 抽象 *sql.DB 与 *sql.Tx 的 QueryRowContext，供账本读取复用。
type budgetQuerier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

const budgetColumns = "id, scope, scope_id, max_model_calls, max_tool_calls, max_tokens, max_cost_micros, max_duration_millis, max_concurrency, model_calls_used, tool_calls_used, tokens_used, cost_used_micros, duration_used_millis, model_calls_reserved, tool_calls_reserved, tokens_reserved, cost_reserved_micros, duration_reserved_millis, concurrency_reserved, created_at, updated_at"

// EnsureBudget 幂等建立预算账本；账本已存在时不覆盖限额。
func (r *SQLRepository) EnsureBudget(ctx context.Context, scope BudgetScope, scopeID string, limits BudgetLimits) error {
	if scope == "" || scopeID == "" {
		return errors.New("ask budget scope is required")
	}
	id, err := newID()
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	if r.isPostgres() {
		_, err = r.db.ExecContext(ctx, "INSERT INTO ask_budgets (id, scope, scope_id, max_model_calls, max_tool_calls, max_tokens, max_cost_micros, max_duration_millis, max_concurrency, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $10) ON CONFLICT (scope, scope_id) DO NOTHING", id, scope, scopeID, limits.MaxModelCalls, limits.MaxToolCalls, limits.MaxTokens, limits.MaxCostMicros, limits.MaxDurationMillis, limits.MaxConcurrency, now)
		return err
	}
	_, err = r.db.ExecContext(ctx, "INSERT OR IGNORE INTO ask_budgets (id, scope, scope_id, max_model_calls, max_tool_calls, max_tokens, max_cost_micros, max_duration_millis, max_concurrency, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)", id, scope, scopeID, limits.MaxModelCalls, limits.MaxToolCalls, limits.MaxTokens, limits.MaxCostMicros, limits.MaxDurationMillis, limits.MaxConcurrency, now, now)
	return err
}

// GetBudget 读取预算账本；不存在时返回 ErrBudgetNotFound。
func (r *SQLRepository) GetBudget(ctx context.Context, scope BudgetScope, scopeID string) (BudgetLedger, error) {
	return r.getBudgetTx(ctx, r.db, string(scope), scopeID, false)
}

// ReserveBudget 对已存在账本做一次原子预留，返回预留记录。
// 预留超出任一维度限额时返回 ErrBudgetExceeded，调用方不得派发。
func (r *SQLRepository) ReserveBudget(ctx context.Context, scope BudgetScope, scopeID string, amount BudgetAmount) (BudgetReservation, error) {
	if scope == "" || scopeID == "" {
		return BudgetReservation{}, errors.New("ask budget scope is required")
	}
	if err := validateBudgetAmount(amount); err != nil {
		return BudgetReservation{}, err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return BudgetReservation{}, err
	}
	defer tx.Rollback()

	ledger, err := r.getBudgetTx(ctx, tx, string(scope), scopeID, true)
	if err != nil {
		return BudgetReservation{}, err
	}
	if exceedsBudget(ledger.Limits, ledger.Used, ledger.Reserved, amount) {
		return BudgetReservation{}, ErrBudgetExceeded
	}

	now := time.Now().UTC()
	result, err := tx.ExecContext(ctx, r.query(`UPDATE ask_budgets SET
		model_calls_reserved = model_calls_reserved + ?,
		tool_calls_reserved = tool_calls_reserved + ?,
		tokens_reserved = tokens_reserved + ?,
		cost_reserved_micros = cost_reserved_micros + ?,
		duration_reserved_millis = duration_reserved_millis + ?,
		concurrency_reserved = concurrency_reserved + ?,
		updated_at = ?
		WHERE id = ?
		  AND (max_model_calls <= 0 OR model_calls_used + model_calls_reserved + ? <= max_model_calls)
		  AND (max_tool_calls <= 0 OR tool_calls_used + tool_calls_reserved + ? <= max_tool_calls)
		  AND (max_tokens <= 0 OR tokens_used + tokens_reserved + ? <= max_tokens)
		  AND (max_cost_micros <= 0 OR cost_used_micros + cost_reserved_micros + ? <= max_cost_micros)
		  AND (max_duration_millis <= 0 OR duration_used_millis + duration_reserved_millis + ? <= max_duration_millis)
		  AND (max_concurrency <= 0 OR concurrency_reserved + ? <= max_concurrency)`),
		amount.ModelCalls, amount.ToolCalls, amount.Tokens, amount.CostMicros, amount.DurationMillis, amount.Concurrency, now,
		ledger.ID,
		amount.ModelCalls, amount.ToolCalls, amount.Tokens, amount.CostMicros, amount.DurationMillis, amount.Concurrency)
	if err != nil {
		return BudgetReservation{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return BudgetReservation{}, err
	}
	if affected == 0 {
		return BudgetReservation{}, ErrBudgetExceeded
	}

	reservationID, err := newID()
	if err != nil {
		return BudgetReservation{}, err
	}
	if _, err := tx.ExecContext(ctx, r.query("INSERT INTO ask_reservations (id, budget_id, model_calls, tool_calls, tokens, cost_micros, duration_millis, concurrency, status, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)"), reservationID, ledger.ID, amount.ModelCalls, amount.ToolCalls, amount.Tokens, amount.CostMicros, amount.DurationMillis, amount.Concurrency, ReservationReserved, now); err != nil {
		return BudgetReservation{}, err
	}
	if err := tx.Commit(); err != nil {
		return BudgetReservation{}, err
	}
	return BudgetReservation{ID: reservationID, BudgetID: ledger.ID, Amount: amount, Status: ReservationReserved, CreatedAt: now}, nil
}

// SettleBudget 按实际用量结算一次预留：把预留量归还（reserved -= 预留量），
// 并把实际用量计入 used。实际用量小于预留量时差额自动归还。
func (r *SQLRepository) SettleBudget(ctx context.Context, reservationID string, actual BudgetAmount) error {
	if reservationID == "" {
		return errors.New("ask budget reservation id is required")
	}
	if err := validateBudgetAmount(actual); err != nil {
		return err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	reservation, err := r.getReservationTx(ctx, tx, reservationID, true)
	if err != nil {
		return err
	}
	if reservation.Status != ReservationReserved {
		return ErrReservationConflict
	}

	now := time.Now().UTC()
	result, err := tx.ExecContext(ctx, r.query(`UPDATE ask_budgets SET
		model_calls_reserved = model_calls_reserved - ?,
		tool_calls_reserved = tool_calls_reserved - ?,
		tokens_reserved = tokens_reserved - ?,
		cost_reserved_micros = cost_reserved_micros - ?,
		duration_reserved_millis = duration_reserved_millis - ?,
		concurrency_reserved = concurrency_reserved - ?,
		model_calls_used = model_calls_used + ?,
		tool_calls_used = tool_calls_used + ?,
		tokens_used = tokens_used + ?,
		cost_used_micros = cost_used_micros + ?,
		duration_used_millis = duration_used_millis + ?,
		updated_at = ?
		WHERE id = ?`),
		reservation.Amount.ModelCalls, reservation.Amount.ToolCalls, reservation.Amount.Tokens, reservation.Amount.CostMicros, reservation.Amount.DurationMillis, reservation.Amount.Concurrency,
		actual.ModelCalls, actual.ToolCalls, actual.Tokens, actual.CostMicros, actual.DurationMillis,
		now, reservation.BudgetID)
	if err != nil {
		return err
	}
	if affected, err := result.RowsAffected(); err != nil {
		return err
	} else if affected == 0 {
		return ErrBudgetNotFound
	}

	result, err = tx.ExecContext(ctx, r.query("UPDATE ask_reservations SET status = ?, settled_at = ? WHERE id = ? AND status = ?"), ReservationSettled, now, reservationID, ReservationReserved)
	if err != nil {
		return err
	}
	if affected, err := result.RowsAffected(); err != nil {
		return err
	} else if affected == 0 {
		return ErrReservationConflict
	}
	return tx.Commit()
}

// ReleaseBudget 归还未发送的预留。仅 reserved 状态可释放，已结算不可释放；
// 重复释放幂等返回 nil。
func (r *SQLRepository) ReleaseBudget(ctx context.Context, reservationID string) error {
	if reservationID == "" {
		return errors.New("ask budget reservation id is required")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	reservation, err := r.getReservationTx(ctx, tx, reservationID, true)
	if err != nil {
		return err
	}
	if reservation.Status == ReservationReleased {
		return tx.Commit()
	}
	if reservation.Status != ReservationReserved {
		return ErrReservationConflict
	}

	now := time.Now().UTC()
	result, err := tx.ExecContext(ctx, r.query(`UPDATE ask_budgets SET
		model_calls_reserved = model_calls_reserved - ?,
		tool_calls_reserved = tool_calls_reserved - ?,
		tokens_reserved = tokens_reserved - ?,
		cost_reserved_micros = cost_reserved_micros - ?,
		duration_reserved_millis = duration_reserved_millis - ?,
		concurrency_reserved = concurrency_reserved - ?,
		updated_at = ?
		WHERE id = ?`),
		reservation.Amount.ModelCalls, reservation.Amount.ToolCalls, reservation.Amount.Tokens, reservation.Amount.CostMicros, reservation.Amount.DurationMillis, reservation.Amount.Concurrency,
		now, reservation.BudgetID)
	if err != nil {
		return err
	}
	if affected, err := result.RowsAffected(); err != nil {
		return err
	} else if affected == 0 {
		return ErrBudgetNotFound
	}

	result, err = tx.ExecContext(ctx, r.query("UPDATE ask_reservations SET status = ?, released_at = ? WHERE id = ? AND status = ?"), ReservationReleased, now, reservationID, ReservationReserved)
	if err != nil {
		return err
	}
	if affected, err := result.RowsAffected(); err != nil {
		return err
	} else if affected == 0 {
		return ErrReservationConflict
	}
	return tx.Commit()
}

func (r *SQLRepository) getBudgetTx(ctx context.Context, querier budgetQuerier, scope, scopeID string, forUpdate bool) (BudgetLedger, error) {
	query := "SELECT " + budgetColumns + " FROM ask_budgets WHERE scope = ? AND scope_id = ?"
	if forUpdate && r.isPostgres() {
		query += " FOR UPDATE"
	}
	return scanBudget(querier.QueryRowContext(ctx, r.query(query), scope, scopeID))
}

func (r *SQLRepository) getReservationTx(ctx context.Context, tx *sql.Tx, reservationID string, forUpdate bool) (BudgetReservation, error) {
	query := "SELECT id, budget_id, model_calls, tool_calls, tokens, cost_micros, duration_millis, concurrency, status, created_at, settled_at, released_at FROM ask_reservations WHERE id = ?"
	if forUpdate && r.isPostgres() {
		query += " FOR UPDATE"
	}
	row := tx.QueryRowContext(ctx, r.query(query), reservationID)
	var value BudgetReservation
	var settledAt, releasedAt sql.NullTime
	err := row.Scan(&value.ID, &value.BudgetID, &value.Amount.ModelCalls, &value.Amount.ToolCalls, &value.Amount.Tokens, &value.Amount.CostMicros, &value.Amount.DurationMillis, &value.Amount.Concurrency, &value.Status, &value.CreatedAt, &settledAt, &releasedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return BudgetReservation{}, ErrReservationNotFound
	}
	if err != nil {
		return BudgetReservation{}, err
	}
	if settledAt.Valid {
		value.SettledAt = &settledAt.Time
	}
	if releasedAt.Valid {
		value.ReleasedAt = &releasedAt.Time
	}
	return value, nil
}

func scanBudget(row *sql.Row) (BudgetLedger, error) {
	var value BudgetLedger
	err := row.Scan(&value.ID, &value.Scope, &value.ScopeID, &value.Limits.MaxModelCalls, &value.Limits.MaxToolCalls, &value.Limits.MaxTokens, &value.Limits.MaxCostMicros, &value.Limits.MaxDurationMillis, &value.Limits.MaxConcurrency, &value.Used.ModelCalls, &value.Used.ToolCalls, &value.Used.Tokens, &value.Used.CostMicros, &value.Used.DurationMillis, &value.Reserved.ModelCalls, &value.Reserved.ToolCalls, &value.Reserved.Tokens, &value.Reserved.CostMicros, &value.Reserved.DurationMillis, &value.Reserved.Concurrency, &value.CreatedAt, &value.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return BudgetLedger{}, ErrBudgetNotFound
	}
	if err != nil {
		return BudgetLedger{}, err
	}
	return value, nil
}
