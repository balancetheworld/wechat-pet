package ask

import (
	"context"
	"testing"
)

// scriptedModel 按脚本依次返回 record_array_v1 记录。
type scriptedModel struct {
	responses [][]ProtocolRecord
	inputs    []StepInput
}

func (m *scriptedModel) Step(_ context.Context, input StepInput) ([]ProtocolRecord, error) {
	m.inputs = append(m.inputs, input)
	if len(m.responses) == 0 {
		return nil, context.DeadlineExceeded
	}
	r := m.responses[0]
	m.responses = m.responses[1:]
	return r, nil
}

// fakeToolExecutor 直接返回空结果，记录收到的批次。
type fakeToolExecutor struct {
	batches []ToolBatch
}

func (e *fakeToolExecutor) ExecuteBatch(_ context.Context, batch ToolBatch) ([]ToolResult, error) {
	e.batches = append(e.batches, batch)
	results := make([]ToolResult, 0, len(batch.Calls))
	for _, c := range batch.Calls {
		results = append(results, ToolResult{
			ToolCallID:  c.ToolCallID,
			Status:      ToolResultOK,
			Completeness: CompletenessComplete,
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
		Text: "请问是哪只宠物？",
		MissingFields: []MissingField{{TaskKey: "t1", Field: "pet_id", Necessity: "blocking"}},
	}}
	return []ProtocolRecord{headerRecord(ActionRequestInput), q, coverageRecord(), endRecord()}
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

func TestRunDecisionLoopToolThenAnswer(t *testing.T) {
	model := &scriptedModel{responses: [][]ProtocolRecord{callToolsRecords(), finalAnswerRecords()}}
	tools := &fakeToolExecutor{}
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

func TestMergeTaskItems(t *testing.T) {
	existing := []TaskItem{{TaskItemID: "t1", ItemRevision: 1, Goal: "旧目标", Outcome: OutcomePending}}
	mapped := []TaskItem{{TaskItemID: "t1", ItemRevision: 2, Goal: "新目标", Outcome: OutcomePending}, {TaskItemID: "t2", ItemRevision: 1, Goal: "新项", Outcome: OutcomePending}}
	merged := mergeTaskItems(existing, mapped)
	if len(merged) != 2 {
		t.Fatalf("merged = %d, want 2", len(merged))
	}
	for _, it := range merged {
		if it.TaskItemID == "t1" && it.Goal != "新目标" {
			t.Fatalf("t1 should be replaced, got %+v", it)
		}
	}
}

func TestToolResultText(t *testing.T) {
	ok := ToolResult{ToolCallID: "c1", Status: ToolResultOK, Completeness: CompletenessComplete}
	if text := toolResultText(ok); text == "" {
		t.Fatal("empty tool result text")
	}
	failed := ToolResult{ToolCallID: "c1", Status: ToolResultError, Error: &ToolError{Category: ToolErrServiceError, Reason: "超时"}}
	if text := toolResultText(failed); text == "" {
		t.Fatal("empty error text")
	}
}
