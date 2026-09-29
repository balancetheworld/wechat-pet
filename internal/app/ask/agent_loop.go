package ask

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"
)

// 本文件固定 v2 单 Agent 决策循环的主循环编排（文档 2、7、8、9）。
// 循环：组装上下文 -> 业务决策(agent_step) -> 解析 record_array_v1 -> 按动作分发
// -> 工具执行与结果回灌 -> 再次决策，直到 final_answer / request_input 或步骤预算耗尽。
// 模型、工具执行通过端口接口注入，编排本身不依赖具体 Provider 或数据层实现。

// AgentModel 是决策循环依赖的模型端口（文档 4.1 agent_step 用途）。
// 调用方是 Runtime；输入是已组装并筛选的上下文与候选工具，输出是一次
// record_array_v1 响应的完整记录。
type AgentModel interface {
	Step(ctx context.Context, input StepInput) (ModelStepResult, error)
}

// ToolExecutor 是决策循环依赖的工具执行端口（文档 7 节）。
// 整批校验、只读并行、局部失败与结果封装由实现负责；返回逐项结果。
type ToolExecutor interface {
	ExecuteBatch(ctx context.Context, batch ToolBatch) ([]ToolResult, error)
}

// StepInput 是一次 agent_step 决策的输入（文档 4.1、5.8）。
type StepInput struct {
	SessionID       string         // 当前 Session 身份
	RunID           string         // 当前 Run 身份
	Blocks          []ContextBlock // 已组装、去重、裁剪的上下文
	Tools           []Tool         // 候选工具（已过滤）；无工具时传空集合
	Budget          ModelBudgetSnapshot
	Images          []ModelImage
	OnAnswerDelta   func(string) error // final_answer 正文段闭合时即时回传预览；nil 表示不使用流式预览
	OnThinkingDelta func(string) error // 模型推理正文分片即时回传；nil 表示不下发思考预览
}

type ModelImage struct {
	AssetID string
	Content []byte
}

type ModelBudgetSnapshot struct {
	Limits      BudgetLimits
	Used        BudgetAmount
	Reserved    BudgetAmount
	Reservation BudgetAmount
}

type ModelUsage struct {
	InputTokens  int
	OutputTokens int
	TotalTokens  int
	Complete     bool
}

type ModelStepResult struct {
	Records           []ProtocolRecord
	ProviderRequestID string
	Usage             ModelUsage
}

// LoopOutcome 是决策循环的最终结果（文档 2.2）。
type LoopOutcome struct {
	Action      ResponseAction // final_answer 或 request_input
	Groups      []AnswerGroup  // final_answer 的回答组
	Questions   []Question     // request_input 的追问
	TaskItems   []TaskItem     // 最终任务项快照
	Coverage    []TaskCoverage // 最终回答或追问的任务覆盖关系
	ToolResults []ToolResult   // 循环中累积的真实工具结果
	Steps       int            // 已消耗的决策步数
}

// maxDecisionSteps 是单 Run 决策循环的保守默认步数上限（文档 9.3「有限模型调用总数」）。
// 实际值由实施 Profile 固定；此处给出保守默认，防止无限循环。
const maxDecisionSteps = 16

// RunDecisionLoop 执行 v2 单 Agent 决策循环（文档 2、7、8、9）。
// maxSteps <= 0 时使用保守默认值。循环每步：
//  1. 调用模型得到 record_array_v1 记录；
//  2. DecideStep 冻结动作并分发；
//  3. call_tools -> 构造批次、执行工具、结果回灌后继续；
//     request_input / final_answer -> 返回最终处置。
func RunDecisionLoop(ctx context.Context, model AgentModel, tools ToolExecutor, initial StepInput, maxSteps int) (LoopOutcome, error) {
	return runDecisionLoop(ctx, model, tools, initial, maxSteps, nil, nil, "", nil, nil)
}

func runDecisionLoop(ctx context.Context, model AgentModel, tools ToolExecutor, initial StepInput, maxSteps int, repository Repository, restored []TaskItem, turnID string, budgets BudgetRepository, recallTools func([]ContextBlock) []Tool) (LoopOutcome, error) {
	if maxSteps <= 0 {
		maxSteps = maxDecisionSteps
	}
	blocks := append([]ContextBlock(nil), initial.Blocks...)
	taskItems := append([]TaskItem(nil), restored...)
	toolResults := []ToolResult{}
	callKeys := make(map[string]struct{})
	operationIDs := make(map[string]struct{})
	for _, tool := range initial.Tools {
		operationIDs[tool.OperationID] = struct{}{}
	}
	outcome := LoopOutcome{}
	validationRetries := 0
	outputRepairs := 0
	validationFeedback := ""

	for step := 0; step < maxSteps; step++ {
		stepTools := initial.Tools
		if recallTools != nil {
			stepTools = recallTools(blocks)
		}
		stepBlocks := blocks
		if len(taskItems) > 0 {
			taskBlock, err := taskStateBlock(initial.RunID, taskItems)
			if err != nil {
				return outcome, err
			}
			stepBlocks = append(append([]ContextBlock(nil), stepBlocks...), taskBlock)
		}
		if validationFeedback != "" {
			stepBlocks = append(stepBlocks, ContextBlock{Layer: LayerControlInstructions, Kind: "instruction", ObjectID: "validation_feedback", Text: "上一次回答未通过校验：" + validationFeedback + "。请重新生成完整的 record_array_v1，覆盖当前任务清单并使用当前回答类型允许的字段。", Required: true})
		}
		var budgetSnapshot ModelBudgetSnapshot
		if budgets != nil {
			ledger, err := budgets.GetBudget(ctx, BudgetRun, initial.RunID)
			if err != nil {
				return outcome, err
			}
			inputBudget := ledger.Limits.MaxTokens - ledger.Used.Tokens - ledger.Reserved.Tokens - agentOutputTokenReserve
			stepBlocks, err = TrimContext(stepBlocks, inputBudget)
			if err != nil {
				return outcome, NewExecutorError("budget_exhausted", false, 0, err)
			}
			budgetSnapshot = ModelBudgetSnapshot{Limits: ledger.Limits, Used: ledger.Used, Reserved: ledger.Reserved}
		}
		evidenceKeys := evidenceKeysForBlocks(stepBlocks)
		addEvidenceKey(evidenceKeys, "turn", turnID, "")
		modelContext, cancelModel, err := callBudgetContext(ctx, budgets, initial.RunID)
		if err != nil {
			return outcome, err
		}
		modelEstimate := BudgetAmount{ModelCalls: 1, Tokens: contextTokenCount(stepBlocks) + agentOutputTokenReserve, DurationMillis: 1, Concurrency: 1}
		modelReservation, err := reserveCallBudget(ctx, budgets, initial.RunID, modelEstimate)
		if err != nil {
			cancelModel()
			return outcome, err
		}
		budgetSnapshot.Reservation = modelEstimate
		modelStartedAt := time.Now()
		modelResult, err := model.Step(modelContext, StepInput{SessionID: initial.SessionID, RunID: initial.RunID, Blocks: stepBlocks, Tools: stepTools, Budget: budgetSnapshot, Images: initial.Images, OnAnswerDelta: initial.OnAnswerDelta, OnThinkingDelta: initial.OnThinkingDelta})
		modelDuration := elapsedMillis(modelStartedAt)
		budgetDeadlineExceeded := errors.Is(modelContext.Err(), context.DeadlineExceeded) && ctx.Err() == nil
		cancelModel()
		if settleErr := accountModelBudget(ctx, budgets, modelReservation, modelResult.Usage, modelDuration); settleErr != nil {
			return outcome, settleErr
		}
		if budgetDeadlineExceeded {
			return outcome, NewExecutorError("budget_exhausted", false, 0, context.DeadlineExceeded)
		}
		if err != nil {
			if isAgentOutputUnparsable(err) && outputRepairs < maxOutputRepairs && step+1 < maxSteps {
				outputRepairs++
				validationFeedback = outputRepairFeedback(err)
				continue
			}
			return outcome, err
		}
		decision, err := DecideStep(modelResult.Records)
		if err != nil {
			if validationRetries == 0 && step+1 < maxSteps {
				validationRetries++
				validationFeedback = err.Error()
				continue
			}
			return outcome, err
		}
		normalizeDecisionEvidence(&decision, evidenceKeys, turnID)
		nextCallKeys := cloneKeySet(callKeys)
		if err := validateDecisionReferences(decision, taskItems, nextCallKeys, operationIDs, evidenceKeys, initial.RunID); err != nil {
			if validationRetries == 0 && step+1 < maxSteps {
				validationRetries++
				validationFeedback = err.Error()
				continue
			}
			return outcome, err
		}
		callKeys = nextCallKeys
		// 合并本轮任务项更新（header.task_updates 映射为待分配稳定 ID 的任务项）。
		if len(decision.TaskUpdates) > 0 {
			mapped, mapErr := MapTaskUpdates(decision.TaskUpdates, "", "")
			if mapErr != nil {
				return outcome, mapErr
			}
			for index := range mapped {
				mapped[index].TaskItemID = stableTaskItemID(initial.RunID, decision.TaskUpdates[index].TaskKey)
				mapped[index].OriginTurnID = turnID
				mapped[index].RunID = initial.RunID
				if len(mapped[index].SourceTurnIDs) == 0 && turnID != "" {
					mapped[index].SourceTurnIDs = []string{turnID}
				}
			}
			taskItems = mergeTaskItems(taskItems, mapped)
			if err := persistTaskItems(ctx, repository, taskItems); err != nil {
				return outcome, err
			}
		}

		outcome.Steps = step + 1
		switch decision.Action {
		case ActionCallTools:
			validationRetries = 0
			validationFeedback = ""
			batch := BuildToolBatch(decision.Calls, "", "", "")
			toolContext, cancelTools, budgetErr := callBudgetContext(ctx, budgets, initial.RunID)
			if budgetErr != nil {
				return outcome, budgetErr
			}
			toolEstimate := BudgetAmount{ToolCalls: len(batch.Calls), DurationMillis: 1, Concurrency: 1}
			toolReservation, reserveErr := reserveCallBudget(ctx, budgets, initial.RunID, toolEstimate)
			if reserveErr != nil {
				cancelTools()
				return outcome, reserveErr
			}
			toolStartedAt := time.Now()
			results, execErr := tools.ExecuteBatch(toolContext, batch)
			toolDuration := elapsedMillis(toolStartedAt)
			budgetDeadlineExceeded := errors.Is(toolContext.Err(), context.DeadlineExceeded) && ctx.Err() == nil
			cancelTools()
			toolActual := toolEstimate
			toolActual.DurationMillis = toolDuration
			if settleErr := settleCallBudget(ctx, budgets, toolReservation, toolActual); settleErr != nil {
				return outcome, settleErr
			}
			if budgetDeadlineExceeded {
				return outcome, NewExecutorError("budget_exhausted", false, 0, context.DeadlineExceeded)
			}
			if execErr != nil {
				var validationErr *BatchValidationError
				if errors.As(execErr, &validationErr) {
					calls := make([]string, 0, len(batch.Calls))
					for _, call := range batch.Calls {
						calls = append(calls, fmt.Sprintf("%s=%s@%s", call.ToolCallID, call.ToolName, call.ToolVersion))
					}
					return outcome, fmt.Errorf("agent_loop: tool batch [%s]: %w", strings.Join(calls, ", "), execErr)
				}
				return outcome, execErr
			}
			toolResults = append(toolResults, results...)
			// 结果回灌：真实工具结果作为低信任参考数据进入下一轮上下文。
			for _, r := range results {
				blocks = append(blocks, toolResultBlock(r))
			}
			// 按 coverage 记录未完成项，继续下一轮决策。
			taskItems = ResolveCoverage(taskItems, stableCoverage(initial.RunID, decision.Coverage))
			if err := persistTaskItems(ctx, repository, taskItems); err != nil {
				return outcome, err
			}
			continue

		case ActionRequestInput:
			// 把被追问的任务项标记为「需补信息」并携带缺失字段（文档 2.5、2.3.2）。
			taskItems = ResolveQuestionCoverage(taskItems, stableQuestions(initial.RunID, decision.Questions))
			if err := persistTaskItems(ctx, repository, taskItems); err != nil {
				return outcome, err
			}
			outcome.Action = ActionRequestInput
			outcome.Questions = BuildQuestions(decision.Questions)
			outcome.TaskItems = taskItems
			outcome.Coverage = decision.Coverage
			outcome.ToolResults = toolResults
			return outcome, nil

		case ActionFinalAnswer:
			// 先按 coverage + 真实回答组裁决任务项终态，再做回答组校验与覆盖闭合检查
			// （文档 2.3.2、8.5：final_answer 必须逐项交代结果或受阻原因，不能遗留无说明的待处理目标）。
			finalItems := ResolveFinalCoverage(taskItems, stableCoverage(initial.RunID, decision.Coverage), decision.Groups)
			if err := ValidateFinalAnswer(decision, finalItems); err != nil {
				if validationRetries == 0 && step+1 < maxSteps {
					validationRetries++
					validationFeedback = err.Error()
					continue
				}
				return outcome, err
			}
			taskItems = finalItems
			if err := persistTaskItems(ctx, repository, taskItems); err != nil {
				return outcome, err
			}
			outcome.Action = ActionFinalAnswer
			outcome.Groups = decision.Groups
			outcome.TaskItems = taskItems
			outcome.Coverage = decision.Coverage
			outcome.ToolResults = toolResults
			return outcome, nil

		default:
			return outcome, fmt.Errorf("agent_loop: unhandled action %q", decision.Action)
		}
	}
	return outcome, fmt.Errorf("agent_loop: exceeded %d decision steps without terminal action", maxSteps)
}

// maxOutputRepairs 是「模型正文无法解析」的修复次数上限（文档 9.3）。
// 它与校验修复各自计数：解析失败不挤占校验修复额度，两类修复合计仍在模型调用预算内。
const maxOutputRepairs = 1

// isAgentOutputUnparsable 报告模型应答正文是否完整返回但无法按协议解析。
func isAgentOutputUnparsable(err error) bool {
	var executorError *ExecutorError
	return errors.As(err, &executorError) && executorError.Code == ErrAgentOutputUnparsable
}

// outputRepairFeedback 把解析失败整理为受控纠错提示：只保留错误类别与位置，
// 不回灌错误信息末尾的模型原文片段（原文片段仅供服务端日志诊断，文档 8.4）。
func outputRepairFeedback(err error) string {
	reason := err.Error()
	if index := strings.Index(reason, " (len="); index >= 0 {
		reason = reason[:index]
	}
	reason = strings.TrimPrefix(reason, ErrAgentOutputUnparsable+": ")
	return "模型回复不是可解析的 record_array_v1（" + reason + "）"
}

const agentOutputTokenReserve = 2048

func contextTokenCount(blocks []ContextBlock) int {
	total := 0
	for _, block := range blocks {
		total += EstimateTokens(block.Text)
	}
	return total
}

func reserveCallBudget(ctx context.Context, budgets BudgetRepository, runID string, amount BudgetAmount) (BudgetReservation, error) {
	if budgets == nil {
		return BudgetReservation{}, nil
	}
	reservation, err := budgets.ReserveBudget(ctx, BudgetRun, runID, amount)
	if errors.Is(err, ErrBudgetExceeded) {
		return BudgetReservation{}, NewExecutorError("budget_exhausted", false, 0, err)
	}
	return reservation, err
}

func settleCallBudget(ctx context.Context, budgets BudgetRepository, reservation BudgetReservation, amount BudgetAmount) error {
	if budgets == nil {
		return nil
	}
	amount.Concurrency = 0
	return budgets.SettleBudget(ctx, reservation.ID, amount)
}

func accountModelBudget(ctx context.Context, budgets BudgetRepository, reservation BudgetReservation, usage ModelUsage, durationMillis int64) error {
	if budgets == nil {
		return nil
	}
	if !usage.Complete {
		amount := reservation.Amount
		amount.DurationMillis = durationMillis
		return settleCallBudget(ctx, budgets, reservation, amount)
	}
	tokens := usage.TotalTokens
	if tokens == 0 {
		tokens = usage.InputTokens + usage.OutputTokens
	}
	return settleCallBudget(ctx, budgets, reservation, BudgetAmount{ModelCalls: 1, Tokens: tokens, DurationMillis: durationMillis})
}

func callBudgetContext(ctx context.Context, budgets BudgetRepository, runID string) (context.Context, context.CancelFunc, error) {
	if budgets == nil {
		callContext, cancel := context.WithCancel(ctx)
		return callContext, cancel, nil
	}
	ledger, err := budgets.GetBudget(ctx, BudgetRun, runID)
	if err != nil {
		return nil, nil, err
	}
	if ledger.Limits.MaxDurationMillis <= 0 {
		callContext, cancel := context.WithCancel(ctx)
		return callContext, cancel, nil
	}
	remaining := ledger.Limits.MaxDurationMillis - ledger.Used.DurationMillis - ledger.Reserved.DurationMillis
	if remaining <= 0 {
		return nil, nil, NewExecutorError("budget_exhausted", false, 0, ErrBudgetExceeded)
	}
	callContext, cancel := context.WithTimeout(ctx, time.Duration(remaining)*time.Millisecond)
	return callContext, cancel, nil
}

func elapsedMillis(startedAt time.Time) int64 {
	elapsed := time.Since(startedAt)
	millis := (elapsed + time.Millisecond - 1) / time.Millisecond
	if millis < 1 {
		return 1
	}
	return int64(millis)
}

func stableTaskItemID(runID, taskKey string) string {
	if runID == "" {
		return taskKey
	}
	return runID + ":" + taskKey
}

func taskKeyFromItem(runID, taskItemID string) string {
	prefix := runID + ":"
	if runID != "" && strings.HasPrefix(taskItemID, prefix) {
		return strings.TrimPrefix(taskItemID, prefix)
	}
	return taskItemID
}

func taskStateBlock(runID string, items []TaskItem) (ContextBlock, error) {
	tasks := make([]struct {
		TaskKey string      `json:"task_key"`
		Goal    string      `json:"goal"`
		Outcome TaskOutcome `json:"outcome"`
	}, 0, len(items))
	for _, item := range items {
		tasks = append(tasks, struct {
			TaskKey string      `json:"task_key"`
			Goal    string      `json:"goal"`
			Outcome TaskOutcome `json:"outcome"`
		}{TaskKey: taskKeyFromItem(runID, item.TaskItemID), Goal: item.Goal, Outcome: item.Outcome})
	}
	data, err := json.Marshal(tasks)
	if err != nil {
		return ContextBlock{}, err
	}
	return ContextBlock{Layer: LayerReferenceData, Kind: "task", ObjectID: "current_run_tasks", Text: "当前 Run 任务清单：" + string(data), Required: true}, nil
}

func stableCoverage(runID string, coverage []TaskCoverage) []TaskCoverage {
	result := append([]TaskCoverage(nil), coverage...)
	for index := range result {
		result[index].TaskKey = stableTaskItemID(runID, result[index].TaskKey)
	}
	return result
}

func stableQuestions(runID string, questions []QuestionRecord) []QuestionRecord {
	result := append([]QuestionRecord(nil), questions...)
	for index := range result {
		result[index].TaskKeys = append([]string(nil), result[index].TaskKeys...)
		for taskIndex := range result[index].TaskKeys {
			result[index].TaskKeys[taskIndex] = stableTaskItemID(runID, result[index].TaskKeys[taskIndex])
		}
		result[index].MissingFields = append([]MissingField(nil), result[index].MissingFields...)
		for fieldIndex := range result[index].MissingFields {
			result[index].MissingFields[fieldIndex].TaskKey = stableTaskItemID(runID, result[index].MissingFields[fieldIndex].TaskKey)
		}
	}
	return result
}

func persistTaskItems(ctx context.Context, repository Repository, items []TaskItem) error {
	if repository == nil || len(items) == 0 {
		return nil
	}
	existing, err := repository.ListTaskItems(ctx, items[0].RunID)
	if err != nil {
		return err
	}
	byID := make(map[string]TaskItem, len(existing))
	for _, item := range existing {
		byID[item.TaskItemID] = item
	}
	for index := range items {
		stored, ok := byID[items[index].TaskItemID]
		if !ok {
			if err := repository.CreateTaskItems(ctx, []TaskItem{items[index]}); err != nil {
				return err
			}
			continue
		}
		candidate := items[index]
		candidate.ItemRevision = stored.ItemRevision
		if reflect.DeepEqual(candidate, stored) {
			items[index].ItemRevision = stored.ItemRevision
			continue
		}
		if err := repository.UpdateTaskItem(ctx, candidate); err != nil {
			return err
		}
		items[index].ItemRevision = stored.ItemRevision + 1
	}
	return nil
}

// toolResultBlock 把一项真实工具结果转换为回灌上下文块（文档 5.8 第 5 层）。
// 工具结果是有来源的低信任参考数据，不获得指令权限。
func toolResultBlock(r ToolResult) ContextBlock {
	return ContextBlock{
		Layer:        LayerToolInteractions,
		Kind:         "tool_result",
		ObjectID:     r.ToolCallID,
		Version:      "",
		Position:     "",
		Text:         toolResultText(r),
		EvidenceRefs: toolEvidenceRefs(r),
	}
}

func evidenceKeysForBlocks(blocks []ContextBlock) map[string]struct{} {
	keys := make(map[string]struct{})
	for _, block := range blocks {
		for _, ref := range block.EvidenceRefs {
			addEvidenceKey(keys, ref.SourceType, ref.SourceID, ref.Version)
		}
	}
	return keys
}

// normalizeDecisionEvidence 归一化模型给出的证据引用：只保留服务端可核验的引用。
// 用户陈述（user_statement）的依据由服务端确定性补齐为当前轮 turn/<turn_id>，不依赖模型；
// 图片观察、一般知识等来源没有可核验 id，模型可能凭格式填出并不存在的引用，这些引用直接丢弃。
// business_fact 且没有任何可核验引用时降级为 general_knowledge：写入预览、尚未确认的操作
// 在 Run 期间本来就没有可核验来源，模型却常标成业务事实；来源标注不准不应打死整轮回答，
// 真正的事实边界由风险等级、限制说明与后续核实负责。
func normalizeDecisionEvidence(decision *StepDecision, evidenceKeys map[string]struct{}, turnID string) {
	for groupIndex := range decision.Groups {
		group := &decision.Groups[groupIndex]
		for segmentIndex := range group.Segments {
			segment := &group.Segments[segmentIndex]
			segment.EvidenceRefs = resolvableEvidenceRefs(segment.EvidenceRefs, evidenceKeys)
			if segment.BasisKind == BasisUserStatement && turnID != "" {
				segment.EvidenceRefs = withTurnEvidenceRef(segment.EvidenceRefs, turnID)
			}
			if segment.BasisKind == BasisBusinessFact && len(segment.EvidenceRefs) == 0 {
				segment.BasisKind = BasisGeneralKnowledge
			}
		}
		for riskIndex := range group.Risks {
			group.Risks[riskIndex].Evidence = resolvableEvidenceRefs(group.Risks[riskIndex].Evidence, evidenceKeys)
		}
	}
}

func withTurnEvidenceRef(refs []EvidenceRef, turnID string) []EvidenceRef {
	for _, ref := range refs {
		if ref.SourceType == "turn" && ref.SourceID == turnID {
			return refs
		}
	}
	return append(refs, EvidenceRef{SourceType: "turn", SourceID: turnID})
}

func resolvableEvidenceRefs(refs []EvidenceRef, evidenceKeys map[string]struct{}) []EvidenceRef {
	if len(refs) == 0 {
		return refs
	}
	result := make([]EvidenceRef, 0, len(refs))
	for _, ref := range refs {
		if evidenceExists(evidenceKeys, ref) {
			result = append(result, ref)
		}
	}
	return result
}

func cloneKeySet(source map[string]struct{}) map[string]struct{} {
	result := make(map[string]struct{}, len(source))
	for key := range source {
		result[key] = struct{}{}
	}
	return result
}

func validateDecisionReferences(decision StepDecision, taskItems []TaskItem, callKeys, operationIDs, evidenceKeys map[string]struct{}, runID string) error {
	tasks := make(map[string]struct{}, len(taskItems)+len(decision.TaskUpdates))
	for _, item := range taskItems {
		tasks[taskKeyFromItem(runID, item.TaskItemID)] = struct{}{}
	}
	for _, update := range decision.TaskUpdates {
		if update.TaskKey == "" || update.Goal == "" {
			return fmt.Errorf("decision_loop: task update must have task_key and goal")
		}
		tasks[update.TaskKey] = struct{}{}
	}
	for _, call := range decision.Calls {
		if _, exists := callKeys[call.CallKey]; exists {
			return fmt.Errorf("decision_loop: duplicate call key %q", call.CallKey)
		}
		for _, taskKey := range call.TaskKeys {
			if _, exists := tasks[taskKey]; !exists {
				return fmt.Errorf("decision_loop: call %q references unknown task %q", call.CallKey, taskKey)
			}
		}
		callKeys[call.CallKey] = struct{}{}
	}
	for _, question := range decision.Questions {
		for _, taskKey := range question.TaskKeys {
			if _, exists := tasks[taskKey]; !exists {
				return fmt.Errorf("decision_loop: question %q references unknown task %q", question.QuestionKey, taskKey)
			}
		}
	}
	groupKeys := make(map[string]struct{}, len(decision.Groups))
	for _, group := range decision.Groups {
		groupKeys[group.GroupKey] = struct{}{}
		for _, taskKey := range group.TaskKeys {
			if _, exists := tasks[taskKey]; !exists {
				return fmt.Errorf("decision_loop: group %q references unknown task %q", group.GroupKey, taskKey)
			}
		}
		for _, segment := range group.Segments {
			for _, evidence := range segment.EvidenceRefs {
				if !evidenceExists(evidenceKeys, evidence) {
					return fmt.Errorf("decision_loop: segment %q references unknown evidence %s/%s", segment.SegmentKey, evidence.SourceType, evidence.SourceID)
				}
			}
		}
		for _, risk := range group.Risks {
			for _, evidence := range risk.Evidence {
				if !evidenceExists(evidenceKeys, evidence) {
					return fmt.Errorf("decision_loop: risk %q references unknown evidence %s/%s", group.GroupKey, evidence.SourceType, evidence.SourceID)
				}
			}
		}
	}
	for _, coverage := range decision.Coverage {
		if _, exists := tasks[coverage.TaskKey]; !exists {
			return fmt.Errorf("decision_loop: coverage references unknown task %q", coverage.TaskKey)
		}
		for _, groupKey := range coverage.AnswerGroupKeys {
			if _, exists := groupKeys[groupKey]; !exists {
				return fmt.Errorf("decision_loop: coverage references unknown answer group %q", groupKey)
			}
		}
		for _, callKey := range coverage.CallKeys {
			if _, exists := callKeys[callKey]; !exists {
				return fmt.Errorf("decision_loop: coverage references unknown call %q", callKey)
			}
		}
		for _, operationID := range coverage.OperationIDs {
			if _, exists := operationIDs[operationID]; !exists {
				return fmt.Errorf("decision_loop: coverage references unknown operation %q", operationID)
			}
		}
	}
	return nil
}

func addToolEvidence(evidenceKeys map[string]struct{}, result ToolResult) {
	for _, ref := range toolEvidenceRefs(result) {
		addEvidenceKey(evidenceKeys, ref.SourceType, ref.SourceID, ref.Version)
	}
}

func toolEvidenceRefs(result ToolResult) []EvidenceRef {
	refs := make([]EvidenceRef, 0)
	if result.Source.SourceType != "" && result.Source.SourceID != "" {
		refs = append(refs, EvidenceRef{SourceType: result.Source.SourceType, SourceID: result.Source.SourceID, Version: result.Source.Version})
	}
	switch value := result.Data.(type) {
	case HealthRecordSearchOutcome:
		for _, record := range value.Records {
			refs = append(refs, EvidenceRef{SourceType: "calendar_record", SourceID: record.ID, Version: value.Source.Version})
		}
	case *HealthRecordSearchOutcome:
		if value != nil {
			for _, record := range value.Records {
				refs = append(refs, EvidenceRef{SourceType: "calendar_record", SourceID: record.ID, Version: value.Source.Version})
			}
		}
	case HealthRecordOutcome:
		refs = append(refs, EvidenceRef{SourceType: "calendar_record", SourceID: value.Record.ID, Version: value.Source.Version})
	case *HealthRecordOutcome:
		if value != nil {
			refs = append(refs, EvidenceRef{SourceType: "calendar_record", SourceID: value.Record.ID, Version: value.Source.Version})
		}
	}
	return refs
}

func addEvidenceKey(keys map[string]struct{}, sourceType, sourceID, version string) {
	if sourceType == "" || sourceID == "" {
		return
	}
	keys[sourceType+"|"+sourceID] = struct{}{}
	if version != "" {
		keys[sourceType+"|"+sourceID+"|"+version] = struct{}{}
	}
}

func evidenceExists(keys map[string]struct{}, evidence EvidenceRef) bool {
	if evidence.SourceType == "" || evidence.SourceID == "" {
		return false
	}
	if evidence.Version != "" {
		if _, ok := keys[evidence.SourceType+"|"+evidence.SourceID+"|"+evidence.Version]; ok {
			return true
		}
	}
	_, ok := keys[evidence.SourceType+"|"+evidence.SourceID]
	return ok
}

// toolResultText 生成工具结果的受控文本（文档 7.3）。
// 只携带可公开的状态与数据摘要，不暴露内部 ID 或敏感字段。
func toolResultText(r ToolResult) string {
	if r.Error != nil {
		return fmt.Sprintf("[tool %s] error: %s", r.ToolCallID, r.Error.Reason)
	}
	payload := map[string]any{
		"tool_call_id": r.ToolCallID,
		"status":       r.Status,
		"completeness": r.Completeness,
		"truncated":    r.Completeness == CompletenessPartial || r.HasMore,
		"schema":       toolResultSchema(r),
		"source":       r.Source,
		"pagination": map[string]any{
			"has_more":    r.HasMore,
			"next_cursor": r.NextCursor,
		},
		"data": r.Data,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Sprintf("[tool %s] status=%s completeness=%s", r.ToolCallID, r.Status, r.Completeness)
	}
	return string(data)
}

func toolResultSchema(r ToolResult) string {
	switch r.Data.(type) {
	case PetResolveOutcome, *PetResolveOutcome:
		return "pet_resolve_outcome_v1"
	case PetProfileOutcome, *PetProfileOutcome:
		return "pet_profile_outcome_v1"
	case HealthRecordOutcome, *HealthRecordOutcome:
		return "health_record_outcome_v1"
	case HealthRecordSearchOutcome, *HealthRecordSearchOutcome:
		return "health_record_search_outcome_v1"
	case HealthRecordAggregateOutcome, *HealthRecordAggregateOutcome:
		return "health_record_aggregate_outcome_v1"
	default:
		return "unknown"
	}
}

// mergeTaskItems 合并任务项：同 TaskItemID 保留已恢复的执行状态，仅更新模型声明的目标信息。
// 稳定 ID 的分配由持久化层负责；此处仅做内存合并。
func mergeTaskItems(existing, mapped []TaskItem) []TaskItem {
	result := append([]TaskItem(nil), existing...)
	for _, item := range mapped {
		replaced := false
		for i := range result {
			if result[i].TaskItemID == item.TaskItemID {
				result[i].Goal = item.Goal
				result[i].Subjects = item.Subjects
				for _, sourceTurnID := range item.SourceTurnIDs {
					if !containsString(result[i].SourceTurnIDs, sourceTurnID) {
						result[i].SourceTurnIDs = append(result[i].SourceTurnIDs, sourceTurnID)
					}
				}
				replaced = true
				break
			}
		}
		if !replaced {
			result = append(result, item)
		}
	}
	return result
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
