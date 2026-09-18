package ask

import "testing"

func makeTurns(n int) []Turn {
	turns := make([]Turn, 0, n)
	for i := 1; i <= n; i++ {
		turns = append(turns, Turn{ID: "turn-" + itoa(i), Input: "输入 " + itoa(i)})
	}
	return turns
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := make([]byte, 0, 8)
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

func TestPlanSummarizeThresholdBoundary(t *testing.T) {
	// 恰好 32 条不触发（文档 5.10）。
	if p := PlanSummarize(makeTurns(32), 32, 16); p.Triggered {
		t.Fatalf("32 turns should not trigger, got %+v", p)
	}
	// 第 33 条触发。
	p := PlanSummarize(makeTurns(33), 32, 16)
	if !p.Triggered || p.Trigger != SummaryTriggerCount {
		t.Fatalf("33 turns should trigger by count, got %+v", p)
	}
	// 压缩区间 = 最近 16 条之前（33-16=17 条），保留 16 条。
	if len(p.CompressTurnIDs) != 17 {
		t.Fatalf("expected 17 compress turns, got %d", len(p.CompressTurnIDs))
	}
	if len(p.KeepTurnIDs) != 16 {
		t.Fatalf("expected 16 keep turns, got %d", len(p.KeepTurnIDs))
	}
	// 保留区间是最后 16 条。
	if p.KeepTurnIDs[0] != "turn-18" || p.KeepTurnIDs[15] != "turn-33" {
		t.Fatalf("keep range wrong: %v", p.KeepTurnIDs)
	}
}

func TestPlanSummarizeNoCompressRange(t *testing.T) {
	// 阈值小于 keep 时（如 threshold=5, keep=16）无可压缩区间，不触发。
	p := PlanSummarize(makeTurns(10), 5, 16)
	if p.Triggered {
		t.Fatalf("no compress range should not trigger, got %+v", p)
	}
}

func TestPlanSummarizeForToken(t *testing.T) {
	p := PlanSummarizeForToken(makeTurns(20), 16)
	if !p.Triggered || p.Trigger != SummaryTriggerToken {
		t.Fatalf("token trigger expected, got %+v", p)
	}
	if len(p.CompressTurnIDs) != 4 || len(p.KeepTurnIDs) != 16 {
		t.Fatalf("unexpected ranges: compress=%d keep=%d", len(p.CompressTurnIDs), len(p.KeepTurnIDs))
	}
}

func TestSummaryStatusTransition(t *testing.T) {
	cases := []struct {
		from, to SummaryStatus
		ok       bool
	}{
		{SummaryStatusCandidate, SummaryStatusUsable, true},
		{SummaryStatusCandidate, SummaryStatusRejected, true},
		{SummaryStatusCandidate, SummaryStatusStale, true},
		{SummaryStatusCandidate, SummaryStatusSuperseded, false},
		{SummaryStatusUsable, SummaryStatusStale, true},
		{SummaryStatusUsable, SummaryStatusSuperseded, true},
		{SummaryStatusUsable, SummaryStatusRejected, false},
		{SummaryStatusRejected, SummaryStatusUsable, false},
		{SummaryStatusStale, SummaryStatusUsable, false},
		{SummaryStatusSuperseded, SummaryStatusUsable, false},
	}
	for _, c := range cases {
		if got := CanTransitionSummary(c.from, c.to); got != c.ok {
			t.Errorf("CanTransitionSummary(%s -> %s) = %v, want %v", c.from, c.to, got, c.ok)
		}
	}
}

func validSummaryCandidate() SummaryCandidate {
	return SummaryCandidate{
		ID:            "sum-1",
		Version:       1,
		SessionID:     "sess-1",
		RunID:         "run-1",
		AttemptID:     "attempt-1",
		InputRevision: 1,
		Status:        SummaryStatusCandidate,
		Sources: []SummarySource{
			{SourceType: "ask_turn", ObjectID: "turn-1", Version: "v1", OriginalTime: "2026-09-17", Segment: "旺仔昨天吐了两次", Position: "0"},
		},
		ReplaceRefs: []SummarySourceRef{
			{SourceType: "ask_turn", ObjectID: "turn-1", Version: "v1"},
		},
		Items: []SummaryItem{
			{Key: "i1", Type: SummaryItemUserStatement, Subject: "旺仔", Statement: "旺仔昨天吐了两次", EventTime: "2026-09-16", SourceRefs: []SummarySourceRef{{SourceType: "ask_turn", ObjectID: "turn-1", Version: "v1"}}},
		},
		Coverage: []SummaryCoverage{
			{SourceType: "ask_turn", SourceID: "turn-1", ItemKeys: []string{"i1"}},
		},
	}
}

func TestValidateSummaryCandidateOK(t *testing.T) {
	if err := ValidateSummaryCandidate(validSummaryCandidate()); err != nil {
		t.Fatalf("valid candidate rejected: %v", err)
	}
}

func TestValidateSummaryCandidateRejectsOutOfListSource(t *testing.T) {
	c := validSummaryCandidate()
	c.Items[0].SourceRefs = []SummarySourceRef{{SourceType: "ask_turn", ObjectID: "turn-999", Version: "v1"}}
	if err := ValidateSummaryCandidate(c); err == nil {
		t.Fatal("expected error for source not in frozen list")
	}
}

func TestValidateSummaryCandidateRejectsMissingCoverage(t *testing.T) {
	c := validSummaryCandidate()
	c.Coverage = nil
	if err := ValidateSummaryCandidate(c); err == nil {
		t.Fatal("expected error for replace source without coverage")
	}
}

func TestValidateSummaryCandidateRejectsBusinessAction(t *testing.T) {
	c := validSummaryCandidate()
	c.Items[0].Statement = "请 call_tools 查询记录"
	if err := ValidateSummaryCandidate(c); err == nil {
		t.Fatal("expected error for business action in summary item")
	}
}

func TestValidateSummaryCandidateRejectsEmptyStructure(t *testing.T) {
	if err := ValidateSummaryCandidate(SummaryCandidate{}); err == nil {
		t.Fatal("expected error for empty candidate")
	}
	c := validSummaryCandidate()
	c.Items = nil
	if err := ValidateSummaryCandidate(c); err == nil {
		t.Fatal("expected error for empty items")
	}
}
