package pet

import (
	"errors"

	petapp "github.com/balancetheworld/wechat-pet/internal/app/pet"
	"github.com/balancetheworld/wechat-pet/internal/middleware"
	appErrors "github.com/balancetheworld/wechat-pet/internal/pkg/errors"
	"github.com/balancetheworld/wechat-pet/internal/pkg/response"
	"github.com/gin-gonic/gin"
)

type Handler struct {
	service *petapp.Service
}

func NewHandler(service *petapp.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) List(c *gin.Context) {
	familyID, ok := middleware.GetCurrentFamilyID(c)
	if !ok {
		response.Fail(c, appErrors.Forbidden())
		return
	}
	result, err := h.service.List(c.Request.Context(), familyID)
	if err != nil {
		response.Fail(c, asAppError(err))
		return
	}
	response.Success(c, result)
}

func (h *Handler) Get(c *gin.Context) {
	familyID, ok := middleware.GetCurrentFamilyID(c)
	if !ok {
		response.Fail(c, appErrors.Forbidden())
		return
	}
	result, err := h.service.Get(c.Request.Context(), familyID, c.Param("pet_id"))
	if err != nil {
		response.Fail(c, asAppError(err))
		return
	}
	response.Success(c, result)
}

func (h *Handler) Create(c *gin.Context) {
	familyID, familyOK := middleware.GetCurrentFamilyID(c)
	userID, userOK := middleware.GetCurrentUserID(c)
	if !familyOK || !userOK {
		response.Fail(c, appErrors.Forbidden())
		return
	}
	var request petapp.CreatePetRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.Fail(c, appErrors.InvalidParam("宠物参数无效"))
		return
	}
	result, err := h.service.Create(c.Request.Context(), familyID, userID, request)
	if err != nil {
		response.Fail(c, asAppError(err))
		return
	}
	response.Success(c, result)
}

func (h *Handler) Update(c *gin.Context) {
	familyID, familyOK := middleware.GetCurrentFamilyID(c)
	userID, userOK := middleware.GetCurrentUserID(c)
	if !familyOK || !userOK {
		response.Fail(c, appErrors.Forbidden())
		return
	}
	var request petapp.UpdatePetRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.Fail(c, appErrors.InvalidParam("宠物参数无效"))
		return
	}
	result, err := h.service.Update(c.Request.Context(), familyID, c.Param("pet_id"), userID, request)
	if err != nil {
		response.Fail(c, asAppError(err))
		return
	}
	response.Success(c, result)
}

func (h *Handler) Delete(c *gin.Context) {
	familyID, familyOK := middleware.GetCurrentFamilyID(c)
	userID, userOK := middleware.GetCurrentUserID(c)
	if !familyOK || !userOK {
		response.Fail(c, appErrors.Forbidden())
		return
	}
	if err := h.service.Delete(c.Request.Context(), familyID, c.Param("pet_id"), userID); err != nil {
		response.Fail(c, asAppError(err))
		return
	}
	response.Success(c, struct{}{})
}

func (h *Handler) Profile(c *gin.Context) {
	familyID, ok := middleware.GetCurrentFamilyID(c)
	if !ok {
		response.Fail(c, appErrors.Forbidden())
		return
	}
	result, err := h.service.Profile(c.Request.Context(), familyID, c.Param("pet_id"))
	if err != nil {
		response.Fail(c, asAppError(err))
		return
	}
	response.Success(c, result)
}

func (h *Handler) Resource(c *gin.Context) {
	familyID, ok := middleware.GetCurrentFamilyID(c)
	userID, userOK := middleware.GetCurrentUserID(c)
	if !ok || !userOK {
		response.Fail(c, appErrors.Forbidden())
		return
	}
	payload := map[string]any{}
	if c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&payload); err != nil {
			response.Fail(c, appErrors.InvalidParam("档案参数无效"))
			return
		}
	}
	if id := c.Param("resource_id"); id != "" {
		payload["id"] = id
	}
	method := c.Request.Method
	result, err := h.service.Resource(c.Request.Context(), familyID, userID, c.Param("pet_id"), c.Param("resource"), method, payload)
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
