package ask

import (
	"context"
	"errors"
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

func (m *failingModel) Step(_ context.Context, _ StepInput) (ModelStepResult, error) {
	if m.err != nil {
		err := m.err
		m.err = nil
		return ModelStepResult{}, err
	}
	return ModelStepResult{Records: m.records, Usage: ModelUsage{InputTokens: 10, OutputTokens: 5, TotalTokens: 15, Complete: true}}, nil
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
		if block.ObjectID == "validation_feedback" && strings.Contains(block.Text, "greeting") {
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
	outcome, err := runDecisionLoop(context.Background(), model, &fakeToolExecutor{resultData: PetListOutcome{Pets: []PetListItem{{PetID: "pet-1", Name: "团子"}}}}, StepInput{RunID: "run-pet-list"}, 3, nil, nil, "", repository, nil)
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
		if block.ObjectID == "validation_feedback" && strings.Contains(block.Text, "task_pet_list") {
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
	model := &scriptedModel{responses: [][]ProtocolRecord{callToolsRecords()}}
	_, err := RunDecisionLoop(context.Background(), model, validationToolExecutor{}, StepInput{}, 0)
	var validationErr *BatchValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %v, want batch validation error", err)
	}
	if !strings.Contains(err.Error(), "c1=read_health_records@v1") {
		t.Fatalf("error missing tool identity: %v", err)
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
	_, err := runDecisionLoop(context.Background(), blockingModel{}, &fakeToolExecutor{}, StepInput{RunID: "run-duration"}, 1, nil, nil, "", repository, nil)
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
	_, err := runDecisionLoop(context.Background(), model, &fakeToolExecutor{}, StepInput{RunID: "run-unknown-usage"}, 1, nil, nil, "", repository, nil)
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
	outcome, retryErr := runDecisionLoop(context.Background(), model, &fakeToolExecutor{}, StepInput{RunID: "run-unknown-usage"}, 1, nil, nil, "", repository, nil)
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
	if _, err := runDecisionLoop(context.Background(), model, &fakeToolExecutor{}, StepInput{RunID: "run-usage"}, 1, nil, nil, "", repository, nil); err != nil {
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
