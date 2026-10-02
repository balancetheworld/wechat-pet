package ask

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// scriptedModel 按脚本依次返回 record_array_v1 记录。
type scriptedModel struct {
	responses [][]ProtocolRecord
	inputs    []StepInput
}

type failingModel struct {
	err     error
	records []ProtocolRecord
	inputs  []StepInput
}

// sequencedModel 按脚本依次返回失败或记录，用于覆盖修复重试的完整顺序。
type sequencedModel struct {
	steps  []scriptedStep
	inputs []StepInput
}

type scriptedStep struct {
	records []ProtocolRecord
	err     error
}

type blockingModel struct{}

type validationToolExecutor struct{}

func (validationToolExecutor) ExecuteBatch(_ context.Context, _ ToolBatch) ([]ToolResult, error) {
	return nil, &BatchValidationError{CallErrors: []CallValidationError{{CallIndex: 0, ToolCallID: "c1", Reason: "tool not found or version mismatch"}}}
}

func (blockingModel) Step(ctx context.Context, _ StepInput) (ModelStepResult, error) {
	<-ctx.Done()
	return ModelStepResult{}, ctx.Err()
}

func (m *scriptedModel) Step(_ context.Context, input StepInput) (ModelStepResult, error) {
	m.inputs = append(m.inputs, input)
	if len(m.responses) == 0 {
		return ModelStepResult{}, context.DeadlineExceeded
	}
	r := m.responses[0]
	m.responses = m.responses[1:]
	return ModelStepResult{Records: r, Usage: ModelUsage{InputTokens: 10, OutputTokens: 5, TotalTokens: 15, Complete: true}}, nil
}

func (m *failingModel) Step(_ context.Context, input StepInput) (ModelStepResult, error) {
	m.inputs = append(m.inputs, input)
	if m.err != nil {
		err := m.err
		m.err = nil
		return ModelStepResult{}, err
	}
	return ModelStepResult{Records: m.records, Usage: ModelUsage{InputTokens: 10, OutputTokens: 5, TotalTokens: 15, Complete: true}}, nil
}

func (m *sequencedModel) Step(_ context.Context, input StepInput) (ModelStepResult, error) {
	m.inputs = append(m.inputs, input)
	if len(m.steps) == 0 {
		return ModelStepResult{}, errors.New("sequencedModel: no scripted step left")
	}
	step := m.steps[0]
	m.steps = m.steps[1:]
	if step.err != nil {
		return ModelStepResult{}, step.err
	}
	return ModelStepResult{Records: step.records, Usage: ModelUsage{InputTokens: 10, OutputTokens: 5, TotalTokens: 15, Complete: true}}, nil
}

// fakeToolExecutor 直接返回空结果，记录收到的批次。
type fakeToolExecutor struct {
	batches    []ToolBatch
	resultData any
}

func (e *fakeToolExecutor) ExecuteBatch(_ context.Context, batch ToolBatch) ([]ToolResult, error) {
	e.batches = append(e.batches, batch)
	results := make([]ToolResult, 0, len(batch.Calls))
	for _, c := range batch.Calls {
		results = append(results, ToolResult{
			ToolCallID:   c.ToolCallID,
			Status:       ToolResultOK,
			Completeness: CompletenessComplete,
			Data:         e.resultData,
		})
	}
	return results, nil
}

func finalAnswerRecords() []ProtocolRecord {
	group := ProtocolRecord{Type: RecordGroup, Group: &GroupRecord{
		Type: RecordGroup, GroupKey: "g1", TaskKeys: []string{"t1"},
		AnswerKind: AnswerCasual, Scope: ScopeFull,
		Subjects: []AnswerSubject{{SubjectKey: "s1", Kind: SubjectUnresolved, Description: "某只宠物"}},
	}}
	segment := ProtocolRecord{Type: RecordSegment, Segment: &SegmentRecord{
		Type: RecordSegment, SegmentKey: "seg1", GroupKey: "g1", SubjectKeys: []string{"s1"},
		Field: "reply", Text: "你好", BasisKind: BasisGeneralKnowledge,
	}}
	header := headerRecord(ActionFinalAnswer)
	coverage := ProtocolRecord{Type: RecordCoverage, Coverage: &CoverageRecord{
		Type:  RecordCoverage,
		Tasks: []TaskCoverage{{TaskKey: "t1", AnswerGroupKeys: []string{"g1"}}},
	}}
	return []ProtocolRecord{header, group, segment, coverage, endRecord()}
}

func callToolsRecords() []ProtocolRecord {
	return []ProtocolRecord{headerRecord(ActionCallTools), callRecord(), coverageRecord(), endRecord()}
}

func requestInputRecords() []ProtocolRecord {
	q := ProtocolRecord{Type: RecordQuestion, Question: &QuestionRecord{
		Type: RecordQuestion, QuestionKey: "q1", TaskKeys: []string{"t1"},
		Text:          "请问是哪只宠物？",
		MissingFields: []MissingField{{TaskKey: "t1", Field: "pet_id", Necessity: "blocking"}},
	}}
	coverage := ProtocolRecord{Type: RecordCoverage, Coverage: &CoverageRecord{
		Type:  RecordCoverage,
		Tasks: []TaskCoverage{{TaskKey: "t1", QuestionKeys: []string{"q1"}}},
	}}
	return []ProtocolRecord{headerRecord(ActionRequestInput), q, coverage, endRecord()}
}

func TestRunDecisionLoopSingleFinalAnswer(t *testing.T) {
	model := &scriptedModel{responses: [][]ProtocolRecord{finalAnswerRecords()}}
	tools := &fakeToolExecutor{}
	outcome, err := RunDecisionLoop(context.Background(), model, tools, StepInput{}, 0)
	if err != nil {
		t.Fatalf("loop failed: %v", err)
	}
	if outcome.Action != ActionFinalAnswer || len(outcome.Groups) != 1 {
		t.Fatalf("unexpected outcome: %+v", outcome)
	}
	if outcome.Steps != 1 {
		t.Fatalf("steps = %d, want 1", outcome.Steps)
	}
}

func TestRunDecisionLoopRetriesInvalidAnswerFields(t *testing.T) {
	invalid := finalAnswerRecords()
	invalid[2].Segment.Field = "greeting"
	model := &scriptedModel{responses: [][]ProtocolRecord{invalid, finalAnswerRecords()}}
	outcome, err := RunDecisionLoop(context.Background(), model, &fakeToolExecutor{}, StepInput{}, 3)
	if err != nil || outcome.Action != ActionFinalAnswer || outcome.Steps != 2 {
		t.Fatalf("outcome = %+v, error = %v", outcome, err)
	}
	foundFeedback := false
	for _, block := range model.inputs[1].Blocks {
		if block.ObjectID != "validation_feedback" {
			continue
		}
		if strings.Contains(block.Text, "not allowed for answer_kind") && !strings.Contains(block.Text, "greeting") {
			foundFeedback = true
		}
	}
	if !foundFeedback {
		t.Fatalf("retry input has no validation feedback: %+v", model.inputs[1].Blocks)
	}
}

func TestRunDecisionLoopStopsAfterOneInvalidResponseRetry(t *testing.T) {
	invalid := finalAnswerRecords()
	invalid[2].Segment.Field = "greeting"
	model := &scriptedModel{responses: [][]ProtocolRecord{invalid, invalid, finalAnswerRecords()}}
	_, err := RunDecisionLoop(context.Background(), model, &fakeToolExecutor{}, StepInput{}, 3)
	if err == nil || !strings.Contains(err.Error(), "not allowed") || len(model.inputs) != 2 {
		t.Fatalf("error = %v, calls = %d", err, len(model.inputs))
	}
}

func TestRunDecisionLoopRepairsUnparsableOutput(t *testing.T) {
	parseErr := NewExecutorError(ErrAgentOutputUnparsable, false, 0, fmt.Errorf("agent_model: record_array_v1: response is neither array nor object, got 'h' (len=42, head=%q, tail=%q)", "hello", "world"))
	model := &failingModel{err: parseErr, records: finalAnswerRecords()}
	outcome, err := RunDecisionLoop(context.Background(), model, &fakeToolExecutor{}, StepInput{}, 3)
	if err != nil || outcome.Action != ActionFinalAnswer || len(model.inputs) != 2 {
		t.Fatalf("outcome = %+v, error = %v, calls = %d", outcome, err, len(model.inputs))
	}
	feedback := ""
	for _, block := range model.inputs[1].Blocks {
		if block.ObjectID == "validation_feedback" {
			feedback = block.Text
		}
	}
	if !strings.Contains(feedback, "neither array nor object") {
		t.Fatalf("repair feedback missing reason: %q", feedback)
	}
	if strings.Contains(feedback, "hello") || strings.Contains(feedback, "tail=") {
		t.Fatalf("repair feedback leaks model text: %q", feedback)
	}
}

func TestRunDecisionLoopStopsAfterOneUnparsableRetry(t *testing.T) {
	parseErr := func() error {
		return NewExecutorError(ErrAgentOutputUnparsable, false, 0, fmt.Errorf("agent_model: record_array_v1: empty response (len=0, head=%q, tail=%q)", "", ""))
	}
	model := &sequencedModel{steps: []scriptedStep{{err: parseErr()}, {err: parseErr()}}}
	_, err := RunDecisionLoop(context.Background(), model, &fakeToolExecutor{}, StepInput{}, 3)
	if err == nil || !strings.Contains(err.Error(), "empty response") || len(model.inputs) != 2 {
		t.Fatalf("error = %v, calls = %d", err, len(model.inputs))
	}
}

func TestRunDecisionLoopRepairsUnparsableOutputAfterValidationFailure(t *testing.T) {
	invalid := finalAnswerRecords()
	invalid[2].Segment.Field = "greeting"
	model := &sequencedModel{steps: []scriptedStep{
		{records: invalid},
		{err: NewExecutorError(ErrAgentOutputUnparsable, false, 0, fmt.Errorf("agent_model: record_array_v1: response is neither array nor object, got '`' (len=12, head=%q, tail=%q)", "```json", "```"))},
		{records: finalAnswerRecords()},
	}}
	outcome, err := RunDecisionLoop(context.Background(), model, &fakeToolExecutor{}, StepInput{}, 4)
	if err != nil || outcome.Action != ActionFinalAnswer || len(model.inputs) != 3 {
		t.Fatalf("outcome = %+v, error = %v, calls = %d", outcome, err, len(model.inputs))
	}
	feedback := ""
	for _, block := range model.inputs[2].Blocks {
		if block.ObjectID == "validation_feedback" {
			feedback = block.Text
		}
	}
	if !strings.Contains(feedback, "neither array nor object") {
		t.Fatalf("second repair feedback missing reason: %q", feedback)
	}
	if strings.Contains(feedback, "```") {
		t.Fatalf("second repair feedback leaks model text: %q", feedback)
	}
}

func TestRunDecisionLoopToolThenAnswer(t *testing.T) {
	model := &scriptedModel{responses: [][]ProtocolRecord{callToolsRecords(), finalAnswerRecords()}}
	tools := &fakeToolExecutor{resultData: HealthRecordSearchOutcome{
		Records: []HealthRecordItem{{MedicalType: "vaccine", Content: "已完成疫苗接种"}},
		Source:  ReadSource{SourceType: "health_record", SourceID: "pet-1", Version: "v1"},
	}}
	outcome, err := RunDecisionLoop(context.Background(), model, tools, StepInput{}, 0)
	if err != nil {
		t.Fatalf("loop failed: %v", err)
	}
	if outcome.Action != ActionFinalAnswer {
		t.Fatalf("action = %s, want final_answer", outcome.Action)
	}
	if len(tools.batches) != 1 {
		t.Fatalf("expected 1 tool batch, got %d", len(tools.batches))
	}
	if len(outcome.ToolResults) != 1 {
		t.Fatalf("expected 1 tool result, got %d", len(outcome.ToolResults))
	}
	// 工具结果应回灌到第二轮决策的上下文。
	if len(model.inputs) != 2 {
		t.Fatalf("expected 2 model calls, got %d", len(model.inputs))
	}
	hasToolResult := false
	for _, b := range model.inputs[1].Blocks {
		if b.Kind == "tool_result" {
			hasToolResult = true
		}
	}
	if !hasToolResult {
		t.Fatal("tool result should be fed back into second step context")
	}
	toolText := ""
	for _, b := range model.inputs[1].Blocks {
		if b.Kind == "tool_result" {
			toolText = b.Text
		}
	}
	for _, expected := range []string{"vaccine", "已完成疫苗接种", "health_record", "health_record_search_outcome_v1"} {
		if !strings.Contains(toolText, expected) {
			t.Fatalf("second model input missing %q: %s", expected, toolText)
		}
	}
}

func TestRunDecisionLoopRetainsTaskKeyAndRetriesIncompleteCoverage(t *testing.T) {
	call := callRecordsWithoutCoverage("list_family_pets", `{}`)
	call[0].Header.TaskUpdates[0].TaskKey = "task_pet_list"
	call[1].Call.TaskKeys = []string{"task_pet_list"}
	answer := finalAnswerRecords()
	answer[0].Header.TaskUpdates = nil
	answer[1].Group.TaskKeys = []string{"task_pet_list"}
	answer[3].Coverage.Tasks = []TaskCoverage{{TaskKey: "task_pet_list", AnswerGroupKeys: []string{"g1"}}}
	incomplete := finalAnswerRecords()
	incomplete[0].Header.TaskUpdates = nil
	incomplete[1].Group.TaskKeys = []string{"task_pet_list"}
	incomplete[3].Coverage.Tasks = []TaskCoverage{{TaskKey: "task_pet_list", CallKeys: []string{"c1"}}}

	repository := newBudgetTestRepository(t)
	if err := repository.EnsureBudget(context.Background(), BudgetRun, "run-pet-list", BudgetLimits{MaxModelCalls: 3, MaxToolCalls: 1, MaxTokens: 12000, MaxDurationMillis: 10000, MaxConcurrency: 1}); err != nil {
		t.Fatal(err)
	}
	model := &scriptedModel{responses: [][]ProtocolRecord{call, incomplete, answer}}
	outcome, err := runDecisionLoop(context.Background(), model, &fakeToolExecutor{resultData: PetListOutcome{Pets: []PetListItem{{PetID: "pet-1", Name: "团子"}}}}, StepInput{RunID: "run-pet-list"}, 3, nil, nil, "", repository, nil, nil)
	if err != nil || outcome.Action != ActionFinalAnswer || len(model.inputs) != 3 {
		t.Fatalf("outcome = %+v, error = %v, calls = %d", outcome, err, len(model.inputs))
	}
	for _, index := range []int{1, 2} {
		found := false
		for _, block := range model.inputs[index].Blocks {
			if block.Kind == "task" && strings.Contains(block.Text, `"task_key":"task_pet_list"`) && strings.Contains(block.Text, `"outcome":"pending"`) {
				found = true
			}
		}
		if !found {
			t.Fatalf("step %d missing pending task: %+v", index+1, model.inputs[index].Blocks)
		}
	}
	feedback := false
	for _, block := range model.inputs[2].Blocks {
		if block.ObjectID != "validation_feedback" {
			continue
		}
		if strings.Contains(block.Text, "unresolved task") && !strings.Contains(block.Text, "task_pet_list") {
			feedback = true
		}
	}
	if !feedback {
		t.Fatalf("budgeted retry missing closure feedback: %+v", model.inputs[2].Blocks)
	}
	if len(outcome.TaskItems) != 1 || outcome.TaskItems[0].Outcome != OutcomeAnswered {
		t.Fatalf("task items = %+v", outcome.TaskItems)
	}
}

func TestRunDecisionLoopDoesNotAcceptRepeatedIncompleteCoverage(t *testing.T) {
	call := callRecordsWithoutCoverage("list_family_pets", `{}`)
	incomplete := finalAnswerRecords()
	incomplete[0].Header.TaskUpdates = nil
	incomplete[3].Coverage.Tasks = []TaskCoverage{{TaskKey: "t1", CallKeys: []string{"c1"}}}
	model := &scriptedModel{responses: [][]ProtocolRecord{call, incomplete, incomplete}}
	_, err := RunDecisionLoop(context.Background(), model, &fakeToolExecutor{}, StepInput{RunID: "run-list"}, 4)
	if err == nil || !strings.Contains(err.Error(), "unresolved task") || len(model.inputs) != 3 {
		t.Fatalf("error = %v, calls = %d", err, len(model.inputs))
	}
}

func TestRunDecisionLoopReportsInvalidToolIdentity(t *testing.T) {
	model := &scriptedModel{responses: [][]ProtocolRecord{callToolsRecords(), callToolsRecords()}}
	_, err := RunDecisionLoop(context.Background(), model, validationToolExecutor{}, StepInput{}, 0)
	var validationErr *BatchValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %v, want batch validation error", err)
	}
	if !strings.Contains(err.Error(), "c1=read_health_records@v1") {
		t.Fatalf("error missing tool identity: %v", err)
	}
	if len(model.inputs) != 2 {
		t.Fatalf("calls = %d, want one retry", len(model.inputs))
	}
	feedback := ""
	for _, block := range model.inputs[1].Blocks {
		if block.ObjectID == "validation_feedback" {
			feedback = block.Text
		}
	}
	if !strings.Contains(feedback, "tool not found or version mismatch") {
		t.Fatalf("retry input has no batch validation feedback: %q", feedback)
	}
}

func TestRunDecisionLoopNormalizesCallToolVersion(t *testing.T) {
	records := callToolsRecords()
	records[1].Call.CatalogVersion = "pet-base-v1"
	model := &scriptedModel{responses: [][]ProtocolRecord{records, finalAnswerRecords()}}
	tools := &fakeToolExecutor{}
	outcome, err := RunDecisionLoop(context.Background(), model, tools, StepInput{Tools: []Tool{{Name: "read_health_records", Version: "v1"}}}, 3)
	if err != nil {
		t.Fatalf("loop failed: %v", err)
	}
	if outcome.Action != ActionFinalAnswer {
		t.Fatalf("action = %s, want final_answer", outcome.Action)
	}
	if len(tools.batches) != 1 || len(tools.batches[0].Calls) != 1 {
		t.Fatalf("batches = %+v", tools.batches)
	}
	if version := tools.batches[0].Calls[0].ToolVersion; version != "v1" {
		t.Fatalf("tool version = %q, want v1", version)
	}
}

func TestRunDecisionLoopRequestInput(t *testing.T) {
	model := &scriptedModel{responses: [][]ProtocolRecord{requestInputRecords()}}
	tools := &fakeToolExecutor{}
	outcome, err := RunDecisionLoop(context.Background(), model, tools, StepInput{}, 0)
	if err != nil {
		t.Fatalf("loop failed: %v", err)
	}
	if outcome.Action != ActionRequestInput || len(outcome.Questions) != 1 {
		t.Fatalf("unexpected outcome: %+v", outcome)
	}
}

func TestRunDecisionLoopExceedsSteps(t *testing.T) {
	// 模型始终返回 call_tools，循环应因步骤预算耗尽而报错。
	model := &scriptedModel{}
	for i := 0; i < maxDecisionSteps; i++ {
		model.responses = append(model.responses, callToolsRecords())
	}
	tools := &fakeToolExecutor{}
	if _, err := RunDecisionLoop(context.Background(), model, tools, StepInput{}, 0); err == nil {
		t.Fatal("expected step budget exceeded error")
	}
}

func TestRunDecisionLoopEnforcesDurationBudget(t *testing.T) {
	repository := newBudgetTestRepository(t)
	if err := repository.EnsureBudget(context.Background(), BudgetRun, "run-duration", BudgetLimits{MaxModelCalls: 1, MaxTokens: 10000, MaxDurationMillis: 1, MaxConcurrency: 1}); err != nil {
		t.Fatal(err)
	}
	_, err := runDecisionLoop(context.Background(), blockingModel{}, &fakeToolExecutor{}, StepInput{RunID: "run-duration"}, 1, nil, nil, "", repository, nil, nil)
	code, _ := ExecutorErrorDetails(err)
	if code != "budget_exhausted" {
		t.Fatalf("error = %v, code = %q, want budget_exhausted", err, code)
	}
	ledger, ledgerErr := repository.GetBudget(context.Background(), BudgetRun, "run-duration")
	if ledgerErr != nil {
		t.Fatal(ledgerErr)
	}
	if ledger.Used.DurationMillis < 1 || ledger.Reserved.DurationMillis != 0 {
		t.Fatalf("duration budget = used %d reserved %d", ledger.Used.DurationMillis, ledger.Reserved.DurationMillis)
	}
}

func TestRunDecisionLoopReleasesConcurrencyAfterUnknownUsage(t *testing.T) {
	repository := newBudgetTestRepository(t)
	if err := repository.EnsureBudget(context.Background(), BudgetRun, "run-unknown-usage", BudgetLimits{MaxModelCalls: 3, MaxTokens: 20000, MaxDurationMillis: 10000, MaxConcurrency: 1}); err != nil {
		t.Fatal(err)
	}
	model := &failingModel{err: NewExecutorError("provider_timeout", true, 0, context.DeadlineExceeded)}
	_, err := runDecisionLoop(context.Background(), model, &fakeToolExecutor{}, StepInput{RunID: "run-unknown-usage"}, 1, nil, nil, "", repository, nil, nil)
	code, _ := ExecutorErrorDetails(err)
	if code != "provider_timeout" {
		t.Fatalf("error = %v, code = %q, want provider_timeout", err, code)
	}
	ledger, ledgerErr := repository.GetBudget(context.Background(), BudgetRun, "run-unknown-usage")
	if ledgerErr != nil {
		t.Fatal(ledgerErr)
	}
	if ledger.Reserved != (BudgetAmount{}) {
		t.Fatalf("reserved = %+v, want none after failed call", ledger.Reserved)
	}
	if ledger.Used.ModelCalls != 1 || ledger.Used.Tokens == 0 || ledger.Used.DurationMillis < 1 {
		t.Fatalf("used = %+v, want 1 call with non-zero tokens and duration", ledger.Used)
	}
	model.records = finalAnswerRecords()
	outcome, retryErr := runDecisionLoop(context.Background(), model, &fakeToolExecutor{}, StepInput{RunID: "run-unknown-usage"}, 1, nil, nil, "", repository, nil, nil)
	if retryErr != nil || outcome.Action != ActionFinalAnswer {
		t.Fatalf("retry after failed call: outcome = %+v, error = %v", outcome, retryErr)
	}
}

func TestRunDecisionLoopSettlesReportedModelUsage(t *testing.T) {
	repository := newBudgetTestRepository(t)
	if err := repository.EnsureBudget(context.Background(), BudgetRun, "run-usage", BudgetLimits{MaxModelCalls: 1, MaxTokens: 10000, MaxDurationMillis: 1000, MaxConcurrency: 1}); err != nil {
		t.Fatal(err)
	}
	model := &scriptedModel{responses: [][]ProtocolRecord{finalAnswerRecords()}}
	if _, err := runDecisionLoop(context.Background(), model, &fakeToolExecutor{}, StepInput{RunID: "run-usage"}, 1, nil, nil, "", repository, nil, nil); err != nil {
		t.Fatal(err)
	}
	ledger, err := repository.GetBudget(context.Background(), BudgetRun, "run-usage")
	if err != nil {
		t.Fatal(err)
	}
	if ledger.Used.ModelCalls != 1 || ledger.Used.Tokens != 15 || ledger.Reserved != (BudgetAmount{}) {
		t.Fatalf("budget = used %+v reserved %+v", ledger.Used, ledger.Reserved)
	}
}

func TestMergeTaskItems(t *testing.T) {
	existing := []TaskItem{{TaskItemID: "t1", ItemRevision: 3, Goal: "旧目标", Outcome: OutcomeNeedsInput, SourceTurnIDs: []string{"turn-1"}, MissingFields: []MissingField{{Field: "pet_id"}}}}
	mapped := []TaskItem{{TaskItemID: "t1", ItemRevision: 1, Goal: "新目标", Outcome: OutcomePending, SourceTurnIDs: []string{"turn-2"}}, {TaskItemID: "t2", ItemRevision: 1, Goal: "新项", Outcome: OutcomePending}}
	merged := mergeTaskItems(existing, mapped)
	if len(merged) != 2 {
		t.Fatalf("merged = %d, want 2", len(merged))
	}
	for _, it := range merged {
		if it.TaskItemID == "t1" && (it.Goal != "新目标" || it.Outcome != OutcomeNeedsInput || it.ItemRevision != 3 || strings.Join(it.SourceTurnIDs, ",") != "turn-1,turn-2") {
			t.Fatalf("t1 recovery state should be preserved, got %+v", it)
		}
	}
}

func TestToolResultText(t *testing.T) {
	ok := ToolResult{ToolCallID: "c1", Status: ToolResultOK, Completeness: CompletenessComplete, Data: HealthRecordOutcome{Record: HealthRecordItem{MedicalType: "vaccine"}}}
	if text := toolResultText(ok); text == "" {
		t.Fatal("empty tool result text")
	}
	failed := ToolResult{ToolCallID: "c1", Status: ToolResultError, Error: &ToolError{Category: ToolErrServiceError, Reason: "超时"}}
	if text := toolResultText(failed); text == "" {
		t.Fatal("empty error text")
	}
}

func TestSanitizeValidationFeedbackStripsModelValues(t *testing.T) {
	referenceDecision := StepDecision{Calls: []CallRecord{{CallKey: "call-secret", TaskKeys: []string{"task-secret"}}}}
	referenceErr := validateDecisionReferences(referenceDecision, nil, map[string]struct{}{}, map[string]struct{}{}, map[string]struct{}{}, "run-1")
	if referenceErr == nil {
		t.Fatal("expected reference validation error")
	}
	evidenceDecision := StepDecision{
		TaskUpdates: []TaskUpdate{{TaskKey: "t1", Goal: "目标"}},
		Groups: []AnswerGroup{{
			GroupKey: "g1",
			TaskKeys: []string{"t1"},
			Segments: []SegmentRecord{{SegmentKey: "seg1", EvidenceRefs: []EvidenceRef{{SourceType: "pet_base", SourceID: "pet-secret"}}}},
		}},
	}
	evidenceErr := validateDecisionReferences(evidenceDecision, nil, map[string]struct{}{}, map[string]struct{}{}, map[string]struct{}{}, "run-1")
	if evidenceErr == nil {
		t.Fatal("expected evidence validation error")
	}
	_, protocolErr := ValidateResponse([]ProtocolRecord{{Type: RecordHeader, Header: &HeaderRecord{SchemaVersion: RecordArrayV1, Action: ResponseAction("model-secret")}}})
	if protocolErr == nil {
		t.Fatal("expected protocol validation error")
	}
	cases := []struct {
		err     error
		leaked  string
		keyword string
	}{
		{referenceErr, "call-secret", "references unknown task"},
		{referenceErr, "task-secret", "references unknown task"},
		{evidenceErr, "pet-secret", "references unknown evidence"},
		{protocolErr, "model-secret", "invalid action"},
	}
	for _, tc := range cases {
		text := sanitizeValidationFeedback(tc.err.Error())
		if strings.Contains(text, tc.leaked) {
			t.Fatalf("feedback leaked %q: %s", tc.leaked, text)
		}
		if !strings.Contains(text, tc.keyword) {
			t.Fatalf("feedback lost %q: %s", tc.keyword, text)
		}
	}
}

func TestTaskStateBlockCarriesFullTaskState(t *testing.T) {
	stableKey := stableTaskItemID("run-1", "t1")
	items := []TaskItem{
		{
			TaskItemID:    stableKey,
			ItemRevision:  2,
			Goal:          "查看旺仔的疫苗记录",
			Outcome:       OutcomeNeedsInput,
			Subjects:      []AnswerSubject{{SubjectKey: "s1", Kind: SubjectUnresolved, Description: "哪只宠物"}},
			MissingFields: []MissingField{{TaskKey: stableKey, Field: "pet_id", Necessity: "required"}},
		},
		{
			TaskItemID:   stableTaskItemID("run-1", "t2"),
			ItemRevision: 1,
			Goal:         "写入疫苗记录",
			Outcome:      OutcomePreviewProvided,
			ResultRef:    &TaskResultRef{Kind: ResultRefOperation, RefID: "op-1", Version: "3"},
		},
	}
	block, err := taskStateBlock("run-1", items)
	if err != nil {
		t.Fatalf("taskStateBlock error: %v", err)
	}
	for _, part := range []string{
		`"task_key":"t1"`,
		`"outcome":"needs_input"`,
		`"missing_fields":[{"task_key":"t1","field":"pet_id","necessity":"required"}]`,
		`"description":"哪只宠物"`,
		`"result_ref":{"kind":"operation","ref_id":"op-1","version":"3"}`,
	} {
		if !strings.Contains(block.Text, part) {
			t.Fatalf("task block missing %q: %s", part, block.Text)
		}
	}
	if strings.Contains(block.Text, "run-1:t1") {
		t.Fatalf("task block should expose short task keys: %s", block.Text)
	}
	if items[0].MissingFields[0].TaskKey != stableKey {
		t.Fatalf("task item must not be mutated: %+v", items[0].MissingFields[0])
	}
}

func TestRunDecisionLoopMarksLatestToolResultsFresh(t *testing.T) {
	model := &scriptedModel{responses: [][]ProtocolRecord{callRecordsWithoutCoverage(petRosterToolName, `{}`), finalAnswerRecords()}}
	if _, err := runDecisionLoop(context.Background(), model, &fakeToolExecutor{}, StepInput{SessionID: "session-1", RunID: "run-1"}, 3, nil, nil, "", nil, nil, nil); err != nil {
		t.Fatalf("loop failed: %v", err)
	}
	fresh := 0
	for _, block := range model.inputs[1].Blocks {
		if block.Kind != "tool_result" {
			continue
		}
		if !block.Fresh {
			t.Fatalf("latest tool result should be fresh: %+v", block)
		}
		fresh++
	}
	if fresh != 1 {
		t.Fatalf("fresh tool result blocks = %d, want 1", fresh)
	}
}
