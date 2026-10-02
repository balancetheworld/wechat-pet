package ask

import (
	"errors"
	"strings"
	"testing"
)

// ---- task_item.go：结果分类与任务项校验 ----

func TestTaskOutcomeValidAndTerminal(t *testing.T) {
	terminals := []TaskOutcome{OutcomeAnswered, OutcomePreviewProvided, OutcomeIncomplete, OutcomeSuperseded}
	nonTerminals := []TaskOutcome{OutcomePending, OutcomeNeedsInput, OutcomeEvidenceReady}
	all := append(append([]TaskOutcome{}, terminals...), nonTerminals...)

	for _, o := range all {
		if !o.Valid() {
			t.Fatalf("%q should be valid", o)
		}
	}
	if TaskOutcome("bogus").Valid() {
		t.Fatal("bogus outcome should be invalid")
	}
	for _, o := range terminals {
		if !o.Terminal() {
			t.Fatalf("%q should be terminal", o)
		}
	}
	for _, o := range nonTerminals {
		if o.Terminal() {
			t.Fatalf("%q should NOT be terminal", o)
		}
	}
}

func validTaskItem() TaskItem {
	return TaskItem{
		TaskItemID:    "ti-1",
		OriginTurnID:  "turn-1",
		RunID:         "run-1",
		ItemRevision:  1,
		Goal:          "查旺仔最近记录",
		SourceTurnIDs: []string{"turn-1"},
		Subjects:      []AnswerSubject{{SubjectKey: "s1", Kind: SubjectPet, PetID: "pet-1"}},
		Outcome:       OutcomePending,
	}
}

func TestValidateTaskItemAnsweredRequiresAnswerGroupRef(t *testing.T) {
	it := validTaskItem()
	it.Outcome = OutcomeAnswered
	if err := ValidateTaskItem(it); err == nil {
		t.Fatal("answered without answer_group ref should fail")
	}

	it.ResultRef = &TaskResultRef{Kind: ResultRefAnswerGroup, RefID: "g1", Version: "v1"}
	if err := ValidateTaskItem(it); err != nil {
		t.Fatalf("answered with answer_group ref should pass: %v", err)
	}

	// 类型错：指向 tool_result 不能冒充已回答。
	it.ResultRef = &TaskResultRef{Kind: ResultRefToolResult, RefID: "tr1"}
	if err := ValidateTaskItem(it); err == nil {
		t.Fatal("answered with wrong ref kind should fail")
	}
}

func TestValidateTaskItemPreviewRequiresOperationRef(t *testing.T) {
	it := validTaskItem()
	it.Outcome = OutcomePreviewProvided
	if err := ValidateTaskItem(it); err == nil {
		t.Fatal("preview_provided without operation ref should fail")
	}
	it.ResultRef = &TaskResultRef{Kind: ResultRefOperation, RefID: "op-1", Version: "v3"}
	if err := ValidateTaskItem(it); err != nil {
		t.Fatalf("preview_provided with operation ref should pass: %v", err)
	}
}

func TestValidateTaskItemNeedsInputAndIncompleteRequireReason(t *testing.T) {
	it := validTaskItem()
	it.Outcome = OutcomeNeedsInput
	if err := ValidateTaskItem(it); err == nil {
		t.Fatal("needs_input without missing_fields should fail")
	}
	it.MissingFields = []MissingField{{TaskKey: "ti-1", Field: "pet", Necessity: "required"}}
	if err := ValidateTaskItem(it); err != nil {
		t.Fatalf("needs_input with missing_fields should pass: %v", err)
	}

	it2 := validTaskItem()
	it2.Outcome = OutcomeIncomplete
	if err := ValidateTaskItem(it2); err == nil {
		t.Fatal("incomplete without reason should fail")
	}
	it2.IncompleteReason = "权限不足"
	if err := ValidateTaskItem(it2); err != nil {
		t.Fatalf("incomplete with reason should pass: %v", err)
	}
}

func TestValidateTaskItemSupersededRequiresBasis(t *testing.T) {
	it := validTaskItem()
	it.Outcome = OutcomeSuperseded
	if err := ValidateTaskItem(it); err == nil {
		t.Fatal("superseded without traceable basis should fail")
	}
	it.SupersededBy = "ti-2"
	if err := ValidateTaskItem(it); err != nil {
		t.Fatalf("superseded with superseded_by should pass: %v", err)
	}

	it2 := validTaskItem()
	it2.Outcome = OutcomeSuperseded
	it2.WithdrawnReason = "用户改换目标"
	if err := ValidateTaskItem(it2); err != nil {
		t.Fatalf("superseded with withdrawn_reason should pass: %v", err)
	}
}

func TestValidateTaskItemSubjectConstraints(t *testing.T) {
	// 已解析宠物缺 pet_id → 失败（不能猜测 pet_id）。
	it := validTaskItem()
	it.Subjects = []AnswerSubject{{SubjectKey: "s1", Kind: SubjectPet}}
	if err := ValidateTaskItem(it); err == nil {
		t.Fatal("pet subject without pet_id should fail")
	}

	// 未明确对象缺描述 → 失败。
	it2 := validTaskItem()
	it2.Subjects = []AnswerSubject{{SubjectKey: "s1", Kind: SubjectUnresolved}}
	if err := ValidateTaskItem(it2); err == nil {
		t.Fatal("unresolved subject without description should fail")
	}
}

func TestValidateTaskItemBasicFields(t *testing.T) {
	it := validTaskItem()
	it.TaskItemID = ""
	if err := ValidateTaskItem(it); err == nil {
		t.Fatal("empty task_item_id should fail")
	}
	it = validTaskItem()
	it.ItemRevision = 0
	if err := ValidateTaskItem(it); err == nil {
		t.Fatal("non-positive item_revision should fail")
	}
	it = validTaskItem()
	it.Goal = ""
	if err := ValidateTaskItem(it); err == nil {
		t.Fatal("empty goal should fail")
	}
	it = validTaskItem()
	it.Outcome = "bogus"
	if err := ValidateTaskItem(it); err == nil {
		t.Fatal("invalid outcome should fail")
	}
}

func TestTaskItemCanStartTool(t *testing.T) {
	resolved := validTaskItem()
	if !resolved.CanStartTool() {
		t.Fatal("resolved pet subject should allow tool")
	}
	unresolved := validTaskItem()
	unresolved.Subjects = []AnswerSubject{{SubjectKey: "s1", Kind: SubjectUnresolved, Description: "照片里的宠物"}}
	if unresolved.CanStartTool() {
		t.Fatal("unresolved subject should block tool")
	}
	if !unresolved.HasSubject("s1") || resolved.HasSubject("s2") {
		t.Fatal("HasSubject misreporting")
	}
}

// ---- coverage.go：Run 完成覆盖校验 ----

func TestUnresolvedItems(t *testing.T) {
	items := []TaskItem{
		{TaskItemID: "a", Outcome: OutcomePending},
		{TaskItemID: "b", Outcome: OutcomeEvidenceReady},
		{TaskItemID: "c", Outcome: OutcomeAnswered, ResultRef: &TaskResultRef{Kind: ResultRefAnswerGroup, RefID: "g"}},
		{TaskItemID: "d", Outcome: OutcomeIncomplete, IncompleteReason: "故障"},
	}
	unresolved := UnresolvedItems(items)
	if len(unresolved) != 2 {
		t.Fatalf("expected 2 unresolved, got %d", len(unresolved))
	}
	if unresolved[0].TaskItemID != "a" || unresolved[1].TaskItemID != "b" {
		t.Fatalf("wrong unresolved set: %+v", unresolved)
	}
}

func TestCheckRunClosureRejectsUnresolved(t *testing.T) {
	items := []TaskItem{
		{TaskItemID: "a", Outcome: OutcomePending, ItemRevision: 1, Goal: "x"},
	}
	err := CheckRunClosure(items)
	if err == nil || !strings.Contains(err.Error(), "unresolved") {
		t.Fatalf("expected unresolved error, got %v", err)
	}
}

func TestCheckRunClosureRejectsTerminalWithoutBasis(t *testing.T) {
	// 不能先把所有项标为 answered 再发现最终校验失败。
	items := []TaskItem{
		{TaskItemID: "a", Outcome: OutcomeAnswered, ItemRevision: 1, Goal: "x"},
	}
	err := CheckRunClosure(items)
	if err == nil || !strings.Contains(err.Error(), "answer_group") {
		t.Fatalf("expected missing-basis error, got %v", err)
	}
}

func TestCheckRunClosurePassesCovered(t *testing.T) {
	items := []TaskItem{
		validAnswered("a"),
		{TaskItemID: "b", ItemRevision: 1, Goal: "x", Outcome: OutcomeIncomplete, IncompleteReason: "无记录"},
		{TaskItemID: "c", ItemRevision: 1, Goal: "y", Outcome: OutcomeSuperseded, SupersededBy: "d"},
	}
	if err := CheckRunClosure(items); err != nil {
		t.Fatalf("covered list should pass: %v", err)
	}
}

func validAnswered(id string) TaskItem {
	return TaskItem{
		TaskItemID:   id,
		ItemRevision: 1,
		Goal:         "x",
		Outcome:      OutcomeAnswered,
		ResultRef:    &TaskResultRef{Kind: ResultRefAnswerGroup, RefID: "g1", Version: "v1"},
	}
}

func TestCheckTerminalConsistency(t *testing.T) {
	if err := CheckTerminalConsistency(OutcomePending, OutcomeAnswered); err != nil {
		t.Fatalf("pending->answered should pass: %v", err)
	}
	if err := CheckTerminalConsistency(OutcomeAnswered, OutcomePending); err == nil {
		t.Fatal("answered->pending should fail (no regression)")
	}
	if err := CheckTerminalConsistency(OutcomePending, "bogus"); err == nil {
		t.Fatal("invalid transition should fail")
	}
}

// ---- first_decision.go：首次业务决策 ----

func TestClassifyFirstResponse(t *testing.T) {
	resolved := []TaskItem{
		{TaskItemID: "t1", ItemRevision: 1, Goal: "查记录", Subjects: []AnswerSubject{{SubjectKey: "s1", Kind: SubjectPet, PetID: "p1"}}},
	}
	unresolved := []TaskItem{
		{TaskItemID: "t1", ItemRevision: 1, Goal: "分析照片", Subjects: []AnswerSubject{{SubjectKey: "s1", Kind: SubjectUnresolved, Description: "照片"}}},
	}

	cases := []struct {
		name     string
		items    []TaskItem
		calls    int
		wantKind FirstStepKind
	}{
		{"no tasks no tools", nil, 0, StepSimpleAnswer},
		{"tool no tasks", nil, 1, StepDirectTool},
		{"tool with resolved tasks", resolved, 1, StepToolWithTasks},
		{"tool with unresolved tasks", unresolved, 1, StepNeedClarify},
		{"tasks only resolved", resolved, 0, StepTasksOnly},
		{"tasks only unresolved", unresolved, 0, StepNeedClarify},
	}
	for _, c := range cases {
		d := ClassifyFirstResponse(c.items, c.calls)
		if d.Kind != c.wantKind {
			t.Errorf("%s: got %q want %q (reason %q)", c.name, d.Kind, c.wantKind, d.Reason)
		}
	}
}

func TestValidateToolReadiness(t *testing.T) {
	items := []TaskItem{
		{TaskItemID: "t1", Subjects: []AnswerSubject{{SubjectKey: "s1", Kind: SubjectPet, PetID: "p1"}}},
	}
	if err := ValidateToolReadiness(items); err != nil {
		t.Fatalf("resolved items should pass: %v", err)
	}
	items = append(items, TaskItem{TaskItemID: "t2", Subjects: []AnswerSubject{{SubjectKey: "s2", Kind: SubjectUnresolved, Description: "照片"}}})
	err := ValidateToolReadiness(items)
	if err == nil {
		t.Fatal("unresolved item should block tool readiness")
	}
	var notReady *TaskNotReadyError
	if !errors.As(err, &notReady) {
		t.Fatalf("expected TaskNotReadyError, got %T", err)
	}
	if notReady.TaskItemID != "t2" {
		t.Fatalf("expected blocking task t2, got %q", notReady.TaskItemID)
	}
}
