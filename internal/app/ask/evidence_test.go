package ask

import (
	"context"
	"testing"
)

func finalAnswerRecordsWithEvidence(text string, refs []EvidenceRef) []ProtocolRecord {
	group := ProtocolRecord{Type: RecordGroup, Group: &GroupRecord{
		Type: RecordGroup, GroupKey: "g1", TaskKeys: []string{"t1"},
		AnswerKind: AnswerCasual, Scope: ScopeFull,
		Subjects: []AnswerSubject{{SubjectKey: "s1", Kind: SubjectUnresolved, Description: "用户"}},
	}}
	segment := ProtocolRecord{Type: RecordSegment, Segment: &SegmentRecord{
		Type: RecordSegment, SegmentKey: "s1", GroupKey: "g1", SubjectKeys: []string{"s1"},
		Field: "reply", Text: text, BasisKind: BasisUserStatement, EvidenceRefs: refs,
	}}
	coverage := ProtocolRecord{Type: RecordCoverage, Coverage: &CoverageRecord{
		Type:  RecordCoverage,
		Tasks: []TaskCoverage{{TaskKey: "t1", AnswerGroupKeys: []string{"g1"}}},
	}}
	return []ProtocolRecord{headerRecord(ActionFinalAnswer), group, segment, coverage, endRecord()}
}

func TestEvidenceExistsToleratesVersionMismatch(t *testing.T) {
	keys := map[string]struct{}{}
	addEvidenceKey(keys, "pet_base", "pet-1", "pet-base-v1")
	if !evidenceExists(keys, EvidenceRef{SourceType: "pet_base", SourceID: "pet-1", Version: "pet-base-v1"}) {
		t.Fatal("exact version match should resolve")
	}
	if !evidenceExists(keys, EvidenceRef{SourceType: "pet_base", SourceID: "pet-1"}) {
		t.Fatal("id match without version should resolve")
	}
	if !evidenceExists(keys, EvidenceRef{SourceType: "pet_base", SourceID: "pet-1", Version: "wrong"}) {
		t.Fatal("id match with unknown version should resolve")
	}
	if evidenceExists(keys, EvidenceRef{SourceType: "pet_base", SourceID: "pet-2"}) {
		t.Fatal("unknown id should not resolve")
	}
}

func TestNormalizeDecisionEvidenceDropsUnresolvableRefs(t *testing.T) {
	decision := StepDecision{Groups: []AnswerGroup{{
		GroupKey:   "g1",
		TaskKeys:   []string{"t1"},
		AnswerKind: AnswerCasual,
		Scope:      ScopeFull,
		Subjects:   []AnswerSubject{{SubjectKey: "s1", Kind: SubjectUnresolved, Description: "用户"}},
		Segments: []SegmentRecord{{
			SegmentKey: "s1", GroupKey: "g1", SubjectKeys: []string{"s1"}, Field: "reply",
			Text: "节哀", BasisKind: BasisUserStatement,
			EvidenceRefs: []EvidenceRef{{SourceType: "turn", SourceID: "t1"}},
		}},
		Risks: []RiskRecord{{
			GroupKey: "g1", SubjectKey: "s1", Level: RiskUnknown,
			Evidence: []EvidenceRef{{SourceType: "calendar_record", SourceID: "missing"}},
		}},
	}}}
	if err := normalizeDecisionEvidence(&decision, map[string]struct{}{}, "turn-1"); err != nil {
		t.Fatalf("fabricated evidence on non-business segment should be dropped, got %v", err)
	}
	refs := decision.Groups[0].Segments[0].EvidenceRefs
	if len(refs) != 1 || refs[0].SourceType != "turn" || refs[0].SourceID != "turn-1" {
		t.Fatalf("segment evidence = %+v, want server-filled turn ref", refs)
	}
	if riskEvidence := decision.Groups[0].Risks[0].Evidence; len(riskEvidence) != 0 {
		t.Fatalf("risk evidence = %+v, want dropped", riskEvidence)
	}
}

func TestNormalizeDecisionEvidenceRejectsBusinessFactWithoutEvidence(t *testing.T) {
	decision := StepDecision{Groups: []AnswerGroup{{
		GroupKey:   "g1",
		TaskKeys:   []string{"t1"},
		AnswerKind: AnswerFact,
		Scope:      ScopeFull,
		Subjects:   []AnswerSubject{{SubjectKey: "s1", Kind: SubjectPet, PetID: "pet-1"}},
		Segments: []SegmentRecord{{
			SegmentKey: "s1", GroupKey: "g1", SubjectKeys: []string{"s1"}, Field: "result",
			Text: "上周洗过澡", BasisKind: BasisBusinessFact,
			EvidenceRefs: []EvidenceRef{{SourceType: "calendar_record", SourceID: "missing"}},
		}},
	}}}
	if err := normalizeDecisionEvidence(&decision, map[string]struct{}{}, "turn-1"); err == nil {
		t.Fatal("business_fact without resolvable evidence should fail")
	}
}

func TestProcessRunV2ToleratesFabricatedUserStatementEvidence(t *testing.T) {
	text := "听到这个消息很难过，节哀。需要我帮你整理这段陪伴的回忆吗？"
	model := &scriptedModel{responses: [][]ProtocolRecord{finalAnswerRecordsWithEvidence(text, []EvidenceRef{{SourceType: "turn", SourceID: "t1", Version: "v1"}})}}
	service, _ := newV2Service(t, model, &fakeBusinessRead{})

	created, err := service.CreateSession(context.Background(), "family-1", "user-1", "pet-1", "我的猫猫两天前去世了呜呜呜", "create-evidence-1")
	if err != nil {
		t.Fatal(err)
	}
	processed, err := service.ProcessRun(context.Background(), "family-1", created.Session.ID, created.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if processed.Run.Status != RunCompleted {
		t.Fatalf("run status = %s (error_code %s), want completed", processed.Run.Status, processed.Run.ErrorCode)
	}
	final := lastEvent(processed.Events)
	if final.Type != "assistant.completed" {
		t.Fatalf("last event = %s, want assistant.completed", final.Type)
	}
	data := eventDataMap(t, final)
	if data["answer"] != text {
		t.Fatalf("answer = %v, want %q", data["answer"], text)
	}
	groups, ok := data["groups"].([]any)
	if !ok || len(groups) != 1 {
		t.Fatalf("groups = %#v", data["groups"])
	}
	segments := groups[0].(map[string]any)["segments"].([]any)
	refs := segments[0].(map[string]any)["evidence_refs"].([]any)
	if len(refs) != 1 {
		t.Fatalf("segment evidence_refs = %#v", refs)
	}
	ref := refs[0].(map[string]any)
	if ref["source_type"] != "turn" || ref["source_id"] != created.Run.TurnID {
		t.Fatalf("evidence ref = %#v, want turn/%s", ref, created.Run.TurnID)
	}
}
