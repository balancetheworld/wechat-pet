package ai

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	askapp "github.com/balancetheworld/wechat-pet/internal/app/ask"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/responses"
)

// OpenAIProvider 是 ai.Provider 的 OpenAI Responses API 适配器（文档 4）。
// 负责统一 Request 到 Responses API 的转换、发送、取消与响应归一化；
// 不自行检索旧聊天、读取业务数据、执行工具、修改任务项、切模型或增加重试。
type OpenAIProvider struct {
	client          openai.Client
	model           string
	reasoningEffort string
	observe         func(OpenAIObservation)
}

// NewOpenAIProvider 构造 OpenAI Provider。配置校验与现有 OpenAI 执行器保持一致；
// MaxRetries 固定为 0，重试由 Runtime 新建 Attempt 处理（文档 4.1 同一 Attempt 不允许隐藏重试）。
func NewOpenAIProvider(config OpenAIConfig) (*OpenAIProvider, error) {
	config.APIKey = strings.TrimSpace(config.APIKey)
	config.BaseURL = strings.TrimSpace(config.BaseURL)
	config.Model = strings.TrimSpace(config.Model)
	config.ReasoningEffort = strings.TrimSpace(config.ReasoningEffort)
	if config.APIKey == "" {
		return nil, errors.New("AI_API_KEY is required when AI is enabled")
	}
	if config.Model == "" {
		return nil, errors.New("AI_MODEL is required when AI is enabled")
	}
	if config.Timeout <= 0 {
		return nil, errors.New("AI_TIMEOUT_SECONDS must be greater than zero")
	}
	if config.BaseURL == "" {
		config.BaseURL = defaultOpenAIBaseURL
	}
	baseURL, err := url.Parse(config.BaseURL)
	if err != nil || (baseURL.Scheme != "http" && baseURL.Scheme != "https") || baseURL.Host == "" {
		return nil, errors.New("AI_BASE_URL must be a valid HTTP URL")
	}
	client := openai.NewClient(
		option.WithAPIKey(config.APIKey),
		option.WithBaseURL(strings.TrimRight(config.BaseURL, "/")),
		option.WithMaxRetries(0),
		option.WithRequestTimeout(config.Timeout),
	)
	return &OpenAIProvider{client: client, model: config.Model, reasoningEffort: config.ReasoningEffort, observe: config.Observer}, nil
}

// Complete 发起一次非流式模型调用并返回归一化响应（文档 4.2）。
// 预检查失败（能力/参数/预算）由调用方负责；本方法只做请求转换、发送与归一化。
func (p *OpenAIProvider) Complete(ctx context.Context, request Request) (result Response, resultErr error) {
	startedAt := time.Now()
	observation := OpenAIObservation{Operation: "provider_complete", Model: p.model, Status: "error"}
	defer func() {
		observation.Duration = time.Since(startedAt)
		if resultErr != nil {
			observation.ErrorCode = ProviderErrorCode(resultErr)
			observation.Retryable = !BlindRetryForbidden(observation.ErrorCode)
		}
		if p.observe != nil {
			p.observe(observation)
		}
	}()

	params, err := p.buildParams(request)
	if err != nil {
		return Response{}, NewProviderError(ErrProviderRequestInvalid, false, 0, err)
	}
	response, err := p.client.Responses.New(ctx, params)
	if err != nil {
		classified := classifyProviderError(ctx, err)
		fallback, ok := strictDisabledParams(params)
		if !ok || ProviderErrorCode(classified) != ErrProviderRequestInvalid {
			return Response{}, classified
		}
		retried, retryErr := p.client.Responses.New(ctx, fallback)
		if retryErr != nil {
			return Response{}, classifyProviderError(ctx, retryErr)
		}
		observation.StrictFallback = true
		response = retried
	}
	if response.Status == responses.ResponseStatusFailed {
		return Response{}, responseFailedError(response.Error)
	}
	observation.Model = string(response.Model)
	observation.InputTokens = response.Usage.InputTokens
	observation.OutputTokens = response.Usage.OutputTokens
	observation.TotalTokens = response.Usage.TotalTokens
	result = normalizeResponse(request, response)
	observation.Status = string(result.FinishReason)
	return result, nil
}

// Stream 发起一次流式模型调用，增量通过 emit 回调，返回完整归一化响应（文档 4.2）。
// 内部流序号不同于页面事件序号；工具参数分片仅在受控缓冲区组装，不形成可执行调用。
func (p *OpenAIProvider) Stream(ctx context.Context, request Request, emit func(StreamEvent) error) (result Response, resultErr error) {
	startedAt := time.Now()
	observation := OpenAIObservation{Operation: "provider_stream", Model: p.model, Status: "error"}
	defer func() {
		observation.Duration = time.Since(startedAt)
		if resultErr != nil {
			observation.ErrorCode = ProviderErrorCode(resultErr)
			observation.Retryable = !BlindRetryForbidden(observation.ErrorCode)
		}
		if p.observe != nil {
			p.observe(observation)
		}
	}()

	params, err := p.buildParams(request)
	if err != nil {
		return Response{}, NewProviderError(ErrProviderRequestInvalid, false, 0, err)
	}
	var response responses.Response
	seq := 0
	emitEvent := func(ev StreamEvent) error {
		ev.AttemptID = request.AttemptID
		ev.Sequence = seq
		seq++
		return emit(ev)
	}
	started := false
	consume := func(params responses.ResponseNewParams) (Response, error) {
		stream := p.client.Responses.NewStreaming(ctx, params)
		for stream.Next() {
			event := stream.Current()
			switch event.Type {
			case "response.created":
				created := event.AsResponseCreated()
				response.ID = created.Response.ID
				if !started {
					if err := emitEvent(StreamEvent{Type: StreamResponseStarted, BlockID: created.Response.ID}); err != nil {
						return Response{}, err
					}
					started = true
				}
			case "response.output_text.delta":
				delta := event.AsResponseOutputTextDelta()
				if err := emitEvent(StreamEvent{Type: StreamContentDelta, BlockID: delta.ItemID, ContentDelta: delta.Delta}); err != nil {
					return Response{}, err
				}
			case "response.reasoning_text.delta":
				if delta := reasoningTextDelta(event); delta != "" {
					if err := emitEvent(StreamEvent{Type: StreamReasoningDelta, BlockID: event.ItemID, ReasoningDelta: delta}); err != nil {
						return Response{}, err
					}
				}
			case "response.function_call_arguments.delta":
				delta := event.AsResponseFunctionCallArgumentsDelta()
				if err := emitEvent(StreamEvent{
					Type:          StreamToolCallDelta,
					BlockID:       delta.ItemID,
					ToolCallDelta: ToolCallDelta{ID: delta.ItemID, ArgumentsPartial: delta.Delta},
				}); err != nil {
					return Response{}, err
				}
			case "response.completed":
				response = event.AsResponseCompleted().Response
			case "response.failed":
				return Response{}, responseFailedError(event.AsResponseFailed().Response.Error)
			}
		}
		if err := stream.Err(); err != nil {
			return Response{}, classifyProviderError(ctx, err)
		}
		if response.ID == "" || response.Model == "" {
			return Response{}, NewProviderError(ErrProviderOutputInvalid, false, 0, errors.New("stream completed without a normalized response"))
		}
		if response.Status == responses.ResponseStatusFailed {
			return Response{}, responseFailedError(response.Error)
		}
		normalized := normalizeResponse(request, &response)
		if response.Usage.TotalTokens > 0 || response.Usage.InputTokens > 0 || response.Usage.OutputTokens > 0 {
			usage := normalizeUsage(response.Usage, request)
			if err := emitEvent(StreamEvent{Type: StreamUsageReported, Usage: &usage}); err != nil {
				return Response{}, err
			}
		}
		if err := emitEvent(StreamEvent{Type: StreamResponseFinished, BlockID: response.ID}); err != nil {
			return Response{}, err
		}
		return normalized, nil
	}
	result, resultErr = consume(params)
	if resultErr != nil {
		var providerErr *ProviderError
		if errors.As(resultErr, &providerErr) && providerErr.Code == ErrProviderRequestInvalid && !started {
			if fallback, ok := strictDisabledParams(params); ok {
				response = responses.Response{}
				observation.StrictFallback = true
				result, resultErr = consume(fallback)
			}
		}
	}
	if resultErr != nil {
		return Response{}, resultErr
	}
	observation.Model = string(response.Model)
	observation.InputTokens = response.Usage.InputTokens
	observation.OutputTokens = response.Usage.OutputTokens
	observation.TotalTokens = response.Usage.TotalTokens
	observation.Status = string(result.FinishReason)
	return result, nil
}

// reasoningTextDelta 读取推理正文分片。网关（如 tokenflux 的 deepseek-flash）会下发
// response.reasoning_text.delta，SDK v1.12 未收录该事件类型，因此只从原始事件取 delta。
func reasoningTextDelta(event responses.ResponseStreamEventUnion) string {
	var payload struct {
		Delta string `json:"delta"`
	}
	if err := json.Unmarshal([]byte(event.RawJSON()), &payload); err != nil {
		return ""
	}
	return payload.Delta
}

// buildParams 把统一 Request 转换为 Responses API 请求参数。
func (p *OpenAIProvider) buildParams(request Request) (responses.ResponseNewParams, error) {
	params := responses.ResponseNewParams{
		Model: p.model,
		Store: openai.Bool(false),
	}
	recordArrayProtocol := request.ResponseProtocol == askapp.RecordArrayV1
	if request.ResponseSchema != nil {
		schema, err := responseSchemaMap(request.ResponseSchema)
		if err != nil {
			return responses.ResponseNewParams{}, err
		}
		topLevelArray := isTopLevelArray(schema)
		recordArrayProtocol = recordArrayProtocol || topLevelArray
		format := responses.ResponseFormatTextConfigParamOfJSONSchema("pet_ask", schema)
		// 真实验证（文档 12.2、docs/t3-provider-verification.md）：strict 要求根节点为 object、
		// 所有 properties 进入 required、additionalProperties 为 false。顶层数组无法满足，只能非 strict；
		// 其余 object schema 保留 strict，Provider 拒绝时由 Complete 回退一次。
		format.OfJSONSchema.Strict = openai.Bool(!topLevelArray)
		params.Text = responses.ResponseTextConfigParam{Format: format}
	}
	input, instructions := buildInput(request)
	if recordArrayProtocol && len(request.Tools) > 0 {
		toolCatalog, err := json.Marshal(request.Tools)
		if err != nil {
			return responses.ResponseNewParams{}, err
		}
		instructions = strings.TrimSpace(strings.Join([]string{instructions, "【可用工具】\n仅在 record_array_v1 的 call 记录中提出工具调用，不要使用原生工具调用。\n" + string(toolCatalog)}, "\n\n"))
	}
	if instructions != "" {
		params.Instructions = openai.String(instructions)
	}
	if input.OfString.Valid() || len(input.OfInputItemList) > 0 {
		params.Input = input
	}
	if len(request.Tools) > 0 && !recordArrayProtocol {
		params.Tools = buildTools(request.Tools)
	}
	if request.MaxOutputTokens > 0 {
		params.MaxOutputTokens = openai.Int(int64(request.MaxOutputTokens))
	}
	applySamplingParams(&params, request.Parameters)
	if p.reasoningEffort != "" {
		params.Reasoning = responses.ReasoningParam{Effort: responses.ReasoningEffort(p.reasoningEffort)}
	}
	return params, nil
}

// buildInput 把有序 Message 与图片组装为 Instructions + Input item list（文档 5.2、11.5）。
// system/developer 消息进入 Instructions；其余进入有序对话；图片受控字节附加到最后一条 user 消息。
func buildInput(request Request) (responses.ResponseNewParamsInputUnion, string) {
	var instr []string
	var items []responses.ResponseInputItemUnionParam

	lastUserIndex := -1
	for i, msg := range request.Messages {
		if msg.Role == "user" {
			lastUserIndex = i
		}
	}
	for i, msg := range request.Messages {
		switch msg.Role {
		case "system", "developer":
			instr = append(instr, msg.Content)
		default:
			content := responses.ResponseInputMessageContentListParam{
				responses.ResponseInputContentParamOfInputText(msg.Content),
			}
			if i == lastUserIndex && len(request.ImageContent) > 0 {
				for _, img := range request.ImageContent {
					content = append(content, imageContentPart(img))
				}
			}
			items = append(items, responses.ResponseInputItemParamOfInputMessage(content, msg.Role))
		}
	}

	var input responses.ResponseNewParamsInputUnion
	if len(items) > 0 {
		input = responses.ResponseNewParamsInputUnion{OfInputItemList: items}
	}
	return input, strings.Join(instr, "\n\n")
}

// imageContentPart 把受控版本字节包装为 image input content part（文档 11.5）。
func imageContentPart(data []byte) responses.ResponseInputContentUnionParam {
	dataURL := "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(data)
	return responses.ResponseInputContentUnionParam{
		OfInputImage: &responses.ResponseInputImageParam{
			Detail:   responses.ResponseInputImageDetailAuto,
			ImageURL: openai.String(dataURL),
		},
	}
}

// buildTools 把候选工具转换为 Responses API 的 function tool 集合（文档 4.1）。
func buildTools(specs []ToolSpec) []responses.ToolUnionParam {
	tools := make([]responses.ToolUnionParam, 0, len(specs))
	for _, spec := range specs {
		parameters, ok := spec.Parameters.(map[string]any)
		if !ok {
			parameters = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		// 真实验证（文档 12.2）：strict 函数调用要求所有属性必填。含可选字段的工具参数
		// （如 search_health_records 的 category/medical_type 等）降级为非 strict，避免跨
		// Provider 的 strict 校验差异导致 400；全必填的简单工具保留 strict。
		tools = append(tools, responses.ToolUnionParam{
			OfFunction: &responses.FunctionToolParam{
				Name:        spec.Name,
				Description: openai.String(spec.Description),
				Parameters:  parameters,
				Strict:      openai.Bool(!hasOptionalProperties(parameters)),
			},
		})
	}
	return tools
}

// isTopLevelArray 报告 JSON Schema 顶层是否为数组（文档 12.2：顶层数组与 strict 不兼容）。
func isTopLevelArray(schema map[string]any) bool {
	t, _ := schema["type"].(string)
	return t == "array"
}

// strictDisabledParams 返回关闭结构化输出 strict 的参数副本；未启用 strict 时 ok 为 false。
func strictDisabledParams(params responses.ResponseNewParams) (responses.ResponseNewParams, bool) {
	if params.Text.Format.OfJSONSchema == nil || !params.Text.Format.OfJSONSchema.Strict.Value {
		return params, false
	}
	format := *params.Text.Format.OfJSONSchema
	format.Strict = openai.Bool(false)
	params.Text.Format.OfJSONSchema = &format
	return params, true
}

// hasOptionalProperties 报告参数 Schema 是否存在未列入 required 的可选属性。
// 兼容 JSON 反序列化的 []any 与 Go 字面量 []string 两种 required 表达（文档 4.1、12.2）。
func hasOptionalProperties(schema map[string]any) bool {
	props, _ := schema["properties"].(map[string]any)
	if len(props) == 0 {
		return false
	}
	required := map[string]bool{}
	switch r := schema["required"].(type) {
	case []string:
		for _, k := range r {
			required[k] = true
		}
	case []any:
		for _, v := range r {
			if s, ok := v.(string); ok {
				required[s] = true
			}
		}
	}
	for k := range props {
		if !required[k] {
			return true
		}
	}
	return false
}

// responseSchemaMap 校验响应 Schema 为 JSON 对象。
func responseSchemaMap(schema any) (map[string]any, error) {
	m, ok := schema.(map[string]any)
	if !ok || len(m) == 0 {
		return nil, errors.New("response schema must be a non-empty JSON object")
	}
	return m, nil
}

// applySamplingParams 应用可选采样参数（文档 4.4）。只做已验证的语义等价转换，
// 不擅自截断数值；不支持的可选参数由调用方按 OmitOptional 决定是否省略。
func applySamplingParams(params *responses.ResponseNewParams, parameters []Parameter) {
	for _, p := range parameters {
		switch p.Name {
		case "temperature":
			if v, ok := asFloat64(p.Value); ok {
				params.Temperature = openai.Float(v)
			}
		case "top_p":
			if v, ok := asFloat64(p.Value); ok {
				params.TopP = openai.Float(v)
			}
		}
	}
}

func asFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	default:
		return 0, false
	}
}

// normalizeResponse 把 Responses API 响应归一化为统一 Response（文档 4.2）。
func normalizeResponse(request Request, response *responses.Response) Response {
	toolCalls := make([]ToolCall, 0)
	hasToolCall := false
	for _, item := range response.Output {
		call := item.AsFunctionCall()
		if call.Name != "" {
			hasToolCall = true
			toolCalls = append(toolCalls, ToolCall{
				ID:        call.CallID,
				Name:      call.Name,
				Arguments: call.Arguments,
			})
		}
	}
	finish := normalizeFinishReason(string(response.Status), response.IncompleteDetails.Reason, hasToolCall)
	return Response{
		AttemptID:         request.AttemptID,
		ProviderRequestID: response.ID,
		Model:             string(response.Model),
		Text:              response.OutputText(),
		ToolCalls:         toolCalls,
		FinishReason:      finish,
		Usage:             normalizeUsage(response.Usage, request),
	}
}

// normalizeFinishReason 归一化结束原因（文档 4.3）：未知不按正常完成，工具调用结束仍属当前 Run。
func normalizeFinishReason(status, incompleteReason string, hasToolCall bool) FinishReason {
	switch status {
	case "completed":
		if hasToolCall {
			return FinishToolCalls
		}
		return FinishStop
	case "incomplete":
		switch incompleteReason {
		case "max_output_tokens":
			return FinishLength
		case "content_filter":
			return FinishRefusal
		default:
			return FinishUnknown
		}
	default:
		return FinishUnknown
	}
}

// normalizeUsage 归一化用量（文档 4.2、11.4）：供应商未报告任何 Token 时记未知，不按零费用处理；
// 图片输入数量从请求推断为可观测的发送下限，不代表供应商已按该数量计费。
func normalizeUsage(u responses.ResponseUsage, request Request) Usage {
	usage := Usage{
		InputTokens:  u.InputTokens,
		OutputTokens: u.OutputTokens,
		TotalTokens:  u.TotalTokens,
		Source:       "provider",
		Complete:     u.TotalTokens > 0 || u.InputTokens > 0 || u.OutputTokens > 0,
	}
	if len(request.ImageContent) > 0 {
		n := len(request.ImageContent)
		usage.ImageCount = &n
	}
	return usage.Normalize()
}

// responseFailedError 把 Response 明确失败归一化为稳定分类。
func responseFailedError(e responses.ResponseError) error {
	code := ErrProviderFailed
	if e.Code == "content_filter" {
		code = ErrProviderRequestInvalid
	}
	return NewProviderError(code, false, 0, errors.New(e.Message))
}

// classifyProviderError 把 OpenAI SDK 错误归类为稳定错误分类（文档 4.1、9）。
// 与 ask 层 ExecutorError 解耦，返回 ai.ProviderError。
func classifyProviderError(ctx context.Context, err error) error {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return NewProviderError(ErrProviderTimeout, true, 0, err)
	}
	if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
		return NewProviderError(ErrProviderCanceled, true, 0, err)
	}
	var apiError *openai.Error
	if errors.As(err, &apiError) {
		retryAfter := retryAfterDuration(apiError.Response)
		if isQuotaExhausted(apiError) {
			return NewProviderError(ErrProviderQuotaExhausted, false, 0, err)
		}
		switch {
		case apiError.StatusCode == http.StatusRequestTimeout:
			return NewProviderError(ErrProviderTimeout, true, retryAfter, err)
		case apiError.StatusCode == http.StatusConflict || apiError.StatusCode == http.StatusTooManyRequests || apiError.StatusCode >= http.StatusInternalServerError:
			code := ErrProviderUnavailable
			if apiError.StatusCode == http.StatusTooManyRequests {
				code = ErrProviderRateLimited
			}
			return NewProviderError(code, true, retryAfter, err)
		case apiError.StatusCode == http.StatusUnauthorized || apiError.StatusCode == http.StatusForbidden:
			return NewProviderError(ErrProviderAuthFailed, false, 0, err)
		case apiError.StatusCode == http.StatusBadRequest || apiError.StatusCode == http.StatusNotFound || apiError.StatusCode == http.StatusUnprocessableEntity:
			return NewProviderError(ErrProviderRequestInvalid, false, 0, err)
		default:
			return NewProviderError(ErrProviderFailed, false, 0, err)
		}
	}
	var networkError net.Error
	if errors.As(err, &networkError) {
		if networkError.Timeout() {
			return NewProviderError(ErrProviderTimeout, true, 0, err)
		}
		return NewProviderError(ErrProviderUnavailable, true, 0, err)
	}
	return NewProviderError(ErrProviderUnavailable, true, 0, err)
}
