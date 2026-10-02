package ask

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

// 本文件固定工具结果的持久化与复用（对应架构设计 v2 文档 7.7）。
// 只读结果允许在同一 Session 的不同 Run 之间复用，复用前重新核验权限与来源集合版本；
// 写入准备结果不参与复用。正文与证据在执行时固化，恢复上下文时直接重放，
// 不依赖具体业务类型重新渲染（ToolResult.Data 反序列化后不再保留原始类型）。

// ErrToolResultNotFound：没有可复用的既有工具结果。
var ErrToolResultNotFound = errors.New("ask tool result not found")

// ToolResultRecord 是一次工具调用的可持久化快照。
type ToolResultRecord struct {
	ID           string
	SessionID    string
	RunID        string
	ToolCallID   string
	ToolName     string
	ToolVersion  string
	Fingerprint  string
	TaskKeys     []string // 稳定任务项标识；恢复时据此判断结果是否仍需要保留
	Status       ToolResultStatus
	Completeness Completeness
	HasMore      bool
	NextCursor   string
	Text         string
	Evidence     []EvidenceRef
	Source       ReadSource
	CreatedAt    time.Time
}

// ToolResultStore 是工具结果持久化与复用端口。
type ToolResultStore interface {
	SaveToolResults(context.Context, []ToolResultRecord) error
	GetToolResult(context.Context, string) (ToolResultRecord, error)
	ListRunToolResults(context.Context, string) ([]ToolResultRecord, error)
	FindReusableToolResult(context.Context, string, string) (ToolResultRecord, error)
}

// newToolResultRecord 把一次真实执行结果固化为可持久化记录（文档 7.7）。
// 指纹用于复用判定；正文与证据此刻渲染，避免恢复时依赖具体业务类型。
func newToolResultRecord(sessionID, runID string, call ToolCall, result ToolResult, now time.Time) (ToolResultRecord, error) {
	fingerprint, err := fingerprintForCall(call)
	if err != nil {
		return ToolResultRecord{}, err
	}
	id, err := newID()
	if err != nil {
		return ToolResultRecord{}, err
	}
	return ToolResultRecord{
		ID:           id,
		SessionID:    sessionID,
		RunID:        runID,
		ToolCallID:   result.ToolCallID,
		ToolName:     call.ToolName,
		ToolVersion:  call.ToolVersion,
		Fingerprint:  fingerprint,
		TaskKeys:     stableTaskKeys(runID, call.TaskKeys),
		Status:       result.Status,
		Completeness: result.Completeness,
		HasMore:      result.HasMore,
		NextCursor:   result.NextCursor,
		Text:         toolResultText(result),
		Evidence:     toolEvidenceRefs(result),
		Source:       result.Source,
		CreatedAt:    now.UTC(),
	}, nil
}

// block 把本次执行产生的工具结果转换为回灌上下文块（文档 5.8 第 5 层）。
func (r ToolResultRecord) block() ContextBlock {
	return r.contextBlock(LayerToolInteractions)
}

// replayBlock 把恢复时重放的既有结果作为「仍有效的工具结果」提供（文档 5.8 第 2 层）。
// 它发生在更早一次用户输入之后，不能冒充当前步骤的工具交互。
func (r ToolResultRecord) replayBlock() ContextBlock {
	return r.contextBlock(LayerReferenceData)
}

func (r ToolResultRecord) contextBlock(layer ContextLayer) ContextBlock {
	return ContextBlock{
		Layer:        layer,
		Kind:         "tool_result",
		ObjectID:     r.ToolCallID,
		Text:         r.Text,
		EvidenceRefs: r.Evidence,
	}
}

// stableTaskKeys 把本次响应内的任务短键换算为稳定任务项标识。
func stableTaskKeys(runID string, taskKeys []string) []string {
	if len(taskKeys) == 0 {
		return []string{}
	}
	keys := make([]string, 0, len(taskKeys))
	for _, key := range taskKeys {
		keys = append(keys, stableTaskItemID(runID, key))
	}
	return keys
}

// replayableToolResults 决定恢复时哪些已持久化结果仍进入本次上下文（文档 5.8、7.7）。
// 未终结任务仍依赖的结果保留；任务关系未知时保守保留；
// 只被已终结任务使用的结果不再占用工作台，其产物由任务清单的 result_ref 表达。
func replayableToolResults(records []ToolResultRecord, taskItems []TaskItem) []ToolResultRecord {
	if len(taskItems) == 0 {
		return records
	}
	pending := make(map[string]struct{}, len(taskItems))
	for _, item := range taskItems {
		if !item.Outcome.Terminal() {
			pending[item.TaskItemID] = struct{}{}
		}
	}
	kept := make([]ToolResultRecord, 0, len(records))
	for _, record := range records {
		if len(record.TaskKeys) == 0 {
			kept = append(kept, record)
			continue
		}
		for _, key := range record.TaskKeys {
			if _, ok := pending[key]; ok {
				kept = append(kept, record)
				break
			}
		}
	}
	return kept
}

// reusedToolResult 把既有结果封装为复用结果：携带结果句柄与来源身份，
// 正文与证据由调用方按句柄读取，避免按业务类型重新渲染（文档 7.7）。
func reusedToolResult(call ToolCall, record ToolResultRecord) ToolResult {
	now := time.Now()
	return ToolResult{
		ToolCallID:   call.ToolCallID,
		CallIndex:    call.CallIndex,
		Status:       record.Status,
		Completeness: record.Completeness,
		Source:       record.Source,
		HasMore:      record.HasMore,
		NextCursor:   record.NextCursor,
		Reused:       true,
		ResultHandle: record.ID,
		QueuedAt:     now,
		StartedAt:    now,
		CompletedAt:  now,
	}
}

// reusableToolResult 判定既有结果能否复用（文档 7.7）。
// 只读、同一 Session、契约与指纹一致、来源集合版本未变时才复用；
// 无法证明仍有效就重查，写入准备结果永不复用。
func reusableToolResult(ctx context.Context, store ToolResultStore, sources SourceVersionRepository, sessionID string, call ToolCall, action ActionType) (ToolResultRecord, bool, error) {
	if store == nil || sessionID == "" || !IsReadOnly(action) {
		return ToolResultRecord{}, false, nil
	}
	fingerprint, err := fingerprintForCall(call)
	if err != nil {
		return ToolResultRecord{}, false, nil
	}
	candidate, err := store.FindReusableToolResult(ctx, sessionID, fingerprint)
	if errors.Is(err, ErrToolResultNotFound) {
		return ToolResultRecord{}, false, nil
	}
	if err != nil {
		return ToolResultRecord{}, false, err
	}
	decision := DecideReuse(ReuseInput{
		SessionID:   sessionID,
		ToolName:    call.ToolName,
		ToolVersion: call.ToolVersion,
		Fingerprint: fingerprint,
		Candidate: ReuseCandidate{
			SessionID:   candidate.SessionID,
			RunID:       candidate.RunID,
			ToolCallID:  candidate.ToolCallID,
			ToolName:    candidate.ToolName,
			ToolVersion: candidate.ToolVersion,
			Fingerprint: candidate.Fingerprint,
		},
		Permitted:        true,
		SourceStillValid: sourceStillValid(ctx, sources, candidate),
	})
	if decision != ReuseReusable {
		return ToolResultRecord{}, false, nil
	}
	return candidate, true, nil
}

// sourceStillValid 比较来源集合版本；来源缺失或无版本一律视为不可复用。
func sourceStillValid(ctx context.Context, sources SourceVersionRepository, record ToolResultRecord) bool {
	if sources == nil || record.Source.SourceType == "" || record.Source.Version == "" {
		return false
	}
	current, err := sources.GetSourceVersion(ctx, record.Source.SourceType, record.Source.SourceID)
	if err != nil {
		return false
	}
	return current.Version == record.Source.Version
}

// SaveToolResults 批量写入工具结果快照。任一写入失败整体回滚。
func (r *SQLRepository) SaveToolResults(ctx context.Context, records []ToolResultRecord) error {
	if len(records) == 0 {
		return nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, record := range records {
		evidence, err := json.Marshal(record.Evidence)
		if err != nil {
			return err
		}
		taskKeys, err := json.Marshal(record.TaskKeys)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, r.query("INSERT INTO ask_tool_results (id, session_id, run_id, tool_call_id, tool_name, tool_version, fingerprint, task_keys, status, completeness, has_more, next_cursor, text, evidence, source_type, source_id, source_version, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)"),
			record.ID, record.SessionID, record.RunID, record.ToolCallID, record.ToolName, record.ToolVersion, record.Fingerprint, string(taskKeys),
			record.Status, record.Completeness, boolCount(record.HasMore), record.NextCursor, record.Text, string(evidence),
			record.Source.SourceType, record.Source.SourceID, record.Source.Version, record.CreatedAt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// GetToolResult 按标识读取一份已持久化的工具结果。
func (r *SQLRepository) GetToolResult(ctx context.Context, id string) (ToolResultRecord, error) {
	row := r.db.QueryRowContext(ctx, r.query("SELECT id, session_id, run_id, tool_call_id, tool_name, tool_version, fingerprint, task_keys, status, completeness, has_more, next_cursor, text, evidence, source_type, source_id, source_version, created_at FROM ask_tool_results WHERE id = ? AND deleted_at IS NULL"), id)
	return scanToolResultRecord(row)
}

// ListRunToolResults 按执行顺序返回某个 Run 已持久化的工具结果。
func (r *SQLRepository) ListRunToolResults(ctx context.Context, runID string) ([]ToolResultRecord, error) {
	rows, err := r.db.QueryContext(ctx, r.query("SELECT id, session_id, run_id, tool_call_id, tool_name, tool_version, fingerprint, task_keys, status, completeness, has_more, next_cursor, text, evidence, source_type, source_id, source_version, created_at FROM ask_tool_results WHERE run_id = ? AND deleted_at IS NULL ORDER BY created_at, id"), runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := make([]ToolResultRecord, 0)
	for rows.Next() {
		record, err := scanToolResultRecord(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

// FindReusableToolResult 按会话与指纹查找最近一次成功的既有结果。
func (r *SQLRepository) FindReusableToolResult(ctx context.Context, sessionID, fingerprint string) (ToolResultRecord, error) {
	row := r.db.QueryRowContext(ctx, r.query("SELECT id, session_id, run_id, tool_call_id, tool_name, tool_version, fingerprint, task_keys, status, completeness, has_more, next_cursor, text, evidence, source_type, source_id, source_version, created_at FROM ask_tool_results WHERE session_id = ? AND fingerprint = ? AND status = ? AND deleted_at IS NULL ORDER BY created_at DESC, id DESC LIMIT 1"), sessionID, fingerprint, ToolResultOK)
	return scanToolResultRecord(row)
}

// toolResultScanner 覆盖单行与多行扫描的公共接口。
type toolResultScanner interface {
	Scan(dest ...any) error
}

func scanToolResultRecord(scanner toolResultScanner) (ToolResultRecord, error) {
	var record ToolResultRecord
	var hasMore int
	var taskKeys, evidence string
	if err := scanner.Scan(&record.ID, &record.SessionID, &record.RunID, &record.ToolCallID, &record.ToolName, &record.ToolVersion,
		&record.Fingerprint, &taskKeys, &record.Status, &record.Completeness, &hasMore, &record.NextCursor, &record.Text, &evidence,
		&record.Source.SourceType, &record.Source.SourceID, &record.Source.Version, &record.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ToolResultRecord{}, ErrToolResultNotFound
		}
		return ToolResultRecord{}, err
	}
	record.HasMore = hasMore != 0
	record.Source.ReadAt = record.CreatedAt
	if err := json.Unmarshal([]byte(taskKeys), &record.TaskKeys); err != nil {
		return ToolResultRecord{}, err
	}
	if err := json.Unmarshal([]byte(evidence), &record.Evidence); err != nil {
		return ToolResultRecord{}, err
	}
	return record, nil
}
