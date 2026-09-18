package calendar

import (
	calendarapp "github.com/balancetheworld/wechat-pet/internal/app/calendar"
	familyapp "github.com/balancetheworld/wechat-pet/internal/app/family"
	"github.com/balancetheworld/wechat-pet/internal/middleware"
	jwtpkg "github.com/balancetheworld/wechat-pet/internal/pkg/jwt"
	"github.com/gin-gonic/gin"
)

func RegisterRoutes(router *gin.RouterGroup, signer *jwtpkg.Signer, familyRepository familyapp.ActiveFamilyRepository, service *calendarapp.Service) {
	if signer == nil || familyRepository == nil || service == nil {
		return
	}
	handler := NewHandler(service)
	group := router.Group("/calendar", middleware.RequireAuth(signer), middleware.RequireFamily(familyRepository))
	group.GET("/months/:month", handler.Month)
	group.GET("/days/:date", handler.Day)
	group.POST("/records", handler.CreateRecord)
	group.PATCH("/records/:record_id", handler.UpdateRecord)
	group.POST("/reminders/:reminder_id/complete", handler.CompleteReminder)
}
