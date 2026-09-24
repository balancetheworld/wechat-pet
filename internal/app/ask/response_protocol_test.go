package ask

import (
	"strings"
	"testing"
)

// 组装 final_answer 完整响应 JSON（含中文、嵌套参数、转义与换行）。
const finalAnswerJSON = `[
  {"type":"header","schema_version":"record_array_v1","action":"final_answer","task_updates":[{"task_key":"t1","goal":"看旺仔情况","source_turn_ids":["turn-1"],"subject_keys":["s1"]}]},
  {"type":"group","group_key":"g1","task_keys":["t1"],"answer_kind":"health","subjects":[{"subject_key":"s1","kind":"pet","pet_id":"pet-1"}],"scope":"full"},
  {"type":"segment","segment_key":"seg1","group_key":"g1","subject_keys":["s1"],"field":"observation","text":"旺仔今天精神状态尚可，无呕吐。","basis_kind":"business_fact","evidence_refs":[{"source_type":"calendar_record","source_id":"r-1"}]},
  {"type":"segment","segment_key":"seg2","group_key":"g1","subject_keys":["s1"],"field":"uncertainty","text":"目前资料不足以判断更具体的方向。","basis_kind":"speculation"},
  {"type":"risk","group_key":"g1","subject_key":"s1","level":"green","uncertainty":"未见急症信号"},
  {"type":"coverage","tasks":[{"task_key":"t1","answer_group_keys":["g1"]}]},
  {"type":"end"}
]`

func parseAll(t *testing.T, raw string) ([]ProtocolRecord, *RecordArrayParser) {
	t.Helper()
	p := NewRecordArrayParser()
	var records []ProtocolRecord
	chunk, err := p.Feed(raw)
	if err != nil {
		t.Fatalf("feed: %v", err)
	}
	records = append(records, chunk...)
	return records, p
}

func TestRecordArrayParserSingleFeed(t *testing.T) {
	records, p := parseAll(t, finalAnswerJSON)
	if !p.Closed() {
		t.Fatal("parser should be closed after full array")
	}
	if len(records) != 7 {
		t.Fatalf("expected 7 records, got %d", len(records))
	}
	if records[0].Type != RecordHeader {
		t.Fatalf("first record should be header, got %q", records[0].Type)
	}
	if records[len(records)-1].Type != RecordEnd {
		t.Fatalf("last record should be end, got %q", records[len(records)-1].Type)
	}
	if _, err := ValidateResponse(records); err != nil {
		t.Fatalf("valid final_answer rejected: %v", err)
	}
}

func TestRecordArrayParserIncrementalByteByByte(t *testing.T) {
	p := NewRecordArrayParser()
	var records []ProtocolRecord
	for i := 0; i < len(finalAnswerJSON); i++ {
		chunk, err := p.Feed(finalAnswerJSON[i : i+1])
		if err != nil {
			t.Fatalf("byte feed at %d: %v", i, err)
		}
		records = append(records, chunk...)
	}
	if !p.Closed() {
		t.Fatal("parser should be closed")
	}
	if len(records) != 7 {
		t.Fatalf("expected 7 records, got %d", len(records))
	}
	if _, err := ValidateResponse(records); err != nil {
		t.Fatalf("incremental final_answer rejected: %v", err)
	}
}

func TestRecordArrayParserHandlesEscapesAndNesting(t *testing.T) {
	// 参数内嵌套对象、字符串含括号、转义引号与转义换行。
	raw := `[
	  {"type":"header","schema_version":"record_array_v1","action":"call_tools","task_updates":[]},
	  {"type":"call","call_key":"c1","task_keys":["t1"],"tool_name":"read_records","catalog_version":"v1","arguments":{"query":"呕吐 } ] \" 换行\n","limit":5,"filter":{"category":"medical"}}},
	  {"type":"coverage","tasks":[{"task_key":"t1","call_keys":["c1"]}]},
	  {"type":"end"}
	]`
	p := NewRecordArrayParser()
	var records []ProtocolRecord
	// 以 7 字节步长切分，制造字符串内跨分片边界。
	for i := 0; i < len(raw); i += 7 {
		end := i + 7
		if end > len(raw) {
			end = len(raw)
		}
		chunk, err := p.Feed(raw[i:end])
		if err != nil {
			t.Fatalf("feed at %d: %v", i, err)
		}
		records = append(records, chunk...)
	}
	if len(records) != 4 {
		t.Fatalf("expected 4 records, got %d", len(records))
	}
	action, err := ValidateResponse(records)
	if err != nil {
		t.Fatalf("call_tools rejected: %v", err)
	}
	if action != ActionCallTools {
		t.Fatalf("expected call_tools, got %q", action)
	}
	args := string(records[1].Call.Arguments)
	if !strings.Contains(args, "呕吐") || !strings.Contains(args, `"filter"`) {
		t.Fatalf("arguments should preserve nested content, got %s", args)
	}
}

func TestRecordArrayParserRejectsTextOutsideArray(t *testing.T) {
	p := NewRecordArrayParser()
	if _, err := p.Feed(`garbage [{"type":"end"}]`); err == nil {
		t.Fatal("expected error for text before array")
	}
}

func TestRecordArrayParserRejectsTrailingContent(t *testing.T) {
	p := NewRecordArrayParser()
	if _, err := p.Feed(`[{"type":"end"}] trailing`); err == nil {
		t.Fatal("expected error for content after ']'")
	}
}

func TestRecordArrayParserRejectsDuplicateObjectKey(t *testing.T) {
	p := NewRecordArrayParser()
	_, err := p.Feed(`[{"type":"end","type":"end"}]`)
	if err == nil || !strings.Contains(err.Error(), "duplicate object key") {
		t.Fatalf("expected duplicate key error, got %v", err)
	}
}

func TestRecordArrayParserRejectsUnknownField(t *testing.T) {
	p := NewRecordArrayParser()
	_, err := p.Feed(`[{"type":"end","secret":"x"}]`)
	if err == nil {
		t.Fatal("expected unknown field error for end record")
	}
}

func TestRecordArrayParserRejectsUnknownRecordType(t *testing.T) {
	p := NewRecordArrayParser()
	_, err := p.Feed(`[{"type":"mystery"}]`)
	if err == nil || !strings.Contains(err.Error(), "unknown record type") {
		t.Fatalf("expected unknown record type error, got %v", err)
	}
}

func TestValidateResponseActionConstraints(t *testing.T) {
	t.Run("call_tools rejects segment", func(t *testing.T) {
		records := []ProtocolRecord{
			{Type: RecordHeader, Header: &HeaderRecord{Type: RecordHeader, SchemaVersion: RecordArrayV1, Action: ActionCallTools, TaskUpdates: []TaskUpdate{}}},
			{Type: RecordSegment, Segment: &SegmentRecord{Type: RecordSegment, SegmentKey: "s", GroupKey: "g", Field: "reply", Text: "x", BasisKind: BasisGeneralKnowledge}},
			{Type: RecordCoverage, Coverage: &CoverageRecord{Type: RecordCoverage, Tasks: []TaskCoverage{}}},
			{Type: RecordEnd},
		}
		if _, err := ValidateResponse(records); err == nil {
			t.Fatal("expected error: segment not allowed for call_tools")
		}
	})
	t.Run("call_tools requires call", func(t *testing.T) {
		records := []ProtocolRecord{
			{Type: RecordHeader, Header: &HeaderRecord{Type: RecordHeader, SchemaVersion: RecordArrayV1, Action: ActionCallTools, TaskUpdates: []TaskUpdate{}}},
			{Type: RecordCoverage, Coverage: &CoverageRecord{Type: RecordCoverage, Tasks: []TaskCoverage{}}},
			{Type: RecordEnd},
		}
		if _, err := ValidateResponse(records); err == nil {
			t.Fatal("expected error: call_tools requires call")
		}
	})
	t.Run("request_input requires question", func(t *testing.T) {
		records := []ProtocolRecord{
			{Type: RecordHeader, Header: &HeaderRecord{Type: RecordHeader, SchemaVersion: RecordArrayV1, Action: ActionRequestInput, TaskUpdates: []TaskUpdate{}}},
			{Type: RecordCoverage, Coverage: &CoverageRecord{Type: RecordCoverage, Tasks: []TaskCoverage{}}},
			{Type: RecordEnd},
		}
		if _, err := ValidateResponse(records); err == nil {
			t.Fatal("expected error: request_input requires question")
		}
	})
	t.Run("final_answer requires group", func(t *testing.T) {
		records := []ProtocolRecord{
			{Type: RecordHeader, Header: &HeaderRecord{Type: RecordHeader, SchemaVersion: RecordArrayV1, Action: ActionFinalAnswer, TaskUpdates: []TaskUpdate{}}},
			{Type: RecordCoverage, Coverage: &CoverageRecord{Type: RecordCoverage, Tasks: []TaskCoverage{}}},
			{Type: RecordEnd},
		}
		if _, err := ValidateResponse(records); err == nil {
			t.Fatal("expected error: final_answer requires group")
		}
	})
}

func TestValidateResponseStructuralErrors(t *testing.T) {
	t.Run("missing header", func(t *testing.T) {
		if _, err := ValidateResponse(nil); err == nil {
			t.Fatal("expected missing header error")
		}
	})
	t.Run("unsupported schema", func(t *testing.T) {
		records := []ProtocolRecord{
			{Type: RecordHeader, Header: &HeaderRecord{Type: RecordHeader, SchemaVersion: "record_array_v0", Action: ActionFinalAnswer, TaskUpdates: []TaskUpdate{}}},
			{Type: RecordEnd},
		}
		if _, err := ValidateResponse(records); err == nil || !strings.Contains(err.Error(), "schema_version") {
			t.Fatalf("expected schema_version error, got %v", err)
		}
	})
	t.Run("invalid action", func(t *testing.T) {
		records := []ProtocolRecord{
			{Type: RecordHeader, Header: &HeaderRecord{Type: RecordHeader, SchemaVersion: RecordArrayV1, Action: ResponseAction("bogus"), TaskUpdates: []TaskUpdate{}}},
			{Type: RecordEnd},
		}
		if _, err := ValidateResponse(records); err == nil || !strings.Contains(err.Error(), "invalid action") {
			t.Fatalf("expected invalid action error, got %v", err)
		}
	})
	t.Run("missing end is derived from array closure", func(t *testing.T) {
		records := []ProtocolRecord{
			{Type: RecordHeader, Header: &HeaderRecord{Type: RecordHeader, SchemaVersion: RecordArrayV1, Action: ActionFinalAnswer, TaskUpdates: []TaskUpdate{}}},
		}
		if _, err := ValidateResponse(records); err != nil {
			t.Fatalf("missing end should not fail structural validation, got %v", err)
		}
	})
	t.Run("missing coverage is derived later", func(t *testing.T) {
		records := []ProtocolRecord{
			{Type: RecordHeader, Header: &HeaderRecord{Type: RecordHeader, SchemaVersion: RecordArrayV1, Action: ActionFinalAnswer, TaskUpdates: []TaskUpdate{}}},
			{Type: RecordEnd},
		}
		if _, err := ValidateResponse(records); err != nil {
			t.Fatalf("missing coverage should not fail structural validation, got %v", err)
		}
	})
	t.Run("coverage not before end", func(t *testing.T) {
		records := []ProtocolRecord{
			{Type: RecordHeader, Header: &HeaderRecord{Type: RecordHeader, SchemaVersion: RecordArrayV1, Action: ActionFinalAnswer, TaskUpdates: []TaskUpdate{}}},
			{Type: RecordCoverage, Coverage: &CoverageRecord{Type: RecordCoverage, Tasks: []TaskCoverage{}}},
			{Type: RecordEnd},
			{Type: RecordEnd},
		}
		if _, err := ValidateResponse(records); err == nil {
			t.Fatal("expected coverage/end ordering error")
		}
	})
}

func TestValidateResponseRejectsDuplicateTaskUpdates(t *testing.T) {
	records := []ProtocolRecord{
		{Type: RecordHeader, Header: &HeaderRecord{Type: RecordHeader, SchemaVersion: RecordArrayV1, Action: ActionRequestInput, TaskUpdates: []TaskUpdate{{TaskKey: "t1", Goal: "确认宠物"}, {TaskKey: "t1", Goal: "确认症状"}}}},
		{Type: RecordQuestion, Question: &QuestionRecord{Type: RecordQuestion, QuestionKey: "q1", TaskKeys: []string{"t1"}, Text: "请补充信息", MissingFields: []MissingField{{TaskKey: "t1", Field: "pet_id", Necessity: "blocking"}}}},
		{Type: RecordCoverage, Coverage: &CoverageRecord{Type: RecordCoverage, Tasks: []TaskCoverage{{TaskKey: "t1", QuestionKeys: []string{"q1"}}}}},
		{Type: RecordEnd},
	}
	if _, err := ValidateResponse(records); err == nil || !strings.Contains(err.Error(), "duplicate task_key") {
		t.Fatalf("expected duplicate task_key error, got %v", err)
	}
}

func TestValidateResponseDuplicateKeys(t *testing.T) {
	base := func() []ProtocolRecord {
		return []ProtocolRecord{
			{Type: RecordHeader, Header: &HeaderRecord{Type: RecordHeader, SchemaVersion: RecordArrayV1, Action: ActionFinalAnswer, TaskUpdates: []TaskUpdate{}}},
			{Type: RecordGroup, Group: &GroupRecord{Type: RecordGroup, GroupKey: "g1", TaskKeys: []string{"t1"}, AnswerKind: AnswerHealth, Subjects: []AnswerSubject{{SubjectKey: "s1", Kind: SubjectPet, PetID: "p1"}}, Scope: ScopeFull}},
			{Type: RecordGroup, Group: &GroupRecord{Type: RecordGroup, GroupKey: "g1", TaskKeys: []string{"t1"}, AnswerKind: AnswerHealth, Subjects: []AnswerSubject{{SubjectKey: "s1", Kind: SubjectPet, PetID: "p1"}}, Scope: ScopeFull}},
			{Type: RecordCoverage, Coverage: &CoverageRecord{Type: RecordCoverage, Tasks: []TaskCoverage{}}},
			{Type: RecordEnd},
		}
	}
	if _, err := ValidateResponse(base()); err == nil || !strings.Contains(err.Error(), "duplicate group key") {
		t.Fatalf("expected duplicate group key error, got %v", err)
	}
}

func TestValidateResponseGroupValidation(t *testing.T) {
	t.Run("invalid answer_kind", func(t *testing.T) {
		records := []ProtocolRecord{
			{Type: RecordHeader, Header: &HeaderRecord{Type: RecordHeader, SchemaVersion: RecordArrayV1, Action: ActionFinalAnswer, TaskUpdates: []TaskUpdate{}}},
			{Type: RecordGroup, Group: &GroupRecord{Type: RecordGroup, GroupKey: "g1", TaskKeys: []string{"t1"}, AnswerKind: AnswerKind("bogus"), Subjects: []AnswerSubject{{SubjectKey: "s1", Kind: SubjectPet, PetID: "p1"}}, Scope: ScopeFull}},
			{Type: RecordCoverage, Coverage: &CoverageRecord{Type: RecordCoverage, Tasks: []TaskCoverage{}}},
			{Type: RecordEnd},
		}
		if _, err := ValidateResponse(records); err == nil || !strings.Contains(err.Error(), "answer_kind") {
			t.Fatalf("expected answer_kind error, got %v", err)
		}
	})
	t.Run("empty subjects", func(t *testing.T) {
		records := []ProtocolRecord{
			{Type: RecordHeader, Header: &HeaderRecord{Type: RecordHeader, SchemaVersion: RecordArrayV1, Action: ActionFinalAnswer, TaskUpdates: []TaskUpdate{}}},
			{Type: RecordGroup, Group: &GroupRecord{Type: RecordGroup, GroupKey: "g1", TaskKeys: []string{"t1"}, AnswerKind: AnswerHealth, Subjects: nil, Scope: ScopeFull}},
			{Type: RecordCoverage, Coverage: &CoverageRecord{Type: RecordCoverage, Tasks: []TaskCoverage{}}},
			{Type: RecordEnd},
		}
		if _, err := ValidateResponse(records); err == nil || !strings.Contains(err.Error(), "subjects") {
			t.Fatalf("expected subjects error, got %v", err)
		}
	})
	t.Run("pet subject missing pet_id", func(t *testing.T) {
		records := []ProtocolRecord{
			{Type: RecordHeader, Header: &HeaderRecord{Type: RecordHeader, SchemaVersion: RecordArrayV1, Action: ActionFinalAnswer, TaskUpdates: []TaskUpdate{}}},
			{Type: RecordGroup, Group: &GroupRecord{Type: RecordGroup, GroupKey: "g1", TaskKeys: []string{"t1"}, AnswerKind: AnswerHealth, Subjects: []AnswerSubject{{SubjectKey: "s1", Kind: SubjectPet}}, Scope: ScopeFull}},
			{Type: RecordCoverage, Coverage: &CoverageRecord{Type: RecordCoverage, Tasks: []TaskCoverage{}}},
			{Type: RecordEnd},
		}
		if _, err := ValidateResponse(records); err == nil || !strings.Contains(err.Error(), "pet_id") {
			t.Fatalf("expected pet_id error, got %v", err)
		}
	})
}

func TestValidateResponseSegmentValidation(t *testing.T) {
	t.Run("unknown group", func(t *testing.T) {
		records := []ProtocolRecord{
			{Type: RecordHeader, Header: &HeaderRecord{Type: RecordHeader, SchemaVersion: RecordArrayV1, Action: ActionFinalAnswer, TaskUpdates: []TaskUpdate{}}},
			{Type: RecordSegment, Segment: &SegmentRecord{Type: RecordSegment, SegmentKey: "s1", GroupKey: "ghost", Field: "reply", Text: "x", BasisKind: BasisGeneralKnowledge}},
			{Type: RecordCoverage, Coverage: &CoverageRecord{Type: RecordCoverage, Tasks: []TaskCoverage{}}},
			{Type: RecordEnd},
		}
		if _, err := ValidateResponse(records); err == nil || !strings.Contains(err.Error(), "unknown group") {
			t.Fatalf("expected unknown group error, got %v", err)
		}
	})
	t.Run("field not allowed for kind", func(t *testing.T) {
		records := []ProtocolRecord{
			{Type: RecordHeader, Header: &HeaderRecord{Type: RecordHeader, SchemaVersion: RecordArrayV1, Action: ActionFinalAnswer, TaskUpdates: []TaskUpdate{}}},
			{Type: RecordGroup, Group: &GroupRecord{Type: RecordGroup, GroupKey: "g1", TaskKeys: []string{"t1"}, AnswerKind: AnswerCasual, Subjects: []AnswerSubject{{SubjectKey: "s1", Kind: SubjectPet, PetID: "p1"}}, Scope: ScopeFull}},
			{Type: RecordSegment, Segment: &SegmentRecord{Type: RecordSegment, SegmentKey: "s1", GroupKey: "g1", Field: "observation", Text: "x", BasisKind: BasisGeneralKnowledge}},
			{Type: RecordCoverage, Coverage: &CoverageRecord{Type: RecordCoverage, Tasks: []TaskCoverage{}}},
			{Type: RecordEnd},
		}
		if _, err := ValidateResponse(records); err == nil || !strings.Contains(err.Error(), "not allowed") {
			t.Fatalf("expected field-not-allowed error, got %v", err)
		}
	})
	t.Run("business fact requires evidence", func(t *testing.T) {
		records := []ProtocolRecord{
			{Type: RecordHeader, Header: &HeaderRecord{Type: RecordHeader, SchemaVersion: RecordArrayV1, Action: ActionFinalAnswer, TaskUpdates: []TaskUpdate{{TaskKey: "t1", Goal: "查记录"}}}},
			{Type: RecordGroup, Group: &GroupRecord{Type: RecordGroup, GroupKey: "g1", TaskKeys: []string{"t1"}, AnswerKind: AnswerFact, Subjects: []AnswerSubject{{SubjectKey: "s1", Kind: SubjectPet, PetID: "p1"}}, Scope: ScopeFull}},
			{Type: RecordSegment, Segment: &SegmentRecord{Type: RecordSegment, SegmentKey: "s1", GroupKey: "g1", SubjectKeys: []string{"s1"}, Field: "result", Text: "没有记录", BasisKind: BasisBusinessFact}},
			{Type: RecordCoverage, Coverage: &CoverageRecord{Type: RecordCoverage, Tasks: []TaskCoverage{{TaskKey: "t1", AnswerGroupKeys: []string{"g1"}}}}},
			{Type: RecordEnd},
		}
		if _, err := ValidateResponse(records); err == nil || !strings.Contains(err.Error(), "evidence_refs") {
			t.Fatalf("expected evidence_refs error, got %v", err)
		}
	})
}

func TestValidateResponseRiskValidation(t *testing.T) {
	t.Run("duplicate risk per subject", func(t *testing.T) {
		records := []ProtocolRecord{
			{Type: RecordHeader, Header: &HeaderRecord{Type: RecordHeader, SchemaVersion: RecordArrayV1, Action: ActionFinalAnswer, TaskUpdates: []TaskUpdate{}}},
			{Type: RecordGroup, Group: &GroupRecord{Type: RecordGroup, GroupKey: "g1", TaskKeys: []string{"t1"}, AnswerKind: AnswerHealth, Subjects: []AnswerSubject{{SubjectKey: "s1", Kind: SubjectPet, PetID: "p1"}}, Scope: ScopeFull}},
			{Type: RecordRisk, Risk: &RiskRecord{Type: RecordRisk, GroupKey: "g1", SubjectKey: "s1", Level: RiskGreen}},
			{Type: RecordRisk, Risk: &RiskRecord{Type: RecordRisk, GroupKey: "g1", SubjectKey: "s1", Level: RiskYellow}},
			{Type: RecordCoverage, Coverage: &CoverageRecord{Type: RecordCoverage, Tasks: []TaskCoverage{}}},
			{Type: RecordEnd},
		}
		if _, err := ValidateResponse(records); err == nil || !strings.Contains(err.Error(), "duplicate risk") {
			t.Fatalf("expected duplicate risk error, got %v", err)
		}
	})
}

func TestValidateResponseCallValidation(t *testing.T) {
	t.Run("invalid arguments", func(t *testing.T) {
		records := []ProtocolRecord{
			{Type: RecordHeader, Header: &HeaderRecord{Type: RecordHeader, SchemaVersion: RecordArrayV1, Action: ActionCallTools, TaskUpdates: []TaskUpdate{}}},
			{Type: RecordCall, Call: &CallRecord{Type: RecordCall, CallKey: "c1", TaskKeys: []string{"t1"}, ToolName: "read_records", CatalogVersion: "v1", Arguments: []byte(`not-json`)}},
			{Type: RecordCoverage, Coverage: &CoverageRecord{Type: RecordCoverage, Tasks: []TaskCoverage{}}},
			{Type: RecordEnd},
		}
		if _, err := ValidateResponse(records); err == nil || !strings.Contains(err.Error(), "arguments") {
			t.Fatalf("expected arguments error, got %v", err)
		}
	})
}

func TestRecordArrayParserPartialTextStreamsSegmentBody(t *testing.T) {
	p := NewRecordArrayParser()
	if _, err := p.Feed(`[{"type":"header","schema_version":"record_array_v1","action":"final_answer","task_updates":[]},{"type":"group","group_key":"g1","task_keys":["t1"],"answer_kind":"casual","subjects":[],"scope":"full"},{"type":"segment","segment_key":"s1","group_key":"g1","subject_keys":[],"field":"reply","text":"多喝温水`); err != nil {
		t.Fatal(err)
	}
	text, ok := p.PartialText()
	if !ok || text != "多喝温水" {
		t.Fatalf("partial text = (%q, %v), want 多喝温水", text, ok)
	}
	records, err := p.Feed(`，注意观察精神。","basis_kind":"general_knowledge"},{"type":"coverage","tasks":[]},{"type":"end"}]`)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 3 || records[0].Segment == nil {
		t.Fatalf("closed records = %+v, want segment, coverage, end", records)
	}
	if records[0].Segment.Text != "多喝温水，注意观察精神。" {
		t.Fatalf("segment text = %q", records[0].Segment.Text)
	}
	if text, ok := p.PartialText(); ok {
		t.Fatalf("closed record should not expose partial text %q", text)
	}
}

func TestRecordArrayParserPartialTextDecodesEscapes(t *testing.T) {
	p := NewRecordArrayParser()
	if _, err := p.Feed(`[{"type":"segment","segment_key":"s1","text":"a\nb\"c`); err != nil {
		t.Fatal(err)
	}
	text, ok := p.PartialText()
	if !ok || text != "a\nb\"c" {
		t.Fatalf("partial text = (%q, %v), want %q", text, ok, "a\nb\"c")
	}
}

func TestRecordArrayParserPartialTextDropsIncompleteRune(t *testing.T) {
	full := "观察"
	p := NewRecordArrayParser()
	if _, err := p.Feed(`[{"type":"segment","segment_key":"s1","text":"` + full[:len(full)-1]); err != nil {
		t.Fatal(err)
	}
	text, ok := p.PartialText()
	if !ok || text != "观" {
		t.Fatalf("partial text = (%q, %v), want 观", text, ok)
	}
}

func TestRecordArrayParserPartialTextIgnoresOtherRecords(t *testing.T) {
	p := NewRecordArrayParser()
	if _, err := p.Feed(`[{"type":"question","question_key":"q1","text":"请问是哪只宠物`); err != nil {
		t.Fatal(err)
	}
	if text, ok := p.PartialText(); ok {
		t.Fatalf("question record exposed partial text %q", text)
	}
}
