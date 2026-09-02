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
	group.GET("/:pet_id/profile", handler.Profile)
	group.PATCH("/:pet_id/profile", handler.Resource)
	group.GET("/:pet_id/dates", handler.Resource)
	group.GET("/:pet_id/:resource", handler.Resource)
	group.POST("/:pet_id/:resource", handler.Resource)
	group.PUT("/:pet_id/:resource", handler.Resource)
	group.PATCH("/:pet_id/:resource/:resource_id", handler.Resource)
	group.DELETE("/:pet_id/:resource/:resource_id", handler.Resource)
}
