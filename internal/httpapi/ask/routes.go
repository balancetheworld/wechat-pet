package ask

import (
	askapp "github.com/balancetheworld/wechat-pet/internal/app/ask"
	familyapp "github.com/balancetheworld/wechat-pet/internal/app/family"
	"github.com/balancetheworld/wechat-pet/internal/middleware"
	jwtpkg "github.com/balancetheworld/wechat-pet/internal/pkg/jwt"
	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.RouterGroup, signer *jwtpkg.Signer, familyRepository familyapp.ActiveFamilyRepository, service *askapp.Service) {
	if signer == nil || familyRepository == nil || service == nil {
		return
	}
	handler := NewHandler(service)
	group := router.Group("", middleware.RequireAuth(signer), middleware.RequireFamily(familyRepository))
	group.POST("/pets/:pet_id/ask/sessions", handler.CreateSession)
	group.POST("/ask/sessions", handler.CreateSessionFromInput)
	group.POST("/ask/sessions/:session_id/runs/:run_id/process", handler.ProcessRun)
	group.POST("/ask/sessions/:session_id/runs/:run_id/reply", handler.Reply)
	group.GET("/ask/sessions/:session_id", handler.GetSession)
	group.GET("/ask/sessions/:session_id/runs/:run_id/events", handler.Events)
}
