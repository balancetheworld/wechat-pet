package auth

import (
	"time"

	appauth "github.com/balancetheworld/wechat-pet/internal/app/auth"
	"github.com/balancetheworld/wechat-pet/internal/middleware"
	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.RouterGroup, service *appauth.Service) {
	if service == nil {
		return
	}
	handler := NewHandler(service)
	group := router.Group("/auth")
	group.POST("/login", middleware.LoginRateLimit(10, time.Minute), handler.Login)
}
