package ask

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

type ExecutorError struct {
	Code       string
	Retryable  bool
	RetryAfter time.Duration
	Cause      error
}

type StreamingExecutor interface {
	ExecuteStream(context.Context, RunInput, func(string) error) (RunDecision, error)
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

type DeterministicExecutor struct{}

func (DeterministicExecutor) Execute(context.Context, RunInput) (RunDecision, error) {
	return RunDecision{
		Status:    RunWaitingInput,
		RiskLevel: RiskUnknown,
		EventType: "assistant.question",
		Data: map[string]any{
			"question": "请补充宠物目前最明显的一个症状，以及症状从什么时候开始。",
		},
	}, nil
}
