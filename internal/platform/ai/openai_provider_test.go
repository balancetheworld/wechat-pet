package ai

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/openai/openai-go/responses"
)

func TestNormalizeFinishReason(t *testing.T) {
	cases := []struct {
		name             string
		status           string
		incompleteReason string
		hasToolCall      bool
		want             FinishReason
	}{
		{"completed stop", "completed", "", false, FinishStop},
		{"completed tool", "completed", "", true, FinishToolCalls},
		{"incomplete max_output_tokens", "incomplete", "max_output_tokens", false, FinishLength},
		{"incomplete content_filter", "incomplete", "content_filter", false, FinishRefusal},
		{"incomplete unknown reason", "incomplete", "something_else", false, FinishUnknown},
		{"failed", "failed", "", false, FinishUnknown},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := normalizeFinishReason(c.status, c.incompleteReason, c.hasToolCall); got != c.want {
				t.Fatalf("normalizeFinishReason(%q,%q,%v) = %q, want %q", c.status, c.incompleteReason, c.hasToolCall, got, c.want)
			}
		})
	}
}

func TestNormalizeUsageUnknownNotZero(t *testing.T) {
	// 供应商未报告任何 Token 时，Complete 为 false——不得当作"免费/已知可结算"。
	request := Request{ImageContent: [][]byte{[]byte("img1"), []byte("img2")}}
	usage := normalizeUsage(responses.ResponseUsage{}, request)
	if usage.ImageCount == nil || *usage.ImageCount != 2 {
		t.Fatalf("ImageCount should be 2, got %v", usage.ImageCount)
	}
	if usage.Complete {
		t.Fatal("empty token usage should be marked incomplete")
	}
	if usage.Known() {
		t.Fatal("empty usage should not be known")
	}
	// 供应商报告了 token 时，Complete 为 true。
	reported := normalizeUsage(responses.ResponseUsage{TotalTokens: 10, InputTokens: 6, OutputTokens: 4}, Request{})
	if !reported.Complete || reported.TotalTokens != 10 {
		t.Fatalf("reported usage should be complete, got %+v", reported)
	}
}

func TestBuildToolsConvertsSpecs(t *testing.T) {
	specs := []ToolSpec{
		{Name: "read_records", Version: "v1", Description: "读取记录", Parameters: map[string]any{"type": "object"}},
		{Name: "read_profile", Version: "v1", Parameters: "not-a-map"},
	}
	tools := buildTools(specs)
	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(tools))
	}
	if tools[0].OfFunction == nil || tools[0].OfFunction.Name != "read_records" {
		t.Fatal("first tool should map to function with name read_records")
	}
	if tools[1].OfFunction == nil || tools[1].OfFunction.Parameters["type"] != "object" {
		t.Fatal("non-map parameters should fall back to empty object schema")
	}
}

func TestResponseSchemaMapRejectsNonObject(t *testing.T) {
	if _, err := responseSchemaMap(nil); err == nil {
		t.Fatal("nil schema should error")
	}
	if _, err := responseSchemaMap("not-a-map"); err == nil {
		t.Fatal("string schema should error")
	}
	if m, err := responseSchemaMap(map[string]any{"type": "array"}); err != nil || m["type"] != "array" {
		t.Fatalf("valid map should pass, got %v %v", m, err)
	}
}

func TestBuildInputSeparatesInstructionsAndImages(t *testing.T) {
	request := Request{
		Messages: []Message{
			{Role: "system", Content: "安全规则"},
			{Role: "user", Content: "第一次问题"},
			{Role: "assistant", Content: "第一次回答"},
			{Role: "user", Content: "带图问题"},
		},
		ImageContent: [][]byte{[]byte("jpeg-bytes")},
	}
	input, instructions := buildInput(request)
	if instructions != "安全规则" {
		t.Fatalf("instructions = %q, want 安全规则", instructions)
	}
	if len(input.OfInputItemList) != 3 {
		t.Fatalf("expected 3 conversation items, got %d", len(input.OfInputItemList))
	}
	// 图片应附加到最后一个 user 消息（第 3 个 item）。
	last := input.OfInputItemList[2]
	if last.OfInputMessage == nil {
		t.Fatal("last item should be a message")
	}
	if len(last.OfInputMessage.Content) != 2 {
		t.Fatalf("last user message should have 2 content parts (text+image), got %d", len(last.OfInputMessage.Content))
	}
	if last.OfInputMessage.Content[1].OfInputImage == nil {
		t.Fatal("second content part should be an image")
	}
}

func TestBuildInputSystemOnly(t *testing.T) {
	request := Request{Messages: []Message{{Role: "system", Content: "only system"}}}
	input, instructions := buildInput(request)
	if instructions != "only system" {
		t.Fatalf("instructions = %q, want only system", instructions)
	}
	if len(input.OfInputItemList) != 0 {
		t.Fatalf("system-only should have empty conversation items, got %d", len(input.OfInputItemList))
	}
}

func TestClassifyProviderErrorContext(t *testing.T) {
	deadlineCtx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	time.Sleep(5 * time.Millisecond)

	if err := classifyProviderError(deadlineCtx, context.DeadlineExceeded); !IsProviderErrorCode(err, ErrProviderTimeout) {
		t.Fatalf("deadline exceeded should map to timeout, got %v", err)
	}

	cancelCtx, cancel2 := context.WithCancel(context.Background())
	cancel2()
	if err := classifyProviderError(cancelCtx, context.Canceled); !IsProviderErrorCode(err, ErrProviderCanceled) {
		t.Fatalf("canceled should map to canceled, got %v", err)
	}
}

func TestIsTopLevelArray(t *testing.T) {
	if !isTopLevelArray(map[string]any{"type": "array", "items": map[string]any{}}) {
		t.Fatal("array schema should be top-level array")
	}
	if isTopLevelArray(map[string]any{"type": "object"}) {
		t.Fatal("object schema should not be top-level array")
	}
}

func TestHasOptionalProperties(t *testing.T) {
	// 含可选字段（required 只列 pet_id，[]any 形式，模拟 JSON 反序列化）。
	withOptional := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"pet_id":   map[string]any{"type": "string"},
			"category": map[string]any{"type": "string"},
		},
		"required": []any{"pet_id"},
	}
	if !hasOptionalProperties(withOptional) {
		t.Fatal("schema with optional field should report optional properties")
	}
	// 全 required（[]string 形式，模拟 Go 字面量）。
	allRequired := map[string]any{
		"type":                 "object",
		"properties":           map[string]any{"query": map[string]any{"type": "string"}},
		"required":             []string{"query"},
		"additionalProperties": false,
	}
	if hasOptionalProperties(allRequired) {
		t.Fatal("schema with all required should not report optional properties")
	}
	// 无 properties 视为无可选字段。
	if hasOptionalProperties(map[string]any{"type": "object"}) {
		t.Fatal("schema without properties should not report optional properties")
	}
}

func TestBuildToolsStrictForOptionalProperties(t *testing.T) {
	optional := ToolSpec{Name: "search", Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"pet_id":   map[string]any{"type": "string"},
			"category": map[string]any{"type": "string"},
		},
		"required": []any{"pet_id"},
	}}
	required := ToolSpec{Name: "resolve", Parameters: map[string]any{
		"type":       "object",
		"properties": map[string]any{"query": map[string]any{"type": "string"}},
		"required":   []string{"query"},
	}}
	tools := buildTools([]ToolSpec{optional, required})
	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(tools))
	}
	// 含可选字段的工具应降级为非 strict（文档 12.2：strict 函数调用要求所有属性必填）。
	if tools[0].OfFunction.Strict.Value {
		t.Fatal("optional-field tool should be non-strict")
	}
	// 全必填工具保留 strict。
	if !tools[1].OfFunction.Strict.Value {
		t.Fatal("all-required tool should be strict")
	}
}

func TestBuildParamsStrictForTopLevelArray(t *testing.T) {
	p := &OpenAIProvider{model: "test-model"}
	// 顶层数组（record_array_v1）应降级为非 strict。
	params, err := p.buildParams(Request{
		ResponseSchema: map[string]any{"type": "array", "items": map[string]any{}},
	})
	if err != nil {
		t.Fatalf("buildParams array: %v", err)
	}
	if params.Text.Format.OfJSONSchema == nil || params.Text.Format.OfJSONSchema.Strict.Value {
		t.Fatal("top-level array schema should be non-strict")
	}
	// object 顶层保留 strict。
	params2, err := p.buildParams(Request{
		ResponseSchema: map[string]any{"type": "object", "properties": map[string]any{}},
	})
	if err != nil {
		t.Fatalf("buildParams object: %v", err)
	}
	if params2.Text.Format.OfJSONSchema == nil || !params2.Text.Format.OfJSONSchema.Strict.Value {
		t.Fatal("object schema should be strict")
	}
}

func TestProviderErrorCodeAndKnownCodes(t *testing.T) {
	err := NewProviderError(ErrProviderQuotaExhausted, false, 0, errors.New("boom"))
	if ProviderErrorCode(err) != ErrProviderQuotaExhausted {
		t.Fatalf("unexpected code %q", ProviderErrorCode(err))
	}
	if !IsProviderErrorCode(err, ErrProviderQuotaExhausted) {
		t.Fatal("IsProviderErrorCode should match")
	}
	if !BlindRetryForbidden(ErrProviderQuotaExhausted) {
		t.Fatal("quota exhausted must be blind-retry forbidden")
	}
	// 非契约内 code 回退 provider_failed。
	unknown := NewProviderError("not_a_code", false, 0, nil)
	if ProviderErrorCode(unknown) != ErrProviderFailed {
		t.Fatalf("unknown code should fall back, got %q", ProviderErrorCode(unknown))
	}
	// 普通 error 提取为 provider_failed。
	if ProviderErrorCode(errors.New("plain")) != ErrProviderFailed {
		t.Fatal("plain error should map to provider_failed")
	}
}
