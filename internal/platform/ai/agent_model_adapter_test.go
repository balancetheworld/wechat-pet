package ai

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	askapp "github.com/balancetheworld/wechat-pet/internal/app/ask"
)

type memoryAttemptRepository struct {
	mu       sync.Mutex
	attempts map[string]askapp.Attempt
}

func (r *memoryAttemptRepository) CreateAttempt(_ context.Context, attempt askapp.Attempt) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.attempts == nil {
		r.attempts = make(map[string]askapp.Attempt)
	}
	if _, exists := r.attempts[attempt.ID]; exists {
		return askapp.ErrAttemptStateConflict
	}
	attempt.Status = askapp.AttemptQueued
	r.attempts[attempt.ID] = attempt
	return nil
}

func (r *memoryAttemptRepository) GetAttempt(_ context.Context, id string) (askapp.Attempt, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	attempt, ok := r.attempts[id]
	if !ok {
		return askapp.Attempt{}, askapp.ErrAttemptNotFound
	}
	return attempt, nil
}

func (r *memoryAttemptRepository) ListAttempts(_ context.Context, runID string) ([]askapp.Attempt, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make([]askapp.Attempt, 0)
	for _, attempt := range r.attempts {
		if attempt.RunID == runID {
			result = append(result, attempt)
		}
	}
	return result, nil
}

func (r *memoryAttemptRepository) TransitionAttempt(_ context.Context, id string, from, to askapp.AttemptStatus, errorCode, requestID, usage string, at time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	attempt, ok := r.attempts[id]
	if !ok || attempt.Status != from {
		return askapp.ErrAttemptStateConflict
	}
	attempt.Status = to
	attempt.ErrorCode = errorCode
	attempt.RequestID = requestID
	attempt.Usage = usage
	if to == askapp.AttemptSucceeded || to == askapp.AttemptFailed {
		attempt.CompletedAt = &at
	}
	r.attempts[id] = attempt
	return nil
}

type concurrentProvider struct{}

type failingAttemptFinishRepository struct {
	memoryAttemptRepository
}

func (r *failingAttemptFinishRepository) TransitionAttempt(_ context.Context, id string, from, to askapp.AttemptStatus, errorCode, requestID, usage string, at time.Time) error {
	if to == askapp.AttemptFailed {
		return errors.New("attempt transition unavailable")
	}
	return r.memoryAttemptRepository.TransitionAttempt(context.Background(), id, from, to, errorCode, requestID, usage, at)
}

func (concurrentProvider) Complete(_ context.Context, request Request) (Response, error) {
	return Response{AttemptID: request.AttemptID, Text: validFinalAnswerJSON}, nil
}

func (concurrentProvider) Stream(_ context.Context, _ Request, _ func(StreamEvent) error) (Response, error) {
	return Response{}, context.Canceled
}

// fakeProvider 实现 Provider，记录最近一次 Complete 请求并返回固定响应。
type fakeProvider struct {
	lastRequest Request
	response    Response
	err         error
	callCount   int
}

// streamingProvider 按给定分片触发流事件，并返回完整归一化响应。
type streamingProvider struct {
	chunks      []string
	response    Response
	streamErr   error
	completeErr error
}

func (p *streamingProvider) Complete(_ context.Context, _ Request) (Response, error) {
	return p.response, p.completeErr
}

func (p *streamingProvider) Stream(_ context.Context, _ Request, emit func(StreamEvent) error) (Response, error) {
	for _, chunk := range p.chunks {
		if err := emit(StreamEvent{Type: StreamContentDelta, ContentDelta: chunk}); err != nil {
			return Response{}, err
		}
	}
	if p.streamErr != nil {
		return Response{}, p.streamErr
	}
	return p.response, nil
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
	idxTask := indexOf(body, "【参考资料】\n任务项")
	idxHistory := indexOf(body, "【历史对话】\n历史消息")
	idxCurrent := indexOf(body, "【当前问题】\n当前问题")
	idxTool := indexOf(body, "【工具结果】\n工具结果")
	if !(idxTask >= 0 && idxHistory > idxTask && idxCurrent > idxHistory && idxTool > idxCurrent) {
		t.Fatalf("user message layer order wrong: %q", body)
	}
}

func TestBlocksToMessagesSeparatesDeveloperTier(t *testing.T) {
	blocks := []askapp.ContextBlock{
		{Layer: askapp.LayerControlInstructions, Kind: askapp.InstructionKindSystem, Text: "身份与安全边界"},
		{Layer: askapp.LayerControlInstructions, Kind: askapp.InstructionKindDeveloper, Text: "输出协议"},
		{Layer: askapp.LayerCurrentTask, Kind: "current_turn", Text: "当前问题"},
	}
	messages := blocksToMessages(blocks)
	if len(messages) != 3 {
		t.Fatalf("messages = %d, want 3 (system + developer + user)", len(messages))
	}
	if messages[0].Role != "system" || messages[0].Content != "身份与安全边界" {
		t.Fatalf("system message = %+v", messages[0])
	}
	if messages[1].Role != "developer" || messages[1].Content != "输出协议" {
		t.Fatalf("developer message = %+v", messages[1])
	}
	if messages[2].Role != "user" {
		t.Fatalf("third message role = %q, want user", messages[2].Role)
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

func TestParseRecordArrayNormalizesCallArguments(t *testing.T) {
	const text = `{"records":[{"type":"header","schema_version":"record_array_v1","action":"call_tools","task_updates":[{"task_key":"t1","goal":"查记录","source_turn_ids":[],"subject_keys":[]}]},{"type":"call","call_key":"c1","task_keys":["t1"],"tool_name":"search_health_records","catalog_version":"ask-tools-v2","depends_on":[],"arguments":"{\"pet_id\":\"pet-1\",\"limit\":2}"}]}`
	records, err := parseRecordArray(text)
	if err != nil {
		t.Fatalf("parseRecordArray error: %v", err)
	}
	if len(records) != 2 || records[1].Call == nil {
		t.Fatalf("records = %+v", records)
	}
	if got := string(records[1].Call.Arguments); got != `{"pet_id":"pet-1","limit":2}` {
		t.Fatalf("arguments = %s, want normalized JSON object", got)
	}
}

func TestParseRecordArrayAcceptsObjectCallArguments(t *testing.T) {
	const text = `[{"type":"call","call_key":"c1","task_keys":["t1"],"tool_name":"search_health_records","catalog_version":"ask-tools-v2","arguments":{"pet_id":"pet-1"}}]`
	records, err := parseRecordArray(text)
	if err != nil {
		t.Fatalf("parseRecordArray error: %v", err)
	}
	if records[0].Call == nil || string(records[0].Call.Arguments) != `{"pet_id":"pet-1"}` {
		t.Fatalf("call = %+v", records[0].Call)
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
	provider := &fakeProvider{response: Response{ProviderRequestID: "request-1", Text: validFinalAnswerJSON, Usage: Usage{InputTokens: 12, OutputTokens: 5, TotalTokens: 17, Complete: true}}}
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
	result, err := adapter.Step(context.Background(), input)
	if err != nil {
		t.Fatalf("Step error: %v", err)
	}
	if len(result.Records) != 5 {
		t.Fatalf("records = %d, want 5", len(result.Records))
	}
	if result.ProviderRequestID != "request-1" || result.Usage.TotalTokens != 17 || !result.Usage.Complete {
		t.Fatalf("model result = %+v", result)
	}
	req := provider.lastRequest
	if req.Purpose != PurposeAgentStep {
		t.Fatalf("purpose = %q, want agent_step", req.Purpose)
	}
	if req.AttemptID == "" {
		t.Fatal("attempt ID should not be empty")
	}
	if req.MaxOutputTokens != 2048 {
		t.Fatalf("max_output_tokens = %d, want 2048", req.MaxOutputTokens)
	}
	schema, ok := req.ResponseSchema.(map[string]any)
	if !ok || !askapp.IsRecordArraySchema(schema) {
		t.Fatalf("response schema should be record_array_v1 object root, got %v", req.ResponseSchema)
	}
	if req.ResponseProtocol != askapp.RecordArrayV1 {
		t.Fatalf("response protocol = %q, want %s", req.ResponseProtocol, askapp.RecordArrayV1)
	}
	if len(req.Messages) == 0 || len(req.Tools) != 1 {
		t.Fatalf("messages=%d tools=%d, want 1 message and 1 tool", len(req.Messages), len(req.Tools))
	}
}

func TestAgentModelAdapterPersistsCompleteRequestSnapshotAndUsage(t *testing.T) {
	repository := &memoryAttemptRepository{}
	provider := &fakeProvider{response: Response{ProviderRequestID: "provider-request-1", Text: validFinalAnswerJSON, Usage: Usage{InputTokens: 20, OutputTokens: 7, TotalTokens: 27, Source: "provider", Complete: true}}}
	profile := Profile{ProviderID: "openai", Model: "model-1", Version: "profile-v1", AdapterVersion: "adapter-v1", Parameters: []Parameter{{Name: "temperature", Value: 0.2}}}
	adapter := NewAgentModelAdapter(provider, profile, repository)
	input := askapp.StepInput{
		SessionID: "session-1", RunID: "run-1",
		Blocks: []askapp.ContextBlock{{Layer: askapp.LayerCurrentTask, Text: "问题"}},
		Tools:  []askapp.Tool{{Name: "search_records", Version: "v2", Parameters: []byte(`{"type":"object"}`)}},
		Budget: askapp.ModelBudgetSnapshot{Limits: askapp.BudgetLimits{MaxTokens: 5000}, Reservation: askapp.BudgetAmount{ModelCalls: 1, Tokens: 2048}},
	}
	if _, err := adapter.Step(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	attempts, err := repository.ListAttempts(context.Background(), "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(attempts) != 1 {
		t.Fatalf("attempts = %d, want 1", len(attempts))
	}
	attempt := attempts[0]
	if attempt.RequestID != "provider-request-1" {
		t.Fatalf("request_id = %q", attempt.RequestID)
	}
	var snapshot map[string]any
	if err := json.Unmarshal([]byte(attempt.InputSnapshot), &snapshot); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"attempt_id", "messages", "tools", "response_schema", "response_protocol", "provider_id", "model", "profile_version", "adapter_version", "capabilities", "parameters", "max_output_tokens", "budget"} {
		if _, ok := snapshot[key]; !ok {
			t.Fatalf("snapshot missing %q: %s", key, attempt.InputSnapshot)
		}
	}
	if _, ok := snapshot["image_content"]; ok {
		t.Fatal("snapshot must not contain image bytes")
	}
	var usage Usage
	if err := json.Unmarshal([]byte(attempt.Usage), &usage); err != nil {
		t.Fatal(err)
	}
	if usage.TotalTokens != 27 || !usage.Complete {
		t.Fatalf("usage = %+v", usage)
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

func TestAgentModelAdapterPersistsConcurrentAttempts(t *testing.T) {
	repository := &memoryAttemptRepository{}
	adapter := NewAgentModelAdapter(concurrentProvider{}, Profile{}, repository)
	const count = 32
	var wg sync.WaitGroup
	errs := make(chan error, count)
	for index := 0; index < count; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := adapter.Step(context.Background(), askapp.StepInput{SessionID: "session-1", RunID: "run-1"})
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	attempts, err := repository.ListAttempts(context.Background(), "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(attempts) != count {
		t.Fatalf("attempts = %d, want %d", len(attempts), count)
	}
	for _, attempt := range attempts {
		if attempt.Status != askapp.AttemptSucceeded || attempt.CompletedAt == nil {
			t.Fatalf("attempt = %+v", attempt)
		}
	}
}

func TestAgentModelAdapterStepPropagatesProviderError(t *testing.T) {
	provider := &fakeProvider{err: NewProviderError(ErrProviderTimeout, true, 0, context.DeadlineExceeded)}
	adapter := NewAgentModelAdapter(provider, Profile{})
	if _, err := adapter.Step(context.Background(), askapp.StepInput{}); err == nil {
		t.Fatal("provider error should propagate")
	}
}

func TestAgentModelAdapterKeepsStepErrorWhenAttemptTransitionFails(t *testing.T) {
	adapter := NewAgentModelAdapter(&fakeProvider{err: NewProviderError(ErrProviderUnavailable, true, 0, errors.New("upstream unavailable"))}, Profile{}, &failingAttemptFinishRepository{})
	_, err := adapter.Step(context.Background(), askapp.StepInput{SessionID: "session-1", RunID: "run-transition"})
	if code, retryable := askapp.ExecutorErrorDetails(err); code != ErrProviderUnavailable || !retryable {
		t.Fatalf("error = (%q, %v), want (%q, true)", code, retryable, ErrProviderUnavailable)
	}
}

func TestAgentModelAdapterStepClassifiesInvalidOutput(t *testing.T) {
	truncatedRepository := &memoryAttemptRepository{}
	truncated := NewAgentModelAdapter(&fakeProvider{response: Response{Text: `{"records":[`, FinishReason: FinishLength}}, Profile{}, truncatedRepository)
	_, truncatedErr := truncated.Step(context.Background(), askapp.StepInput{SessionID: "session-1", RunID: "run-2"})
	if truncatedErr == nil {
		t.Fatal("truncated output should fail")
	}
	if code, retryable := askapp.ExecutorErrorDetails(truncatedErr); code != ErrProviderOutputTruncated || !retryable {
		t.Fatalf("truncated output error = (%q, %v), want (%q, true)", code, retryable, ErrProviderOutputTruncated)
	}

	repository := &memoryAttemptRepository{}
	provider := &fakeProvider{response: Response{Text: ""}}
	adapter := NewAgentModelAdapter(provider, Profile{}, repository)
	_, err := adapter.Step(context.Background(), askapp.StepInput{SessionID: "session-1", RunID: "run-1"})
	if code, retryable := askapp.ExecutorErrorDetails(err); code != askapp.ErrAgentOutputUnparsable || retryable {
		t.Fatalf("unparsable output error = (%q, %v), want (%q, false)", code, retryable, askapp.ErrAgentOutputUnparsable)
	}
	attempts, err := repository.ListAttempts(context.Background(), "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(attempts) != 1 || attempts[0].Status != askapp.AttemptFailed || attempts[0].ErrorCode != askapp.ErrAgentOutputUnparsable {
		t.Fatalf("attempt = %+v", attempts)
	}
}

func TestAgentModelAdapterStreamsFinalAnswerDeltas(t *testing.T) {
	full := validFinalAnswerJSON
	half := len(full) / 2
	provider := &streamingProvider{
		chunks:   []string{full[:half], full[half:]},
		response: Response{AttemptID: "attempt-1", Model: "test-model", Text: full, FinishReason: FinishStop},
	}
	adapter := NewAgentModelAdapter(provider, Profile{ProviderID: "test", Model: "test-model"})
	var deltas []string
	result, err := adapter.Step(context.Background(), askapp.StepInput{OnAnswerDelta: func(delta string) error {
		deltas = append(deltas, delta)
		return nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Records) == 0 {
		t.Fatal("streamed response should still parse into records")
	}
	if joined := strings.Join(deltas, ""); joined != "你好呀" {
		t.Fatalf("streamed deltas = %q, want %q", joined, "你好呀")
	}
}

func TestAgentModelAdapterStreamsPartialSegmentText(t *testing.T) {
	answer := strings.Repeat("旺仔今天精神状态尚可，无呕吐。", 6)
	full := `[` +
		`{"type":"header","schema_version":"record_array_v1","action":"final_answer","task_updates":[]},` +
		`{"type":"group","group_key":"g1","task_keys":["t1"],"answer_kind":"casual","subjects":[{"subject_key":"s1","kind":"unresolved","description":"某只宠物","source_turn_ids":[]}],"scope":"full"},` +
		`{"type":"segment","segment_key":"seg1","group_key":"g1","subject_keys":["s1"],"field":"reply","text":"` + answer + `","basis_kind":"general_knowledge"},` +
		`{"type":"coverage","tasks":[{"task_key":"t1","answer_group_keys":["g1"]}]},` +
		`{"type":"end"}]`
	chunks := make([]string, 0, len(full))
	for i := 0; i < len(full); i += 8 {
		end := i + 8
		if end > len(full) {
			end = len(full)
		}
		chunks = append(chunks, full[i:end])
	}
	provider := &streamingProvider{
		chunks:   chunks,
		response: Response{AttemptID: "attempt-1", Model: "test-model", Text: full, FinishReason: FinishStop},
	}
	adapter := NewAgentModelAdapter(provider, Profile{ProviderID: "test", Model: "test-model"})
	var deltas []string
	result, err := adapter.Step(context.Background(), askapp.StepInput{OnAnswerDelta: func(delta string) error {
		deltas = append(deltas, delta)
		return nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Records) == 0 {
		t.Fatal("streamed response should still parse into records")
	}
	if len(deltas) < 2 {
		t.Fatalf("deltas = %d, want progressive emission before the response completes", len(deltas))
	}
	for index, delta := range deltas {
		if delta == "" {
			t.Fatalf("delta[%d] is empty", index)
		}
	}
	if joined := strings.Join(deltas, ""); joined != answer {
		t.Fatalf("streamed deltas = %q, want %q", joined, answer)
	}
}

func TestAgentModelAdapterFallsBackWhenStreamFailsBeforePublish(t *testing.T) {
	provider := &streamingProvider{
		streamErr: NewProviderError(ErrProviderUnavailable, true, 0, nil),
		response:  Response{AttemptID: "attempt-1", Model: "test-model", Text: validFinalAnswerJSON, FinishReason: FinishStop},
	}
	adapter := NewAgentModelAdapter(provider, Profile{ProviderID: "test", Model: "test-model"})
	var deltas []string
	result, err := adapter.Step(context.Background(), askapp.StepInput{OnAnswerDelta: func(delta string) error {
		deltas = append(deltas, delta)
		return nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Records) == 0 {
		t.Fatal("fallback complete should parse into records")
	}
	if len(deltas) != 0 {
		t.Fatalf("streamed deltas = %#v, want none before fallback", deltas)
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
