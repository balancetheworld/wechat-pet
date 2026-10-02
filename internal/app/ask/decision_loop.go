package ask

import (
	"encoding/json"
	"fmt"
)

// 本文件固定 v2 单 Agent 决策循环的领域编排（文档 2.2、7.4、8.4、8.5、2.3.2）。
// 决策循环：组装上下文 -> 首次业务决策(agent_step) -> 解析 record_array_v1 ->
// 按动作分发（call_tools / request_input / final_answer）-> 工具结果回灌 -> 再次决策。
// 一个完整业务决策模型响应只提出一种控制动作；动作互斥，不能既等待又启动新工具，
// 也不能一边声明完成一边新增工具调用。

// StepDecision 是一次 agent_step 响应经校验后的处置决策（文档 2.2、8.4）。
type StepDecision struct {
	Action      ResponseAction
	TaskUpdates []TaskUpdate     // header 冻结的任务项更新
	Calls       []CallRecord     // call_tools 的调用集合
	Questions   []QuestionRecord // request_input 的问题集合
	Groups      []AnswerGroup    // final_answer 的按对象回答组
	Coverage    []TaskCoverage   // coverage 的覆盖关系
}

// DecideStep 把一次已解析的 record_array_v1 响应分发为动作处置（文档 8.4）。
// 先 ValidateResponse 冻结动作并校验结构，再按动作提取对应记录。
// 动作互斥由 ValidateResponse 保证：call_tools 不含 group/question，
// request_input 不含 call/group，final_answer 不含 call/question。
func DecideStep(records []ProtocolRecord) (StepDecision, error) {
	action, err := ValidateResponse(records)
	if err != nil {
		return StepDecision{}, err
	}
	decision := StepDecision{Action: action}
	if len(records) > 0 && records[0].Header != nil {
		decision.TaskUpdates = records[0].Header.TaskUpdates
	}
	for _, rec := range records {
		switch rec.Type {
		case RecordCall:
			decision.Calls = append(decision.Calls, *rec.Call)
		case RecordQuestion:
			decision.Questions = append(decision.Questions, *rec.Question)
		case RecordCoverage:
			decision.Coverage = rec.Coverage.Tasks
		}
	}
	switch action {
	case ActionFinalAnswer:
		groups, assembleErr := AssembleAnswerGroups(records)
		if assembleErr != nil {
			return StepDecision{}, assembleErr
		}
		decision.Groups = groups
	}
	if len(decision.Coverage) == 0 {
		decision.Coverage = deriveStepCoverage(decision)
	}
	return decision, nil
}

// deriveStepCoverage 在模型漏写 coverage 时按记录推导覆盖关系：
// 任务项来自 header.task_updates 与各记录引用的 task_key，
// 调用/追问/回答组分别归到对应任务的 call_keys/question_keys/answer_group_keys。
// 顺序按任务首次出现固定，保证结果确定、可重放。
func deriveStepCoverage(decision StepDecision) []TaskCoverage {
	index := make(map[string]int, len(decision.TaskUpdates)+1)
	coverage := make([]TaskCoverage, 0, len(decision.TaskUpdates)+1)
	lookup := func(taskKey string) int {
		if position, ok := index[taskKey]; ok {
			return position
		}
		coverage = append(coverage, TaskCoverage{TaskKey: taskKey})
		index[taskKey] = len(coverage) - 1
		return index[taskKey]
	}
	for _, update := range decision.TaskUpdates {
		lookup(update.TaskKey)
	}
	for _, call := range decision.Calls {
		for _, taskKey := range call.TaskKeys {
			position := lookup(taskKey)
			coverage[position].CallKeys = append(coverage[position].CallKeys, call.CallKey)
		}
	}
	for _, question := range decision.Questions {
		for _, taskKey := range question.TaskKeys {
			position := lookup(taskKey)
			coverage[position].QuestionKeys = append(coverage[position].QuestionKeys, question.QuestionKey)
		}
	}
	for _, group := range decision.Groups {
		for _, taskKey := range group.TaskKeys {
			position := lookup(taskKey)
			coverage[position].AnswerGroupKeys = append(coverage[position].AnswerGroupKeys, group.GroupKey)
		}
	}
	return coverage
}

// BuildToolBatch 把 call_tools 的调用记录构造为不可变调用批次（文档 7.4）。
// CallIndex 由记录在完整模型响应中的顺序固定；ToolCallID 用响应内的 call_key
// 作为本次 Attempt 内的稳定调用标识。
func BuildToolBatch(calls []CallRecord, attemptID, runID, batchID string) ToolBatch {
	toolCalls := make([]ToolCall, 0, len(calls))
	for index, call := range calls {
		toolCalls = append(toolCalls, ToolCall{
			ToolCallID:  call.CallKey,
			CallIndex:   index,
			AttemptID:   attemptID,
			ToolName:    call.ToolName,
			ToolVersion: call.CatalogVersion,
			Arguments:   call.Arguments,
			TaskKeys:    append([]string(nil), call.TaskKeys...),
			DependsOn:   append([]string(nil), call.DependsOn...),
		})
	}
	return ToolBatch{BatchID: batchID, RunID: runID, AttemptID: attemptID, Calls: toolCalls}
}

func normalizeCallToolVersions(calls []CallRecord, tools []Tool) {
	versions := make(map[string]string, len(tools))
	ambiguous := make(map[string]struct{})
	for _, tool := range tools {
		if existing, ok := versions[tool.Name]; ok {
			if existing != tool.Version {
				ambiguous[tool.Name] = struct{}{}
			}
			continue
		}
		versions[tool.Name] = tool.Version
	}
	for index := range calls {
		if _, ok := ambiguous[calls[index].ToolName]; ok {
			continue
		}
		if version, ok := versions[calls[index].ToolName]; ok {
			calls[index].CatalogVersion = version
		}
	}
}

// BuildQuestions 把 request_input 的问题记录构造为待校验的追问（文档 2.5）。
// 问题标识、消息、输入版本由服务端在提交时分配（此处不分配）。
func BuildQuestions(records []QuestionRecord) []Question {
	questions := make([]Question, 0, len(records))
	for _, q := range records {
		items := make([]MissingItem, 0, len(q.MissingFields))
		for _, f := range q.MissingFields {
			items = append(items, MissingItem{
				TaskItemID: f.TaskKey,
				SubjectKey: f.SubjectKey,
				Field:      f.Field,
				Necessity:  necessityFromString(f.Necessity),
				KnownValue: f.KnownValue,
				Status:     MissingUnanswered,
			})
		}
		questions = append(questions, Question{
			Text:         q.Text,
			MissingItems: items,
		})
	}
	return questions
}

// necessityFromString 把协议中的必要性字符串转换为 Necessity 枚举。
// 未知值按 blocking 处理（保守：视为阻塞项，不静默降级为辅助）。
func necessityFromString(s string) Necessity {
	if s == string(NecessitySupport) {
		return NecessitySupport
	}
	return NecessityBlocking
}

// ValidateFinalAnswer 校验 final_answer 的回答组并做覆盖检查（文档 8.5、2.3.2）。
// 每个回答组必须通过分类型字段校验；Run 完成前必须覆盖所有有效任务项。
func ValidateFinalAnswer(decision StepDecision, taskItems []TaskItem) error {
	if decision.Action != ActionFinalAnswer {
		return fmt.Errorf("decision_loop: action %q is not final_answer", decision.Action)
	}
	if len(decision.Groups) == 0 {
		return fmt.Errorf("decision_loop: final_answer has no answer groups")
	}
	for _, g := range decision.Groups {
		if err := ValidateAnswerGroup(g); err != nil {
			return err
		}
	}
	// 覆盖检查：模型提出的 coverage 不能写入成功状态，Runtime 结合真实结果裁决。
	if err := CheckRunClosure(taskItems); err != nil {
		return err
	}
	return nil
}

// MapTaskUpdates 把 header 的任务项更新映射为带输入依据的任务项（文档 2.3.1）。
// TaskKey 仅在本次响应内唯一，服务端校验后分配稳定 task_item_id。
// 此函数把响应内临时键映射为待分配稳定 ID 的任务项（任务项标识由调用方分配）。
func MapTaskUpdates(updates []TaskUpdate, originTurnID, runID string) ([]TaskItem, error) {
	items := make([]TaskItem, 0, len(updates))
	for _, u := range updates {
		subjects := make([]AnswerSubject, 0, len(u.SubjectKeys))
		for _, key := range u.SubjectKeys {
			subjects = append(subjects, AnswerSubject{SubjectKey: key, Kind: SubjectUnresolved, Description: key})
		}
		items = append(items, TaskItem{
			TaskItemID:    u.TaskKey,
			OriginTurnID:  originTurnID,
			RunID:         runID,
			ItemRevision:  1,
			Goal:          u.Goal,
			SourceTurnIDs: u.SourceTurnIDs,
			Subjects:      subjects,
			Outcome:       OutcomePending,
		})
	}
	return items, nil
}

// ResolveCoverage 把模型提出的 coverage 与真实任务项结果合并（文档 8.4、2.3.2）。
// 模型只能提出覆盖关系，不能凭此写入成功状态；真正的结果分类由 Runtime 结合
// 工具结果、Operation 状态、回答块持久化事实裁决。此函数把 coverage 中
// 已声明 incomplete_reason 的任务项标记为未完成，其余保持现状。
func ResolveCoverage(items []TaskItem, coverage []TaskCoverage) []TaskItem {
	reasonByKey := make(map[string]string, len(coverage))
	for _, cov := range coverage {
		if cov.IncompleteReason != "" {
			reasonByKey[cov.TaskKey] = cov.IncompleteReason
		}
	}
	out := make([]TaskItem, 0, len(items))
	for _, it := range items {
		if reason, ok := reasonByKey[it.TaskItemID]; ok {
			it.Outcome = OutcomeIncomplete
			it.IncompleteReason = reason
		}
		out = append(out, it)
	}
	return out
}

// ResolveFinalCoverage 在 final_answer 时把模型提出的 coverage 与真实回答组/Operation
// 裁决为任务项终态快照（文档 2.3.2、8.4）。
// 模型 coverage 只能提出覆盖关系，不能凭此写入成功状态：answer_group_keys 必须指向
// final_answer 中真实存在的回答组才可判为 answered；operation_ids 判为 preview_provided；
// incomplete_reason 判为 incomplete。三者均无的项保持原状态（pending 等），
// 交由后续 CheckRunClosure 拒绝完成，从而杜绝「留无说明的待处理目标后完成 Run」。
func ResolveFinalCoverage(items []TaskItem, coverage []TaskCoverage, groups []AnswerGroup) []TaskItem {
	groupSet := make(map[string]bool, len(groups))
	for _, g := range groups {
		groupSet[g.GroupKey] = true
	}
	covByKey := make(map[string]TaskCoverage, len(coverage))
	for _, c := range coverage {
		covByKey[c.TaskKey] = c
	}
	out := make([]TaskItem, 0, len(items))
	for _, it := range items {
		cov, ok := covByKey[it.TaskItemID]
		if !ok {
			// coverage 未提及：保持原状态，CheckRunClosure 会因非终态而拒绝。
			out = append(out, it)
			continue
		}
		if cov.IncompleteReason != "" {
			it.Outcome = OutcomeIncomplete
			it.IncompleteReason = cov.IncompleteReason
			out = append(out, it)
			continue
		}
		if gk := firstCoveredGroup(cov.AnswerGroupKeys, groupSet); gk != "" {
			it.Outcome = OutcomeAnswered
			it.ResultRef = &TaskResultRef{Kind: ResultRefAnswerGroup, RefID: gk}
			out = append(out, it)
			continue
		}
		if len(cov.OperationIDs) > 0 {
			it.Outcome = OutcomePreviewProvided
			it.ResultRef = &TaskResultRef{Kind: ResultRefOperation, RefID: cov.OperationIDs[0]}
			out = append(out, it)
			continue
		}
		// coverage 提及但无有效关联（如只有 call_keys）：保持原状态。
		out = append(out, it)
	}
	return out
}

// firstCoveredGroup 返回 coverage 声明的 answer_group_keys 中第一个真实存在于
// final_answer 回答组集合里的组键；不存在则返回空串。
func firstCoveredGroup(keys []string, groupSet map[string]bool) string {
	for _, k := range keys {
		if groupSet[k] {
			return k
		}
	}
	return ""
}

// ResolveQuestionCoverage 在 request_input 时把被追问的任务项标记为「需补信息」
// （OutcomeNeedsInput）并携带缺失字段（文档 2.5、2.3.2）。
// 追问只针对「阻止当前目标安全推进」的必要资料；question 的 task_keys 与 missing_fields
// 的 task_key 共同指向需要补信息的任务项。已处于终态的任务项不回退（防止用追问
// 把已答完/已完成项悄悄改回等待，规避覆盖校验）。
func ResolveQuestionCoverage(items []TaskItem, questions []QuestionRecord) []TaskItem {
	touched := make(map[string]bool)
	missingByTask := make(map[string][]MissingField)
	for _, q := range questions {
		for _, tk := range q.TaskKeys {
			touched[tk] = true
		}
		for _, f := range q.MissingFields {
			missingByTask[f.TaskKey] = append(missingByTask[f.TaskKey], f)
			touched[f.TaskKey] = true
		}
	}
	out := make([]TaskItem, 0, len(items))
	for _, it := range items {
		if !touched[it.TaskItemID] {
			out = append(out, it)
			continue
		}
		// 终态项不回退；非终态项转为 needs_input 并合并缺失字段。
		if !it.Outcome.Terminal() {
			it.Outcome = OutcomeNeedsInput
		}
		if fields := missingByTask[it.TaskItemID]; len(fields) > 0 {
			it.MissingFields = append(it.MissingFields, fields...)
		}
		out = append(out, it)
	}
	return out
}

// agentStepPrompt 是 agent_step 决策调用前的系统级约束占位（文档 9.1）。
// 系统提示词顺序固定为不可变安全与权限规则、已版本化业务 Skill、动作及输出契约、
// 当前任务约束；用户消息、图片文字、历史、工具结果及摘要始终是有来源的数据。
// 具体提示词文本由 T5 固定，此处仅保留顺序约束常量，供 Provider 组装使用。
const agentStepPromptOrder = "security_rules -> skills -> output_contract -> task_constraints"

// toolArgsJSON 校验工具参数是否可解析为 JSON 对象（文档 7.4 整批校验的一环）。
func toolArgsJSON(args json.RawMessage) error {
	if len(args) == 0 || !json.Valid(args) {
		return fmt.Errorf("tool arguments invalid")
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(args, &obj); err != nil {
		return fmt.Errorf("tool arguments must be a JSON object: %w", err)
	}
	return nil
}
