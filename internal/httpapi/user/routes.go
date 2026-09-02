package user

import (
	appuser "github.com/balancetheworld/wechat-pet/internal/app/user"
	"github.com/balancetheworld/wechat-pet/internal/middleware"
	jwtpkg "github.com/balancetheworld/wechat-pet/internal/pkg/jwt"
	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.RouterGroup, signer *jwtpkg.Signer, service *appuser.Service) {
	if signer == nil || service == nil {
		return
	}
	handler := NewHandler(service)
	group := router.Group("/users", middleware.RequireAuth(signer))
	group.GET("/me", handler.Me)
	group.PATCH("/me", handler.UpdateProfile)
}
