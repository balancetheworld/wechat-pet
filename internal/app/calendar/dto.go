package calendar

type PetDTO struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	AvatarAssetID string `json:"avatar_asset_id"`
}

type MemberDTO struct {
	UserID        string `json:"user_id"`
	Nickname      string `json:"nickname"`
	AvatarAssetID string `json:"avatar_asset_id"`
}

type MediaDTO struct {
	ID        string `json:"id"`
	AssetID   string `json:"asset_id"`
	SortOrder int    `json:"sort_order"`
}

type ReminderDTO struct {
	ID                   string    `json:"id"`
	SourceRecordID       string    `json:"source_record_id"`
	ReminderDate         string    `json:"reminder_date"`
	Pet                  PetDTO    `json:"pet"`
	Content              string    `json:"content"`
	MedicalType          string    `json:"medical_type"`
	CreatedBy            MemberDTO `json:"created_by"`
	RepeatType           string    `json:"repeat_type"`
	RepeatIntervalDays   *int      `json:"repeat_interval_days"`
	AdvanceDays          int       `json:"advance_days"`
	NotificationChannels []string  `json:"notification_channels"`
	Status               string    `json:"status"`
}

type RecordDTO struct {
	ID          string       `json:"id"`
	Category    string       `json:"category"`
	MedicalType string       `json:"medical_type"`
	Content     string       `json:"content"`
	OccurredAt  string       `json:"occurred_at"`
	Pet         PetDTO       `json:"pet"`
	Media       []MediaDTO   `json:"media"`
	CreatedBy   MemberDTO    `json:"created_by"`
	Reminder    *ReminderDTO `json:"reminder"`
}

type DayMarkerDTO struct {
	Date               string `json:"date"`
	HasMedicalRecord   bool   `json:"has_medical_record"`
	HasDailyRecord     bool   `json:"has_daily_record"`
	HasPendingReminder bool   `json:"has_pending_reminder"`
}

type DaySummaryDTO struct {
	RecordCount          int `json:"record_count"`
	PendingReminderCount int `json:"pending_reminder_count"`
}

type DayDTO struct {
	Date      string        `json:"date"`
	Summary   DaySummaryDTO `json:"summary"`
	Reminders []ReminderDTO `json:"reminders"`
	Records   []RecordDTO   `json:"records"`
}

type MonthDTO struct {
	Month string         `json:"month"`
	PetID string         `json:"pet_id"`
	Days  []DayMarkerDTO `json:"days"`
}

type CreateReminderRequest struct {
	ReminderDate         string   `json:"reminder_date"`
	RepeatType           string   `json:"repeat_type"`
	RepeatIntervalDays   *int     `json:"repeat_interval_days"`
	AdvanceDays          *int     `json:"advance_days"`
	NotificationChannels []string `json:"notification_channels"`
}

type CreateRecordRequest struct {
	Category      string                 `json:"category"`
	MedicalType   string                 `json:"medical_type"`
	PetID         string                 `json:"pet_id"`
	Content       string                 `json:"content"`
	MediaAssetIDs []string               `json:"media_asset_ids"`
	OccurredAt    string                 `json:"occurred_at"`
	Reminder      *CreateReminderRequest `json:"reminder"`
}

type CompleteReminderRequest struct {
	CompletedAt   string   `json:"completed_at"`
	Content       string   `json:"content"`
	MediaAssetIDs []string `json:"media_asset_ids"`
}

type CompleteReminderDTO struct {
	CompletedRecord   RecordDTO    `json:"completed_record"`
	CompletedReminder ReminderDTO  `json:"completed_reminder"`
	NextReminder      *ReminderDTO `json:"next_reminder"`
}
