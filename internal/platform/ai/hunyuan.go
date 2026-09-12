package ai

import (
	"context"
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
)

const defaultHunyuanBaseURL = "https://api.hunyuan.cloud.tencent.com/v1"

type HunyuanExecutor struct {
	client  openai.Client
	model   string
	observe func(OpenAIObservation)
}

func NewHunyuanExecutor(config OpenAIConfig) (*HunyuanExecutor, error) {
	config.APIKey = strings.TrimSpace(config.APIKey)
	config.BaseURL = strings.TrimSpace(config.BaseURL)
	config.Model = strings.TrimSpace(config.Model)
	if config.APIKey == "" {
		return nil, errors.New("AI_API_KEY is required when AI is enabled")
	}
	if config.Model == "" {
		return nil, errors.New("AI_MODEL is required when AI is enabled")
	}
	if config.Timeout <= 0 {
		return nil, errors.New("AI_TIMEOUT_SECONDS must be greater than zero")
	}
	if config.BaseURL == "" || config.BaseURL == defaultOpenAIBaseURL {
		config.BaseURL = defaultHunyuanBaseURL
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
	return &HunyuanExecutor{client: client, model: config.Model, observe: config.Observer}, nil
}

func (e *HunyuanExecutor) Route(ctx context.Context, input askapp.IntentInput) (decision askapp.IntentDecision, resultErr error) {
	startedAt := time.Now()
	observation := OpenAIObservation{Operation: "intent_route", Model: e.model, Status: "error"}
	defer func() {
		observation.Duration = time.Since(startedAt)
		if resultErr != nil {
			observation.ErrorCode, observation.Retryable = askapp.ExecutorErrorDetails(resultErr)
		}
		if e.observe != nil {
			e.observe(observation)
		}
	}()
	prompt, err := json.Marshal(map[string]any{"question": input.Turn.Input, "messages": input.Messages})
	if err != nil {
		return askapp.IntentDecision{}, askapp.NewExecutorError("provider_request_invalid", false, 0, err)
	}
	response, err := e.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model: e.model,
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(intentInstructions + " 请严格返回一个 JSON 对象，不要使用 Markdown 代码块。"),
			openai.UserMessage(string(prompt)),
		},
		MaxTokens: openai.Int(300),
	})
	if err != nil {
		return askapp.IntentDecision{}, classifyHunyuanError(ctx, err)
	}
	if len(response.Choices) == 0 {
		return askapp.IntentDecision{}, askapp.NewExecutorError("provider_output_invalid", false, 0, errors.New("chat completion returned no choices"))
	}
	observation.Model = response.Model
	observation.InputTokens = response.Usage.PromptTokens
	observation.OutputTokens = response.Usage.CompletionTokens
	observation.TotalTokens = response.Usage.TotalTokens
	decision, err = parseIntentDecision(response.Choices[0].Message.Content)
	if err != nil {
		return askapp.IntentDecision{}, err
	}
	observation.Status = string(decision.Intent)
	return decision, nil
}

func (e *HunyuanExecutor) Execute(ctx context.Context, input askapp.RunInput) (decision askapp.RunDecision, resultErr error) {
	startedAt := time.Now()
	observation := OpenAIObservation{Operation: "analysis", Model: e.model, Status: "error"}
	defer func() {
		observation.Duration = time.Since(startedAt)
		if resultErr != nil {
			observation.ErrorCode, observation.Retryable = askapp.ExecutorErrorDetails(resultErr)
		}
		if e.observe != nil {
			e.observe(observation)
		}
	}()
	messages, err := hunyuanMessages(input)
	if err != nil {
		return askapp.RunDecision{}, askapp.NewExecutorError("provider_request_invalid", false, 0, err)
	}
	response, err := e.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model:     e.model,
		Messages:  messages,
		MaxTokens: openai.Int(1200),
	})
	if err != nil {
		return askapp.RunDecision{}, classifyHunyuanError(ctx, err)
	}
	if len(response.Choices) == 0 {
		return askapp.RunDecision{}, askapp.NewExecutorError("provider_output_invalid", false, 0, errors.New("chat completion returned no choices"))
	}
	observation.Model = response.Model
	observation.InputTokens = response.Usage.PromptTokens
	observation.OutputTokens = response.Usage.CompletionTokens
	observation.TotalTokens = response.Usage.TotalTokens
	decision, err = parseHunyuanDecision(response.Choices[0].Message.Content)
	if err != nil {
		return askapp.RunDecision{}, err
	}
	observation.Status = string(decision.Status)
	return decision, nil
}

func (e *HunyuanExecutor) ExecuteStream(ctx context.Context, input askapp.RunInput, emit func(string) error) (decision askapp.RunDecision, resultErr error) {
	startedAt := time.Now()
	observation := OpenAIObservation{Operation: "analysis", Model: e.model, Status: "error"}
	defer func() {
		observation.Duration = time.Since(startedAt)
		if resultErr != nil {
			observation.ErrorCode, observation.Retryable = askapp.ExecutorErrorDetails(resultErr)
		}
		if e.observe != nil {
			e.observe(observation)
		}
	}()
	messages, err := hunyuanMessages(input)
	if err != nil {
		return askapp.RunDecision{}, askapp.NewExecutorError("provider_request_invalid", false, 0, err)
	}
	stream := e.client.Chat.Completions.NewStreaming(ctx, openai.ChatCompletionNewParams{
		Model:     e.model,
		Messages:  messages,
		MaxTokens: openai.Int(1200),
	})
	var content strings.Builder
	for stream.Next() {
		chunk := stream.Current()
		if len(chunk.Choices) > 0 {
			content.WriteString(chunk.Choices[0].Delta.Content)
		}
		if chunk.Model != "" {
			observation.Model = chunk.Model
		}
		observation.InputTokens = chunk.Usage.PromptTokens
		observation.OutputTokens = chunk.Usage.CompletionTokens
		observation.TotalTokens = chunk.Usage.TotalTokens
	}
	if err := stream.Err(); err != nil {
		return askapp.RunDecision{}, classifyHunyuanError(ctx, err)
	}
	decision, err = parseHunyuanDecision(content.String())
	if err != nil {
		return askapp.RunDecision{}, err
	}
	for _, delta := range safeDeltasFromDecision(decision) {
		if err := emit(delta); err != nil {
			return askapp.RunDecision{}, err
		}
	}
	observation.Status = string(decision.Status)
	return decision, nil
}

func hunyuanMessages(input askapp.RunInput) ([]openai.ChatCompletionMessageParamUnion, error) {
	prompt, err := json.Marshal(newPromptInput(input))
	if err != nil {
		return nil, err
	}
	return []openai.ChatCompletionMessageParamUnion{
		openai.SystemMessage(analysisInstructions + " 请严格返回一个 JSON 对象，不要使用 Markdown 代码块。"),
		openai.UserMessage(string(prompt)),
	}, nil
}

func parseHunyuanDecision(value string) (askapp.RunDecision, error) {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "```") && strings.HasSuffix(value, "```") {
		value = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(value, "```json"), "```"))
	}
	var output analysisOutput
	if err := json.Unmarshal([]byte(value), &output); err != nil {
		return askapp.RunDecision{}, askapp.NewExecutorError("provider_output_invalid", false, 0, err)
	}
	decision := output.runDecision()
	if err := askapp.ValidateAnalysisOutput(decision); err != nil {
		return askapp.RunDecision{}, askapp.NewExecutorError("provider_output_invalid", false, 0, err)
	}
	return decision, nil
}

func safeDeltasFromDecision(decision askapp.RunDecision) []string {
	if decision.Status == askapp.RunWaitingInput {
		if value, ok := decision.Data["question"].(string); ok {
			return safeDeltas(analysisOutput{Question: value})
		}
	}
	if value, ok := decision.Data["current_assessment"].(string); ok {
		return safeDeltas(analysisOutput{Status: string(askapp.RunCompleted), CurrentAssessment: value})
	}
	return nil
}

func classifyHunyuanError(ctx context.Context, err error) error {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return askapp.NewExecutorError("provider_timeout", true, 0, err)
	}
	if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
		return askapp.NewExecutorError("provider_canceled", true, 0, err)
	}
	var apiError *openai.Error
	if errors.As(err, &apiError) {
		retryAfter := retryAfterDuration(apiError.Response)
		if isQuotaExhausted(apiError) {
			return askapp.NewExecutorError("provider_quota_exhausted", false, 0, err)
		}
		switch {
		case apiError.StatusCode == http.StatusRequestTimeout:
			return askapp.NewExecutorError("provider_timeout", true, retryAfter, err)
		case apiError.StatusCode == http.StatusConflict || apiError.StatusCode == http.StatusTooManyRequests || apiError.StatusCode >= http.StatusInternalServerError:
			code := "provider_unavailable"
			if apiError.StatusCode == http.StatusTooManyRequests {
				code = "provider_rate_limited"
			}
			return askapp.NewExecutorError(code, true, retryAfter, err)
		case apiError.StatusCode == http.StatusUnauthorized || apiError.StatusCode == http.StatusForbidden:
			return askapp.NewExecutorError("provider_auth_failed", false, 0, err)
		case apiError.StatusCode == http.StatusBadRequest || apiError.StatusCode == http.StatusNotFound || apiError.StatusCode == http.StatusUnprocessableEntity:
			return askapp.NewExecutorError("provider_request_invalid", false, 0, err)
		default:
			return askapp.NewExecutorError("provider_failed", false, 0, err)
		}
	}
	var networkError net.Error
	if errors.As(err, &networkError) {
		if networkError.Timeout() {
			return askapp.NewExecutorError("provider_timeout", true, 0, err)
		}
		return askapp.NewExecutorError("provider_unavailable", true, 0, err)
	}
	return askapp.NewExecutorError("provider_unavailable", true, 0, err)
}
