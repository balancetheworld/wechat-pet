package ask

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// ExecutorError 是模型调用失败的错误分类（v2 决策循环与 Run Worker 共用）。
// 与 platform/ai 层的 ProviderError 解耦：ai 层适配器（AgentModelAdapter）负责把
// ProviderError 桥接为 ExecutorError，ask 层不直接依赖 ai 包。
type ExecutorError struct {
	Code       string
	Retryable  bool
	RetryAfter time.Duration
	Cause      error
}

func (e *ExecutorError) Error() string {
	if e.Cause == nil {
		return e.Code
	}
	return fmt.Sprintf("%s: %v", e.Code, e.Cause)
}

func (e *ExecutorError) Unwrap() error {
	return e.Cause
}

func NewExecutorError(code string, retryable bool, retryAfter time.Duration, cause error) error {
	code = strings.TrimSpace(code)
	if code == "" {
		code = "provider_failed"
	}
	return &ExecutorError{Code: code, Retryable: retryable, RetryAfter: retryAfter, Cause: cause}
}

func ExecutorRetryDelay(err error, fallback time.Duration) time.Duration {
	var executorError *ExecutorError
	if errors.As(err, &executorError) && executorError.RetryAfter > fallback {
		return executorError.RetryAfter
	}
	return fallback
}

func ExecutorErrorDetails(err error) (string, bool) {
	var executorError *ExecutorError
	if !errors.As(err, &executorError) {
		return "executor_failed", false
	}
	return executorError.Code, executorError.Retryable
}
