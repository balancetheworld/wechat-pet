package calendar

import (
	"errors"

	calendarapp "github.com/balancetheworld/wechat-pet/internal/app/calendar"
	"github.com/balancetheworld/wechat-pet/internal/middleware"
	appErrors "github.com/balancetheworld/wechat-pet/internal/pkg/errors"
	"github.com/balancetheworld/wechat-pet/internal/pkg/response"
	"github.com/gin-gonic/gin"
)

type Handler struct {
	service *calendarapp.Service
}

func NewHandler(service *calendarapp.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Month(c *gin.Context) {
	familyID, ok := middleware.GetCurrentFamilyID(c)
	if !ok {
		response.Fail(c, appErrors.Forbidden())
		return
	}
	result, err := h.service.ListMonth(c.Request.Context(), familyID, c.Param("month"), c.Query("pet_id"))
	if err != nil {
		response.Fail(c, asAppError(err))
		return
	}
	response.Success(c, result)
}

func (h *Handler) Day(c *gin.Context) {
	familyID, ok := middleware.GetCurrentFamilyID(c)
	if !ok {
		response.Fail(c, appErrors.Forbidden())
		return
	}
	result, err := h.service.GetDay(c.Request.Context(), familyID, c.Param("date"))
	if err != nil {
		response.Fail(c, asAppError(err))
		return
	}
	response.Success(c, result)
}

func (h *Handler) CreateRecord(c *gin.Context) {
	familyID, familyOK := middleware.GetCurrentFamilyID(c)
	userID, userOK := middleware.GetCurrentUserID(c)
	if !familyOK || !userOK {
		response.Fail(c, appErrors.Forbidden())
		return
	}
	var request calendarapp.CreateRecordRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.Fail(c, appErrors.InvalidParam("日历记录参数无效"))
		return
	}
	result, err := h.service.CreateRecord(c.Request.Context(), familyID, userID, request)
	if err != nil {
		response.Fail(c, asAppError(err))
		return
	}
	response.Success(c, result)
}

func (h *Handler) UpdateRecord(c *gin.Context) {
	familyID, familyOK := middleware.GetCurrentFamilyID(c)
	userID, userOK := middleware.GetCurrentUserID(c)
	if !familyOK || !userOK {
		response.Fail(c, appErrors.Forbidden())
		return
	}
	var request calendarapp.UpdateRecordRequest
	if c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&request); err != nil {
			response.Fail(c, appErrors.InvalidParam("日历记录参数无效"))
			return
		}
	}
	result, err := h.service.UpdateRecord(c.Request.Context(), familyID, userID, c.Param("record_id"), request)
	if err != nil {
		response.Fail(c, asAppError(err))
		return
	}
	response.Success(c, result)
}

func (h *Handler) CompleteReminder(c *gin.Context) {
	familyID, familyOK := middleware.GetCurrentFamilyID(c)
	userID, userOK := middleware.GetCurrentUserID(c)
	if !familyOK || !userOK {
		response.Fail(c, appErrors.Forbidden())
		return
	}
	var request calendarapp.CompleteReminderRequest
	if c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&request); err != nil {
			response.Fail(c, appErrors.InvalidParam("完成待办参数无效"))
			return
		}
	}
	result, err := h.service.CompleteReminder(c.Request.Context(), familyID, userID, c.Param("reminder_id"), request)
	if err != nil {
		response.Fail(c, asAppError(err))
		return
	}
	response.Success(c, result)
}

func asAppError(err error) *appErrors.AppError {
	var appError *appErrors.AppError
	if errors.As(err, &appError) {
		return appError
	}
	return appErrors.Internal(err)
}
