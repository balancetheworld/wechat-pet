// Command t3provider 用真实的 OpenAIProvider（Responses API）验证 Provider v2 适配器的
// 模型与协议组合能力（架构设计 v2 文档 12.2「模型与协议组合」前置）。
//
// 与 scripts/t3_provider_verify.mjs（Chat Completions 探活）不同，本程序直接走生产
// 代码路径 internal/platform/ai 的 OpenAIProvider，验证文字 + 图片 + 工具 + 结构化输出
// + 流式 + 截断的真实行为，尤其实测 Strict:true 与 record_array_v1 顶层数组、含可选字段
// 的工具参数是否兼容。SDK 类型与模拟测试不足以标记组合能力通过，须以本程序真实接口结果为准。
//
// 用法：go run ./cmd/t3provider
// 从项目根目录 .env 读取 AI_API_KEY / AI_BASE_URL / AI_MODEL / AI_TIMEOUT_SECONDS，不回显密钥。
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/responses"

	askapp "github.com/balancetheworld/wechat-pet/internal/app/ask"
	ai "github.com/balancetheworld/wechat-pet/internal/platform/ai"
)

const imageSample = "data/uploads/067d49d58ce229174e2e6bb03f26867d.png"

func loadEnv() map[string]string {
	raw, err := os.ReadFile(".env")
	if err != nil {
		return map[string]string{}
	}
	env := map[string]string{}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if idx := strings.Index(line, "="); idx >= 0 {
			env[strings.TrimSpace(line[:idx])] = strings.TrimSpace(line[idx+1:])
		}
	}
	return env
}

type result struct {
	name string
	ok   bool
	info string
}

var results []result

func report(name string, ok bool, info string) {
	results = append(results, result{name: name, ok: ok})
	tag := "PASS"
	if !ok {
		tag = "FAIL"
	}
	fmt.Printf("[%s] %s\n", tag, name)
	if info != "" {
		fmt.Printf("      %s\n", info)
	}
}

func main() {
	env := loadEnv()
	key := env["AI_API_KEY"]
	base := strings.TrimRight(env["AI_BASE_URL"], "/")
	if base == "" {
		base = "https://api.openai.com/v1"
	}
	model := env["AI_MODEL"]
	if model == "" {
		model = "deepseek-flash"
	}
	timeout := 30 * time.Second
	if v := env["AI_TIMEOUT_SECONDS"]; v != "" {
		if d, err := time.ParseDuration(v + "s"); err == nil {
			timeout = d
		}
	}

	fmt.Printf("=== T3 Provider 真实验证（Responses API） %s @ %s ===\n", model, base)
	fmt.Printf("开始: %s\n\n", time.Now().Format(time.RFC3339))

	if key == "" {
		fmt.Println("AI_API_KEY 缺失，退出")
		os.Exit(1)
	}

	// raw client 用于 strict 兼容性探测（绕开 OpenAIProvider.buildParams 的硬编码 strict）。
	raw := openai.NewClient(
		option.WithAPIKey(key),
		option.WithBaseURL(base),
		option.WithMaxRetries(0),
		option.WithRequestTimeout(timeout),
	)

	// ---- Part 1: 基础文字（确认 key 与 /responses 端点有效）----
	probeBasicText(raw, model, timeout)

	// ---- Part 2: 结构化输出 Strict 兼容性 ----
	probeStructuredStrict(raw, model, timeout)

	// ---- Part 3: 工具参数 Strict 兼容性（含可选字段）----
	probeToolStrict(raw, model, timeout)

	// ---- Part 4: OpenAIProvider 端到端（走生产 buildParams/buildTools）----
	provider, err := ai.NewOpenAIProvider(ai.OpenAIConfig{
		APIKey:  key,
		BaseURL: base,
		Model:   model,
		Timeout: timeout,
	})
	if err != nil {
		fmt.Printf("构造 OpenAIProvider 失败: %v\n", err)
		os.Exit(1)
	}
	probeProviderComplete(provider, model, timeout)
	probeProviderStructuredOutput(provider, model, timeout)
	probeProviderVision(provider, model, timeout)
	probeProviderStream(provider, model, timeout)
	probeProviderTruncation(provider, model, timeout)
	probeProviderToolCall(provider, model, timeout)

	pass := 0
	for _, r := range results {
		if r.ok {
			pass++
		}
	}
	fmt.Printf("\n=== 汇总: %d/%d 通过 ===\n", pass, len(results))
	fmt.Printf("结束: %s\n", time.Now().Format(time.RFC3339))
}

func ctx(timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), timeout)
}

// probeBasicText 确认 key 有效、/responses 端点可用。
func probeBasicText(raw openai.Client, model string, timeout time.Duration) {
	c, cancel := ctx(timeout)
	defer cancel()
	resp, err := raw.Responses.New(c, responses.ResponseNewParams{
		Model: model,
		Input: responses.ResponseNewParamsInputUnion{OfString: openai.String("回复两个字：收到")},
	})
	if err != nil {
		report("基础文字(responses端点)", false, fmt.Sprintf("err=%v", err))
		return
	}
	report("基础文字(responses端点)", true,
		fmt.Sprintf("text=%q model=%s finish=%s", resp.OutputText(), resp.Model, resp.Status))
}

// probeStructuredStrict 探测 record_array_v1（顶层数组 + oneOf + 可选字段）在 strict
// 开/关两种模式下的真实行为。顶层数组不满足 strict 的「根节点必须 object」，预期 strict=true 被拒。
func probeStructuredStrict(raw openai.Client, model string, timeout time.Duration) {
	schema := askapp.RecordArraySchema()
	prompt := "输出一个合法的 record_array_v1 数组，首元素 header、末尾 end。"

	// strict=true
	formatStrict := responses.ResponseFormatTextConfigParamOfJSONSchema("record_array_v1", schema)
	formatStrict.OfJSONSchema.Strict = openai.Bool(true)
	c1, cancel1 := ctx(timeout)
	_, errStrict := raw.Responses.New(c1, responses.ResponseNewParams{
		Model: model,
		Input: responses.ResponseNewParamsInputUnion{OfString: openai.String(prompt)},
		Text:  responses.ResponseTextConfigParam{Format: formatStrict},
	})
	cancel1()
	if errStrict != nil {
		report("结构化输出 strict=true(顶层数组)", false, fmt.Sprintf("被拒 err=%v", errStrict))
	} else {
		report("结构化输出 strict=true(顶层数组)", true, "被接受（记录该 Provider 兼容顶层数组 strict）")
	}

	// strict=false
	formatLoose := responses.ResponseFormatTextConfigParamOfJSONSchema("record_array_v1", schema)
	formatLoose.OfJSONSchema.Strict = openai.Bool(false)
	c2, cancel2 := ctx(timeout)
	resp, errLoose := raw.Responses.New(c2, responses.ResponseNewParams{
		Model: model,
		Input: responses.ResponseNewParamsInputUnion{OfString: openai.String(prompt)},
		Text:  responses.ResponseTextConfigParam{Format: formatLoose},
	})
	cancel2()
	if errLoose != nil {
		report("结构化输出 strict=false(顶层数组)", false, fmt.Sprintf("err=%v", errLoose))
		return
	}
	text := resp.OutputText()
	var parsed []any
	jsonErr := json.Unmarshal([]byte(text), &parsed)
	report("结构化输出 strict=false(顶层数组)", jsonErr == nil && len(parsed) > 0,
		fmt.Sprintf("可解析数组=%v 元素数=%d text=%q", jsonErr == nil, len(parsed), truncate(text, 120)))
}

// probeToolStrict 探测含可选字段的工具参数在 strict 开/关两种模式下的真实行为。
// search_health_records 的 required 仅 pet_id，其余为可选，strict 要求所有属性必填，预期 strict=true 被拒。
func probeToolStrict(raw openai.Client, model string, timeout time.Duration) {
	var searchParams map[string]any
	for _, t := range askapp.DefaultTools() {
		if t.Name == "search_health_records" {
			_ = json.Unmarshal(t.Parameters, &searchParams)
			break
		}
	}
	if searchParams == nil {
		report("工具 strict 兼容性", false, "未找到 search_health_records 参数")
		return
	}
	prompt := "查询旺仔最近两次呕吐记录"

	// strict=true
	toolsStrict := []responses.ToolUnionParam{{
		OfFunction: &responses.FunctionToolParam{
			Name:       "search_health_records",
			Parameters: searchParams,
			Strict:     openai.Bool(true),
		},
	}}
	c1, cancel1 := ctx(timeout)
	_, errStrict := raw.Responses.New(c1, responses.ResponseNewParams{
		Model: model,
		Input: responses.ResponseNewParamsInputUnion{OfString: openai.String(prompt)},
		Tools: toolsStrict,
	})
	cancel1()
	if errStrict != nil {
		report("工具 strict=true(含可选字段)", false, fmt.Sprintf("被拒 err=%v", errStrict))
	} else {
		report("工具 strict=true(含可选字段)", true, "被接受（记录该 Provider 兼容可选字段 strict）")
	}

	// strict=false
	toolsLoose := []responses.ToolUnionParam{{
		OfFunction: &responses.FunctionToolParam{
			Name:       "search_health_records",
			Parameters: searchParams,
			Strict:     openai.Bool(false),
		},
	}}
	c2, cancel2 := ctx(timeout)
	resp, errLoose := raw.Responses.New(c2, responses.ResponseNewParams{
		Model: model,
		Input: responses.ResponseNewParamsInputUnion{OfString: openai.String(prompt)},
		Tools: toolsLoose,
	})
	cancel2()
	if errLoose != nil {
		report("工具 strict=false(含可选字段)", false, fmt.Sprintf("err=%v", errLoose))
		return
	}
	hasCall := false
	for _, item := range resp.Output {
		if call := item.AsFunctionCall(); call.Name != "" {
			hasCall = true
			report("工具 strict=false(含可选字段)", true, fmt.Sprintf("tool_call name=%s args=%s", call.Name, truncate(call.Arguments, 160)))
			return
		}
	}
	report("工具 strict=false(含可选字段)", hasCall, fmt.Sprintf("无工具调用 finish=%s text=%q", resp.Status, truncate(resp.OutputText(), 120)))
}

// probeProviderComplete 走生产 OpenAIProvider.Complete（当前 buildParams 硬编码 strict=true）。
func probeProviderComplete(provider *ai.OpenAIProvider, model string, timeout time.Duration) {
	c, cancel := ctx(timeout)
	defer cancel()
	resp, err := provider.Complete(c, ai.Request{
		Purpose:  ai.PurposeAgentStep,
		Messages: []ai.Message{{Role: "user", Content: "回复两个字：收到"}},
	})
	if err != nil {
		report("Provider.Complete 基础文字", false, fmt.Sprintf("err=%v code=%s", err, ai.ProviderErrorCode(err)))
		return
	}
	report("Provider.Complete 基础文字", resp.Text != "",
		fmt.Sprintf("text=%q finish=%s usage.complete=%v", truncate(resp.Text, 80), resp.FinishReason, resp.Usage.Complete))
}

// probeProviderStructuredOutput 走生产 OpenAIProvider.Complete（修复后 buildParams 对顶层数组
// 降级 non-strict），验证 record_array_v1 结构化输出端到端可用。
func probeProviderStructuredOutput(provider *ai.OpenAIProvider, model string, timeout time.Duration) {
	c, cancel := ctx(timeout)
	defer cancel()
	resp, err := provider.Complete(c, ai.Request{
		Purpose:        ai.PurposeAgentStep,
		Messages:       []ai.Message{{Role: "user", Content: "输出一个合法 record_array_v1 数组，首元素 header、末尾 end。"}},
		ResponseSchema: askapp.RecordArraySchema(),
	})
	if err != nil {
		report("Provider 结构化输出(record_array_v1)", false, fmt.Sprintf("err=%v code=%s", err, ai.ProviderErrorCode(err)))
		return
	}
	var parsed []any
	jsonErr := json.Unmarshal([]byte(resp.Text), &parsed)
	report("Provider 结构化输出(record_array_v1)", jsonErr == nil && len(parsed) > 0,
		fmt.Sprintf("可解析数组=%v 元素数=%d text=%q", jsonErr == nil, len(parsed), truncate(resp.Text, 120)))
}

// probeProviderVision 走生产 OpenAIProvider.Complete 发送受控图片字节（data URL）。
func probeProviderVision(provider *ai.OpenAIProvider, model string, timeout time.Duration) {
	img, err := os.ReadFile(imageSample)
	if err != nil {
		report("Provider 图片理解", false, fmt.Sprintf("读图失败 %v", err))
		return
	}
	c, cancel := ctx(timeout)
	defer cancel()
	resp, err := provider.Complete(c, ai.Request{
		Purpose:      ai.PurposeAgentStep,
		Messages:     []ai.Message{{Role: "user", Content: "请简短描述图片内容"}},
		ImageContent: [][]byte{img},
	})
	if err != nil {
		report("Provider 图片理解", false, fmt.Sprintf("err=%v code=%s", err, ai.ProviderErrorCode(err)))
		return
	}
	report("Provider 图片理解", resp.Text != "",
		fmt.Sprintf("text=%q usage.image_count=%v", truncate(resp.Text, 80), imageCount(resp.Usage)))
}

// probeProviderStream 走生产 OpenAIProvider.Stream，验证有序增量与用量报告。
func probeProviderStream(provider *ai.OpenAIProvider, model string, timeout time.Duration) {
	c, cancel := ctx(timeout)
	defer cancel()
	var deltas []string
	usageReported := false
	finished := false
	_, err := provider.Stream(c, ai.Request{
		Purpose:  ai.PurposeAgentStep,
		Messages: []ai.Message{{Role: "user", Content: "依次说出数字：一、二、三"}},
	}, func(ev ai.StreamEvent) error {
		switch ev.Type {
		case ai.StreamContentDelta:
			deltas = append(deltas, ev.ContentDelta)
		case ai.StreamUsageReported:
			usageReported = true
		case ai.StreamResponseFinished:
			finished = true
		}
		return nil
	})
	joined := strings.Join(deltas, "")
	ordered := strings.Contains(joined, "一") && strings.Contains(joined, "二") && strings.Contains(joined, "三")
	if err != nil {
		report("Provider 流式(有序+用量)", false, fmt.Sprintf("err=%v code=%s", err, ai.ProviderErrorCode(err)))
		return
	}
	report("Provider 流式(有序+用量)", finished && ordered,
		fmt.Sprintf("finished=%v 顺序=%v 用量报告=%v text=%q", finished, ordered, usageReported, truncate(joined, 60)))
}

// probeProviderTruncation 极小 MaxOutputTokens 应归一化为 FinishLength。
func probeProviderTruncation(provider *ai.OpenAIProvider, model string, timeout time.Duration) {
	c, cancel := ctx(timeout)
	defer cancel()
	resp, err := provider.Complete(c, ai.Request{
		Purpose:         ai.PurposeAgentStep,
		Messages:        []ai.Message{{Role: "user", Content: "用至少 500 字详细介绍猫的饲养方法"}},
		MaxOutputTokens: 8,
	})
	if err != nil {
		report("Provider 截断(finish=length)", false, fmt.Sprintf("err=%v code=%s", err, ai.ProviderErrorCode(err)))
		return
	}
	report("Provider 截断(finish=length)", resp.FinishReason == ai.FinishLength,
		fmt.Sprintf("finish=%s text=%q", resp.FinishReason, truncate(resp.Text, 60)))
}

// probeProviderToolCall 走生产 OpenAIProvider.Complete（buildTools 硬编码 strict=true），
// 用全 required 的简单工具验证工具调用链路。
func probeProviderToolCall(provider *ai.OpenAIProvider, model string, timeout time.Duration) {
	c, cancel := ctx(timeout)
	defer cancel()
	specs := []ai.ToolSpec{{
		Name:        "resolve_pet",
		Version:     "v1",
		Description: "解析宠物名称到 pet_id",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{"type": "string", "description": "宠物名称或线索"},
			},
			"required":             []string{"query"},
			"additionalProperties": false,
		},
	}}
	resp, err := provider.Complete(c, ai.Request{
		Purpose:  ai.PurposeAgentStep,
		Messages: []ai.Message{{Role: "user", Content: "我想看看旺仔的情况"}},
		Tools:    specs,
	})
	if err != nil {
		report("Provider 工具调用(全required)", false, fmt.Sprintf("err=%v code=%s", err, ai.ProviderErrorCode(err)))
		return
	}
	if len(resp.ToolCalls) == 0 {
		report("Provider 工具调用(全required)", false, fmt.Sprintf("无 tool_call finish=%s text=%q", resp.FinishReason, truncate(resp.Text, 80)))
		return
	}
	tc := resp.ToolCalls[0]
	report("Provider 工具调用(全required)", tc.Name != "",
		fmt.Sprintf("finish=%s name=%s args=%s", resp.FinishReason, tc.Name, truncate(tc.Arguments, 120)))
}

func imageCount(u ai.Usage) int {
	if u.ImageCount != nil {
		return *u.ImageCount
	}
	return 0
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
