package ai

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/openai/openai-go"
)

// ProviderError 是 Provider 层的稳定错误分类（文档 4.1、4.3、9）。
// 实现 error 接口；与 ask 层的 ExecutorError 解耦：Provider 只返回稳定分类，
// 由 Runtime 依据 BlindRetryForbidden 决定恢复策略，不在适配器内部静默重试或切 Provider。
type ProviderError struct {
	Code       string        // 稳定错误码（见 ErrProvider* 常量）
	Retryable  bool          // 是否具备有限恢复条件
	RetryAfter time.Duration // 建议等待时长（限流时；0 表示无建议）
	Cause      error         // 底层原因
}

func (e *ProviderError) Error() string {
	if e.Cause == nil {
		return e.Code
	}
	return fmt.Sprintf("%s: %v", e.Code, e.Cause)
}

func (e *ProviderError) Unwrap() error { return e.Cause }

// NewProviderError 构造稳定分类错误。code 为空或非契约内值时回退为 ErrProviderFailed。
func NewProviderError(code string, retryable bool, retryAfter time.Duration, cause error) error {
	code = strings.TrimSpace(code)
	if !knownProviderErrorCode(code) {
		code = ErrProviderFailed
	}
	return &ProviderError{Code: code, Retryable: retryable, RetryAfter: retryAfter, Cause: cause}
}

func knownProviderErrorCode(code string) bool {
	switch code {
	case ErrProviderTimeout, ErrProviderCanceled, ErrProviderRateLimited,
		ErrProviderQuotaExhausted, ErrProviderUnavailable, ErrProviderAuthFailed,
		ErrProviderRequestInvalid, ErrProviderContextTooLong, ErrProviderOutputInvalid,
		ErrProviderCapabilityUnsupported, ErrProviderFailed:
		return true
	default:
		return false
	}
}

// ProviderErrorCode 从 error 提取稳定错误码；非 ProviderError 返回 ErrProviderFailed。
func ProviderErrorCode(err error) string {
	var pe *ProviderError
	if errors.As(err, &pe) {
		return pe.Code
	}
	return ErrProviderFailed
}

// IsProviderErrorCode 报告 error 是否为指定稳定错误码。
func IsProviderErrorCode(err error, code string) bool {
	var pe *ProviderError
	if errors.As(err, &pe) {
		return pe.Code == code
	}
	return false
}

// retryAfterDuration 从 HTTP 响应头解析 Retry-After（秒数或 HTTP 日期）。
func retryAfterDuration(response *http.Response) time.Duration {
	if response == nil {
		return 0
	}
	value := strings.TrimSpace(response.Header.Get("Retry-After"))
	if seconds, err := strconv.Atoi(value); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	if at, err := http.ParseTime(value); err == nil {
		return max(time.Until(at), 0)
	}
	return 0
}

// isQuotaExhausted 判断 OpenAI API 错误是否表示额度耗尽。
func isQuotaExhausted(apiError *openai.Error) bool {
	if apiError == nil {
		return false
	}
	value := strings.ToUpper(strings.TrimSpace(apiError.Code + " " + apiError.Message + " " + apiError.RawJSON()))
	if apiError.Response != nil && apiError.Response.Body != nil {
		if body, err := io.ReadAll(apiError.Response.Body); err == nil {
			apiError.Response.Body = io.NopCloser(bytes.NewReader(body))
			value += " " + strings.ToUpper(string(body))
		}
	}
	return strings.Contains(value, "EXCEED_TOKEN_QUOTA_LIMIT") || strings.Contains(value, "QUOTA_EXCEEDED")
}
