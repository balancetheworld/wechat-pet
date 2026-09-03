package auth

import (
	"errors"

	appauth "github.com/balancetheworld/wechat-pet/internal/app/auth"
	appErrors "github.com/balancetheworld/wechat-pet/internal/pkg/errors"
	"github.com/balancetheworld/wechat-pet/internal/pkg/response"
	"github.com/gin-gonic/gin"
)

type Handler struct {
	service *appauth.Service
}

func NewHandler(service *appauth.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Login(c *gin.Context) {
	var request appauth.LoginRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.Fail(c, appErrors.InvalidParam("登录参数无效"))
		return
	}
	result, err := h.service.LoginWithWeChat(c.Request.Context(), request.Code)
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
