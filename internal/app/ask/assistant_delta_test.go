package ask

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"
)

func finalAnswerRecordsWithText(text string) []ProtocolRecord {
	group := ProtocolRecord{Type: RecordGroup, Group: &GroupRecord{
		Type: RecordGroup, GroupKey: "g1", TaskKeys: []string{"t1"},
		AnswerKind: AnswerCasual, Scope: ScopeFull,
		Subjects: []AnswerSubject{{SubjectKey: "s1", Kind: SubjectUnresolved, Description: "某只宠物"}},
	}}
	segment := ProtocolRecord{Type: RecordSegment, Segment: &SegmentRecord{
		Type: RecordSegment, SegmentKey: "seg1", GroupKey: "g1", SubjectKeys: []string{"s1"},
		Field: "reply", Text: text, BasisKind: BasisGeneralKnowledge,
	}}
	coverage := ProtocolRecord{Type: RecordCoverage, Coverage: &CoverageRecord{
		Type:  RecordCoverage,
		Tasks: []TaskCoverage{{TaskKey: "t1", AnswerGroupKeys: []string{"g1"}}},
	}}
	return []ProtocolRecord{headerRecord(ActionFinalAnswer), group, segment, coverage, endRecord()}
}

func assistantDeltaEvents(events []Event) []Event {
	result := make([]Event, 0, len(events))
	for _, event := range events {
		if event.Type == "assistant.delta" {
			result = append(result, event)
		}
	}
	return result
}

func TestSplitAssistantAnswerChunksKeepsUnicodeBoundaries(t *testing.T) {
	answer := strings.Repeat("观察", 15) + "🐶需要复诊"
	chunks := splitAssistantAnswerChunks(answer)
	if joined := strings.Join(chunks, ""); joined != answer {
		t.Fatalf("joined chunks = %q, want %q", joined, answer)
	}
	if len(chunks) < 2 {
		t.Fatalf("chunks = %d, want multiple chunks", len(chunks))
	}
	for _, chunk := range chunks {
		if !utf8.ValidString(chunk) {
			t.Fatalf("chunk is not valid utf8: %q", chunk)
		}
		if count := len([]rune(chunk)); count > MaxAssistantDeltaChunkRunes {
			t.Fatalf("chunk runes = %d, want <= %d", count, MaxAssistantDeltaChunkRunes)
		}
	}
}

func TestSplitAssistantAnswerChunksLimitsChunkCount(t *testing.T) {
	answer := strings.Repeat("观", MaxAssistantDeltaChunkRunes*MaxAssistantDeltaChunks*2)
	chunks := splitAssistantAnswerChunks(answer)
	if len(chunks) > MaxAssistantDeltaChunks {
		t.Fatalf("chunks = %d, want <= %d", len(chunks), MaxAssistantDeltaChunks)
	}
	if joined := strings.Join(chunks, ""); joined != answer {
		t.Fatalf("joined chunks dropped content: %d runes, want %d", len([]rune(joined)), len([]rune(answer)))
	}
	size := (len([]rune(answer)) + MaxAssistantDeltaChunks - 1) / MaxAssistantDeltaChunks
	for _, chunk := range chunks {
		if chunk == "" {
			t.Fatal("chunk is empty")
		}
		if count := len([]rune(chunk)); count > size {
			t.Fatalf("chunk runes = %d, want <= %d", count, size)
		}
	}
}

func TestSplitAssistantAnswerChunksIgnoresEmptyAnswer(t *testing.T) {
	if chunks := splitAssistantAnswerChunks("   "); len(chunks) != 0 {
		t.Fatalf("chunks = %#v, want none", chunks)
	}
}

func TestProcessRunV2EmitsAssistantDeltaBeforeCompleted(t *testing.T) {
	answer := strings.Repeat("多喝温水，注意观察精神变化。", 8)
	model := &scriptedModel{responses: [][]ProtocolRecord{finalAnswerRecordsWithText(answer)}}
	service, repository := newV2Service(t, model, &fakeBusinessRead{})

	created, err := service.CreateSession(context.Background(), "family-1", "user-1", "pet-1", "你好", "create-delta-1")
	if err != nil {
		t.Fatal(err)
	}
	processed, err := service.ProcessRun(context.Background(), "family-1", created.Session.ID, created.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if processed.Run.Status != RunCompleted {
		t.Fatalf("run status = %s, want completed", processed.Run.Status)
	}
	messages, err := repository.ListMessages(context.Background(), created.Session.ID, created.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	assistantMessageID := ""
	for _, message := range messages {
		if message.Role == "assistant" {
			assistantMessageID = message.ID
		}
	}
	if assistantMessageID == "" {
		t.Fatalf("messages = %+v, want assistant message", messages)
	}
	expected := strings.TrimSpace(answer)
	final := lastEvent(processed.Events)
	if final.Type != "assistant.completed" {
		t.Fatalf("last event = %s, want assistant.completed", final.Type)
	}
	if data := eventDataMap(t, final); data["answer"] != expected {
		t.Fatalf("answer = %v, want %q", data["answer"], expected)
	}
	deltas := assistantDeltaEvents(processed.Events)
	if len(deltas) < 2 {
		t.Fatalf("deltas = %d, want multiple chunks", len(deltas))
	}
	var joined strings.Builder
	for index, event := range deltas {
		data := eventDataMap(t, event)
		delta, ok := data["delta"].(string)
		if !ok || delta == "" {
			t.Fatalf("delta data = %#v", data)
		}
		if data["message_id"] != assistantMessageID {
			t.Fatalf("delta message_id = %v, want %s", data["message_id"], assistantMessageID)
		}
		if count := len([]rune(delta)); count > MaxAssistantDeltaChunkRunes {
			t.Fatalf("delta runes = %d, want <= %d", count, MaxAssistantDeltaChunkRunes)
		}
		if event.Sequence >= final.Sequence {
			t.Fatalf("delta sequence = %d, want < terminal %d", event.Sequence, final.Sequence)
		}
		if index > 0 && event.Sequence != deltas[index-1].Sequence+1 {
			t.Fatalf("delta sequences = %d then %d, want contiguous", deltas[index-1].Sequence, event.Sequence)
		}
		joined.WriteString(delta)
	}
	if joined.String() != expected {
		t.Fatalf("joined deltas = %q, want %q", joined.String(), expected)
	}
	persisted, err := repository.ListEvents(context.Background(), created.Session.ID, created.Run.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if last := lastEvent(persisted); last.Type != "assistant.completed" {
		t.Fatalf("last persisted event = %s, want assistant.completed", last.Type)
	}
	if len(assistantDeltaEvents(persisted)) != len(deltas) {
		t.Fatalf("persisted deltas = %d, want %d", len(assistantDeltaEvents(persisted)), len(deltas))
	}
}

func TestProcessRunV2SkipsAssistantDeltaForQuestion(t *testing.T) {
	model := &scriptedModel{responses: [][]ProtocolRecord{requestInputRecords()}}
	service, repository := newV2Service(t, model, &fakeBusinessRead{})

	created, err := service.CreateSession(context.Background(), "family-1", "user-1", "pet-1", "请问是哪只宠物？", "create-delta-2")
	if err != nil {
		t.Fatal(err)
	}
	processed, err := service.ProcessRun(context.Background(), "family-1", created.Session.ID, created.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if processed.Run.Status != RunWaitingInput {
		t.Fatalf("run status = %s, want waiting_input", processed.Run.Status)
	}
	if deltas := assistantDeltaEvents(processed.Events); len(deltas) != 0 {
		t.Fatalf("deltas = %+v, want none", deltas)
	}
	persisted, err := repository.ListEvents(context.Background(), created.Session.ID, created.Run.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if deltas := assistantDeltaEvents(persisted); len(deltas) != 0 {
		t.Fatalf("persisted deltas = %+v, want none", deltas)
	}
}

// thinkingAgentModel 模拟流式决策模型：先按分片回传推理正文，再返回完整记录。
type thinkingAgentModel struct {
	records  []ProtocolRecord
	thinking []string
}

func (m *thinkingAgentModel) Step(_ context.Context, input StepInput) (ModelStepResult, error) {
	for _, delta := range m.thinking {
		if input.OnThinkingDelta == nil {
			continue
		}
		if err := input.OnThinkingDelta(delta); err != nil {
			return ModelStepResult{}, err
		}
	}
	return ModelStepResult{Records: m.records, Usage: ModelUsage{InputTokens: 10, OutputTokens: 5, TotalTokens: 15, Complete: true}}, nil
}

func assistantThinkingEvents(events []Event) []Event {
	result := make([]Event, 0, len(events))
	for _, event := range events {
		if event.Type == "assistant.thinking" {
			result = append(result, event)
		}
	}
	return result
}

func TestProcessRunV2CoalescesThinkingPreview(t *testing.T) {
	long := strings.Repeat("先看精神", 30)
	model := &thinkingAgentModel{records: finalAnswerRecordsWithText("旺仔今天状态还行"), thinking: []string{long, "再看饮水"}}
	service, repository := newV2Service(t, model, &fakeBusinessRead{})

	created, err := service.CreateSession(context.Background(), "family-1", "user-1", "pet-1", "旺仔今天怎么样", "create-thinking-1")
	if err != nil {
		t.Fatal(err)
	}
	processed, err := service.ProcessRun(context.Background(), "family-1", created.Session.ID, created.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if processed.Run.Status != RunCompleted {
		t.Fatalf("run status = %s, want completed", processed.Run.Status)
	}
	final := lastEvent(processed.Events)
	if final.Type != "assistant.completed" {
		t.Fatalf("last event = %s, want assistant.completed", final.Type)
	}
	thinking := assistantThinkingEvents(processed.Events)
	if len(thinking) != 2 {
		t.Fatalf("thinking events = %d, want 2 coalesced chunks", len(thinking))
	}
	var joined strings.Builder
	for index, event := range thinking {
		data := eventDataMap(t, event)
		delta, ok := data["delta"].(string)
		if !ok || delta == "" {
			t.Fatalf("thinking data = %#v", data)
		}
		if event.Sequence >= final.Sequence {
			t.Fatalf("thinking sequence = %d, want < terminal %d", event.Sequence, final.Sequence)
		}
		if index > 0 && event.Sequence != thinking[index-1].Sequence+1 {
			t.Fatalf("thinking sequences = %d then %d, want contiguous", thinking[index-1].Sequence, event.Sequence)
		}
		joined.WriteString(delta)
	}
	if joined.String() != long+"再看饮水" {
		t.Fatalf("joined thinking = %q", joined.String())
	}
	persisted, err := repository.ListEvents(context.Background(), created.Session.ID, created.Run.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if persistedThinking := assistantThinkingEvents(persisted); len(persistedThinking) != 2 {
		t.Fatalf("persisted thinking = %d, want 2", len(persistedThinking))
	}
}

// streamingAgentModel 模拟流式决策模型：final_answer 时先按分片回传正文预览，
// 再返回完整记录，复现适配器「动作先行 + 正文增量」的发布顺序。
type streamingAgentModel struct {
	records []ProtocolRecord
	deltas  []string
}

func (m *streamingAgentModel) Step(_ context.Context, input StepInput) (ModelStepResult, error) {
	finalAnswer := len(m.records) > 0 && m.records[0].Header != nil && m.records[0].Header.Action == ActionFinalAnswer
	if input.OnAnswerDelta != nil && finalAnswer {
		for _, delta := range m.deltas {
			if err := input.OnAnswerDelta(delta); err != nil {
				return ModelStepResult{}, err
			}
		}
	}
	return ModelStepResult{Records: m.records, Usage: ModelUsage{InputTokens: 10, OutputTokens: 5, TotalTokens: 15, Complete: true}}, nil
}

func TestProcessRunV2PublishesStreamedPreviewWithoutChunking(t *testing.T) {
	answer := strings.Repeat("多喝温水，注意观察精神变化。", 8)
	streamed := []string{"多喝温水，注意观察精神变化。", "多喝温水，注意观察精神变化。"}
	model := &streamingAgentModel{records: finalAnswerRecordsWithText(answer), deltas: streamed}
	service, repository := newV2Service(t, model, &fakeBusinessRead{})

	created, err := service.CreateSession(context.Background(), "family-1", "user-1", "pet-1", "你好", "create-stream-1")
	if err != nil {
		t.Fatal(err)
	}
	processed, err := service.ProcessRun(context.Background(), "family-1", created.Session.ID, created.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if processed.Run.Status != RunCompleted {
		t.Fatalf("run status = %s, want completed", processed.Run.Status)
	}
	final := lastEvent(processed.Events)
	if final.Type != "assistant.completed" {
		t.Fatalf("last event = %s, want assistant.completed", final.Type)
	}
	deltas := assistantDeltaEvents(processed.Events)
	if len(deltas) != len(streamed) {
		t.Fatalf("deltas = %d, want %d streamed previews without post-hoc chunking", len(deltas), len(streamed))
	}
	messages, err := repository.ListMessages(context.Background(), created.Session.ID, created.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	assistantMessageID := ""
	for _, message := range messages {
		if message.Role == "assistant" {
			assistantMessageID = message.ID
		}
	}
	if assistantMessageID == "" {
		t.Fatalf("messages = %+v, want assistant message", messages)
	}
	for index, event := range deltas {
		data := eventDataMap(t, event)
		if data["delta"] != streamed[index] {
			t.Fatalf("delta[%d] = %v, want %q", index, data["delta"], streamed[index])
		}
		if data["message_id"] != assistantMessageID {
			t.Fatalf("delta message_id = %v, want %s", data["message_id"], assistantMessageID)
		}
		if event.Sequence >= final.Sequence {
			t.Fatalf("delta sequence = %d, want < terminal %d", event.Sequence, final.Sequence)
		}
	}
	persisted, err := repository.ListEvents(context.Background(), created.Session.ID, created.Run.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if persistedDeltas := assistantDeltaEvents(persisted); len(persistedDeltas) != len(streamed) {
		t.Fatalf("persisted deltas = %d, want %d", len(persistedDeltas), len(streamed))
	}
}

func TestProcessRunV2SkipsStreamedPreviewForQuestion(t *testing.T) {
	model := &streamingAgentModel{records: requestInputRecords(), deltas: []string{"不该出现"}}
	service, _ := newV2Service(t, model, &fakeBusinessRead{})

	created, err := service.CreateSession(context.Background(), "family-1", "user-1", "pet-1", "请问是哪只宠物？", "create-stream-2")
	if err != nil {
		t.Fatal(err)
	}
	processed, err := service.ProcessRun(context.Background(), "family-1", created.Session.ID, created.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if processed.Run.Status != RunWaitingInput {
		t.Fatalf("run status = %s, want waiting_input", processed.Run.Status)
	}
	if deltas := assistantDeltaEvents(processed.Events); len(deltas) != 0 {
		t.Fatalf("deltas = %+v, want none", deltas)
	}
}

func TestProcessRunV2SkipsAssistantDeltaForEscalation(t *testing.T) {
	model := &scriptedModel{responses: [][]ProtocolRecord{finalAnswerRecords()}}
	service, repository := newV2Service(t, model, &fakeBusinessRead{})

	created, err := service.CreateSession(context.Background(), "family-1", "user-1", "pet-1", "团子呼吸困难", "create-delta-3")
	if err != nil {
		t.Fatal(err)
	}
	processed, err := service.ProcessRun(context.Background(), "family-1", created.Session.ID, created.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	final := lastEvent(processed.Events)
	if final.Type != "risk.escalated" {
		t.Fatalf("last event = %s, want risk.escalated", final.Type)
	}
	if deltas := assistantDeltaEvents(processed.Events); len(deltas) != 0 {
		t.Fatalf("deltas = %+v, want none", deltas)
	}
	persisted, err := repository.ListEvents(context.Background(), created.Session.ID, created.Run.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if deltas := assistantDeltaEvents(persisted); len(deltas) != 0 {
		t.Fatalf("persisted deltas = %+v, want none", deltas)
	}
}
