package ask

import (
	askapp "github.com/balancetheworld/wechat-pet/internal/app/ask"
	familyapp "github.com/balancetheworld/wechat-pet/internal/app/family"
	"github.com/balancetheworld/wechat-pet/internal/middleware"
	jwtpkg "github.com/balancetheworld/wechat-pet/internal/pkg/jwt"
	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.RouterGroup, signer *jwtpkg.Signer, familyRepository familyapp.ActiveFamilyRepository, service *askapp.Service, worker askapp.RunEnqueuer) {
	if signer == nil || familyRepository == nil || service == nil || worker == nil {
		return
	}
	handler := NewHandler(service, worker)
	group := router.Group("", middleware.RequireAuth(signer), middleware.RequireFamily(familyRepository))
	group.POST("/pets/:pet_id/ask/sessions", handler.CreateSession)
	group.POST("/ask/sessions", handler.CreateSessionFromInput)
	group.POST("/ask/sessions/:session_id/turns", handler.ContinueSession)
	group.POST("/ask/sessions/:session_id/runs/:run_id/process", handler.ProcessRun)
	group.POST("/ask/sessions/:session_id/runs/:run_id/reply", handler.Reply)
	group.POST("/ask/sessions/:session_id/runs/:run_id/stop", handler.StopRun)
	group.POST("/ask/sessions/:session_id/runs/:run_id/retry", handler.RetryRun)
	group.POST("/ask/sessions/:session_id/runs/:run_id/operations", handler.CreateOperationPreview)
	group.GET("/ask/sessions/:session_id/operations", handler.ListOperations)
	group.GET("/ask/sessions/:session_id/operations/:operation_id", handler.GetOperation)
	group.POST("/ask/sessions/:session_id/operations/:operation_id/confirm", handler.ConfirmOperation)
	group.POST("/ask/sessions/:session_id/operations/:operation_id/abandon", handler.AbandonOperation)
	group.POST("/ask/sessions/:session_id/operations/:operation_id/withdraw", handler.WithdrawOperation)
	group.POST("/ask/sessions/:session_id/operations/:operation_id/execute", handler.ExecuteOperation)
	group.GET("/ask/sessions", handler.ListHistory)
	group.GET("/ask/sessions/:session_id", handler.GetSession)
	group.GET("/ask/sessions/:session_id/snapshot", handler.GetSnapshot)
	group.GET("/ask/sessions/:session_id/runs/:run_id/events", handler.Events)
	group.GET("/ask/sessions/:session_id/runs/:run_id/events/stream", handler.StreamEvents)
}
