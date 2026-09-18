package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	askapp "github.com/balancetheworld/wechat-pet/internal/app/ask"
)

// AgentModelAdapter 实现 ask.AgentModel（文档 2、4.1、8.4）。
// 把决策循环的 StepInput 转换为统一 Request，调用 Provider 取得 record_array_v1 文本，
// 解析为 ProtocolRecord 供领域编排校验。适配器不自行检索数据、执行工具或切模型。
type AgentModelAdapter struct {
	provider Provider
	profile  Profile
	seq      int
}

// NewAgentModelAdapter 构造决策循环模型端口。profile 固定本次 Run 的 Provider/Model/参数。
func NewAgentModelAdapter(provider Provider, profile Profile) *AgentModelAdapter {
	return &AgentModelAdapter{provider: provider, profile: profile}
}

// Step 发起一次 agent_step 决策（文档 4.1）。每次调用分配新的 attempt 身份，
// 满足「同一 Attempt 至多发出一次请求」。
func (a *AgentModelAdapter) Step(ctx context.Context, input askapp.StepInput) ([]askapp.ProtocolRecord, error) {
	a.seq++
	attemptID := fmt.Sprintf("attempt-%d", a.seq)
	request := Request{
		SchemaVersion:   "request_v1",
		AttemptID:       attemptID,
		Purpose:         PurposeAgentStep,
		ProviderID:      a.profile.ProviderID,
		Model:           a.profile.Model,
		ProfileVersion:  a.profile.Version,
		AdapterVersion:  a.profile.AdapterVersion,
		Capabilities:    a.profile.Capabilities,
		Messages:        blocksToMessages(input.Blocks),
		Tools:           toolsToSpecs(input.Tools),
		ResponseSchema:  askapp.RecordArraySchema(),
		Parameters:      a.profile.Parameters,
		MaxOutputTokens: maxOutputTokens(a.profile),
	}
	response, err := a.provider.Complete(ctx, request)
	if err != nil {
		return nil, toExecutorError(err)
	}
	return parseRecordArray(response.Text)
}

// toExecutorError 把 Provider 层稳定错误映射为 ask 层可识别的执行错误（文档 4.1、9）。
// 依赖方向保持 ai -> ask：Runtime（ask 层）只识别 ExecutorError，不反向依赖 ai 的错误类型。
// 非 ProviderError（如协议解析错误）原样透传。
func toExecutorError(err error) error {
	var pe *ProviderError
	if errors.As(err, &pe) {
		return askapp.NewExecutorError(pe.Code, pe.Retryable, pe.RetryAfter, pe.Cause)
	}
	return err
}

// blocksToMessages 把上下文块转换为有序模型消息（文档 5.8）。
// 受控指令层进入 system 消息；其余层（参考数据、历史、当前任务、工具交互）
// 按层拼接为 user 消息。图片由 Runtime 经 Request.ImageContent 注入，此处不处理。
func blocksToMessages(blocks []askapp.ContextBlock) []Message {
	var instructions []string
	var reference []string
	var history []string
	var current []string
	var toolInteractions []string

	for _, b := range blocks {
		if b.Layer == askapp.LayerControlInstructions || b.Kind == "instruction" {
			instructions = append(instructions, b.Text)
			continue
		}
		switch b.Layer {
		case askapp.LayerReferenceData:
			reference = append(reference, b.Text)
		case askapp.LayerHistory:
			history = append(history, b.Text)
		case askapp.LayerCurrentTask:
			current = append(current, b.Text)
		case askapp.LayerToolInteractions:
			toolInteractions = append(toolInteractions, b.Text)
		default:
			reference = append(reference, b.Text)
		}
	}

	messages := make([]Message, 0, 2)
	if joined := strings.TrimSpace(strings.Join(instructions, "\n\n")); joined != "" {
		messages = append(messages, Message{Role: "system", Content: joined})
	}
	var body []string
	body = append(body, reference...)
	body = append(body, history...)
	body = append(body, current...)
	body = append(body, toolInteractions...)
	if joined := strings.TrimSpace(strings.Join(body, "\n\n")); joined != "" {
		messages = append(messages, Message{Role: "user", Content: joined})
	}
	return messages
}

// toolsToSpecs 把目录工具转换为候选工具 Schema（文档 4.1）。
// 参数 Schema 取工具声明的 parameters JSON；描述用 UseCases 拼接，供模型选择。
func toolsToSpecs(tools []askapp.Tool) []ToolSpec {
	specs := make([]ToolSpec, 0, len(tools))
	for _, t := range tools {
		var params any
		if len(t.Parameters) > 0 {
			var m map[string]any
			if json.Unmarshal(t.Parameters, &m) == nil {
				params = m
			}
		}
		if params == nil {
			params = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		specs = append(specs, ToolSpec{
			Name:        t.Name,
			Version:     t.Version,
			Description: strings.Join(t.UseCases, "；"),
			Parameters:  params,
		})
	}
	return specs
}

// parseRecordArray 把 Provider 返回的 record_array_v1 文本解析为 ProtocolRecord。
// 数组必须完整闭合；半截响应视为协议错误，不当作可执行动作。
// 非 strict 结构化输出下模型可能把 schema 关键词（如 items）误当输出结构，
// 输出 {"items":[...]} 包裹；此处先解包再交给严格解析器（文档 12.2 实测结论）。
func parseRecordArray(text string) ([]askapp.ProtocolRecord, error) {
	text, err := unwrapRecordArray(text)
	if err != nil {
		return nil, fmt.Errorf("agent_model: %w", err)
	}
	parser := askapp.NewRecordArrayParser()
	records, err := parser.Feed(text)
	if err != nil {
		return nil, fmt.Errorf("agent_model: %w", err)
	}
	if !parser.Closed() {
		return nil, fmt.Errorf("agent_model: record_array_v1 response is incomplete")
	}
	return records, nil
}

// unwrapRecordArray 解包 Provider 文本为 record_array_v1 顶层数组。
// 顶层已为数组时原样返回；顶层为 object 且恰好含一个数组字段（如 {"items":[...]}）时
// 提取该数组；否则报错。仅容忍「schema 关键词误入输出结构」这一种非 strict 偏差，
// 不放过其它协议错误（文档 12.2 实测）。
func unwrapRecordArray(text string) (string, error) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return "", fmt.Errorf("record_array_v1: empty response")
	}
	if trimmed[0] == '[' {
		return text, nil
	}
	if trimmed[0] != '{' {
		return "", fmt.Errorf("record_array_v1: response is neither array nor object, got %q", trimmed[0])
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(trimmed), &obj); err != nil {
		return "", fmt.Errorf("record_array_v1: invalid object wrapper: %w", err)
	}
	var array json.RawMessage
	count := 0
	for _, raw := range obj {
		var probe any
		if err := json.Unmarshal(raw, &probe); err != nil {
			continue
		}
		if _, ok := probe.([]any); ok {
			array = raw
			count++
		}
	}
	if count != 1 {
		return "", fmt.Errorf("record_array_v1: wrapper object has %d array fields, want exactly one", count)
	}
	return string(array), nil
}

// maxOutputTokens 从 Profile 参数中取本次输出上限；未配置时用保守默认。
func maxOutputTokens(profile Profile) int {
	for _, p := range profile.Parameters {
		if p.Name != "max_output_tokens" {
			continue
		}
		switch v := p.Value.(type) {
		case float64:
			return int(v)
		case int:
			return v
		case int64:
			return int(v)
		}
	}
	return defaultAgentStepOutputTokens
}

const defaultAgentStepOutputTokens = 4096
