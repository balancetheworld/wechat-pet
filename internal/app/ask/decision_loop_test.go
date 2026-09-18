package ask

import (
	"context"
	"encoding/json"
	"testing"
)

func headerRecord(action ResponseAction) ProtocolRecord {
	return ProtocolRecord{Type: RecordHeader, Header: &HeaderRecord{
		Type:          RecordHeader,
		SchemaVersion: RecordArrayV1,
		Action:        action,
		TaskUpdates:   []TaskUpdate{{TaskKey: "t1", Goal: "查旺仔呕吐记录"}},
	}}
}

func coverageRecord() ProtocolRecord {
	return ProtocolRecord{Type: RecordCoverage, Coverage: &CoverageRecord{
		Type: RecordCoverage,
		Tasks: []TaskCoverage{{TaskKey: "t1", CallKeys: []string{"c1"}}},
	}}
}

func endRecord() ProtocolRecord {
	return ProtocolRecord{Type: RecordEnd}
}

func callRecord() ProtocolRecord {
	return ProtocolRecord{Type: RecordCall, Call: &CallRecord{
		Type:           RecordCall,
		CallKey:        "c1",
		TaskKeys:       []string{"t1"},
		ToolName:       "read_health_records",
		CatalogVersion: "v1",
		Arguments:      json.RawMessage(`{"pet_id":"p1"}`),
	}}
}

func TestDecideStepCallTools(t *testing.T) {
	records := []ProtocolRecord{headerRecord(ActionCallTools), callRecord(), coverageRecord(), endRecord()}
	decision, err := DecideStep(records)
	if err != nil {
		t.Fatalf("DecideStep failed: %v", err)
	}
	if decision.Action != ActionCallTools {
		t.Fatalf("action = %s, want call_tools", decision.Action)
	}
	if len(decision.Calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(decision.Calls))
	}
	if len(decision.TaskUpdates) != 1 {
		t.Fatalf("task_updates = %d, want 1", len(decision.TaskUpdates))
	}
}

func TestDecideStepBuildsToolBatch(t *testing.T) {
	records := []ProtocolRecord{headerRecord(ActionCallTools), callRecord(), coverageRecord(), endRecord()}
	decision, _ := DecideStep(records)
	batch := BuildToolBatch(decision.Calls, "attempt-1", "run-1", "batch-1")
	if len(batch.Calls) != 1 {
		t.Fatalf("batch calls = %d", len(batch.Calls))
	}
	call := batch.Calls[0]
	if call.ToolCallID != "c1" || call.CallIndex != 0 || call.ToolName != "read_health_records" {
		t.Fatalf("unexpected call: %+v", call)
	}
	if call.AttemptID != "attempt-1" || batch.RunID != "run-1" {
		t.Fatalf("unexpected identity: %+v", batch)
	}
}

func TestDecideStepRejectsMixedAction(t *testing.T) {
	// call_tools 动作里混入 group 记录应被拒绝（动作互斥，文档 2.2）。
	group := ProtocolRecord{Type: RecordGroup, Group: &GroupRecord{
		Type: RecordGroup, GroupKey: "g1", TaskKeys: []string{"t1"},
		AnswerKind: AnswerCasual, Scope: ScopeFull,
		Subjects: []AnswerSubject{{SubjectKey: "s1", Kind: SubjectUnresolved, Description: "某只宠物"}},
	}}
	records := []ProtocolRecord{headerRecord(ActionCallTools), callRecord(), group, coverageRecord(), endRecord()}
	if _, err := DecideStep(records); err == nil {
		t.Fatal("expected error for mixed action records")
	}
}

func TestDecideStepFinalAnswerAssemblesGroups(t *testing.T) {
	group := ProtocolRecord{Type: RecordGroup, Group: &GroupRecord{
		Type: RecordGroup, GroupKey: "g1", TaskKeys: []string{"t1"},
		AnswerKind: AnswerCasual, Scope: ScopeFull,
		Subjects: []AnswerSubject{{SubjectKey: "s1", Kind: SubjectUnresolved, Description: "某只宠物"}},
	}}
	segment := ProtocolRecord{Type: RecordSegment, Segment: &SegmentRecord{
		Type: RecordSegment, SegmentKey: "seg1", GroupKey: "g1", SubjectKeys: []string{"s1"},
		Field: "reply", Text: "你好", BasisKind: BasisGeneralKnowledge,
	}}
	records := []ProtocolRecord{headerRecord(ActionFinalAnswer), group, segment, coverageRecord(), endRecord()}
	decision, err := DecideStep(records)
	if err != nil {
		t.Fatalf("DecideStep failed: %v", err)
	}
	if len(decision.Groups) != 1 {
		t.Fatalf("groups = %d, want 1", len(decision.Groups))
	}
}

func TestValidateFinalAnswerRequiresClosure(t *testing.T) {
	group := ProtocolRecord{Type: RecordGroup, Group: &GroupRecord{
		Type: RecordGroup, GroupKey: "g1", TaskKeys: []string{"t1"},
		AnswerKind: AnswerCasual, Scope: ScopeFull,
		Subjects: []AnswerSubject{{SubjectKey: "s1", Kind: SubjectUnresolved, Description: "某只宠物"}},
	}}
	segment := ProtocolRecord{Type: RecordSegment, Segment: &SegmentRecord{
		Type: RecordSegment, SegmentKey: "seg1", GroupKey: "g1", SubjectKeys: []string{"s1"},
		Field: "reply", Text: "你好", BasisKind: BasisGeneralKnowledge,
	}}
	records := []ProtocolRecord{headerRecord(ActionFinalAnswer), group, segment, coverageRecord(), endRecord()}
	decision, _ := DecideStep(records)
	// 任务项仍为 pending，覆盖未闭合，应拒绝完成。
	taskItems := []TaskItem{{TaskItemID: "t1", OriginTurnID: "turn-1", RunID: "run-1", ItemRevision: 1, Goal: "查记录", Outcome: OutcomePending}}
	if err := ValidateFinalAnswer(decision, taskItems); err == nil {
		t.Fatal("expected closure error for unresolved task item")
	}
}

func TestValidateFinalAnswerOK(t *testing.T) {
	group := ProtocolRecord{Type: RecordGroup, Group: &GroupRecord{
		Type: RecordGroup, GroupKey: "g1", TaskKeys: []string{"t1"},
		AnswerKind: AnswerCasual, Scope: ScopeFull,
		Subjects: []AnswerSubject{{SubjectKey: "s1", Kind: SubjectUnresolved, Description: "某只宠物"}},
	}}
	segment := ProtocolRecord{Type: RecordSegment, Segment: &SegmentRecord{
		Type: RecordSegment, SegmentKey: "seg1", GroupKey: "g1", SubjectKeys: []string{"s1"},
		Field: "reply", Text: "你好", BasisKind: BasisGeneralKnowledge,
	}}
	records := []ProtocolRecord{headerRecord(ActionFinalAnswer), group, segment, coverageRecord(), endRecord()}
	decision, _ := DecideStep(records)
	taskItems := []TaskItem{{TaskItemID: "t1", OriginTurnID: "turn-1", RunID: "run-1", ItemRevision: 1, Goal: "查记录", Outcome: OutcomeAnswered,
		ResultRef: &TaskResultRef{Kind: ResultRefAnswerGroup, RefID: "g1", Version: "v1"}}}
	if err := ValidateFinalAnswer(decision, taskItems); err != nil {
		t.Fatalf("expected closure OK, got %v", err)
	}
}

func TestBuildQuestions(t *testing.T) {
	question := ProtocolRecord{Type: RecordQuestion, Question: &QuestionRecord{
		Type: RecordQuestion, QuestionKey: "q1", TaskKeys: []string{"t1"},
		SubjectKeys: []string{"s1"}, Text: "请问是哪只宠物？",
		MissingFields: []MissingField{{TaskKey: "t1", SubjectKey: "s1", Field: "pet_id", Necessity: "blocking"}},
	}}
	records := []ProtocolRecord{headerRecord(ActionRequestInput), question, coverageRecord(), endRecord()}
	decision, err := DecideStep(records)
	if err != nil {
		t.Fatalf("DecideStep failed: %v", err)
	}
	questions := BuildQuestions(decision.Questions)
	if len(questions) != 1 {
		t.Fatalf("questions = %d, want 1", len(questions))
	}
	if questions[0].MissingItems[0].Necessity != NecessityBlocking {
		t.Fatalf("necessity not mapped: %+v", questions[0].MissingItems[0])
	}
}

func TestResolveCoverage(t *testing.T) {
	items := []TaskItem{{TaskItemID: "t1", ItemRevision: 1, Goal: "查记录", Outcome: OutcomePending}}
	coverage := []TaskCoverage{{TaskKey: "t1", IncompleteReason: "资料不足"}}
	out := ResolveCoverage(items, coverage)
	if out[0].Outcome != OutcomeIncomplete || out[0].IncompleteReason != "资料不足" {
		t.Fatalf("coverage not resolved: %+v", out[0])
	}
}

func TestMapTaskUpdates(t *testing.T) {
	updates := []TaskUpdate{{TaskKey: "t1", Goal: "查记录", SourceTurnIDs: []string{"turn-1"}, SubjectKeys: []string{"s1"}}}
	items, err := MapTaskUpdates(updates, "turn-1", "run-1")
	if err != nil {
		t.Fatalf("MapTaskUpdates failed: %v", err)
	}
	if len(items) != 1 || items[0].Goal != "查记录" || items[0].Outcome != OutcomePending {
		t.Fatalf("unexpected items: %+v", items)
	}
}

// ---- ResolveFinalCoverage：final_answer 的覆盖裁决 ----

func finalAnswerGroups() []AnswerGroup {
	return []AnswerGroup{{GroupKey: "g1", TaskKeys: []string{"t1"}, AnswerKind: AnswerCasual, Scope: ScopeFull}}
}

func pendingItem(id string) TaskItem {
	return TaskItem{TaskItemID: id, ItemRevision: 1, Goal: "查记录", Outcome: OutcomePending}
}

func TestResolveFinalCoverageAnswersByGroup(t *testing.T) {
	items := []TaskItem{pendingItem("t1")}
	coverage := []TaskCoverage{{TaskKey: "t1", AnswerGroupKeys: []string{"g1"}}}
	out := ResolveFinalCoverage(items, coverage, finalAnswerGroups())
	if out[0].Outcome != OutcomeAnswered {
		t.Fatalf("outcome = %s, want answered", out[0].Outcome)
	}
	if out[0].ResultRef == nil || out[0].ResultRef.Kind != ResultRefAnswerGroup || out[0].ResultRef.RefID != "g1" {
		t.Fatalf("result ref not set to answer_group g1: %+v", out[0].ResultRef)
	}
}

func TestResolveFinalCoverageIncomplete(t *testing.T) {
	items := []TaskItem{pendingItem("t1")}
	coverage := []TaskCoverage{{TaskKey: "t1", IncompleteReason: "资料不足"}}
	out := ResolveFinalCoverage(items, coverage, finalAnswerGroups())
	if out[0].Outcome != OutcomeIncomplete || out[0].IncompleteReason != "资料不足" {
		t.Fatalf("incomplete not resolved: %+v", out[0])
	}
}

func TestResolveFinalCoveragePreview(t *testing.T) {
	items := []TaskItem{pendingItem("t1")}
	coverage := []TaskCoverage{{TaskKey: "t1", OperationIDs: []string{"op-1"}}}
	out := ResolveFinalCoverage(items, coverage, finalAnswerGroups())
	if out[0].Outcome != OutcomePreviewProvided {
		t.Fatalf("outcome = %s, want preview_provided", out[0].Outcome)
	}
	if out[0].ResultRef == nil || out[0].ResultRef.Kind != ResultRefOperation || out[0].ResultRef.RefID != "op-1" {
		t.Fatalf("result ref not set to operation op-1: %+v", out[0].ResultRef)
	}
}

func TestResolveFinalCoverageUnmentionedStaysPending(t *testing.T) {
	// coverage 未提及的任务项保持原状态（pending），交由 CheckRunClosure 拒绝完成。
	items := []TaskItem{pendingItem("t1"), pendingItem("t2")}
	coverage := []TaskCoverage{{TaskKey: "t1", AnswerGroupKeys: []string{"g1"}}}
	out := ResolveFinalCoverage(items, coverage, finalAnswerGroups())
	if out[0].Outcome != OutcomeAnswered {
		t.Fatalf("t1 should be answered, got %s", out[0].Outcome)
	}
	if out[1].Outcome != OutcomePending {
		t.Fatalf("t2 should stay pending, got %s", out[1].Outcome)
	}
}

func TestResolveFinalCoverageUnknownGroupStaysPending(t *testing.T) {
	// answer_group_keys 指向不存在的组（模型编造组键）不能判为 answered。
	items := []TaskItem{pendingItem("t1")}
	coverage := []TaskCoverage{{TaskKey: "t1", AnswerGroupKeys: []string{"ghost"}}}
	out := ResolveFinalCoverage(items, coverage, finalAnswerGroups())
	if out[0].Outcome != OutcomePending {
		t.Fatalf("outcome = %s, want pending (unknown group)", out[0].Outcome)
	}
}

func TestResolveFinalCoverageCallKeysOnlyStaysPending(t *testing.T) {
	// 只有 call_keys、无 answer_group/operation/incomplete 时，不能判为已覆盖。
	items := []TaskItem{pendingItem("t1")}
	coverage := []TaskCoverage{{TaskKey: "t1", CallKeys: []string{"c1"}}}
	out := ResolveFinalCoverage(items, coverage, finalAnswerGroups())
	if out[0].Outcome != OutcomePending {
		t.Fatalf("outcome = %s, want pending (call_keys only)", out[0].Outcome)
	}
}

func TestRunDecisionLoopRejectsUncoveredFinalAnswer(t *testing.T) {
	// final_answer 时 coverage 未覆盖某任务项，决策循环应报错而非静默完成（文档 2.3.2）。
	group := ProtocolRecord{Type: RecordGroup, Group: &GroupRecord{
		Type: RecordGroup, GroupKey: "g1", TaskKeys: []string{"t1"},
		AnswerKind: AnswerCasual, Scope: ScopeFull,
		Subjects: []AnswerSubject{{SubjectKey: "s1", Kind: SubjectUnresolved, Description: "某只宠物"}},
	}}
	segment := ProtocolRecord{Type: RecordSegment, Segment: &SegmentRecord{
		Type: RecordSegment, SegmentKey: "seg1", GroupKey: "g1", SubjectKeys: []string{"s1"},
		Field: "reply", Text: "你好", BasisKind: BasisGeneralKnowledge,
	}}
	// header 声明两个任务项 t1、t2，coverage 只覆盖 t1，t2 仍是 pending。
	header := ProtocolRecord{Type: RecordHeader, Header: &HeaderRecord{
		Type: RecordHeader, SchemaVersion: RecordArrayV1, Action: ActionFinalAnswer,
		TaskUpdates: []TaskUpdate{{TaskKey: "t1", Goal: "查记录"}, {TaskKey: "t2", Goal: "查另一只"}},
	}}
	coverage := ProtocolRecord{Type: RecordCoverage, Coverage: &CoverageRecord{
		Type:  RecordCoverage,
		Tasks: []TaskCoverage{{TaskKey: "t1", AnswerGroupKeys: []string{"g1"}}},
	}}
	records := []ProtocolRecord{header, group, segment, coverage, endRecord()}

	model := &scriptedModel{responses: [][]ProtocolRecord{records}}
	tools := &fakeToolExecutor{}
	if _, err := RunDecisionLoop(context.Background(), model, tools, StepInput{}, 0); err == nil {
		t.Fatal("expected closure error for uncovered task t2")
	}
}

// ---- ResolveQuestionCoverage：request_input 的覆盖裁决 ----

func TestResolveQuestionCoverageMarksNeedsInput(t *testing.T) {
	items := []TaskItem{pendingItem("t1")}
	questions := []QuestionRecord{{
		Type:     RecordQuestion,
		TaskKeys: []string{"t1"},
		MissingFields: []MissingField{
			{TaskKey: "t1", Field: "pet_id", Necessity: string(NecessityBlocking)},
		},
	}}
	out := ResolveQuestionCoverage(items, questions)
	if out[0].Outcome != OutcomeNeedsInput {
		t.Fatalf("outcome = %s, want needs_input", out[0].Outcome)
	}
	if len(out[0].MissingFields) != 1 || out[0].MissingFields[0].Field != "pet_id" {
		t.Fatalf("missing fields not carried: %+v", out[0].MissingFields)
	}
}

func TestResolveQuestionCoverageUntouchedStays(t *testing.T) {
	// 未被追问的任务项保持原状态（如已可独立回答的项不应被强制标记）。
	items := []TaskItem{pendingItem("t1"), pendingItem("t2")}
	questions := []QuestionRecord{{Type: RecordQuestion, TaskKeys: []string{"t1"}, MissingFields: []MissingField{{TaskKey: "t1", Field: "pet_id", Necessity: string(NecessityBlocking)}}}}
	out := ResolveQuestionCoverage(items, questions)
	if out[0].Outcome != OutcomeNeedsInput {
		t.Fatalf("t1 should be needs_input, got %s", out[0].Outcome)
	}
	if out[1].Outcome != OutcomePending {
		t.Fatalf("t2 should stay pending, got %s", out[1].Outcome)
	}
}

func TestResolveQuestionCoverageTerminalNotRegressed(t *testing.T) {
	// 终态项（answered）不能因追问被回退成 needs_input（规避覆盖校验）。
	answered := validAnswered("t1")
	items := []TaskItem{answered}
	questions := []QuestionRecord{{Type: RecordQuestion, TaskKeys: []string{"t1"}, MissingFields: []MissingField{{TaskKey: "t1", Field: "pet_id", Necessity: string(NecessityBlocking)}}}}
	out := ResolveQuestionCoverage(items, questions)
	if out[0].Outcome != OutcomeAnswered {
		t.Fatalf("terminal answered should not regress to needs_input, got %s", out[0].Outcome)
	}
}

func TestRunDecisionLoopRequestInputMarksNeedsInput(t *testing.T) {
	// 端到端：request_input 决策循环返回的 TaskItems 应含 needs_input 项。
	model := &scriptedModel{responses: [][]ProtocolRecord{requestInputRecords()}}
	tools := &fakeToolExecutor{}
	outcome, err := RunDecisionLoop(context.Background(), model, tools, StepInput{}, 0)
	if err != nil {
		t.Fatalf("loop failed: %v", err)
	}
	if outcome.Action != ActionRequestInput {
		t.Fatalf("action = %s, want request_input", outcome.Action)
	}
	if len(outcome.TaskItems) != 1 {
		t.Fatalf("task items = %d, want 1", len(outcome.TaskItems))
	}
	if outcome.TaskItems[0].Outcome != OutcomeNeedsInput {
		t.Fatalf("task item outcome = %s, want needs_input", outcome.TaskItems[0].Outcome)
	}
}
