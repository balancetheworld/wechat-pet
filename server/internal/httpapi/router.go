package httpapi

import (
	"log/slog"

	appauth "github.com/balancetheworld/wechat-pet/server/internal/app/auth"
	appuser "github.com/balancetheworld/wechat-pet/server/internal/app/user"
	jwtpkg "github.com/balancetheworld/wechat-pet/server/internal/pkg/jwt"
	"github.com/balancetheworld/wechat-pet/server/internal/pkg/response"
	"github.com/gin-gonic/gin"
)

type Dependencies struct {
	AuthService *appauth.Service
	UserService *appuser.Service
	TokenSigner *jwtpkg.Signer
}

// New creates the API router and installs process-level middleware.
func New(loggers ...*slog.Logger) *gin.Engine {
	return NewWithDependencies(Dependencies{}, loggers...)
}

func NewWithDependencies(dependencies Dependencies, loggers ...*slog.Logger) *gin.Engine {
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
	registerV1Routes(v1, dependencies)

	return router
}
