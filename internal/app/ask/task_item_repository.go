package ask

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

// 本文件固定任务项持久化（文档 2.3.1）。
// 任务项标识由服务端分配，执行结果按 Run 及任务项版本保存；手动重试不覆盖旧 Run 的记录。
// 结果分类只记录覆盖情况，不新增可自行恢复的状态机；JSON 列承载 subjects / missing_fields /
// source_turn_ids / result_ref，反序列化失败不静默丢弃。

// ErrTaskItemNotFound：任务项不存在或已删除。
var ErrTaskItemNotFound = errors.New("ask task item not found")

// ErrTaskItemConflict：任务项版本冲突（并发更新）。
var ErrTaskItemConflict = errors.New("ask task item conflict")

// CreateTaskItems 批量创建任务项（事务内插入）。任一插入失败整体回滚。
func (r *SQLRepository) CreateTaskItems(ctx context.Context, items []TaskItem) error {
	if len(items) == 0 {
		return nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, item := range items {
		if err := r.insertTaskItem(ctx, tx, item); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ListTaskItems 返回某个 Run 的有效任务项，按创建顺序排序。
func (r *SQLRepository) ListTaskItems(ctx context.Context, runID string) ([]TaskItem, error) {
	rows, err := r.db.QueryContext(ctx, r.query("SELECT id, run_id, origin_turn_id, item_revision, goal, source_turn_ids, subjects, outcome, missing_fields, incomplete_reason, result_ref, supersedes, superseded_by, withdrawn_reason FROM ask_task_items WHERE run_id = ? AND deleted_at IS NULL ORDER BY created_at, id"), runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]TaskItem, 0)
	for rows.Next() {
		item, err := scanTaskItem(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

// UpdateTaskItem 乐观锁更新任务项：以 item_revision 为预期版本，成功递增版本。
// 只更新允许变动的字段（goal/subjects/outcome/missing_fields/incomplete_reason/result_ref/
// supersedes/superseded_by/withdrawn_reason），不改变任务项身份。
func (r *SQLRepository) UpdateTaskItem(ctx context.Context, item TaskItem) error {
	if item.TaskItemID == "" {
		return errors.New("task item id is required")
	}
	sourceTurnIDs := marshalTaskItemJSON(item.SourceTurnIDs)
	subjects := marshalTaskItemJSON(item.Subjects)
	missingFields := marshalTaskItemJSON(item.MissingFields)
	resultRef := marshalTaskItemJSON(item.ResultRef)
	result, err := r.db.ExecContext(ctx, r.query("UPDATE ask_task_items SET goal = ?, subjects = ?, outcome = ?, missing_fields = ?, incomplete_reason = ?, result_ref = ?, supersedes = ?, superseded_by = ?, withdrawn_reason = ?, source_turn_ids = ?, item_revision = item_revision + 1, updated_at = ? WHERE id = ? AND item_revision = ? AND deleted_at IS NULL"), item.Goal, subjects, item.Outcome, missingFields, item.IncompleteReason, resultRef, item.Supersedes, item.SupersededBy, item.WithdrawnReason, sourceTurnIDs, time.Now().UTC(), item.TaskItemID, item.ItemRevision)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		// 区分「不存在」与「版本冲突」：优先判断是否存在。
		var exists int
		if err := r.db.QueryRowContext(ctx, r.query("SELECT COUNT(*) FROM ask_task_items WHERE id = ? AND deleted_at IS NULL"), item.TaskItemID).Scan(&exists); err != nil {
			return err
		}
		if exists == 0 {
			return ErrTaskItemNotFound
		}
		return ErrTaskItemConflict
	}
	return nil
}

func (r *SQLRepository) insertTaskItem(ctx context.Context, tx *sql.Tx, item TaskItem) error {
	sourceTurnIDs := marshalTaskItemJSON(item.SourceTurnIDs)
	subjects := marshalTaskItemJSON(item.Subjects)
	missingFields := marshalTaskItemJSON(item.MissingFields)
	resultRef := marshalTaskItemJSON(item.ResultRef)
	revision := item.ItemRevision
	if revision <= 0 {
		revision = 1
	}
	_, err := tx.ExecContext(ctx, r.query("INSERT INTO ask_task_items (id, run_id, origin_turn_id, item_revision, goal, source_turn_ids, subjects, outcome, missing_fields, incomplete_reason, result_ref, supersedes, superseded_by, withdrawn_reason, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)"), item.TaskItemID, item.RunID, item.OriginTurnID, revision, item.Goal, sourceTurnIDs, subjects, item.Outcome, missingFields, item.IncompleteReason, resultRef, item.Supersedes, item.SupersededBy, item.WithdrawnReason, time.Now().UTC(), time.Now().UTC())
	return err
}

type taskItemScanner interface {
	Scan(dest ...any) error
}

func scanTaskItem(scanner taskItemScanner) (TaskItem, error) {
	var item TaskItem
	var sourceTurnIDs, subjects, missingFields, resultRef string
	if err := scanner.Scan(&item.TaskItemID, &item.RunID, &item.OriginTurnID, &item.ItemRevision, &item.Goal, &sourceTurnIDs, &subjects, &item.Outcome, &missingFields, &item.IncompleteReason, &resultRef, &item.Supersedes, &item.SupersededBy, &item.WithdrawnReason); err != nil {
		return TaskItem{}, err
	}
	item.SourceTurnIDs = unmarshalTaskItemStringSlice(sourceTurnIDs)
	item.Subjects = unmarshalTaskItemSubjects(subjects)
	item.MissingFields = unmarshalTaskItemMissingFields(missingFields)
	item.ResultRef = unmarshalTaskItemResultRef(resultRef)
	return item, nil
}

// marshalTaskItemJSON 序列化一个字段组；nil 返回空串（不落 'null'）。
func marshalTaskItemJSON(v any) string {
	if v == nil {
		return ""
	}
	data, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(data)
}

func unmarshalTaskItemStringSlice(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil
	}
	return out
}

func unmarshalTaskItemSubjects(s string) []AnswerSubject {
	if s == "" {
		return nil
	}
	var out []AnswerSubject
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil
	}
	return out
}

func unmarshalTaskItemMissingFields(s string) []MissingField {
	if s == "" {
		return nil
	}
	var out []MissingField
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil
	}
	return out
}

func unmarshalTaskItemResultRef(s string) *TaskResultRef {
	if s == "" {
		return nil
	}
	var out TaskResultRef
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil
	}
	return &out
}
