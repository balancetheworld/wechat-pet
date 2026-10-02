package ask

import (
	"strings"
	"testing"
)

func mi(task, subject, field, infoType string, necessity Necessity, status MissingInfoStatus) MissingItem {
	return MissingItem{
		TaskItemID: task, SubjectKey: subject, Field: field, InfoType: infoType,
		Necessity: necessity, Status: status,
	}
}

func validQuestion() Question {
	return Question{
		QuestionID:    "q-1",
		RunID:         "run-1",
		OriginTurnID:  "turn-1",
		MessageID:     "msg-1",
		InputRevision: 2,
		Text:          "照片属于哪只宠物？",
		MissingItems: []MissingItem{
			mi("t1", "s1", "pet", "pet_identity", NecessityBlocking, MissingUnanswered),
		},
		Status: QuestionOpen,
	}
}

func TestValidateQuestion(t *testing.T) {
	if err := ValidateQuestion(validQuestion()); err != nil {
		t.Fatalf("valid question should pass: %v", err)
	}
	for name, mutate := range map[string]func(*Question){
		"empty question_id":    func(q *Question) { q.QuestionID = "" },
		"empty run_id":         func(q *Question) { q.RunID = "" },
		"empty origin_turn_id": func(q *Question) { q.OriginTurnID = "" },
		"bad revision":         func(q *Question) { q.InputRevision = 0 },
		"bad status":           func(q *Question) { q.Status = "bogus" },
		"empty content":        func(q *Question) { q.Text = ""; q.MissingItems = nil },
		"bad missing item":     func(q *Question) { q.MissingItems[0].Field = "" },
		"bad necessity":        func(q *Question) { q.MissingItems[0].Necessity = "bogus" },
		"bad missing status":   func(q *Question) { q.MissingItems[0].Status = "bogus" },
	} {
		q := validQuestion()
		mutate(&q)
		if err := ValidateQuestion(q); err == nil {
			t.Errorf("%s: should fail", name)
		}
	}
}

func TestValidateReply(t *testing.T) {
	r := Reply{QuestionID: "q-1", RunID: "run-1", IdempotencyKey: "ik-1", Text: "不知道"}
	if err := ValidateReply(r); err != nil {
		t.Fatalf("valid reply should pass: %v", err)
	}
	for name, mutate := range map[string]func(*Reply){
		"empty question_id": func(r *Reply) { r.QuestionID = "" },
		"empty run_id":      func(r *Reply) { r.RunID = "" },
		"empty idempotency": func(r *Reply) { r.IdempotencyKey = "" },
		"empty content":     func(r *Reply) { r.Text = ""; r.ImageIDs = nil; r.OptionKeys = nil },
	} {
		rr := Reply{QuestionID: "q-1", RunID: "run-1", IdempotencyKey: "ik-1", Text: "不知道"}
		mutate(&rr)
		if err := ValidateReply(rr); err == nil {
			t.Errorf("%s: should fail", name)
		}
	}
}

func TestIsStaleReply(t *testing.T) {
	open := validQuestion()
	if IsStaleReply(Reply{QuestionID: "q-1"}, open) {
		t.Fatal("matching open reply should not be stale")
	}
	if !IsStaleReply(Reply{QuestionID: "q-2"}, open) {
		t.Fatal("different question_id should be stale")
	}
	closed := validQuestion()
	closed.Status = QuestionClosed
	if !IsStaleReply(Reply{QuestionID: "q-1"}, closed) {
		t.Fatal("reply to closed question should be stale")
	}
}

func TestRemainingToAsk(t *testing.T) {
	items := []MissingItem{
		mi("t1", "s1", "pet", "identity", NecessityBlocking, MissingUnanswered),
		mi("t2", "s2", "time", "when", NecessityBlocking, MissingConflicting),
		mi("t3", "s3", "pet", "identity", NecessityBlocking, MissingKnown),
		mi("t4", "s4", "pet", "identity", NecessityBlocking, MissingUnknown),
		mi("t5", "s5", "pet", "identity", NecessityBlocking, MissingRefused),
		mi("t6", "s6", "note", "extra", NecessitySupport, MissingUnanswered),
	}
	remaining := RemainingToAsk(items)
	if len(remaining) != 2 {
		t.Fatalf("expected 2 remaining, got %d", len(remaining))
	}
	if remaining[0].TaskItemID != "t1" || remaining[1].TaskItemID != "t2" {
		t.Fatalf("wrong remaining set: %+v", remaining)
	}
}

func TestDefinitiveMissing(t *testing.T) {
	items := []MissingItem{
		mi("t1", "s1", "pet", "identity", NecessityBlocking, MissingUnknown),
		mi("t2", "s2", "pet", "identity", NecessityBlocking, MissingRefused),
		mi("t3", "s3", "pet", "identity", NecessityBlocking, MissingUnanswered),
		mi("t4", "s4", "pet", "identity", NecessityBlocking, MissingKnown),
		mi("t5", "s5", "note", "extra", NecessitySupport, MissingUnknown),
	}
	definitive := DefinitiveMissing(items)
	if len(definitive) != 2 {
		t.Fatalf("expected 2 definitive, got %d", len(definitive))
	}
}

func TestShouldConcludeLimited(t *testing.T) {
	if ShouldConcludeLimited([]MissingItem{
		mi("t1", "s1", "pet", "identity", NecessityBlocking, MissingUnknown),
	}) != true {
		t.Fatal("unknown blocking item should conclude limited")
	}
	if ShouldConcludeLimited([]MissingItem{
		mi("t1", "s1", "pet", "identity", NecessityBlocking, MissingUnanswered),
	}) != false {
		t.Fatal("unanswered blocking item should NOT conclude limited yet")
	}
	if ShouldConcludeLimited([]MissingItem{
		mi("t1", "s1", "pet", "identity", NecessitySupport, MissingUnknown),
	}) != false {
		t.Fatal("support-only unknown should not conclude limited")
	}
}

func TestHasRepeatedAsk(t *testing.T) {
	prev := Question{MissingItems: []MissingItem{
		mi("t1", "s1", "pet", "identity", NecessityBlocking, MissingKnown),
		mi("t2", "s2", "pet", "identity", NecessityBlocking, MissingUnknown),
		mi("t3", "s3", "time", "when", NecessityBlocking, MissingUnanswered),
	}}

	// 换措辞重复询问已 known 的同一资料 → 阻断。
	next := Question{MissingItems: []MissingItem{
		mi("t1", "s1", "pet", "identity", NecessityBlocking, MissingUnanswered),
	}}
	if !HasRepeatedAsk(prev, next) {
		t.Fatal("re-asking settled field should be blocked")
	}

	// 询问另一个尚未回答的字段 → 不阻断。
	next2 := Question{MissingItems: []MissingItem{
		mi("t3", "s3", "time", "when", NecessityBlocking, MissingUnanswered),
	}}
	if HasRepeatedAsk(prev, next2) {
		t.Fatal("re-asking unsettled field should not be blocked")
	}

	// 字段/信息类型不同 → 不阻断。
	next3 := Question{MissingItems: []MissingItem{
		mi("t1", "s1", "pet", "breed", NecessityBlocking, MissingUnanswered),
	}}
	if HasRepeatedAsk(prev, next3) {
		t.Fatal("different info_type should not be blocked")
	}
}

func TestValidateVersionMonotonic(t *testing.T) {
	if err := ValidateVersionMonotonic(1, 2); err != nil {
		t.Fatalf("1->2 should pass: %v", err)
	}
	if err := ValidateVersionMonotonic(2, 2); err == nil {
		t.Fatal("2->2 should fail")
	}
	if err := ValidateVersionMonotonic(3, 2); err == nil || !strings.Contains(err.Error(), "strictly increasing") {
		t.Fatalf("3->2 should fail with monotonic message, got %v", err)
	}
}

func TestSameAnswerGroup(t *testing.T) {
	a := AnswerVersion{OriginTurnID: "turn-1", Version: 1}
	b := AnswerVersion{OriginTurnID: "turn-1", Version: 2}
	c := AnswerVersion{OriginTurnID: "turn-2", Version: 1}
	d := AnswerVersion{Version: 1}
	if !SameAnswerGroup(a, b) {
		t.Fatal("same origin_turn_id should be same group")
	}
	if SameAnswerGroup(a, c) {
		t.Fatal("different origin_turn_id should be different group")
	}
	if SameAnswerGroup(a, d) {
		t.Fatal("empty origin_turn_id should not match")
	}
}
