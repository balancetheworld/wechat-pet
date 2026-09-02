package pet

import (
	familyapp "github.com/balancetheworld/wechat-pet/internal/app/family"
	petapp "github.com/balancetheworld/wechat-pet/internal/app/pet"
	"github.com/balancetheworld/wechat-pet/internal/middleware"
	jwtpkg "github.com/balancetheworld/wechat-pet/internal/pkg/jwt"
	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.RouterGroup, signer *jwtpkg.Signer, familyRepository familyapp.ActiveFamilyRepository, service *petapp.Service) {
	if signer == nil || familyRepository == nil || service == nil {
		return
	}
	handler := NewHandler(service)
	group := router.Group("/pets", middleware.RequireAuth(signer), middleware.RequireFamily(familyRepository))
	group.GET("", handler.List)
	group.POST("", handler.Create)
	group.GET("/:pet_id", handler.Get)
	group.PATCH("/:pet_id", handler.Update)
	group.DELETE("/:pet_id", handler.Delete)
}
