package httpapi

import (
	"log/slog"
	"runtime/debug"

	appErrors "github.com/balancetheworld/wechat-pet/server/internal/pkg/errors"
	"github.com/balancetheworld/wechat-pet/server/internal/pkg/response"
	"github.com/gin-gonic/gin"
)

func recovery(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if recovered := recover(); recovered != nil {
				logger.Error("http panic", "request_id", requestIDValue(c), "panic", recovered, "stack", string(debug.Stack()))
				response.Fail(c, appErrors.Internal(nil))
				c.Abort()
			}
		}()
		c.Next()
	}
}

func requestIDValue(c *gin.Context) string {
	if value, exists := c.Get(requestIDKey); exists {
		if id, ok := value.(string); ok && id != "" {
			return id
		}
	}
	return "unknown"
}
