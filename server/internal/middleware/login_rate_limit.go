package middleware

import (
	"net/http"
	"strings"
	"sync"
	"time"

	appErrors "github.com/balancetheworld/wechat-pet/server/internal/pkg/errors"
	"github.com/balancetheworld/wechat-pet/server/internal/pkg/response"
	"github.com/gin-gonic/gin"
)

type rateLimitEntry struct {
	count     int
	resetTime time.Time
}

func LoginRateLimit(limit int, window time.Duration) gin.HandlerFunc {
	if limit <= 0 {
		limit = 10
	}
	if window <= 0 {
		window = time.Minute
	}
	var mutex sync.Mutex
	entries := map[string]rateLimitEntry{}
	return func(c *gin.Context) {
		key := c.ClientIP() + ":" + strings.TrimSpace(c.GetHeader("X-Device-ID"))
		now := time.Now()
		mutex.Lock()
		entry := entries[key]
		if now.After(entry.resetTime) {
			entry = rateLimitEntry{resetTime: now.Add(window)}
		}
		entry.count++
		entries[key] = entry
		allowed := entry.count <= limit
		mutex.Unlock()
		if !allowed {
			response.Fail(c, &appErrors.AppError{HTTPStatus: http.StatusTooManyRequests, Code: 42901, Message: "登录请求过于频繁"})
			c.Abort()
			return
		}
		c.Next()
	}
}
