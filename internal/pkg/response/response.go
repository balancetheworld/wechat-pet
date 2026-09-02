package response

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"

	appErrors "github.com/balancetheworld/wechat-pet/internal/pkg/errors"
	"github.com/gin-gonic/gin"
)

type Response[T any] struct {
	Code      int    `json:"code"`
	Message   string `json:"msg"`
	Data      T      `json:"data"`
	RequestID string `json:"request_id"`
}

func Success[T any](c *gin.Context, data T) {
	write(c, http.StatusOK, Response[T]{Code: 0, Message: "ok", Data: data, RequestID: requestID(c)})
}

func Fail(c *gin.Context, appError *appErrors.AppError) {
	if appError == nil {
		appError = appErrors.Internal(nil)
	}
	status := appError.HTTPStatus
	if status < http.StatusBadRequest || status > 599 {
		status = http.StatusInternalServerError
	}
	code := appError.Code
	message := appError.Message
	if status >= http.StatusInternalServerError {
		code = appErrors.CodeInternal
		message = "服务器内部错误"
	}
	if code == 0 {
		code = appErrors.CodeInternal
	}
	if message == "" {
		message = "请求失败"
	}
	write(c, status, Response[any]{Code: code, Message: message, Data: nil, RequestID: requestID(c)})
}

func write[T any](c *gin.Context, status int, body Response[T]) {
	c.JSON(status, body)
}

func requestID(c *gin.Context) string {
	if value, exists := c.Get("request_id"); exists {
		if id, ok := value.(string); ok && id != "" {
			return id
		}
	}
	if id := c.GetHeader("X-Request-ID"); id != "" {
		c.Header("X-Request-ID", id)
		return id
	}
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err == nil {
		id := hex.EncodeToString(bytes[:])
		c.Header("X-Request-ID", id)
		return id
	}
	return "unknown"
}
