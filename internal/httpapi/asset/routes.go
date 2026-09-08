package asset

import (
	familyapp "github.com/balancetheworld/wechat-pet/internal/app/family"
	"github.com/balancetheworld/wechat-pet/internal/middleware"
	jwtpkg "github.com/balancetheworld/wechat-pet/internal/pkg/jwt"
	fileservice "github.com/balancetheworld/wechat-pet/internal/service/file"
	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.RouterGroup, signer *jwtpkg.Signer, service *fileservice.Service, localUploadDir string, family ...familyapp.ActiveFamilyRepository) {
	if signer == nil || service == nil {
		return
	}
	handler := NewHandler(service, family...)
	handler.SetUploadDir(localUploadDir)
	router.Group("/assets", middleware.RequireAuth(signer)).POST("/upload", handler.Upload)
	router.GET("/uploads/:asset_id", func(c *gin.Context) {
		c.Set("asset_token_signer", signer)
		handler.Download(c)
	})
}
