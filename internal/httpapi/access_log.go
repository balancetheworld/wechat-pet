package httpapi

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
)

func accessLog(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		started := time.Now()
		c.Next()
		logger.Info("http request",
			"path", c.Request.URL.Path,
			"method", c.Request.Method,
			"status", c.Writer.Status(),
			"latency", time.Since(started).String(),
			"request_id", requestIDValue(c),
			"user_id", c.GetString("user_id"),
		)
	}
}
