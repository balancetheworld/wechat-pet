package user

import (
	"errors"

	appauth "github.com/balancetheworld/wechat-pet/internal/app/auth"
	appuser "github.com/balancetheworld/wechat-pet/internal/app/user"
	"github.com/balancetheworld/wechat-pet/internal/middleware"
	appErrors "github.com/balancetheworld/wechat-pet/internal/pkg/errors"
	"github.com/balancetheworld/wechat-pet/internal/pkg/response"
	"github.com/gin-gonic/gin"
)

type Handler struct {
	service *appuser.Service
}

func NewHandler(service *appuser.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Me(c *gin.Context) {
	userID, ok := middleware.GetCurrentUserID(c)
	if !ok {
		response.Fail(c, appErrors.Unauthorized())
		return
	}
	profile, err := h.service.Me(c.Request.Context(), userID)
	if err != nil {
		response.Fail(c, asAppError(err))
		return
	}
	response.Success(c, toMeResponse(profile))
}

func (h *Handler) UpdateProfile(c *gin.Context) {
	userID, ok := middleware.GetCurrentUserID(c)
	if !ok {
		response.Fail(c, appErrors.Unauthorized())
		return
	}
	var request appuser.UpdateProfileRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.Fail(c, appErrors.InvalidParam("资料参数无效"))
		return
	}
	profile, err := h.service.UpdateProfile(c.Request.Context(), userID, request)
	if err != nil {
		response.Fail(c, asAppError(err))
		return
	}
	response.Success(c, toMeResponse(profile))
}

func toMeResponse(profile appuser.Profile) appauth.MeResponse {
	var family *appauth.FamilySummaryDTO
	if profile.Family != nil {
		family = &appauth.FamilySummaryDTO{ID: profile.Family.ID, Name: profile.Family.Name}
	}
	return appauth.MeResponse{User: profile.User, Family: family, Identity: profile.Identity}
}

func asAppError(err error) *appErrors.AppError {
	var appError *appErrors.AppError
	if errors.As(err, &appError) {
		return appError
	}
	return appErrors.Internal(err)
}
