package ai

import (
	"context"
	"testing"

	askapp "github.com/balancetheworld/wechat-pet/internal/app/ask"
)

// fakeProvider 实现 Provider，记录最近一次 Complete 请求并返回固定响应。
type fakeProvider struct {
	lastRequest Request
	response    Response
	err         error
	callCount   int
}

func (f *fakeProvider) Complete(_ context.Context, request Request) (Response, error) {
	f.callCount++
	f.lastRequest = request
	return f.response, f.err
}

func (f *fakeProvider) Stream(_ context.Context, _ Request, _ func(StreamEvent) error) (Response, error) {
	return Response{}, context.Canceled
}

const validFinalAnswerJSON = `[
{"type":"header","schema_version":"record_array_v1","action":"final_answer","task_updates":[]},
{"type":"group","group_key":"g1","task_keys":["t1"],"answer_kind":"casual","subjects":[{"subject_key":"s1","kind":"unresolved","description":"某只宠物"}],"scope":"full"},
{"type":"segment","segment_key":"seg1","group_key":"g1","subject_keys":["s1"],"field":"reply","text":"你好呀","basis_kind":"general_knowledge"},
{"type":"coverage","tasks":[{"task_key":"t1","answer_group_keys":["g1"]}]},
{"type":"end"}
]`

func TestBlocksToMessagesSeparatesInstructions(t *testing.T) {
	blocks := []askapp.ContextBlock{
		{Layer: askapp.LayerControlInstructions, Kind: "instruction", Text: "安全规则"},
		{Layer: askapp.LayerReferenceData, Kind: "task", Text: "任务项"},
		{Layer: askapp.LayerHistory, Kind: "history", Text: "历史消息"},
		{Layer: askapp.LayerCurrentTask, Kind: "current_turn", Text: "当前问题"},
		{Layer: askapp.LayerToolInteractions, Kind: "tool_result", Text: "工具结果"},
	}
	messages := blocksToMessages(blocks)
	if len(messages) != 2 {
		t.Fatalf("messages = %d, want 2 (system + user)", len(messages))
	}
	if messages[0].Role != "system" || messages[0].Content != "安全规则" {
		t.Fatalf("system message = %+v", messages[0])
	}
	// user 消息按层顺序拼接：参考数据 -> 历史 -> 当前任务 -> 工具交互。
	if messages[1].Role != "user" {
		t.Fatalf("second message role = %q, want user", messages[1].Role)
	}
	body := messages[1].Content
	idxTask := indexOf(body, "任务项")
	idxHistory := indexOf(body, "历史消息")
	idxCurrent := indexOf(body, "当前问题")
	idxTool := indexOf(body, "工具结果")
	if !(idxTask >= 0 && idxHistory > idxTask && idxCurrent > idxHistory && idxTool > idxCurrent) {
		t.Fatalf("user message layer order wrong: %q", body)
	}
}

func TestBlocksToMessagesKindInstructionGoesSystem(t *testing.T) {
	// Kind=="instruction" 即使层级非受控指令，也进入 system。
	blocks := []askapp.ContextBlock{{Layer: askapp.LayerReferenceData, Kind: "instruction", Text: "输出协议"}}
	messages := blocksToMessages(blocks)
	if len(messages) != 1 || messages[0].Role != "system" {
		t.Fatalf("instruction kind should route to system, got %+v", messages)
	}
}

func TestBlocksToMessagesEmpty(t *testing.T) {
	if got := blocksToMessages(nil); len(got) != 0 {
		t.Fatalf("empty blocks should yield no messages, got %d", len(got))
	}
}

func TestToolsToSpecs(t *testing.T) {
	tools := []askapp.Tool{
		{
			Name:       "search_records",
			Version:    "v1",
			UseCases:   []string{"查记录", "按时间过滤"},
			Parameters: []byte(`{"type":"object","properties":{"pet_id":{"type":"string"}}}`),
		},
		{Name: "bad_params", Version: "v1", Parameters: []byte(`not-json`)},
		{Name: "no_params", Version: "v1"},
	}
	specs := toolsToSpecs(tools)
	if len(specs) != 3 {
		t.Fatalf("specs = %d, want 3", len(specs))
	}
	if specs[0].Name != "search_records" || specs[0].Description != "查记录；按时间过滤" {
		t.Fatalf("spec[0] = %+v", specs[0])
	}
	if m, ok := specs[0].Parameters.(map[string]any); !ok || m["type"] != "object" {
		t.Fatalf("valid JSON parameters should map to object, got %+v", specs[0].Parameters)
	}
	// 非法 JSON 与缺失参数都回退到空 object schema。
	for _, idx := range []int{1, 2} {
		m, ok := specs[idx].Parameters.(map[string]any)
		if !ok || m["type"] != "object" {
			t.Fatalf("spec[%d] parameters should fall back to empty object, got %+v", idx, specs[idx].Parameters)
		}
	}
}

func TestParseRecordArrayValid(t *testing.T) {
	records, err := parseRecordArray(validFinalAnswerJSON)
	if err != nil {
		t.Fatalf("parseRecordArray error: %v", err)
	}
	if len(records) != 5 {
		t.Fatalf("records = %d, want 5", len(records))
	}
	if records[0].Type != askapp.RecordHeader || records[0].Header == nil {
		t.Fatalf("first record should be header, got %+v", records[0])
	}
	if records[len(records)-1].Type != askapp.RecordEnd {
		t.Fatalf("last record should be end, got %+v", records[len(records)-1])
	}
}

func TestParseRecordArrayIncomplete(t *testing.T) {
	if _, err := parseRecordArray(`[{"type":"header","schema_version":"record_array_v1","action":"final_answer","task_updates":[]}`); err == nil {
		t.Fatal("incomplete array should error")
	}
}

func TestUnwrapRecordArrayTopLevelArray(t *testing.T) {
	got, err := unwrapRecordArray(validFinalAnswerJSON)
	if err != nil {
		t.Fatalf("unwrap error: %v", err)
	}
	if got != validFinalAnswerJSON {
		t.Fatal("top-level array should be returned unchanged")
	}
}

func TestUnwrapRecordArrayWrappedItems(t *testing.T) {
	// 非 strict 下模型可能把 schema 关键词 items 误当输出结构（文档 12.2 实测）。
	got, err := unwrapRecordArray(`{"items": ` + validFinalAnswerJSON + `}`)
	if err != nil {
		t.Fatalf("unwrap error: %v", err)
	}
	if got != validFinalAnswerJSON {
		t.Fatalf("wrapped array not extracted, got %q", got)
	}
}

func TestUnwrapRecordArrayWrappedRecords(t *testing.T) {
	got, err := unwrapRecordArray(`{"records": ` + validFinalAnswerJSON + `}`)
	if err != nil {
		t.Fatalf("unwrap error: %v", err)
	}
	if got != validFinalAnswerJSON {
		t.Fatalf("records-wrapped array not extracted, got %q", got)
	}
}

func TestUnwrapRecordArrayRejectsAmbiguousAndInvalid(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"multiple array fields", `{"items":[],"meta":[1,2]}`},
		{"no array field", `{"foo":"bar"}`},
		{"invalid json", `{broken`},
		{"scalar", `"just a string"`},
		{"empty", ``},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := unwrapRecordArray(c.in); err == nil {
				t.Fatalf("unwrap(%q) should error", c.in)
			}
		})
	}
}

func TestParseRecordArrayUnwrapsItemsWrapper(t *testing.T) {
	// 端到端：{"items":[...]} 包裹应被容错解析出 5 条记录。
	records, err := parseRecordArray(`{"items": ` + validFinalAnswerJSON + `}`)
	if err != nil {
		t.Fatalf("parseRecordArray wrapped error: %v", err)
	}
	if len(records) != 5 {
		t.Fatalf("records = %d, want 5", len(records))
	}
	if records[0].Type != askapp.RecordHeader || records[len(records)-1].Type != askapp.RecordEnd {
		t.Fatalf("wrapped records should still be header...end, got %v ... %v", records[0].Type, records[len(records)-1].Type)
	}
}

func TestMaxOutputTokens(t *testing.T) {
	cases := []struct {
		params []Parameter
		want   int
	}{
		{nil, defaultAgentStepOutputTokens},
		{[]Parameter{{Name: "temperature", Value: 0.7}}, defaultAgentStepOutputTokens},
		{[]Parameter{{Name: "max_output_tokens", Value: float64(2048)}}, 2048},
		{[]Parameter{{Name: "max_output_tokens", Value: 8192}}, 8192},
	}
	for _, c := range cases {
		if got := maxOutputTokens(Profile{Parameters: c.params}); got != c.want {
			t.Fatalf("maxOutputTokens(%v) = %d, want %d", c.params, got, c.want)
		}
	}
}

func TestAgentModelAdapterStepBuildsRequestAndParses(t *testing.T) {
	provider := &fakeProvider{response: Response{Text: validFinalAnswerJSON}}
	profile := Profile{
		ProviderID: "openai", Model: "deepseek-flash", Version: "v1",
		AdapterVersion: "adapter-v1",
		Parameters:     []Parameter{{Name: "max_output_tokens", Value: float64(2048)}},
	}
	adapter := NewAgentModelAdapter(provider, profile)
	input := askapp.StepInput{
		Blocks: []askapp.ContextBlock{{Layer: askapp.LayerCurrentTask, Kind: "current_turn", Text: "你好"}},
		Tools:  []askapp.Tool{{Name: "search_records", Version: "v1"}},
	}
	records, err := adapter.Step(context.Background(), input)
	if err != nil {
		t.Fatalf("Step error: %v", err)
	}
	if len(records) != 5 {
		t.Fatalf("records = %d, want 5", len(records))
	}
	req := provider.lastRequest
	if req.Purpose != PurposeAgentStep {
		t.Fatalf("purpose = %q, want agent_step", req.Purpose)
	}
	if req.AttemptID != "attempt-1" {
		t.Fatalf("attempt = %q, want attempt-1", req.AttemptID)
	}
	if req.MaxOutputTokens != 2048 {
		t.Fatalf("max_output_tokens = %d, want 2048", req.MaxOutputTokens)
	}
	schema, ok := req.ResponseSchema.(map[string]any)
	if !ok || schema["type"] != "array" {
		t.Fatalf("response schema should be record_array_v1 array, got %v", req.ResponseSchema)
	}
	if len(req.Messages) == 0 || len(req.Tools) != 1 {
		t.Fatalf("messages=%d tools=%d, want 1 message and 1 tool", len(req.Messages), len(req.Tools))
	}
}

func TestAgentModelAdapterStepAssignsDistinctAttemptIDs(t *testing.T) {
	provider := &fakeProvider{response: Response{Text: validFinalAnswerJSON}}
	adapter := NewAgentModelAdapter(provider, Profile{})
	if _, err := adapter.Step(context.Background(), askapp.StepInput{}); err != nil {
		t.Fatal(err)
	}
	first := provider.lastRequest.AttemptID
	if _, err := adapter.Step(context.Background(), askapp.StepInput{}); err != nil {
		t.Fatal(err)
	}
	if provider.lastRequest.AttemptID == first {
		t.Fatalf("attempt IDs should be distinct, both %q", first)
	}
	if provider.callCount != 2 {
		t.Fatalf("call count = %d, want 2", provider.callCount)
	}
}

func TestAgentModelAdapterStepPropagatesProviderError(t *testing.T) {
	provider := &fakeProvider{err: NewProviderError(ErrProviderTimeout, true, 0, context.DeadlineExceeded)}
	adapter := NewAgentModelAdapter(provider, Profile{})
	if _, err := adapter.Step(context.Background(), askapp.StepInput{}); err == nil {
		t.Fatal("provider error should propagate")
	}
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
