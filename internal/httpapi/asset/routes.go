package asset

import (
	"github.com/balancetheworld/wechat-pet/internal/middleware"
	jwtpkg "github.com/balancetheworld/wechat-pet/internal/pkg/jwt"
	fileservice "github.com/balancetheworld/wechat-pet/internal/service/file"
	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.RouterGroup, signer *jwtpkg.Signer, service *fileservice.Service) {
	if signer == nil || service == nil {
		return
	}
	handler := NewHandler(service)
	router.Group("/assets", middleware.RequireAuth(signer)).POST("/upload", handler.Upload)
}
