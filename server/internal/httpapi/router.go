package httpapi

import (
	"log/slog"

	"github.com/balancetheworld/wechat-pet/server/internal/pkg/response"
	"github.com/gin-gonic/gin"
)

// New creates the API router and installs process-level middleware.
func New(loggers ...*slog.Logger) *gin.Engine {
	logger := slog.Default()
	if len(loggers) > 0 && loggers[0] != nil {
		logger = loggers[0]
	}
	router := gin.New()
	router.Use(requestID(), accessLog(logger), recovery(logger))
	router.GET("/healthz", func(c *gin.Context) {
		response.Success(c, map[string]string{"status": "ok"})
	})
	v1 := router.Group("/api/v1")
	registerV1Routes(v1)

	return router
}
