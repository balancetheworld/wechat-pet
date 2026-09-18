package ask

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// ToolExecutorAdapter 实现 ToolExecutor（文档 7 节）。
// 整批校验通过后，独立只读调用并行执行；一项失败不取消其他独立查询，结果逐项保留。
// 一期只支持只读工具（resolve/read/search/aggregate）；写入准备工具（prepare_*）与
// 核实（verify）在 P6 落地，本适配器对未支持动作返回明确局限，不伪造结果。
type ToolExecutorAdapter struct {
	catalog  *Catalog
	business BusinessReadRepository
	filter   Filter
	familyID string
}

// NewToolExecutorAdapter 构造工具执行端口。familyID 固定本次 Run 的授权范围。
func NewToolExecutorAdapter(catalog *Catalog, business BusinessReadRepository, filter Filter, familyID string) *ToolExecutorAdapter {
	return &ToolExecutorAdapter{catalog: catalog, business: business, filter: filter, familyID: familyID}
}

// ExecuteBatch 整批校验并执行（文档 7.4）。校验失败整批不启动；
// 校验通过后逐项执行，单项失败/无记录分别封装，不影响其余独立查询。
func (a *ToolExecutorAdapter) ExecuteBatch(ctx context.Context, batch ToolBatch) ([]ToolResult, error) {
	if err := ValidateBatch(batch, a.catalog, a.filter); err != nil {
		return nil, err
	}
	results := make([]ToolResult, len(batch.Calls))
	var wg sync.WaitGroup
	for i, call := range batch.Calls {
		wg.Add(1)
		go func(index int, call ToolCall) {
			defer wg.Done()
			results[index] = a.executeCall(ctx, call)
		}(i, call)
	}
	wg.Wait()
	return results, nil
}

// executeCall 执行单项调用并封装结果（文档 7.3）。执行错误与无记录分别表达。
func (a *ToolExecutorAdapter) executeCall(ctx context.Context, call ToolCall) ToolResult {
	queuedAt := time.Now()
	tool, ok := findToolInCatalog(a.catalog, call.ToolName, call.ToolVersion)
	if !ok {
		return a.failedResult(call, ToolErrInvalidTool, "validate", "tool not found", false)
	}
	args, err := parseToolArguments(call.Arguments)
	if err != nil {
		return a.failedResult(call, ToolErrInvalidArgument, "validate", err.Error(), false)
	}
	switch tool.ActionType {
	case ActionResolve:
		return a.resolvePet(ctx, call, args, queuedAt)
	case ActionRead:
		return a.readByResource(ctx, call, tool, args, queuedAt)
	case ActionSearch:
		return a.searchRecords(ctx, call, args, queuedAt)
	case ActionAggregate:
		return a.aggregateRecords(ctx, call, args, queuedAt)
	default:
		return a.failedResult(call, ToolErrInvalidTool, "validate", "action not supported in phase 1", false)
	}
}

func (a *ToolExecutorAdapter) resolvePet(ctx context.Context, call ToolCall, args map[string]any, queuedAt time.Time) ToolResult {
	query, _ := args["query"].(string)
	outcome, err := a.business.ResolvePet(ctx, a.familyID, query)
	if err != nil {
		return a.failedResult(call, ToolErrServiceError, "execute", err.Error(), true)
	}
	return ToolResult{
		ToolCallID:   call.ToolCallID,
		CallIndex:    call.CallIndex,
		Status:       ToolResultOK,
		Completeness: CompletenessComplete,
		Data:         outcome,
		Source:       outcome.Source,
		QueuedAt:     queuedAt,
		StartedAt:    queuedAt,
		CompletedAt:  time.Now(),
	}
}

func (a *ToolExecutorAdapter) readByResource(ctx context.Context, call ToolCall, tool Tool, args map[string]any, queuedAt time.Time) ToolResult {
	switch tool.ResourceType {
	case ResourcePetProfile:
		petID, _ := args["pet_id"].(string)
		outcome, err := a.business.ReadPetProfile(ctx, a.familyID, petID)
		if err != nil {
			return a.failedResult(call, ToolErrServiceError, "execute", err.Error(), true)
		}
		return ToolResult{
			ToolCallID:   call.ToolCallID,
			CallIndex:    call.CallIndex,
			Status:       ToolResultOK,
			Completeness: CompletenessComplete,
			Data:         outcome,
			Source:       outcome.Source,
			QueuedAt:     queuedAt,
			StartedAt:    queuedAt,
			CompletedAt:  time.Now(),
		}
	case ResourceHealthRecord:
		recordID, _ := args["record_id"].(string)
		outcome, err := a.business.ReadHealthRecord(ctx, a.familyID, recordID)
		if err != nil {
			if err == ErrHealthRecordNotFound {
				return a.emptyResult(call, queuedAt)
			}
			return a.failedResult(call, ToolErrServiceError, "execute", err.Error(), true)
		}
		return ToolResult{
			ToolCallID:   call.ToolCallID,
			CallIndex:    call.CallIndex,
			Status:       ToolResultOK,
			Completeness: CompletenessComplete,
			Data:         outcome,
			Source:       outcome.Source,
			QueuedAt:     queuedAt,
			StartedAt:    queuedAt,
			CompletedAt:  time.Now(),
		}
	default:
		return a.failedResult(call, ToolErrInvalidTool, "validate", "unsupported read resource", false)
	}
}

func (a *ToolExecutorAdapter) searchRecords(ctx context.Context, call ToolCall, args map[string]any, queuedAt time.Time) ToolResult {
	petID, _ := args["pet_id"].(string)
	query, err := healthRecordSearchQuery(args)
	if err != nil {
		return a.failedResult(call, ToolErrInvalidArgument, "validate", err.Error(), false)
	}
	outcome, err := a.business.SearchHealthRecords(ctx, a.familyID, petID, query)
	if err != nil {
		return a.failedResult(call, ToolErrServiceError, "execute", err.Error(), true)
	}
	return ToolResult{
		ToolCallID:   call.ToolCallID,
		CallIndex:    call.CallIndex,
		Status:       ToolResultOK,
		Completeness: completenessForSearch(outcome.HasMore),
		Data:         outcome,
		Source:       outcome.Source,
		HasMore:      outcome.HasMore,
		NextCursor:   outcome.NextCursor,
		QueuedAt:     queuedAt,
		StartedAt:    queuedAt,
		CompletedAt:  time.Now(),
	}
}

func (a *ToolExecutorAdapter) aggregateRecords(ctx context.Context, call ToolCall, args map[string]any, queuedAt time.Time) ToolResult {
	petID, _ := args["pet_id"].(string)
	query, err := healthRecordAggregateQuery(args)
	if err != nil {
		return a.failedResult(call, ToolErrInvalidArgument, "validate", err.Error(), false)
	}
	outcome, err := a.business.AggregateHealthRecords(ctx, a.familyID, petID, query)
	if err != nil {
		return a.failedResult(call, ToolErrServiceError, "execute", err.Error(), true)
	}
	return ToolResult{
		ToolCallID:   call.ToolCallID,
		CallIndex:    call.CallIndex,
		Status:       ToolResultOK,
		Completeness: CompletenessComplete,
		Data:         outcome,
		Source:       outcome.Source,
		QueuedAt:     queuedAt,
		StartedAt:    queuedAt,
		CompletedAt:  time.Now(),
	}
}

func (a *ToolExecutorAdapter) failedResult(call ToolCall, category ToolErrorCategory, stage, reason string, retryable bool) ToolResult {
	return ToolResult{
		ToolCallID: call.ToolCallID,
		CallIndex:  call.CallIndex,
		Status:     ToolResultError,
		Error:      &ToolError{Category: category, Stage: stage, Reason: reason, Retryable: retryable},
		QueuedAt:   time.Now(),
		StartedAt:  time.Now(),
		CompletedAt: time.Now(),
	}
}

func (a *ToolExecutorAdapter) emptyResult(call ToolCall, queuedAt time.Time) ToolResult {
	return ToolResult{
		ToolCallID:   call.ToolCallID,
		CallIndex:    call.CallIndex,
		Status:       ToolResultOK,
		Completeness: CompletenessComplete,
		Data:         HealthRecordOutcome{},
		QueuedAt:     queuedAt,
		StartedAt:    queuedAt,
		CompletedAt:  time.Now(),
	}
}

func completenessForSearch(hasMore bool) Completeness {
	if hasMore {
		return CompletenessPartial
	}
	return CompletenessComplete
}

func parseToolArguments(args json.RawMessage) (map[string]any, error) {
	if len(args) == 0 || !json.Valid(args) {
		return nil, fmt.Errorf("arguments must be valid JSON")
	}
	var m map[string]any
	if err := json.Unmarshal(args, &m); err != nil {
		return nil, fmt.Errorf("arguments must be a JSON object: %w", err)
	}
	return m, nil
}

func healthRecordSearchQuery(args map[string]any) (HealthRecordSearchQuery, error) {
	var q HealthRecordSearchQuery
	if v, ok := args["category"].(string); ok {
		q.Category = v
	}
	if v, ok := args["medical_type"].(string); ok {
		q.MedicalType = v
	}
	if v, ok := args["limit"].(float64); ok {
		q.Limit = int(v)
	}
	if v, ok := args["cursor"].(string); ok {
		q.Cursor = v
	}
	start, err := optionalTime(args, "start_at")
	if err != nil {
		return q, err
	}
	q.StartAt = start
	end, err := optionalTime(args, "end_at")
	if err != nil {
		return q, err
	}
	q.EndAt = end
	return q, nil
}

func healthRecordAggregateQuery(args map[string]any) (HealthRecordAggregateQuery, error) {
	var q HealthRecordAggregateQuery
	if v, ok := args["category"].(string); ok {
		q.Category = v
	}
	if v, ok := args["medical_type"].(string); ok {
		q.MedicalType = v
	}
	if v, ok := args["custom_medical_type"].(string); ok {
		q.CustomMedicalType = v
	}
	start, err := optionalTime(args, "start_at")
	if err != nil {
		return q, err
	}
	q.StartAt = start
	end, err := optionalTime(args, "end_at")
	if err != nil {
		return q, err
	}
	q.EndAt = end
	return q, nil
}

func optionalTime(args map[string]any, key string) (time.Time, error) {
	raw, ok := args[key]
	if !ok || raw == nil {
		return time.Time{}, nil
	}
	switch v := raw.(type) {
	case string:
		if v == "" {
			return time.Time{}, nil
		}
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return time.Time{}, fmt.Errorf("%s must be RFC3339: %w", key, err)
		}
		return t, nil
	default:
		return time.Time{}, fmt.Errorf("%s must be a string", key)
	}
}
