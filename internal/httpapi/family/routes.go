package family

import (
	familyapp "github.com/balancetheworld/wechat-pet/internal/app/family"
	"github.com/balancetheworld/wechat-pet/internal/middleware"
	jwtpkg "github.com/balancetheworld/wechat-pet/internal/pkg/jwt"
	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.RouterGroup, signer *jwtpkg.Signer, service *familyapp.Service, repository familyapp.Repository) {
	if signer == nil || service == nil || repository == nil {
		return
	}
	handler := NewHandler(service)
	authenticated := router.Group("/families", middleware.RequireAuth(signer))
	authenticated.POST("", handler.Create)
	authenticated.POST("/join-applications", handler.ApplyJoin)
	authenticated.GET("/join-applications/me", handler.MyJoinApplication)

	familyGroup := authenticated.Group("", middleware.RequireFamily(repository))
	familyGroup.GET("/current", handler.Current)
	familyGroup.GET("/members", handler.Members)

	ownerGroup := familyGroup.Group("", middleware.RequireOwner())
	ownerGroup.GET("/join-applications", handler.PendingApplications)
	ownerGroup.POST("/join-applications/:member_id/approve", handler.ApproveApplication)
	ownerGroup.POST("/join-applications/:member_id/reject", handler.RejectApplication)
	ownerGroup.POST("/members/:member_id/remove", handler.RemoveMember)
}
