package family

import (
	"context"
	"errors"

	familyapp "github.com/balancetheworld/wechat-pet/server/internal/app/family"
	"github.com/balancetheworld/wechat-pet/server/internal/middleware"
	appErrors "github.com/balancetheworld/wechat-pet/server/internal/pkg/errors"
	"github.com/balancetheworld/wechat-pet/server/internal/pkg/response"
	"github.com/gin-gonic/gin"
)

type Handler struct {
	service *familyapp.Service
}

func NewHandler(service *familyapp.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Create(c *gin.Context) {
	userID, ok := middleware.GetCurrentUserID(c)
	if !ok {
		response.Fail(c, appErrors.Unauthorized())
		return
	}
	var request familyapp.CreateFamilyRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.Fail(c, appErrors.InvalidParam("家庭参数无效"))
		return
	}
	result, err := h.service.Create(c.Request.Context(), userID, request)
	if err != nil {
		response.Fail(c, asAppError(err))
		return
	}
	response.Success(c, result)
}

func (h *Handler) Current(c *gin.Context) {
	userID, ok := middleware.GetCurrentUserID(c)
	if !ok {
		response.Fail(c, appErrors.Unauthorized())
		return
	}
	result, err := h.service.Current(c.Request.Context(), userID)
	if err != nil {
		response.Fail(c, asAppError(err))
		return
	}
	response.Success(c, result)
}

func (h *Handler) ApplyJoin(c *gin.Context) {
	userID, ok := middleware.GetCurrentUserID(c)
	if !ok {
		response.Fail(c, appErrors.Unauthorized())
		return
	}
	var request familyapp.JoinFamilyRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.Fail(c, appErrors.InvalidParam("家庭码无效"))
		return
	}
	result, err := h.service.ApplyJoin(c.Request.Context(), userID, request)
	if err != nil {
		response.Fail(c, asAppError(err))
		return
	}
	response.Success(c, result)
}

func (h *Handler) MyJoinApplication(c *gin.Context) {
	userID, ok := middleware.GetCurrentUserID(c)
	if !ok {
		response.Fail(c, appErrors.Unauthorized())
		return
	}
	result, err := h.service.MyJoinApplication(c.Request.Context(), userID)
	if err != nil {
		response.Fail(c, asAppError(err))
		return
	}
	response.Success(c, result)
}

func (h *Handler) Members(c *gin.Context) {
	familyID, ok := middleware.GetCurrentFamilyID(c)
	if !ok {
		response.Fail(c, appErrors.Forbidden())
		return
	}
	result, err := h.service.Members(c.Request.Context(), familyID)
	if err != nil {
		response.Fail(c, asAppError(err))
		return
	}
	response.Success(c, result)
}

func (h *Handler) PendingApplications(c *gin.Context) {
	familyID, ok := middleware.GetCurrentFamilyID(c)
	if !ok {
		response.Fail(c, appErrors.Forbidden())
		return
	}
	result, err := h.service.PendingApplications(c.Request.Context(), familyID)
	if err != nil {
		response.Fail(c, asAppError(err))
		return
	}
	response.Success(c, result)
}

func (h *Handler) ApproveApplication(c *gin.Context) {
	h.handleApplication(c, h.service.ApproveApplication)
}

func (h *Handler) RejectApplication(c *gin.Context) {
	h.handleApplication(c, h.service.RejectApplication)
}

func (h *Handler) handleApplication(c *gin.Context, action func(context.Context, string, string) error) {
	familyID, ok := middleware.GetCurrentFamilyID(c)
	if !ok {
		response.Fail(c, appErrors.Forbidden())
		return
	}
	if err := action(c.Request.Context(), familyID, c.Param("member_id")); err != nil {
		response.Fail(c, asAppError(err))
		return
	}
	response.Success(c, struct{}{})
}

func (h *Handler) RemoveMember(c *gin.Context) {
	familyID, familyOK := middleware.GetCurrentFamilyID(c)
	userID, userOK := middleware.GetCurrentUserID(c)
	if !familyOK || !userOK {
		response.Fail(c, appErrors.Forbidden())
		return
	}
	if err := h.service.RemoveMember(c.Request.Context(), familyID, c.Param("member_id"), userID); err != nil {
		response.Fail(c, asAppError(err))
		return
	}
	response.Success(c, struct{}{})
}

func asAppError(err error) *appErrors.AppError {
	var appError *appErrors.AppError
	if errors.As(err, &appError) {
		return appError
	}
	return appErrors.Internal(err)
}
