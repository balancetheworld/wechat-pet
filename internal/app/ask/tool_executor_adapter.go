package ask

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	calendarapp "github.com/balancetheworld/wechat-pet/internal/app/calendar"
)

// operationPreparer 是写入准备端口：把工具参数冻结为待用户确认的 Operation 预览。
// 实现由 Service 提供（复用 CreateOperationPreview 的会话归属校验），
// 适配器本身不直接写业务数据。
type operationPreparer interface {
	PrepareOperation(ctx context.Context, sessionID, runID string, input OperationPreviewInput) (Operation, error)
}

type operationPreparerFunc func(context.Context, string, string, OperationPreviewInput) (Operation, error)

func (f operationPreparerFunc) PrepareOperation(ctx context.Context, sessionID, runID string, input OperationPreviewInput) (Operation, error) {
	return f(ctx, sessionID, runID, input)
}

// ToolExecutionScope 固定一次 Run 内写入准备所需的调用身份。
type ToolExecutionScope struct {
	SessionID  string
	RunID      string
	Operations operationPreparer
}

// ToolExecutorAdapter 实现 ToolExecutor（文档 7 节）。
// 整批校验通过后，独立只读调用并行执行；一项失败不取消其他独立查询，结果逐项保留。
// 支持只读工具（resolve/read/search/aggregate）与写入准备（prepare_create，只形成待确认预览）；
// 核实（verify）尚未落地，本适配器对未支持动作返回明确局限，不伪造结果。
type ToolExecutorAdapter struct {
	catalog  *Catalog
	business BusinessReadRepository
	filter   Filter
	familyID string
	scope    ToolExecutionScope
	guard    *LoopGuard
}

// NewToolExecutorAdapter 构造工具执行端口。familyID 固定本次 Run 的授权范围。
func NewToolExecutorAdapter(catalog *Catalog, business BusinessReadRepository, filter Filter, familyID string, scope ...ToolExecutionScope) *ToolExecutorAdapter {
	adapter := &ToolExecutorAdapter{catalog: catalog, business: business, filter: filter, familyID: familyID}
	adapter.guard = NewLoopGuard()
	if len(scope) > 0 {
		adapter.scope = scope[0]
	}
	return adapter
}

// ExecuteBatch 整批校验并执行（文档 7.4）。校验失败整批不启动；
// 校验通过后逐项执行，单项失败/无记录分别封装，不影响其余独立查询。
func (a *ToolExecutorAdapter) ExecuteBatch(ctx context.Context, batch ToolBatch) ([]ToolResult, error) {
	if err := ValidateBatch(batch, a.catalog, a.filter); err != nil {
		return nil, err
	}
	results := make([]ToolResult, len(batch.Calls))
	completed := make(map[string]ToolResult, len(batch.Calls))
	pending := make(map[string]int, len(batch.Calls))
	for index, call := range batch.Calls {
		pending[call.ToolCallID] = index
	}
	for len(pending) > 0 {
		ready := make([]int, 0)
		for id, index := range pending {
			call := batch.Calls[index]
			waiting := false
			blocked := false
			for _, dependencyID := range call.DependsOn {
				dependency, ok := completed[dependencyID]
				if !ok {
					waiting = true
					break
				}
				if dependency.Status != ToolResultOK {
					blocked = true
				}
			}
			if waiting {
				continue
			}
			if blocked {
				result := a.blockedResult(call)
				results[index] = result
				completed[id] = result
				delete(pending, id)
				continue
			}
			ready = append(ready, index)
		}
		executable := make([]int, 0, len(ready))
		seenFingerprints := make(map[string]struct{}, len(ready))
		sort.Ints(ready)
		for _, index := range ready {
			call := batch.Calls[index]
			fingerprint, fingerprintErr := fingerprintForCall(call)
			if fingerprintErr == nil {
				if _, duplicate := seenFingerprints[fingerprint]; duplicate {
					result := a.loopGuardResult(call, "同一批次内已存在相同参数的调用")
					results[index] = result
					completed[call.ToolCallID] = result
					delete(pending, call.ToolCallID)
					continue
				}
				seenFingerprints[fingerprint] = struct{}{}
			}
			executable = append(executable, index)
		}
		var wg sync.WaitGroup
		for _, index := range executable {
			call := batch.Calls[index]
			wg.Add(1)
			go func(index int, call ToolCall) {
				defer wg.Done()
				action := ActionType("")
				if tool, ok := findToolInCatalog(a.catalog, call.ToolName, call.ToolVersion); ok {
					action = tool.ActionType
				}
				if reason, blocked := a.guard.BlockReason(call, action); blocked {
					results[index] = a.loopGuardResult(call, reason)
					return
				}
				result := a.executeCall(ctx, call)
				a.guard.Record(call, result)
				results[index] = result
			}(index, call)
		}
		wg.Wait()
		for _, index := range executable {
			call := batch.Calls[index]
			completed[call.ToolCallID] = results[index]
			delete(pending, call.ToolCallID)
		}
	}
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
	case ActionPrepareCreate:
		return a.prepareOperation(ctx, call, args, queuedAt)
	case ActionPrepareUpdate:
		return a.prepareOperation(ctx, call, args, queuedAt)
	default:
		return a.failedResult(call, ToolErrInvalidTool, "validate", "action not supported in phase 1", false)
	}
}

// PreparedOperation 是写入准备结果：只包含待确认的 Operation 身份与预览摘要，
// 回灌给模型的正文里不包含可直接执行写入的凭证。
type PreparedOperation struct {
	OperationID string `json:"operation_id"`
	Status      string `json:"status"`
	Preview     string `json:"preview"`
	Target      string `json:"target"`
}

func (a *ToolExecutorAdapter) prepareOperation(ctx context.Context, call ToolCall, args map[string]any, queuedAt time.Time) ToolResult {
	if a.scope.Operations == nil || a.scope.SessionID == "" || a.scope.RunID == "" {
		return a.failedResult(call, ToolErrInvalidTool, "validate", "operation preparation unavailable", false)
	}
	input, err := prepareInputForTool(call.ToolName, args)
	if err != nil {
		return a.failedResult(call, ToolErrInvalidArgument, "validate", err.Error(), false)
	}
	operation, err := a.scope.Operations.PrepareOperation(ctx, a.scope.SessionID, a.scope.RunID, input)
	if err != nil {
		return a.failedResult(call, ToolErrServiceError, "execute", err.Error(), true)
	}
	return ToolResult{
		ToolCallID:   call.ToolCallID,
		CallIndex:    call.CallIndex,
		Status:       ToolResultOK,
		Completeness: CompletenessComplete,
		Data:         PreparedOperation{OperationID: operation.ID, Status: string(operation.Status), Preview: operation.Preview, Target: operation.Target},
		QueuedAt:     queuedAt,
		StartedAt:    queuedAt,
		CompletedAt:  time.Now(),
	}
}

// prepareInputForTool 把工具参数转换为待冻结的写入预览。目标与载荷由服务端构造，
// 模型只提供业务字段；字段白名单与格式在此校验，不合法即整项失败。
func prepareInputForTool(toolName string, args map[string]any) (OperationPreviewInput, error) {
	switch toolName {
	case "create_calendar_record":
		petID, _ := args["pet_id"].(string)
		category, _ := args["category"].(string)
		content, _ := args["content"].(string)
		occurredAt, _ := args["occurred_at"].(string)
		if petID == "" || (category != "daily" && category != "medical") || content == "" || occurredAt == "" {
			return OperationPreviewInput{}, fmt.Errorf("pet_id/category/content/occurred_at are required")
		}
		if _, err := time.Parse(time.RFC3339, occurredAt); err != nil {
			return OperationPreviewInput{}, fmt.Errorf("occurred_at must be RFC3339")
		}
		request := calendarapp.CreateRecordRequest{PetID: petID, Category: category, Content: content, OccurredAt: occurredAt}
		if medicalType, ok := args["medical_type"].(string); ok {
			request.MedicalType = medicalType
		}
		return OperationPreviewInput{Target: operationTargetCalendarRecordCreate, Summary: calendarRecordSummary(request), Payload: request}, nil
	case "update_calendar_record":
		recordID, _ := args["record_id"].(string)
		if recordID == "" {
			return OperationPreviewInput{}, fmt.Errorf("record_id is required")
		}
		request := calendarapp.UpdateRecordRequest{}
		if content, ok := args["content"].(string); ok {
			request.Content = &content
		}
		if occurredAt, ok := args["occurred_at"].(string); ok {
			if _, err := time.Parse(time.RFC3339, occurredAt); err != nil {
				return OperationPreviewInput{}, fmt.Errorf("occurred_at must be RFC3339")
			}
			request.OccurredAt = &occurredAt
		}
		if request.Content == nil && request.OccurredAt == nil {
			return OperationPreviewInput{}, fmt.Errorf("content or occurred_at is required")
		}
		payload := struct {
			RecordID string                          `json:"record_id"`
			Request  calendarapp.UpdateRecordRequest `json:"request"`
		}{RecordID: recordID, Request: request}
		return OperationPreviewInput{Target: operationTargetCalendarRecordUpdate, Summary: calendarRecordUpdateSummary(recordID, request), Payload: payload}, nil
	case "update_pet_profile":
		petID, _ := args["pet_id"].(string)
		if petID == "" {
			return OperationPreviewInput{}, fmt.Errorf("pet_id is required")
		}
		fields := make(map[string]any, len(args))
		for _, key := range []string{"name", "breed", "gender"} {
			if value, ok := args[key].(string); ok && value != "" {
				fields[key] = value
			}
		}
		if value, ok := args["sterilized"].(bool); ok {
			fields["sterilized"] = value
		}
		if value, ok := args["birthday"].(string); ok && value != "" {
			if _, err := time.Parse("2006-01-02", value); err != nil {
				return OperationPreviewInput{}, fmt.Errorf("birthday must be YYYY-MM-DD")
			}
			fields["birthday"] = value
		}
		if value, ok := args["home_date"].(string); ok && value != "" {
			if _, err := time.Parse("2006-01-02", value); err != nil {
				return OperationPreviewInput{}, fmt.Errorf("home_date must be YYYY-MM-DD")
			}
			fields["home_date"] = value
		}
		if len(fields) == 0 {
			return OperationPreviewInput{}, fmt.Errorf("at least one profile field is required")
		}
		payload := struct {
			PetID  string         `json:"pet_id"`
			Fields map[string]any `json:"fields"`
		}{PetID: petID, Fields: fields}
		return OperationPreviewInput{Target: operationTargetPetProfileUpdate, Summary: petProfileUpdateSummary(petID, fields), Payload: payload}, nil
	case "complete_calendar_reminder":
		reminderID, _ := args["reminder_id"].(string)
		if reminderID == "" {
			return OperationPreviewInput{}, fmt.Errorf("reminder_id is required")
		}
		request := calendarapp.CompleteReminderRequest{}
		if completedAt, ok := args["completed_at"].(string); ok && completedAt != "" {
			if _, err := time.Parse(time.RFC3339, completedAt); err != nil {
				return OperationPreviewInput{}, fmt.Errorf("completed_at must be RFC3339")
			}
			request.CompletedAt = completedAt
		}
		if content, ok := args["content"].(string); ok {
			request.Content = content
		}
		payload := struct {
			ReminderID string                              `json:"reminder_id"`
			Request    calendarapp.CompleteReminderRequest `json:"request"`
		}{ReminderID: reminderID, Request: request}
		return OperationPreviewInput{Target: operationTargetCalendarReminderComplete, Summary: "完成待办提醒（" + reminderID + "）", Payload: payload}, nil
	case "update_pet_health":
		petID, _ := args["pet_id"].(string)
		if petID == "" {
			return OperationPreviewInput{}, fmt.Errorf("pet_id is required")
		}
		fields := make(map[string]any, 3)
		for _, key := range []string{"status", "allergies", "long_term_medication"} {
			if value, ok := args[key].(string); ok && value != "" {
				fields[key] = value
			}
		}
		if len(fields) == 0 {
			return OperationPreviewInput{}, fmt.Errorf("at least one health field is required")
		}
		payload := struct {
			PetID  string         `json:"pet_id"`
			Fields map[string]any `json:"fields"`
		}{PetID: petID, Fields: fields}
		return OperationPreviewInput{Target: operationTargetPetHealthUpdate, Summary: "修改宠物健康档案（" + petID + "）：" + healthFieldsSummary(fields), Payload: payload}, nil
	case "create_pet":
		name, _ := args["name"].(string)
		if name == "" {
			return OperationPreviewInput{}, fmt.Errorf("name is required")
		}
		payload := struct {
			Name       string `json:"name"`
			Breed      string `json:"breed"`
			Gender     string `json:"gender"`
			Birthday   string `json:"birthday"`
			HomeDate   string `json:"home_date"`
			Sterilized *bool  `json:"sterilized"`
		}{Name: name}
		if value, ok := args["breed"].(string); ok {
			payload.Breed = value
		}
		if value, ok := args["gender"].(string); ok {
			payload.Gender = value
		}
		if value, ok := args["birthday"].(string); ok && value != "" {
			if _, err := time.Parse("2006-01-02", value); err != nil {
				return OperationPreviewInput{}, fmt.Errorf("birthday must be YYYY-MM-DD")
			}
			payload.Birthday = value
		}
		if value, ok := args["home_date"].(string); ok && value != "" {
			if _, err := time.Parse("2006-01-02", value); err != nil {
				return OperationPreviewInput{}, fmt.Errorf("home_date must be YYYY-MM-DD")
			}
			payload.HomeDate = value
		}
		if value, ok := args["sterilized"].(bool); ok {
			payload.Sterilized = &value
		}
		summary := "新建宠物：" + name
		if payload.Breed != "" {
			summary += "，品种=" + payload.Breed
		}
		if payload.Birthday != "" {
			summary += "，生日=" + payload.Birthday
		}
		return OperationPreviewInput{Target: operationTargetPetCreate, Summary: summary, Payload: payload}, nil
	default:
		return OperationPreviewInput{}, fmt.Errorf("prepare action not supported for tool %s", toolName)
	}
}

// calendarRecordSummary 生成待确认预览摘要。文案由服务端确定性生成，
// 不采用模型撰写的文字，避免确认界面出现不可核验的描述。
func calendarRecordSummary(request calendarapp.CreateRecordRequest) string {
	label := "日常记录"
	if request.Category == "medical" {
		label = "医疗记录"
	}
	if request.MedicalType != "" {
		label += "（" + request.MedicalType + "）"
	}
	occurred := request.OccurredAt
	if parsed, err := time.Parse(time.RFC3339, request.OccurredAt); err == nil {
		occurred = parsed.In(askTimezone).Format("2006-01-02 15:04")
	}
	return "新增" + label + "：" + occurred + " " + request.Content
}

// calendarRecordUpdateSummary 生成记录修改预览摘要（服务端确定性文案）。
func calendarRecordUpdateSummary(recordID string, request calendarapp.UpdateRecordRequest) string {
	parts := make([]string, 0, 2)
	if request.Content != nil {
		parts = append(parts, "内容改为「"+*request.Content+"」")
	}
	if request.OccurredAt != nil {
		occurred := *request.OccurredAt
		if parsed, err := time.Parse(time.RFC3339, *request.OccurredAt); err == nil {
			occurred = parsed.In(askTimezone).Format("2006-01-02 15:04")
		}
		parts = append(parts, "时间改为 "+occurred)
	}
	return "修改记录（" + recordID + "）：" + strings.Join(parts, "，")
}

// petProfileUpdateSummary 生成档案修改预览摘要（字段顺序固定，便于用户核对）。
// healthFieldsSummary 生成健康档案字段摘要（顺序固定）。
func healthFieldsSummary(fields map[string]any) string {
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%v", key, fields[key]))
	}
	return strings.Join(parts, "，")
}

func petProfileUpdateSummary(petID string, fields map[string]any) string {
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%v", key, fields[key]))
	}
	return "修改宠物档案（" + petID + "）：" + strings.Join(parts, "，")
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
	if tool.Name == "list_family_pets" {
		outcome, err := a.business.ListFamilyPets(ctx, a.familyID)
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
	if tool.Name == "list_calendar_records" {
		since := time.Now().AddDate(0, 0, -30)
		if value, ok := args["start_at"].(string); ok && value != "" {
			parsed, err := time.Parse(time.RFC3339, value)
			if err != nil {
				return a.failedResult(call, ToolErrInvalidArgument, "validate", "start_at must be RFC3339", false)
			}
			since = parsed
		}
		before := time.Now()
		if value, ok := args["end_at"].(string); ok && value != "" {
			parsed, err := time.Parse(time.RFC3339, value)
			if err != nil {
				return a.failedResult(call, ToolErrInvalidArgument, "validate", "end_at must be RFC3339", false)
			}
			before = parsed
		}
		limit := 0
		if value, ok := args["limit"].(float64); ok {
			limit = int(value)
		}
		outcome, err := a.business.ListCalendarRecords(ctx, a.familyID, since, before, limit)
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
	if tool.Name == "list_reminders" {
		before := time.Now().In(askTimezone).Format("2006-01-02")
		if value, ok := args["due_before"].(string); ok && value != "" {
			if _, err := time.Parse("2006-01-02", value); err != nil {
				return a.failedResult(call, ToolErrInvalidArgument, "validate", "due_before must be YYYY-MM-DD", false)
			}
			before = value
		}
		limit := 0
		if value, ok := args["limit"].(float64); ok {
			limit = int(value)
		}
		outcome, err := a.business.ListReminders(ctx, a.familyID, before, limit)
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
		ToolCallID:  call.ToolCallID,
		CallIndex:   call.CallIndex,
		Status:      ToolResultError,
		Error:       &ToolError{Category: category, Stage: stage, Reason: reason, Retryable: retryable},
		QueuedAt:    time.Now(),
		StartedAt:   time.Now(),
		CompletedAt: time.Now(),
	}
}

func (a *ToolExecutorAdapter) blockedResult(call ToolCall) ToolResult {
	now := time.Now()
	return ToolResult{
		ToolCallID:   call.ToolCallID,
		CallIndex:    call.CallIndex,
		Status:       ToolResultNotExecuted,
		Completeness: CompletenessNotApplicable,
		Error:        &ToolError{Category: ToolErrPrecondition, Stage: "dispatch", Reason: "dependency failed", Retryable: false},
		QueuedAt:     now,
		CompletedAt:  now,
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

// loopGuardResult 是执行层 Loop Guard 的阻断结果：不执行、不产生副作用，
// 只把阻断原因返回给模型，让它调整参数或换用其他能力（文档 7.4）。
func (a *ToolExecutorAdapter) loopGuardResult(call ToolCall, reason string) ToolResult {
	now := time.Now()
	return ToolResult{
		ToolCallID:   call.ToolCallID,
		CallIndex:    call.CallIndex,
		Status:       ToolResultNotExecuted,
		Completeness: CompletenessNotApplicable,
		Error:        &ToolError{Category: ToolErrPrecondition, Stage: "dispatch", Reason: reason, Retryable: false},
		QueuedAt:     now,
		CompletedAt:  now,
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
