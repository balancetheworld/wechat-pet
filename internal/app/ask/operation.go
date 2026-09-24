package ask

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	calendarapp "github.com/balancetheworld/wechat-pet/internal/app/calendar"
	petapp "github.com/balancetheworld/wechat-pet/internal/app/pet"
	appErrors "github.com/balancetheworld/wechat-pet/internal/pkg/errors"
)

const (
	operationTargetCalendarRecordCreate     = "calendar.record.create"
	operationTargetCalendarRecordUpdate     = "calendar.record.update"
	operationTargetCalendarReminderComplete = "calendar.reminder.complete"
	operationTargetPetProfileUpdate         = "pet.profile.update"
	operationTargetPetHealthUpdate          = "pet.health.update"
	operationTargetPetCreate                = "pet.create"
	operationPreviewTTL                     = 15 * time.Minute
	operationExecutionTTL                   = 5 * time.Minute
)

// OperationPreviewInput 是一份待冻结的写入预览。写入目标与载荷由服务端按工具
// 参数构造，模型只能通过 prepare_* 工具提出，不能直接写入。
type OperationPreviewInput struct {
	FamilyID string
	UserID   string
	Target   string
	Summary  string
	Payload  any
}

type CalendarRecordPreview struct {
	Request calendarapp.CreateRecordRequest `json:"request"`
	Summary string                          `json:"summary"`
}

type operationRepository interface {
	CreateOperation(context.Context, Operation) error
	GetOperation(context.Context, string, string) (Operation, error)
	ListOperations(context.Context, string) ([]Operation, error)
	TransitionOperation(context.Context, string, int, OperationStatus, OperationStatus, time.Time, *time.Time, string) error
}

func (s *Service) CreateOperationPreview(ctx context.Context, familyID, userID, sessionID, runID string, preview CalendarRecordPreview) (Operation, error) {
	if preview.Request.PetID == "" || (preview.Request.Category != "daily" && preview.Request.Category != "medical") {
		return Operation{}, appErrors.InvalidParam("冻结写入内容无效")
	}
	return s.PrepareOperation(ctx, sessionID, runID, OperationPreviewInput{
		FamilyID: familyID,
		UserID:   userID,
		Target:   operationTargetCalendarRecordCreate,
		Summary:  preview.Summary,
		Payload:  preview.Request,
	})
}

// PrepareOperation 冻结一份写入预览（工具 prepare_* 路径与 HTTP 预览接口共用）。
func (s *Service) PrepareOperation(ctx context.Context, sessionID, runID string, input OperationPreviewInput) (Operation, error) {
	if strings.TrimSpace(input.Summary) == "" {
		return Operation{}, appErrors.InvalidParam("预览摘要不能为空")
	}
	if !validOperationTarget(input.Target) {
		return Operation{}, appErrors.InvalidParam("不支持的写入类型")
	}
	if _, err := s.GetExecution(ctx, input.FamilyID, sessionID, runID, input.UserID); err != nil {
		return Operation{}, err
	}
	repository, ok := s.repository.(operationRepository)
	if !ok {
		return Operation{}, appErrors.Internal(errors.New("ask operation repository unavailable"))
	}
	payload, err := json.Marshal(input.Payload)
	if err != nil {
		return Operation{}, appErrors.Internal(err)
	}
	id, err := newID()
	if err != nil {
		return Operation{}, appErrors.Internal(err)
	}
	now := s.now().UTC()
	expiresAt := now.Add(operationPreviewTTL)
	value := Operation{ID: id, SessionID: sessionID, RunID: runID, CreatedBy: input.UserID, Status: OperationPending, Preview: input.Summary, Target: input.Target, Payload: string(payload), Version: 1, ExpiresAt: &expiresAt, CreatedAt: now, UpdatedAt: now}
	if err := repository.CreateOperation(ctx, value); err != nil {
		return Operation{}, appErrors.Internal(err)
	}
	return value, nil
}

func validOperationTarget(target string) bool {
	switch target {
	case operationTargetCalendarRecordCreate, operationTargetCalendarRecordUpdate, operationTargetCalendarReminderComplete, operationTargetPetProfileUpdate, operationTargetPetHealthUpdate, operationTargetPetCreate:
		return true
	default:
		return false
	}
}

func (s *Service) GetOperation(ctx context.Context, familyID, userID, sessionID, operationID string) (Operation, error) {
	if _, err := s.GetSession(ctx, familyID, sessionID, userID); err != nil {
		return Operation{}, err
	}
	repository, ok := s.repository.(operationRepository)
	if !ok {
		return Operation{}, appErrors.Internal(errors.New("ask operation repository unavailable"))
	}
	value, err := repository.GetOperation(ctx, sessionID, operationID)
	if errors.Is(err, ErrOperationNotFound) {
		return Operation{}, appErrors.NotFound("写入预览不存在")
	}
	if err != nil {
		return Operation{}, appErrors.Internal(err)
	}
	if value.CreatedBy != userID {
		return Operation{}, appErrors.Forbidden()
	}
	return value, nil
}

func (s *Service) ListOperations(ctx context.Context, familyID, userID, sessionID string) ([]Operation, error) {
	if _, err := s.GetSession(ctx, familyID, sessionID, userID); err != nil {
		return nil, err
	}
	repository, ok := s.repository.(operationRepository)
	if !ok {
		return nil, appErrors.Internal(errors.New("ask operation repository unavailable"))
	}
	values, err := repository.ListOperations(ctx, sessionID)
	if err != nil {
		return nil, appErrors.Internal(err)
	}
	result := make([]Operation, 0, len(values))
	for _, value := range values {
		if value.CreatedBy == userID {
			result = append(result, value)
		}
	}
	return result, nil
}

func (s *Service) ConfirmOperation(ctx context.Context, familyID, userID, sessionID, operationID string, expectedVersion int, summary string) (Operation, error) {
	value, err := s.GetOperation(ctx, familyID, userID, sessionID, operationID)
	if err != nil {
		return Operation{}, err
	}
	now := s.now().UTC()
	if value.Status != OperationPending || value.Version != expectedVersion || value.ExpiresAt == nil || !value.ExpiresAt.After(now) || strings.TrimSpace(summary) != value.Preview {
		return Operation{}, appErrors.Conflict("写入预览已失效")
	}
	repository := s.repository.(operationRepository)
	expiresAt := now.Add(operationExecutionTTL)
	if err := repository.TransitionOperation(ctx, value.ID, value.Version, OperationPending, OperationConfirmed, now, &expiresAt, value.Result); err != nil {
		if errors.Is(err, ErrOperationConflict) {
			return Operation{}, appErrors.Conflict("写入预览状态已改变")
		}
		return Operation{}, appErrors.Internal(err)
	}
	value.Status, value.Version, value.ConfirmedAt, value.ExpiresAt, value.UpdatedAt = OperationConfirmed, value.Version+1, &now, &expiresAt, now
	return value, nil
}

func (s *Service) AbandonOperation(ctx context.Context, familyID, userID, sessionID, operationID string, expectedVersion int) (Operation, error) {
	return s.transitionOperation(ctx, familyID, userID, sessionID, operationID, expectedVersion, OperationPending, OperationAbandoned)
}

func (s *Service) WithdrawOperation(ctx context.Context, familyID, userID, sessionID, operationID string, expectedVersion int) (Operation, error) {
	return s.transitionOperation(ctx, familyID, userID, sessionID, operationID, expectedVersion, OperationConfirmed, OperationWithdrawn)
}

func (s *Service) transitionOperation(ctx context.Context, familyID, userID, sessionID, operationID string, expectedVersion int, from, to OperationStatus) (Operation, error) {
	value, err := s.GetOperation(ctx, familyID, userID, sessionID, operationID)
	if err != nil {
		return Operation{}, err
	}
	if value.Status != from || value.Version != expectedVersion {
		return Operation{}, appErrors.Conflict("写入预览状态已改变")
	}
	now := s.now().UTC()
	if err := s.repository.(operationRepository).TransitionOperation(ctx, value.ID, value.Version, from, to, now, value.ExpiresAt, value.Result); err != nil {
		if errors.Is(err, ErrOperationConflict) {
			return Operation{}, appErrors.Conflict("写入预览状态已改变")
		}
		return Operation{}, appErrors.Internal(err)
	}
	value.Status, value.Version, value.UpdatedAt = to, value.Version+1, now
	return value, nil
}

func (s *Service) ExecuteOperation(ctx context.Context, familyID, userID, sessionID, operationID string, expectedVersion int) (Operation, error) {
	value, err := s.GetOperation(ctx, familyID, userID, sessionID, operationID)
	if err != nil {
		return Operation{}, err
	}
	now := s.now().UTC()
	if value.Status != OperationConfirmed || value.Version != expectedVersion || value.ExpiresAt == nil || !value.ExpiresAt.After(now) {
		return Operation{}, appErrors.Conflict("写入确认已失效")
	}
	repository := s.repository.(operationRepository)
	if err := repository.TransitionOperation(ctx, value.ID, value.Version, OperationConfirmed, OperationExecuting, now, value.ExpiresAt, value.Result); err != nil {
		if errors.Is(err, ErrOperationConflict) {
			return Operation{}, appErrors.Conflict("写入预览状态已改变")
		}
		return Operation{}, appErrors.Internal(err)
	}
	switch value.Target {
	case operationTargetCalendarRecordCreate:
		return s.executeCalendarRecordCreate(ctx, repository, value, familyID, userID, now)
	case operationTargetCalendarRecordUpdate:
		return s.executeCalendarRecordUpdate(ctx, repository, value, familyID, userID, now)
	case operationTargetCalendarReminderComplete:
		return s.executeCalendarReminderComplete(ctx, repository, value, familyID, userID, now)
	case operationTargetPetProfileUpdate:
		return s.executePetProfileUpdate(ctx, repository, value, familyID, userID, now)
	case operationTargetPetHealthUpdate:
		return s.executePetHealthUpdate(ctx, repository, value, familyID, userID, now)
	case operationTargetPetCreate:
		return s.executePetCreate(ctx, repository, value, familyID, userID, now)
	default:
		return s.finishOperation(ctx, repository, value, now, OperationFailed, "operation_target_unavailable")
	}
}

func (s *Service) executeCalendarRecordCreate(ctx context.Context, repository operationRepository, value Operation, familyID, userID string, now time.Time) (Operation, error) {
	if s.calendarWriter == nil {
		return s.finishOperation(ctx, repository, value, now, OperationFailed, "operation_target_unavailable")
	}
	var request calendarapp.CreateRecordRequest
	if err := json.Unmarshal([]byte(value.Payload), &request); err != nil {
		return s.finishOperation(ctx, repository, value, now, OperationFailed, "operation_payload_invalid")
	}
	record, err := s.calendarWriter.CreateRecord(ctx, familyID, userID, request)
	if err != nil {
		return s.finishOperation(ctx, repository, value, now, OperationFailed, "calendar_record_create_failed")
	}
	result, _ := json.Marshal(map[string]any{"record_id": record.ID, "verified": s.verifyCalendarRecord(ctx, familyID, record.ID)})
	return s.finishOperation(ctx, repository, value, now, OperationSucceeded, string(result))
}

func (s *Service) executeCalendarRecordUpdate(ctx context.Context, repository operationRepository, value Operation, familyID, userID string, now time.Time) (Operation, error) {
	if s.calendarWriter == nil {
		return s.finishOperation(ctx, repository, value, now, OperationFailed, "operation_target_unavailable")
	}
	var payload struct {
		RecordID string                          `json:"record_id"`
		Request  calendarapp.UpdateRecordRequest `json:"request"`
	}
	if err := json.Unmarshal([]byte(value.Payload), &payload); err != nil || payload.RecordID == "" {
		return s.finishOperation(ctx, repository, value, now, OperationFailed, "operation_payload_invalid")
	}
	record, err := s.calendarWriter.UpdateRecord(ctx, familyID, userID, payload.RecordID, payload.Request)
	if err != nil {
		return s.finishOperation(ctx, repository, value, now, OperationFailed, "calendar_record_update_failed")
	}
	result, _ := json.Marshal(map[string]any{"record_id": record.ID, "verified": s.verifyCalendarRecord(ctx, familyID, record.ID)})
	return s.finishOperation(ctx, repository, value, now, OperationSucceeded, string(result))
}

// executeCalendarReminderComplete 完成一条待办提醒（需要用户先确认）。
// 完成会生成一条记录，用该记录 ID 回读核实。
func (s *Service) executeCalendarReminderComplete(ctx context.Context, repository operationRepository, value Operation, familyID, userID string, now time.Time) (Operation, error) {
	if s.calendarWriter == nil {
		return s.finishOperation(ctx, repository, value, now, OperationFailed, "operation_target_unavailable")
	}
	var payload struct {
		ReminderID string                              `json:"reminder_id"`
		Request    calendarapp.CompleteReminderRequest `json:"request"`
	}
	if err := json.Unmarshal([]byte(value.Payload), &payload); err != nil || payload.ReminderID == "" {
		return s.finishOperation(ctx, repository, value, now, OperationFailed, "operation_payload_invalid")
	}
	completed, err := s.calendarWriter.CompleteReminder(ctx, familyID, userID, payload.ReminderID, payload.Request)
	if err != nil {
		return s.finishOperation(ctx, repository, value, now, OperationFailed, "calendar_reminder_complete_failed")
	}
	result, _ := json.Marshal(map[string]any{
		"reminder_id": payload.ReminderID,
		"record_id":   completed.CompletedRecord.ID,
		"verified":    s.verifyCalendarRecord(ctx, familyID, completed.CompletedRecord.ID),
	})
	return s.finishOperation(ctx, repository, value, now, OperationSucceeded, string(result))
}

// petProfileWriter 是档案更新的写入端口，由 pet 仓储提供（与 HTTP 档案更新同一路径）。
type petProfileWriter interface {
	Resource(context.Context, string, string, string, string, string, map[string]any) (any, error)
}

func (s *Service) executePetProfileUpdate(ctx context.Context, repository operationRepository, value Operation, familyID, userID string, now time.Time) (Operation, error) {
	writer, ok := s.pets.(petProfileWriter)
	if !ok {
		return s.finishOperation(ctx, repository, value, now, OperationFailed, "operation_target_unavailable")
	}
	var payload struct {
		PetID  string         `json:"pet_id"`
		Fields map[string]any `json:"fields"`
	}
	if err := json.Unmarshal([]byte(value.Payload), &payload); err != nil || payload.PetID == "" || len(payload.Fields) == 0 {
		return s.finishOperation(ctx, repository, value, now, OperationFailed, "operation_payload_invalid")
	}
	if _, err := writer.Resource(ctx, familyID, userID, payload.PetID, "profile", "PATCH", payload.Fields); err != nil {
		return s.finishOperation(ctx, repository, value, now, OperationFailed, "pet_profile_update_failed")
	}
	result, _ := json.Marshal(map[string]any{"pet_id": payload.PetID, "verified": s.verifyPetProfile(ctx, familyID, payload.PetID, payload.Fields)})
	return s.finishOperation(ctx, repository, value, now, OperationSucceeded, string(result))
}

// executePetHealthUpdate 修改宠物健康档案（过敏、长期用药、健康状态）。
func (s *Service) executePetHealthUpdate(ctx context.Context, repository operationRepository, value Operation, familyID, userID string, now time.Time) (Operation, error) {
	writer, ok := s.pets.(petProfileWriter)
	if !ok {
		return s.finishOperation(ctx, repository, value, now, OperationFailed, "operation_target_unavailable")
	}
	var payload struct {
		PetID  string         `json:"pet_id"`
		Fields map[string]any `json:"fields"`
	}
	if err := json.Unmarshal([]byte(value.Payload), &payload); err != nil || payload.PetID == "" || len(payload.Fields) == 0 {
		return s.finishOperation(ctx, repository, value, now, OperationFailed, "operation_payload_invalid")
	}
	if _, err := writer.Resource(ctx, familyID, userID, payload.PetID, "health", "PATCH", payload.Fields); err != nil {
		return s.finishOperation(ctx, repository, value, now, OperationFailed, "pet_health_update_failed")
	}
	result, _ := json.Marshal(map[string]any{"pet_id": payload.PetID, "verified": s.verifyPetHealth(ctx, familyID, payload.PetID, payload.Fields)})
	return s.finishOperation(ctx, repository, value, now, OperationSucceeded, string(result))
}

func (s *Service) verifyPetHealth(ctx context.Context, familyID, petID string, fields map[string]any) bool {
	reader, ok := s.pets.(petHealthReader)
	if !ok {
		return false
	}
	health, err := reader.GetHealth(ctx, familyID, petID)
	if err != nil {
		return false
	}
	for key, want := range fields {
		value, ok := want.(string)
		if !ok {
			return false
		}
		switch key {
		case "status":
			if health.Status != value {
				return false
			}
		case "allergies":
			if health.Allergies != value {
				return false
			}
		case "long_term_medication":
			if health.LongTermMedication != value {
				return false
			}
		}
	}
	return true
}

func (s *Service) verifyPetProfile(ctx context.Context, familyID, petID string, fields map[string]any) bool {
	reader, ok := s.pets.(petProfileReader)
	if !ok {
		return false
	}
	profile, err := reader.GetProfile(ctx, familyID, petID)
	if err != nil {
		return false
	}
	return profileMatchesFields(profile, fields)
}

// petCreator 是新建宠物的写端口，由 pet 仓储提供（与 App 内新建流程同一路径）。
type petCreator interface {
	Create(context.Context, string, string, string) (petapp.Pet, error)
	Update(context.Context, string, string, string, petapp.UpdatePetRequest) (petapp.Pet, error)
}

// executePetCreate 新建宠物：先按名字创建，再补档案字段，最后回读核实。
// 补档案失败时返回失败但保留 pet_id，便于用户继续编辑而不是重复创建。
func (s *Service) executePetCreate(ctx context.Context, repository operationRepository, value Operation, familyID, userID string, now time.Time) (Operation, error) {
	creator, ok := s.pets.(petCreator)
	if !ok {
		return s.finishOperation(ctx, repository, value, now, OperationFailed, "operation_target_unavailable")
	}
	var payload struct {
		Name       string `json:"name"`
		Breed      string `json:"breed"`
		Gender     string `json:"gender"`
		Birthday   string `json:"birthday"`
		HomeDate   string `json:"home_date"`
		Sterilized *bool  `json:"sterilized"`
	}
	if err := json.Unmarshal([]byte(value.Payload), &payload); err != nil || payload.Name == "" {
		return s.finishOperation(ctx, repository, value, now, OperationFailed, "operation_payload_invalid")
	}
	pet, err := creator.Create(ctx, familyID, userID, payload.Name)
	if err != nil {
		return s.finishOperation(ctx, repository, value, now, OperationFailed, "pet_create_failed")
	}
	if payload.Breed != "" || payload.Gender != "" || payload.Birthday != "" || payload.HomeDate != "" || payload.Sterilized != nil {
		update := petapp.UpdatePetRequest{Name: payload.Name, Breed: payload.Breed, Gender: payload.Gender, Birthday: payload.Birthday, HomeDate: payload.HomeDate}
		if payload.Sterilized != nil {
			update.Sterilized = *payload.Sterilized
		}
		if _, err := creator.Update(ctx, familyID, pet.ID, userID, update); err != nil {
			partial, _ := json.Marshal(map[string]any{"pet_id": pet.ID, "verified": false})
			return s.finishOperation(ctx, repository, value, now, OperationFailed, string(partial))
		}
	}
	result, _ := json.Marshal(map[string]any{"pet_id": pet.ID, "verified": s.verifyPetCreated(ctx, familyID, pet.ID, payload.Name)})
	return s.finishOperation(ctx, repository, value, now, OperationSucceeded, string(result))
}

func (s *Service) verifyPetCreated(ctx context.Context, familyID, petID, name string) bool {
	reader, ok := s.pets.(petProfileReader)
	if !ok {
		return false
	}
	profile, err := reader.GetProfile(ctx, familyID, petID)
	if err != nil {
		return false
	}
	return profile.Name == name
}

func profileMatchesFields(profile petapp.PetProfile, fields map[string]any) bool {
	for key, want := range fields {
		switch key {
		case "name":
			value, ok := want.(string)
			if !ok || profile.Name != value {
				return false
			}
		case "breed":
			value, ok := want.(string)
			if !ok || profile.Breed != value {
				return false
			}
		case "gender":
			value, ok := want.(string)
			if !ok || profile.Gender != value {
				return false
			}
		case "sterilized":
			value, ok := want.(bool)
			if !ok || profile.Sterilized != value {
				return false
			}
		case "birthday":
			value, ok := want.(string)
			if !ok || profile.Birthday == nil || *profile.Birthday != value {
				return false
			}
		case "home_date":
			value, ok := want.(string)
			if !ok || profile.HomeDate == nil || *profile.HomeDate != value {
				return false
			}
		}
	}
	return true
}

// verifyCalendarRecord 执行后核实：按 record_id 回读一次真实记录。
// 读取失败或记录不存在时返回 false，使调用方与用户能区分「写入成功但未核实」。
func (s *Service) verifyCalendarRecord(ctx context.Context, familyID, recordID string) bool {
	if s.businessRead == nil || recordID == "" {
		return false
	}
	outcome, err := s.businessRead.ReadHealthRecord(ctx, familyID, recordID)
	if err != nil {
		return false
	}
	return outcome.Record.ID == recordID
}

func (s *Service) finishOperation(ctx context.Context, repository operationRepository, value Operation, now time.Time, status OperationStatus, result string) (Operation, error) {
	if err := repository.TransitionOperation(ctx, value.ID, value.Version+1, OperationExecuting, status, now, value.ExpiresAt, result); err != nil {
		return Operation{}, appErrors.Internal(err)
	}
	value.Status, value.Version, value.Result, value.UpdatedAt = status, value.Version+2, result, now
	return value, nil
}
