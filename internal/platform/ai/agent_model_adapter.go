package ai

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	askapp "github.com/balancetheworld/wechat-pet/internal/app/ask"
)

// AgentModelAdapter 实现 ask.AgentModel（文档 2、4.1、8.4）。
// 把决策循环的 StepInput 转换为统一 Request，调用 Provider 取得 record_array_v1 文本，
// 解析为 ProtocolRecord 供领域编排校验。适配器不自行检索数据、执行工具或切模型。
type AgentModelAdapter struct {
	provider Provider
	profile  Profile
	attempts askapp.AttemptRepository
}

// NewAgentModelAdapter 构造决策循环模型端口。profile 固定本次 Run 的 Provider/Model/参数。
func NewAgentModelAdapter(provider Provider, profile Profile, attempts ...askapp.AttemptRepository) *AgentModelAdapter {
	adapter := &AgentModelAdapter{provider: provider, profile: profile}
	if len(attempts) > 0 {
		adapter.attempts = attempts[0]
	}
	return adapter
}

// Step 发起一次 agent_step 决策（文档 4.1）。每次调用分配新的 attempt 身份，
// 满足「同一 Attempt 至多发出一次请求」。
func (a *AgentModelAdapter) Step(ctx context.Context, input askapp.StepInput) (askapp.ModelStepResult, error) {
	attemptID, err := newAttemptID()
	if err != nil {
		return askapp.ModelStepResult{}, err
	}
	request := Request{
		SchemaVersion:    "request_v1",
		AttemptID:        attemptID,
		Purpose:          PurposeAgentStep,
		ProviderID:       a.profile.ProviderID,
		Model:            a.profile.Model,
		ProfileVersion:   a.profile.Version,
		AdapterVersion:   a.profile.AdapterVersion,
		Capabilities:     a.profile.Capabilities,
		Messages:         blocksToMessages(input.Blocks),
		Tools:            toolsToSpecs(input.Tools),
		ResponseSchema:   askapp.RecordArraySchema(),
		ResponseProtocol: askapp.RecordArrayV1,
		Parameters:       a.profile.Parameters,
		MaxOutputTokens:  maxOutputTokens(a.profile),
	}
	for _, image := range input.Images {
		request.Images = append(request.Images, ImageRef{AssetID: image.AssetID, Version: "controlled-jpeg-v1"})
		request.ImageContent = append(request.ImageContent, image.Content)
	}
	if a.attempts != nil {
		if input.SessionID == "" || input.RunID == "" {
			return askapp.ModelStepResult{}, fmt.Errorf("agent_model: session_id and run_id are required for persisted attempts")
		}
		snapshot, marshalErr := marshalAttemptInput(request, input.Budget)
		if marshalErr != nil {
			return askapp.ModelStepResult{}, fmt.Errorf("agent_model: marshal attempt input: %w", marshalErr)
		}
		now := time.Now().UTC()
		attempt := askapp.Attempt{ID: attemptID, SessionID: input.SessionID, RunID: input.RunID, Purpose: string(PurposeAgentStep), InputSnapshot: string(snapshot), StartedAt: now}
		if err := a.attempts.CreateAttempt(ctx, attempt); err != nil {
			return askapp.ModelStepResult{}, fmt.Errorf("agent_model: create attempt: %w", err)
		}
		if err := a.attempts.TransitionAttempt(ctx, attemptID, askapp.AttemptQueued, askapp.AttemptRunning, "", "", "", now); err != nil {
			return askapp.ModelStepResult{}, fmt.Errorf("agent_model: start attempt: %w", err)
		}
	}
	response, err := a.provider.Complete(ctx, request)
	if err != nil {
		executorErr := toExecutorError(err)
		if transitionErr := a.finishAttempt(ctx, attemptID, askapp.AttemptFailed, executorErr, Response{}); transitionErr != nil {
			return askapp.ModelStepResult{}, transitionErr
		}
		return askapp.ModelStepResult{}, executorErr
	}
	result := modelStepResult(response)
	records, err := parseRecordArray(response.Text)
	if err != nil {
		executorErr := askapp.NewExecutorError(ErrProviderOutputInvalid, false, 0, err)
		if transitionErr := a.finishAttempt(ctx, attemptID, askapp.AttemptFailed, executorErr, response); transitionErr != nil {
			return result, transitionErr
		}
		return result, executorErr
	}
	result.Records = records
	if err := a.finishAttempt(ctx, attemptID, askapp.AttemptSucceeded, nil, response); err != nil {
		return result, err
	}
	return result, nil
}

func (a *AgentModelAdapter) finishAttempt(ctx context.Context, attemptID string, status askapp.AttemptStatus, stepErr error, response Response) error {
	if a.attempts == nil {
		return nil
	}
	errorCode := ""
	if stepErr != nil {
		errorCode, _ = askapp.ExecutorErrorDetails(stepErr)
	}
	usage, err := json.Marshal(response.Usage.Normalize())
	if err != nil {
		return fmt.Errorf("agent_model: marshal attempt usage: %w", err)
	}
	if err := a.attempts.TransitionAttempt(ctx, attemptID, askapp.AttemptRunning, status, errorCode, response.ProviderRequestID, string(usage), time.Now().UTC()); err != nil {
		return fmt.Errorf("agent_model: finish attempt: %w", err)
	}
	return nil
}

type attemptInputSnapshot struct {
	SchemaVersion    string                     `json:"schema_version"`
	AttemptID        string                     `json:"attempt_id"`
	Purpose          Purpose                    `json:"purpose"`
	ProviderID       string                     `json:"provider_id"`
	Model            string                     `json:"model"`
	ProfileVersion   string                     `json:"profile_version"`
	AdapterVersion   string                     `json:"adapter_version"`
	Capabilities     Capabilities               `json:"capabilities"`
	Messages         []Message                  `json:"messages"`
	Images           []ImageRef                 `json:"images"`
	Tools            []ToolSpec                 `json:"tools"`
	ResponseProtocol string                     `json:"response_protocol"`
	ResponseSchema   any                        `json:"response_schema"`
	Parameters       []Parameter                `json:"parameters"`
	MaxOutputTokens  int                        `json:"max_output_tokens"`
	TimeoutMillis    int64                      `json:"timeout_millis"`
	Deadline         time.Time                  `json:"deadline"`
	Budget           askapp.ModelBudgetSnapshot `json:"budget"`
}

func marshalAttemptInput(request Request, budget askapp.ModelBudgetSnapshot) ([]byte, error) {
	return json.Marshal(attemptInputSnapshot{
		SchemaVersion: request.SchemaVersion, AttemptID: request.AttemptID, Purpose: request.Purpose, ProviderID: request.ProviderID,
		Model: request.Model, ProfileVersion: request.ProfileVersion, AdapterVersion: request.AdapterVersion,
		Capabilities: request.Capabilities, Messages: request.Messages, Images: request.Images, Tools: request.Tools,
		ResponseProtocol: request.ResponseProtocol, ResponseSchema: request.ResponseSchema, Parameters: request.Parameters,
		MaxOutputTokens: request.MaxOutputTokens, TimeoutMillis: request.Timeout.Milliseconds(), Deadline: request.Deadline,
		Budget: budget,
	})
}

func modelStepResult(response Response) askapp.ModelStepResult {
	usage := response.Usage.Normalize()
	return askapp.ModelStepResult{
		ProviderRequestID: response.ProviderRequestID,
		Usage:             askapp.ModelUsage{InputTokens: int(usage.InputTokens), OutputTokens: int(usage.OutputTokens), TotalTokens: int(usage.TotalTokens), Complete: usage.Complete},
	}
}

func newAttemptID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("agent_model: generate attempt id: %w", err)
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", value[0:4], value[4:6], value[6:8], value[8:10], value[10:16]), nil
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
	if len(reference) > 0 {
		body = append(body, "【参考资料】\n"+strings.Join(reference, "\n\n"))
	}
	if len(history) > 0 {
		body = append(body, "【历史对话】\n"+strings.Join(history, "\n\n"))
	}
	if len(current) > 0 {
		body = append(body, "【当前问题】\n"+strings.Join(current, "\n\n"))
	}
	if len(toolInteractions) > 0 {
		body = append(body, "【工具结果】\n"+strings.Join(toolInteractions, "\n\n"))
	}
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
