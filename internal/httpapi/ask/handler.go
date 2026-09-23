package ask

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	askapp "github.com/balancetheworld/wechat-pet/internal/app/ask"
	calendarapp "github.com/balancetheworld/wechat-pet/internal/app/calendar"
	"github.com/balancetheworld/wechat-pet/internal/middleware"
	appErrors "github.com/balancetheworld/wechat-pet/internal/pkg/errors"
	"github.com/balancetheworld/wechat-pet/internal/pkg/response"
	"github.com/gin-gonic/gin"
)

type Handler struct {
	service *askapp.Service
	worker  askapp.RunEnqueuer
}

type createSessionRequest struct {
	Input     string   `json:"input" binding:"max=4000"`
	AssetRefs []string `json:"asset_refs" binding:"max=4"`
}

type replyRequest struct {
	Input           string   `json:"input" binding:"max=4000"`
	ExpectedVersion int      `json:"expected_version" binding:"required,min=1"`
	AssetRefs       []string `json:"asset_refs" binding:"max=4"`
}

type runControlRequest struct {
	ExpectedVersion int `json:"expected_version" binding:"required,min=1"`
}

type operationPreviewRequest struct {
	Summary string                          `json:"summary" binding:"required,max=2000"`
	Request calendarapp.CreateRecordRequest `json:"request" binding:"required"`
}

type operationControlRequest struct {
	ExpectedVersion int    `json:"expected_version" binding:"required,min=1"`
	Summary         string `json:"summary"`
}

func NewHandler(service *askapp.Service, worker askapp.RunEnqueuer) *Handler {
	return &Handler{service: service, worker: worker}
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
	result, err := h.service.CreateSessionWithAssets(c.Request.Context(), familyID, userID, c.Param("pet_id"), request.Input, request.AssetRefs, c.GetHeader("Idempotency-Key"))
	if err != nil {
		response.Fail(c, asAppError(err))
		return
	}
	if err := h.enqueue(c, result); err != nil {
		response.Fail(c, appErrors.Internal(err))
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
	result, resolution, err := h.service.CreateSessionFromInputWithAssets(c.Request.Context(), familyID, userID, request.Input, request.AssetRefs, c.GetHeader("Idempotency-Key"))
	if err != nil {
		response.Fail(c, asAppErrorWithResolution(err, resolution))
		return
	}
	if err := h.enqueue(c, result); err != nil {
		response.Fail(c, appErrors.Internal(err))
		return
	}
	response.Success(c, askapp.NewExecutionDTO(result))
}

func (h *Handler) ContinueSession(c *gin.Context) {
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
	result, err := h.service.ContinueSessionWithAssets(c.Request.Context(), familyID, userID, c.Param("session_id"), request.Input, request.AssetRefs, c.GetHeader("Idempotency-Key"))
	if err != nil {
		response.Fail(c, asAppError(err))
		return
	}
	if err := h.enqueue(c, result); err != nil {
		response.Fail(c, appErrors.Internal(err))
		return
	}
	response.Success(c, askapp.NewExecutionDTO(result))
}

func (h *Handler) ProcessRun(c *gin.Context) {
	familyID, familyOK := middleware.GetCurrentFamilyID(c)
	userID, userOK := middleware.GetCurrentUserID(c)
	if !familyOK || !userOK {
		response.Fail(c, appErrors.Forbidden())
		return
	}
	result, err := h.service.GetExecution(c.Request.Context(), familyID, c.Param("session_id"), c.Param("run_id"), userID)
	if err != nil {
		response.Fail(c, asAppError(err))
		return
	}
	if result.Run.Status == askapp.RunQueued {
		if err := h.enqueue(c, result); err != nil {
			response.Fail(c, appErrors.Internal(err))
			return
		}
	}
	response.Success(c, askapp.NewExecutionDTO(result))
}

func (h *Handler) enqueue(c *gin.Context, result askapp.ExecutionResult) error {
	if h.worker == nil {
		return nil
	}
	return h.worker.Enqueue(c.Request.Context(), askapp.RunJob{FamilyID: result.Session.FamilyID, SessionID: result.Session.ID, RunID: result.Run.ID, RowVersion: result.Run.RowVersion})
}

func (h *Handler) Reply(c *gin.Context) {
	familyID, familyOK := middleware.GetCurrentFamilyID(c)
	userID, userOK := middleware.GetCurrentUserID(c)
	if !familyOK || !userOK {
		response.Fail(c, appErrors.Forbidden())
		return
	}
	var request replyRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.Fail(c, appErrors.InvalidParam("问问参数无效"))
		return
	}
	result, err := h.service.ReplyWithAssets(c.Request.Context(), familyID, userID, c.Param("session_id"), c.Param("run_id"), request.Input, request.AssetRefs, request.ExpectedVersion, c.GetHeader("Idempotency-Key"))
	if err != nil {
		response.Fail(c, asAppError(err))
		return
	}
	if err := h.enqueue(c, result); err != nil {
		response.Fail(c, appErrors.Internal(err))
		return
	}
	response.Success(c, askapp.NewExecutionDTO(result))
}

func (h *Handler) GetSession(c *gin.Context) {
	familyID, familyOK := middleware.GetCurrentFamilyID(c)
	userID, userOK := middleware.GetCurrentUserID(c)
	if !familyOK || !userOK {
		response.Fail(c, appErrors.Forbidden())
		return
	}
	result, err := h.service.GetSession(c.Request.Context(), familyID, c.Param("session_id"), userID)
	if err != nil {
		response.Fail(c, asAppError(err))
		return
	}
	response.Success(c, askapp.NewSessionDTO(result))
}

func (h *Handler) StopRun(c *gin.Context) {
	h.controlRun(c, false)
}

func (h *Handler) RetryRun(c *gin.Context) {
	h.controlRun(c, true)
}

func (h *Handler) controlRun(c *gin.Context, retry bool) {
	familyID, familyOK := middleware.GetCurrentFamilyID(c)
	userID, userOK := middleware.GetCurrentUserID(c)
	if !familyOK || !userOK {
		response.Fail(c, appErrors.Forbidden())
		return
	}
	var request runControlRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.Fail(c, appErrors.InvalidParam("问问参数无效"))
		return
	}
	var result askapp.ExecutionResult
	var err error
	if retry {
		result, err = h.service.RetryRun(c.Request.Context(), familyID, userID, c.Param("session_id"), c.Param("run_id"), request.ExpectedVersion)
	} else {
		result, err = h.service.StopRun(c.Request.Context(), familyID, userID, c.Param("session_id"), c.Param("run_id"), request.ExpectedVersion)
	}
	if err != nil {
		response.Fail(c, asAppError(err))
		return
	}
	if retry {
		if err := h.enqueue(c, result); err != nil {
			response.Fail(c, appErrors.Internal(err))
			return
		}
	}
	response.Success(c, askapp.NewExecutionDTO(result))
}

func (h *Handler) ListHistory(c *gin.Context) {
	familyID, familyOK := middleware.GetCurrentFamilyID(c)
	userID, userOK := middleware.GetCurrentUserID(c)
	if !familyOK || !userOK {
		response.Fail(c, appErrors.Forbidden())
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	values, err := h.service.ListSessionHistory(c.Request.Context(), familyID, userID, limit)
	if err != nil {
		response.Fail(c, asAppError(err))
		return
	}
	result := make([]askapp.SessionDTO, 0, len(values))
	for _, value := range values {
		result = append(result, askapp.NewSessionDTO(value))
	}
	response.Success(c, result)
}

func (h *Handler) CreateOperationPreview(c *gin.Context) {
	familyID, userID, ok := h.currentIdentity(c)
	if !ok {
		return
	}
	var request operationPreviewRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.Fail(c, appErrors.InvalidParam("写入预览参数无效"))
		return
	}
	result, err := h.service.CreateOperationPreview(c.Request.Context(), familyID, userID, c.Param("session_id"), c.Param("run_id"), askapp.CalendarRecordPreview{Request: request.Request, Summary: request.Summary})
	if err != nil {
		response.Fail(c, asAppError(err))
		return
	}
	response.Success(c, askapp.NewOperationDTO(result))
}

func (h *Handler) GetOperation(c *gin.Context) {
	familyID, userID, ok := h.currentIdentity(c)
	if !ok {
		return
	}
	result, err := h.service.GetOperation(c.Request.Context(), familyID, userID, c.Param("session_id"), c.Param("operation_id"))
	if err != nil {
		response.Fail(c, asAppError(err))
		return
	}
	response.Success(c, askapp.NewOperationDTO(result))
}

func (h *Handler) ListOperations(c *gin.Context) {
	familyID, userID, ok := h.currentIdentity(c)
	if !ok {
		return
	}
	values, err := h.service.ListOperations(c.Request.Context(), familyID, userID, c.Param("session_id"))
	if err != nil {
		response.Fail(c, asAppError(err))
		return
	}
	result := make([]askapp.OperationDTO, 0, len(values))
	for _, value := range values {
		result = append(result, askapp.NewOperationDTO(value))
	}
	response.Success(c, result)
}

func (h *Handler) ConfirmOperation(c *gin.Context)  { h.controlOperation(c, "confirm") }
func (h *Handler) AbandonOperation(c *gin.Context)  { h.controlOperation(c, "abandon") }
func (h *Handler) WithdrawOperation(c *gin.Context) { h.controlOperation(c, "withdraw") }
func (h *Handler) ExecuteOperation(c *gin.Context)  { h.controlOperation(c, "execute") }

func (h *Handler) controlOperation(c *gin.Context, action string) {
	familyID, userID, ok := h.currentIdentity(c)
	if !ok {
		return
	}
	var request operationControlRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.Fail(c, appErrors.InvalidParam("写入操作参数无效"))
		return
	}
	var result askapp.Operation
	var err error
	switch action {
	case "confirm":
		result, err = h.service.ConfirmOperation(c.Request.Context(), familyID, userID, c.Param("session_id"), c.Param("operation_id"), request.ExpectedVersion, request.Summary)
	case "abandon":
		result, err = h.service.AbandonOperation(c.Request.Context(), familyID, userID, c.Param("session_id"), c.Param("operation_id"), request.ExpectedVersion)
	case "withdraw":
		result, err = h.service.WithdrawOperation(c.Request.Context(), familyID, userID, c.Param("session_id"), c.Param("operation_id"), request.ExpectedVersion)
	case "execute":
		result, err = h.service.ExecuteOperation(c.Request.Context(), familyID, userID, c.Param("session_id"), c.Param("operation_id"), request.ExpectedVersion)
	}
	if err != nil {
		response.Fail(c, asAppError(err))
		return
	}
	response.Success(c, askapp.NewOperationDTO(result))
}

func (h *Handler) currentIdentity(c *gin.Context) (string, string, bool) {
	familyID, familyOK := middleware.GetCurrentFamilyID(c)
	userID, userOK := middleware.GetCurrentUserID(c)
	if !familyOK || !userOK {
		response.Fail(c, appErrors.Forbidden())
		return "", "", false
	}
	return familyID, userID, true
}

func (h *Handler) GetSnapshot(c *gin.Context) {
	familyID, familyOK := middleware.GetCurrentFamilyID(c)
	userID, userOK := middleware.GetCurrentUserID(c)
	if !familyOK || !userOK {
		response.Fail(c, appErrors.Forbidden())
		return
	}
	result, err := h.service.GetSnapshot(c.Request.Context(), familyID, c.Param("session_id"), userID)
	if err != nil {
		response.Fail(c, asAppError(err))
		return
	}
	response.Success(c, askapp.NewSnapshotDTO(result))
}

func (h *Handler) Events(c *gin.Context) {
	familyID, familyOK := middleware.GetCurrentFamilyID(c)
	userID, userOK := middleware.GetCurrentUserID(c)
	if !familyOK || !userOK {
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
	result, err := h.service.GetEvents(c.Request.Context(), familyID, c.Param("session_id"), c.Param("run_id"), after, userID)
	if err != nil {
		response.Fail(c, asAppError(err))
		return
	}
	response.Success(c, askapp.NewEventDTOs(result))
}

func (h *Handler) StreamEvents(c *gin.Context) {
	familyID, familyOK := middleware.GetCurrentFamilyID(c)
	userID, userOK := middleware.GetCurrentUserID(c)
	if !familyOK || !userOK {
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
	values, err := h.service.GetEvents(c.Request.Context(), familyID, c.Param("session_id"), c.Param("run_id"), after, userID)
	if err != nil {
		response.Fail(c, asAppError(err))
		return
	}
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		response.Fail(c, appErrors.Internal(errors.New("streaming unsupported")))
		return
	}
	c.Header("Content-Type", "application/x-ndjson; charset=utf-8")
	c.Header("Cache-Control", "no-cache")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)
	encoder := json.NewEncoder(c.Writer)
	writeEvents := func(events []askapp.Event) (bool, error) {
		terminal := false
		for _, event := range events {
			if err := encoder.Encode(askapp.NewEventDTO(event)); err != nil {
				return false, err
			}
			if event.Sequence > after {
				after = event.Sequence
			}
			if isTerminalAskEvent(event.Type) {
				terminal = true
			}
		}
		if len(events) > 0 {
			flusher.Flush()
		}
		return terminal, nil
	}
	terminal, err := writeEvents(values)
	if err != nil || terminal {
		return
	}
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	timeout := time.NewTimer(25 * time.Second)
	defer timeout.Stop()
	for {
		select {
		case <-c.Request.Context().Done():
			return
		case <-timeout.C:
			return
		case <-ticker.C:
			values, err = h.service.GetEvents(c.Request.Context(), familyID, c.Param("session_id"), c.Param("run_id"), after, userID)
			if err != nil {
				return
			}
			terminal, err = writeEvents(values)
			if err != nil || terminal {
				return
			}
		}
	}
}

func isTerminalAskEvent(eventType string) bool {
	switch eventType {
	case "assistant.completed", "assistant.question", "fact.completed", "family.pets.completed", "run.completed", "risk.escalated", "run.failed":
		return true
	default:
		return false
	}
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
