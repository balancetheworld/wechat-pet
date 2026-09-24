package ask

import (
	"strings"
	"testing"
)

func TestValidateAnswerGroup(t *testing.T) {
	petSubject := AnswerSubject{SubjectKey: "s1", Kind: SubjectPet, PetID: "p1"}

	t.Run("valid casual", func(t *testing.T) {
		g := AnswerGroup{GroupKey: "g1", TaskKeys: []string{"t1"}, AnswerKind: AnswerCasual, Subjects: []AnswerSubject{petSubject}, Scope: ScopeFull, Segments: []SegmentRecord{{Field: "reply", Text: "你好"}}}
		if err := ValidateAnswerGroup(g); err != nil {
			t.Fatalf("valid casual rejected: %v", err)
		}
	})
	t.Run("casual requires reply", func(t *testing.T) {
		g := AnswerGroup{GroupKey: "g1", TaskKeys: []string{"t1"}, AnswerKind: AnswerCasual, Subjects: []AnswerSubject{petSubject}, Scope: ScopeFull}
		if err := ValidateAnswerGroup(g); err == nil || !strings.Contains(err.Error(), "reply") {
			t.Fatalf("expected reply required error, got %v", err)
		}
	})
	t.Run("fact requires result", func(t *testing.T) {
		g := AnswerGroup{GroupKey: "g1", TaskKeys: []string{"t1"}, AnswerKind: AnswerFact, Subjects: []AnswerSubject{petSubject}, Scope: ScopeFull}
		if err := ValidateAnswerGroup(g); err == nil || !strings.Contains(err.Error(), "result") {
			t.Fatalf("expected result required error, got %v", err)
		}
	})
	t.Run("business fact requires evidence", func(t *testing.T) {
		g := AnswerGroup{GroupKey: "g1", TaskKeys: []string{"t1"}, AnswerKind: AnswerFact, Subjects: []AnswerSubject{petSubject}, Scope: ScopeFull, Segments: []SegmentRecord{{SegmentKey: "seg1", Field: "result", Text: "没有记录", BasisKind: BasisBusinessFact}}}
		if err := ValidateAnswerGroup(g); err == nil || !strings.Contains(err.Error(), "evidence_refs") {
			t.Fatalf("expected evidence_refs error, got %v", err)
		}
	})
	t.Run("business fact with evidence is ok", func(t *testing.T) {
		g := AnswerGroup{GroupKey: "g1", TaskKeys: []string{"t1"}, AnswerKind: AnswerFact, Subjects: []AnswerSubject{petSubject}, Scope: ScopeFull, Segments: []SegmentRecord{{SegmentKey: "seg1", Field: "result", Text: "有一条记录", BasisKind: BasisBusinessFact, EvidenceRefs: []EvidenceRef{{SourceType: "calendar_record", SourceID: "r1"}}}}}
		if err := ValidateAnswerGroup(g); err != nil {
			t.Fatalf("business fact with evidence rejected: %v", err)
		}
	})
	t.Run("non-full scope requires limitation", func(t *testing.T) {
		g := AnswerGroup{GroupKey: "g1", TaskKeys: []string{"t1"}, AnswerKind: AnswerFact, Subjects: []AnswerSubject{petSubject}, Scope: ScopeDeclined, Segments: []SegmentRecord{{Field: "result", Text: "查不到"}}}
		if err := ValidateAnswerGroup(g); err == nil || !strings.Contains(err.Error(), "limitation") {
			t.Fatalf("expected limitation error, got %v", err)
		}
	})
	t.Run("non-full scope with limitation is ok", func(t *testing.T) {
		g := AnswerGroup{GroupKey: "g1", TaskKeys: []string{"t1"}, AnswerKind: AnswerFact, Subjects: []AnswerSubject{petSubject}, Scope: ScopeDeclined, Segments: []SegmentRecord{
			{Field: "result", Text: "查不到"},
			{Field: "limitation", Text: "该宠物档案不可读"},
		}}
		if err := ValidateAnswerGroup(g); err != nil {
			t.Fatalf("declined scope with limitation rejected: %v", err)
		}
	})
	t.Run("health requires risk per subject", func(t *testing.T) {
		g := AnswerGroup{GroupKey: "g1", TaskKeys: []string{"t1"}, AnswerKind: AnswerHealth, Subjects: []AnswerSubject{petSubject}, Scope: ScopeFull, Segments: []SegmentRecord{{Field: "observation", Text: "未见异常"}}}
		if err := ValidateAnswerGroup(g); err == nil || !strings.Contains(err.Error(), "risk") {
			t.Fatalf("expected risk required error, got %v", err)
		}
	})
	t.Run("health with risk is ok", func(t *testing.T) {
		g := AnswerGroup{
			GroupKey: "g1", TaskKeys: []string{"t1"}, AnswerKind: AnswerHealth, Subjects: []AnswerSubject{petSubject}, Scope: ScopeFull,
			Segments: []SegmentRecord{{Field: "observation", Text: "未见异常"}, {Field: "uncertainty", Text: "资料不足"}},
			Risks:    []RiskRecord{{SubjectKey: "s1", Level: RiskUnknown}},
		}
		if err := ValidateAnswerGroup(g); err != nil {
			t.Fatalf("health with risk rejected: %v", err)
		}
	})
	t.Run("unresolved subject needs description", func(t *testing.T) {
		g := AnswerGroup{GroupKey: "g1", TaskKeys: []string{"t1"}, AnswerKind: AnswerCasual, Subjects: []AnswerSubject{{SubjectKey: "s1", Kind: SubjectUnresolved}}, Scope: ScopeFull, Segments: []SegmentRecord{{Field: "reply", Text: "x"}}}
		if err := ValidateAnswerGroup(g); err != nil {
			// 结构上 ValidateAnswerGroup 不校验 description，但协议层 validateGroup 已校验；
			// 这里仅确认通过，避免重复校验。
			t.Fatalf("unresolved subject should pass group-level validation: %v", err)
		}
	})
}

func TestAssembleAnswerGroups(t *testing.T) {
	records := []ProtocolRecord{
		{Type: RecordHeader, Header: &HeaderRecord{Type: RecordHeader, SchemaVersion: RecordArrayV1, Action: ActionFinalAnswer, TaskUpdates: []TaskUpdate{}}},
		{Type: RecordGroup, Group: &GroupRecord{Type: RecordGroup, GroupKey: "g1", TaskKeys: []string{"t1"}, AnswerKind: AnswerHealth, Subjects: []AnswerSubject{{SubjectKey: "s1", Kind: SubjectPet, PetID: "p1"}}, Scope: ScopeFull}},
		{Type: RecordSegment, Segment: &SegmentRecord{Type: RecordSegment, SegmentKey: "seg1", GroupKey: "g1", SubjectKeys: []string{"s1"}, Field: "observation", Text: "未见异常", BasisKind: BasisBusinessFact, EvidenceRefs: []EvidenceRef{{SourceType: "calendar_record", SourceID: "r1"}}}},
		{Type: RecordRisk, Risk: &RiskRecord{Type: RecordRisk, GroupKey: "g1", SubjectKey: "s1", Level: RiskGreen}},
		{Type: RecordCoverage, Coverage: &CoverageRecord{Type: RecordCoverage, Tasks: []TaskCoverage{}}},
		{Type: RecordEnd},
	}
	groups, err := AssembleAnswerGroups(records)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	g := groups[0]
	if g.GroupKey != "g1" || len(g.Segments) != 1 || len(g.Risks) != 1 {
		t.Fatalf("unexpected group assembly: %+v", g)
	}
	if err := ValidateAnswerGroup(g); err != nil {
		t.Fatalf("assembled health group missing required fields: %v", err)
	}
}

func TestAssembleAnswerGroupsUnknownGroup(t *testing.T) {
	records := []ProtocolRecord{
		{Type: RecordSegment, Segment: &SegmentRecord{Type: RecordSegment, SegmentKey: "seg1", GroupKey: "ghost", Field: "reply", Text: "x", BasisKind: BasisGeneralKnowledge}},
	}
	if _, err := AssembleAnswerGroups(records); err == nil || !strings.Contains(err.Error(), "unknown group") {
		t.Fatalf("expected unknown group error, got %v", err)
	}
}

func TestAnswerVersion(t *testing.T) {
	valid := AnswerVersion{OriginTurnID: "turn-1", Version: 1, Completeness: AnswerComplete, Applicability: ApplicabilityCurrent, InputRevision: 1}
	if !valid.Valid() {
		t.Fatal("valid version should be valid")
	}
	if !valid.IsEffectiveAnswer() {
		t.Fatal("complete+current should be effective")
	}
	if !valid.CanPublish() {
		t.Fatal("complete+current should be publishable")
	}

	t.Run("streaming can publish but not effective", func(t *testing.T) {
		v := AnswerVersion{OriginTurnID: "t", Version: 1, Completeness: AnswerStreaming, Applicability: ApplicabilityCurrent}
		if !v.CanPublish() {
			t.Fatal("streaming+current should publish")
		}
		if v.IsEffectiveAnswer() {
			t.Fatal("streaming should not be effective")
		}
	})
	t.Run("superseded cannot publish", func(t *testing.T) {
		v := AnswerVersion{OriginTurnID: "t", Version: 1, Completeness: AnswerComplete, Applicability: ApplicabilitySuperseded}
		if v.CanPublish() {
			t.Fatal("superseded should not publish")
		}
		if v.IsEffectiveAnswer() {
			t.Fatal("superseded should not be effective")
		}
	})
	t.Run("incomplete not effective", func(t *testing.T) {
		v := AnswerVersion{OriginTurnID: "t", Version: 1, Completeness: AnswerIncomplete, Applicability: ApplicabilityCurrent}
		if v.IsEffectiveAnswer() {
			t.Fatal("incomplete should not be effective")
		}
	})
	t.Run("rejected cannot publish", func(t *testing.T) {
		v := AnswerVersion{OriginTurnID: "t", Version: 1, Completeness: AnswerComplete, Applicability: ApplicabilityRejected}
		if v.IsEffectiveAnswer() || v.CanPublish() {
			t.Fatal("rejected should neither be effective nor publish")
		}
	})
	t.Run("invalid version fields", func(t *testing.T) {
		if err := ValidateAnswerVersion(AnswerVersion{}); err == nil {
			t.Fatal("empty version should fail")
		}
		if err := ValidateAnswerVersion(AnswerVersion{OriginTurnID: "t", Version: 0}); err == nil {
			t.Fatal("zero version should fail")
		}
	})
}
