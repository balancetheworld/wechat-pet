package calendar

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	appErrors "github.com/balancetheworld/wechat-pet/internal/pkg/errors"
)

var ErrReminderCompleted = errors.New("reminder already completed")

type Repository interface {
	ListMonth(context.Context, string, string, string) ([]DayMarkerDTO, error)
	GetDay(context.Context, string, string) (DayDTO, error)
	CreateRecord(context.Context, string, string, CreateRecordRequest, time.Time) (RecordDTO, error)
	UpdateRecord(context.Context, string, string, string, UpdateRecordRequest, *time.Time) (RecordDTO, error)
	DeleteRecord(context.Context, string, string, string) error
	CompleteReminder(context.Context, string, string, string, CompleteReminderRequest, time.Time) (CompleteReminderDTO, error)
}

type Service struct {
	repository Repository
	assetURLs  AssetURLResolver
	assetAuth  interface {
		AuthorizeFamily(context.Context, string, string) error
	}
}

type AssetURLResolver interface {
	URL(ctx context.Context, key string) (string, error)
}

func NewService(repository Repository, assetURLs ...AssetURLResolver) (*Service, error) {
	if repository == nil {
		return nil, errors.New("calendar service repository is required")
	}
	var assetURLResolver AssetURLResolver
	if len(assetURLs) > 0 {
		assetURLResolver = assetURLs[0]
	}
	return &Service{repository: repository, assetURLs: assetURLResolver}, nil
}

func (s *Service) SetAssetAuthorizer(value interface {
	AuthorizeFamily(context.Context, string, string) error
}) {
	s.assetAuth = value
}

func (s *Service) ListMonth(ctx context.Context, familyID, month, petID string) (MonthDTO, error) {
	if _, err := time.Parse("2006-01", month); err != nil {
		return MonthDTO{}, appErrors.InvalidParam("月份格式应为 YYYY-MM")
	}
	days, err := s.repository.ListMonth(ctx, familyID, month, strings.TrimSpace(petID))
	if err != nil {
		return MonthDTO{}, mapError(err)
	}
	return MonthDTO{Month: month, PetID: strings.TrimSpace(petID), Days: days}, nil
}

func (s *Service) GetDay(ctx context.Context, familyID, date string) (DayDTO, error) {
	if _, err := time.Parse("2006-01-02", date); err != nil {
		return DayDTO{}, appErrors.InvalidParam("日期格式应为 YYYY-MM-DD")
	}
	value, err := s.repository.GetDay(ctx, familyID, date)
	if err != nil {
		return DayDTO{}, mapError(err)
	}
	if err := s.resolveDayMedia(ctx, &value); err != nil {
		return DayDTO{}, err
	}
	return value, nil
}

func (s *Service) CreateRecord(ctx context.Context, familyID, userID string, request CreateRecordRequest) (RecordDTO, error) {
	if err := validateCreateRequest(&request); err != nil {
		return RecordDTO{}, err
	}
	if err := s.authorizeMedia(ctx, familyID, request.MediaAssetIDs); err != nil {
		return RecordDTO{}, err
	}
	occurredAt, err := parseDateTime(request.OccurredAt)
	if err != nil {
		return RecordDTO{}, err
	}
	value, err := s.repository.CreateRecord(ctx, familyID, userID, request, occurredAt)
	if err != nil {
		return RecordDTO{}, mapError(err)
	}
	if err := s.resolveRecordMedia(ctx, &value); err != nil {
		return RecordDTO{}, err
	}
	return value, nil
}

/* UpdateRecord 部分更新日历记录(内容/发生时间): 供档案事件编辑等场景同步日历 */
func (s *Service) UpdateRecord(ctx context.Context, familyID, userID, recordID string, request UpdateRecordRequest) (RecordDTO, error) {
	if strings.TrimSpace(recordID) == "" {
		return RecordDTO{}, appErrors.InvalidParam("日历记录 ID 不能为空")
	}
	if request.Content == nil && request.OccurredAt == nil && request.MediaAssetIDs == nil {
		return RecordDTO{}, appErrors.InvalidParam("没有可更新字段")
	}
	content := request.Content
	if content != nil {
		trimmed := strings.TrimSpace(*content)
		content = &trimmed
		if len(trimmed) > 2000 {
			return RecordDTO{}, appErrors.InvalidParam("记录内容不能超过 2000 个字符")
		}
	}
	var occurredAt *time.Time
	if request.OccurredAt != nil {
		parsed, err := parseDateTime(*request.OccurredAt)
		if err != nil {
			return RecordDTO{}, err
		}
		occurredAt = &parsed
	}
	/* 图片整体替换: 与创建记录同一套校验 + 家庭资产鉴权 */
	if request.MediaAssetIDs != nil {
		if err := validateMedia(request.MediaAssetIDs); err != nil {
			return RecordDTO{}, err
		}
		if err := s.authorizeMedia(ctx, familyID, request.MediaAssetIDs); err != nil {
			return RecordDTO{}, err
		}
	}
	value, err := s.repository.UpdateRecord(ctx, familyID, userID, recordID, UpdateRecordRequest{Content: content, MediaAssetIDs: request.MediaAssetIDs}, occurredAt)
	if err != nil {
		return RecordDTO{}, mapError(err)
	}
	if err := s.resolveRecordMedia(ctx, &value); err != nil {
		return RecordDTO{}, err
	}
	return value, nil
}

/* DeleteRecord 软删除日历记录: 前端二次确认后调用 */
func (s *Service) DeleteRecord(ctx context.Context, familyID, userID, recordID string) error {
	if strings.TrimSpace(recordID) == "" {
		return appErrors.InvalidParam("日历记录 ID 不能为空")
	}
	/* 注意: 不能无条件 mapError — 成功时 err 为 nil, mapError 会把 nil 包装成 Internal(500) */
	if err := s.repository.DeleteRecord(ctx, familyID, userID, recordID); err != nil {
		return mapError(err)
	}
	return nil
}

func (s *Service) CompleteReminder(ctx context.Context, familyID, userID, reminderID string, request CompleteReminderRequest) (CompleteReminderDTO, error) {
	if strings.TrimSpace(reminderID) == "" {
		return CompleteReminderDTO{}, appErrors.InvalidParam("待办提醒 ID 不能为空")
	}
	if err := validateMedia(request.MediaAssetIDs); err != nil {
		return CompleteReminderDTO{}, err
	}
	if err := s.authorizeMedia(ctx, familyID, request.MediaAssetIDs); err != nil {
		return CompleteReminderDTO{}, err
	}
	request.Content = strings.TrimSpace(request.Content)
	if len(request.Content) > 2000 {
		return CompleteReminderDTO{}, appErrors.InvalidParam("记录内容不能超过 2000 个字符")
	}
	completedAt, err := parseDateTime(request.CompletedAt)
	if err != nil {
		return CompleteReminderDTO{}, err
	}
	value, err := s.repository.CompleteReminder(ctx, familyID, userID, reminderID, request, completedAt)
	if err != nil {
		return CompleteReminderDTO{}, mapError(err)
	}
	if err := s.resolveRecordMedia(ctx, &value.CompletedRecord); err != nil {
		return CompleteReminderDTO{}, err
	}
	return value, nil
}

func (s *Service) authorizeMedia(ctx context.Context, familyID string, assetIDs []string) error {
	if s.assetAuth == nil {
		return nil
	}
	for _, assetID := range assetIDs {
		if err := s.assetAuth.AuthorizeFamily(ctx, assetID, familyID); err != nil {
			return appErrors.Forbidden()
		}
	}
	return nil
}

func (s *Service) resolveDayMedia(ctx context.Context, value *DayDTO) error {
	for index := range value.Records {
		if err := s.resolveRecordMedia(ctx, &value.Records[index]); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) resolveRecordMedia(ctx context.Context, value *RecordDTO) error {
	for index := range value.Media {
		assetID := strings.TrimSpace(value.Media[index].AssetID)
		if assetID == "" {
			continue
		}
		if s.assetURLs == nil {
			value.Media[index].URL = assetID
			continue
		}
		url, err := s.assetURLs.URL(ctx, assetID)
		if err != nil {
			return appErrors.Internal(err)
		}
		value.Media[index].URL = url
	}
	return nil
}

func validateCreateRequest(request *CreateRecordRequest) error {
	request.Category = strings.TrimSpace(request.Category)
	request.MedicalType = strings.TrimSpace(request.MedicalType)
	request.CustomMedicalType = strings.TrimSpace(request.CustomMedicalType)
	request.PetID = strings.TrimSpace(request.PetID)
	request.Content = strings.TrimSpace(request.Content)
	if request.Category != "medical" && request.Category != "daily" {
		return appErrors.InvalidParam("记录分类仅支持医疗或日常")
	}
	if request.PetID == "" {
		return appErrors.InvalidParam("宠物 ID 不能为空")
	}
	if request.Category == "daily" && request.Content == "" && len(request.MediaAssetIDs) == 0 {
		return appErrors.InvalidParam("记录内容和图片至少填写一项")
	}
	if len(request.Content) > 2000 {
		return appErrors.InvalidParam("记录内容不能超过 2000 个字符")
	}
	if err := validateMedia(request.MediaAssetIDs); err != nil {
		return err
	}
	if request.Category == "daily" {
		if request.MedicalType != "" || request.CustomMedicalType != "" || request.Reminder != nil || len(request.Reminders) > 0 {
			return appErrors.InvalidParam("日常记录不能设置医疗类型或待办提醒")
		}
		return nil
	}
	if request.MedicalType != "" && !isMedicalType(request.MedicalType) {
		return appErrors.InvalidParam("医疗类型无效")
	}
	if request.MedicalType != "other" && request.CustomMedicalType != "" {
		return appErrors.InvalidParam("仅其他医疗类型可填写自定义名称")
	}
	if request.MedicalType == "other" && request.CustomMedicalType == "" {
		return appErrors.InvalidParam("请填写自定义医疗类型")
	}
	if len(request.CustomMedicalType) > 50 {
		return appErrors.InvalidParam("自定义医疗类型不能超过 50 个字符")
	}
	if request.Reminder != nil && len(request.Reminders) > 0 {
		return appErrors.InvalidParam("提醒参数不能同时使用单个和多个提醒")
	}
	if request.Reminder != nil {
		return validateReminder(request.Reminder)
	}
	for index := range request.Reminders {
		if err := validateReminder(&request.Reminders[index]); err != nil {
			return err
		}
	}
	return nil
}

func validateReminder(request *CreateReminderRequest) error {
	if _, err := time.Parse("2006-01-02", request.ReminderDate); err != nil {
		return appErrors.InvalidParam("提醒日期格式应为 YYYY-MM-DD")
	}
	if request.RepeatType != "once" && request.RepeatType != "monthly" && request.RepeatType != "yearly" && request.RepeatType != "custom_days" {
		return appErrors.InvalidParam("提醒周期无效")
	}
	if request.RepeatType == "custom_days" {
		if request.RepeatIntervalDays == nil || *request.RepeatIntervalDays <= 0 {
			return appErrors.InvalidParam("自定义周期天数必须大于 0")
		}
	} else if request.RepeatIntervalDays != nil {
		return appErrors.InvalidParam("非自定义周期不能设置周期天数")
	}
	if request.AdvanceDays == nil {
		value := 3
		request.AdvanceDays = &value
	}
	if *request.AdvanceDays < 0 {
		return appErrors.InvalidParam("提前提醒天数不能小于 0")
	}
	if len(request.NotificationChannels) == 0 {
		request.NotificationChannels = []string{"in_app"}
	}
	for _, channel := range request.NotificationChannels {
		if channel != "in_app" && channel != "push" {
			return appErrors.InvalidParam("提醒方式无效")
		}
	}
	return nil
}

func validateMedia(assetIDs []string) error {
	if len(assetIDs) > 9 {
		return appErrors.InvalidParam("图片最多 9 张")
	}
	for _, assetID := range assetIDs {
		if value := strings.TrimSpace(assetID); value == "" || len(value) > 128 {
			return appErrors.InvalidParam("图片资源无效")
		}
	}
	return nil
}

func isMedicalType(value string) bool {
	switch value {
	case "vaccine", "deworming", "checkup", "visit", "medication", "other":
		return true
	}
	return false
}

func parseDateTime(value string) (time.Time, error) {
	if strings.TrimSpace(value) == "" {
		return time.Now(), nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, appErrors.InvalidParam("时间格式应为 RFC3339")
	}
	return parsed, nil
}

func mapError(err error) *appErrors.AppError {
	if errors.Is(err, sql.ErrNoRows) {
		return appErrors.NotFound("资源不存在")
	}
	if errors.Is(err, ErrReminderCompleted) {
		return appErrors.Conflict("待办提醒已完成")
	}
	if appError := new(appErrors.AppError); errors.As(err, &appError) {
		return appError
	}
	return appErrors.Internal(err)
}
