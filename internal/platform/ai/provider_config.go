package ai

import "time"

const defaultOpenAIBaseURL = "https://api.openai.com/v1"

// OpenAIConfig 是 Provider 适配器的连接配置（v2，文档 4.1）。APIKey/BaseURL/Model 为
// 必填，Timeout 为单次调用的上限，Observer 用于回传每次调用的可观测结果。
type OpenAIConfig struct {
	APIKey   string
	BaseURL  string
	Model    string
	Timeout  time.Duration
	Observer func(OpenAIObservation)
}

// OpenAIObservation 是 Provider 单次调用的可观测结果（v2，文档 11.4）。
type OpenAIObservation struct {
	Operation    string
	Model        string
	Duration     time.Duration
	InputTokens  int64
	OutputTokens int64
	TotalTokens  int64
	Status       string
	ErrorCode    string
	Retryable    bool
}
