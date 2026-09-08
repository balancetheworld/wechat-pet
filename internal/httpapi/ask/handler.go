package ask

import (
	"errors"
	"strconv"

	askapp "github.com/balancetheworld/wechat-pet/internal/app/ask"
	"github.com/balancetheworld/wechat-pet/internal/middleware"
	appErrors "github.com/balancetheworld/wechat-pet/internal/pkg/errors"
	"github.com/balancetheworld/wechat-pet/internal/pkg/response"
	"github.com/gin-gonic/gin"
)

type Handler struct {
	service *askapp.Service
}

type createSessionRequest struct {
	Input string `json:"input" binding:"required,max=4000"`
}

func NewHandler(service *askapp.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) CreateSession(c *gin.Context) {
	familyID, familyOK := middleware.GetCurrentFamilyID(c)
	userID, userOK := middleware.GetCurrentUserID(c)
	if !familyOK || !userOK {
		response.Fail(c, appErrors.Forbidden())
		return
	}
	var request createSessionRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.Fail(c, appErrors.InvalidParam("问问参数无效"))
		return
	}
	result, err := h.service.CreateSession(c.Request.Context(), familyID, userID, c.Param("pet_id"), request.Input)
	if err != nil {
		response.Fail(c, asAppError(err))
		return
	}
	response.Success(c, askapp.NewExecutionDTO(result))
}

func (h *Handler) CreateSessionFromInput(c *gin.Context) {
	familyID, familyOK := middleware.GetCurrentFamilyID(c)
	userID, userOK := middleware.GetCurrentUserID(c)
	if !familyOK || !userOK {
		response.Fail(c, appErrors.Forbidden())
		return
	}
	var request createSessionRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.Fail(c, appErrors.InvalidParam("问问参数无效"))
		return
	}
	result, resolution, err := h.service.CreateSessionFromInput(c.Request.Context(), familyID, userID, request.Input)
	if err != nil {
		response.Fail(c, asAppErrorWithResolution(err, resolution))
		return
	}
	response.Success(c, askapp.NewExecutionDTO(result))
}

func (h *Handler) ProcessRun(c *gin.Context) {
	familyID, ok := middleware.GetCurrentFamilyID(c)
	if !ok {
		response.Fail(c, appErrors.Forbidden())
		return
	}
	result, err := h.service.ProcessRun(c.Request.Context(), familyID, c.Param("session_id"), c.Param("run_id"))
	if err != nil {
		response.Fail(c, asAppError(err))
		return
	}
	response.Success(c, askapp.NewExecutionDTO(result))
}

func (h *Handler) Reply(c *gin.Context) {
	familyID, ok := middleware.GetCurrentFamilyID(c)
	if !ok {
		response.Fail(c, appErrors.Forbidden())
		return
	}
	var request createSessionRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.Fail(c, appErrors.InvalidParam("问问参数无效"))
		return
	}
	result, err := h.service.Reply(c.Request.Context(), familyID, c.Param("session_id"), c.Param("run_id"), request.Input)
	if err != nil {
		response.Fail(c, asAppError(err))
		return
	}
	response.Success(c, askapp.NewExecutionDTO(result))
}

func (h *Handler) GetSession(c *gin.Context) {
	familyID, ok := middleware.GetCurrentFamilyID(c)
	if !ok {
		response.Fail(c, appErrors.Forbidden())
		return
	}
	result, err := h.service.GetSession(c.Request.Context(), familyID, c.Param("session_id"))
	if err != nil {
		response.Fail(c, asAppError(err))
		return
	}
	response.Success(c, askapp.NewSessionDTO(result))
}

func (h *Handler) Events(c *gin.Context) {
	familyID, ok := middleware.GetCurrentFamilyID(c)
	if !ok {
		response.Fail(c, appErrors.Forbidden())
		return
	}
	after := 0
	if value := c.Query("after"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 0 {
			response.Fail(c, appErrors.InvalidParam("事件游标无效"))
			return
		}
		after = parsed
	}
	result, err := h.service.GetEvents(c.Request.Context(), familyID, c.Param("session_id"), c.Param("run_id"), after)
	if err != nil {
		response.Fail(c, asAppError(err))
		return
	}
	response.Success(c, askapp.NewEventDTOs(result))
}

func asAppError(err error) *appErrors.AppError {
	var appError *appErrors.AppError
	if errors.As(err, &appError) {
		return appError
	}
	return appErrors.Internal(err)
}

func asAppErrorWithResolution(err error, resolution askapp.PetResolution) *appErrors.AppError {
	result := asAppError(err)
	if resolution.Status == askapp.PetResolveAmbiguous {
		result.Message = "宠物名称存在歧义，请补充更多信息"
	}
	return result
}
