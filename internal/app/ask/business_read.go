package ask

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	petapp "github.com/balancetheworld/wechat-pet/internal/app/pet"
)

// ErrHealthRecordNotFound：单条健康记录不存在或已删除。与读取故障（其他 error）
// 分开表达，落实「读取错误与无记录始终分开」。
var ErrHealthRecordNotFound = errors.New("ask health record not found")

// ReadSource：业务读取结果的来源与时效（对应 7.3「来源与时效」）。
// 读取时间不等于事实发生时间；Version 用于跨 Run 复用前的失效核验。
type ReadSource struct {
	SourceType string    `json:"source_type"` // pet / health_record / record
	SourceID   string    `json:"source_id"`   // family_id / pet_id / record_id
	Version    string    `json:"version"`     // 集合版本
	ReadAt     time.Time `json:"read_at"`     // 实际读取时间
}

// HealthRecordItem：单条健康记录（业务读取视角）。MediaAssetIDs 只保留资产
// 身份，不携带临时访问链接。
type HealthRecordItem struct {
	ID                string    `json:"id"`
	Category          string    `json:"category"`
	MedicalType       string    `json:"medical_type"`
	CustomMedicalType string    `json:"custom_medical_type"`
	Content           string    `json:"content"`
	OccurredAt        time.Time `json:"occurred_at"`
	MediaAssetIDs     []string  `json:"media_asset_ids"`
}

// PetResolveOutcome：宠物消歧结果。Status 复用 pet_resolver 的 none/resolved/ambiguous。
type PetResolveOutcome struct {
	Status    PetResolveStatus `json:"status"`
	Resolved  []petapp.Pet     `json:"resolved,omitempty"`
	Ambiguous []PetCandidate   `json:"ambiguous,omitempty"`
	Source    ReadSource       `json:"source"`
}

// PetProfileOutcome：宠物档案读取结果。
type PetProfileOutcome struct {
	Profile petapp.PetProfile `json:"profile"`
	Health  petapp.PetHealth  `json:"health"`
	Source  ReadSource        `json:"source"`
}

// HealthRecordSearchOutcome：健康记录检索结果。
type HealthRecordSearchOutcome struct {
	Records    []HealthRecordItem `json:"records"`
	HasMore    bool               `json:"has_more"`
	NextCursor string             `json:"next_cursor,omitempty"`
	Source     ReadSource         `json:"source"`
}

// HealthRecordAggregateOutcome：健康记录聚合结果。严格区分记录条数与发生次数，
// 未知次数保留 UnknownRecords 范围，不冒充精确统计。
type HealthRecordAggregateOutcome struct {
	RecordCount     int        `json:"record_count"`
	OccurrenceCount int        `json:"occurrence_count"`
	OccurrenceKnown bool       `json:"occurrence_known"`
	UnknownRecords  int        `json:"unknown_records"`
	LatestAt        *time.Time `json:"latest_at,omitempty"`
	CoveredStartAt  *time.Time `json:"covered_start_at,omitempty"`
	CoveredEndAt    *time.Time `json:"covered_end_at,omitempty"`
	Source          ReadSource `json:"source"`
}

// HealthRecordOutcome：单条记录读取结果。
type HealthRecordOutcome struct {
	Record HealthRecordItem `json:"record"`
	Source ReadSource       `json:"source"`
}

// PetListItem：家庭宠物列表项（业务读取视角）。
type PetListItem struct {
	PetID string `json:"pet_id"`
	Name  string `json:"name"`
}

// PetListOutcome：家庭宠物列表。用于「有几只宠物/都有谁」这类问题，
// 让模型先拿到合法 pet_id，而不是靠猜名字去解析。
type PetListOutcome struct {
	Pets   []PetListItem `json:"pets"`
	Source ReadSource    `json:"source"`
}

// CalendarRecordListItem：家庭日程条目（业务读取视角）。
type CalendarRecordListItem struct {
	RecordID    string    `json:"record_id"`
	PetID       string    `json:"pet_id"`
	PetName     string    `json:"pet_name"`
	Category    string    `json:"category"`
	MedicalType string    `json:"medical_type,omitempty"`
	Content     string    `json:"content"`
	OccurredAt  time.Time `json:"occurred_at"`
}

// CalendarRecordListOutcome：家庭日程列表（跨宠物、按时间倒序）。
type CalendarRecordListOutcome struct {
	Records []CalendarRecordListItem `json:"records"`
	Source  ReadSource               `json:"source"`
}

// ReminderListItem：家庭待办提醒条目（业务读取视角）。
type ReminderListItem struct {
	ReminderID   string `json:"reminder_id"`
	PetID        string `json:"pet_id"`
	PetName      string `json:"pet_name"`
	ReminderDate string `json:"reminder_date"`
	Category     string `json:"category"`
	MedicalType  string `json:"medical_type,omitempty"`
	Content      string `json:"content"`
}

// ReminderListOutcome：家庭待办提醒列表（跨宠物、按提醒日期升序）。
type ReminderListOutcome struct {
	Reminders []ReminderListItem `json:"reminders"`
	Source    ReadSource         `json:"source"`
}

// HealthRecordSearchQuery：健康记录检索条件（ask 层契约，与数据层解耦）。
type HealthRecordSearchQuery struct {
	StartAt     time.Time // 起点包含，零值不限
	EndAt       time.Time // 终点不包含，零值不限
	Category    string    // medical / daily
	MedicalType string    // vaccine / deworming / ...
	Limit       int
	Cursor      string
}

// HealthRecordAggregateQuery：健康记录聚合条件。
type HealthRecordAggregateQuery struct {
	StartAt           time.Time
	EndAt             time.Time
	Category          string
	MedicalType       string
	CustomMedicalType string
}

// BusinessReadRepository：业务读取工具端口。T5 决策循环通过它读取业务数据，
// 不直接触碰 pet / calendar 数据层。候选仅来自授权范围（familyID）。
type BusinessReadRepository interface {
	ResolvePet(context.Context, string, string) (PetResolveOutcome, error)
	ListFamilyPets(context.Context, string) (PetListOutcome, error)
	ListCalendarRecords(context.Context, string, time.Time, time.Time, int) (CalendarRecordListOutcome, error)
	ListReminders(context.Context, string, string, int) (ReminderListOutcome, error)
	ReadPetProfile(context.Context, string, string) (PetProfileOutcome, error)
	SearchHealthRecords(context.Context, string, string, HealthRecordSearchQuery) (HealthRecordSearchOutcome, error)
	AggregateHealthRecords(context.Context, string, string, HealthRecordAggregateQuery) (HealthRecordAggregateOutcome, error)
	ReadHealthRecord(context.Context, string, string) (HealthRecordOutcome, error)
}

// hashSourceVersion 计算确定性集合版本。输入顺序固定时输出稳定，任一内容变化
// 都会改变版本，供复用失效判断使用。
func hashSourceVersion(parts ...string) string {
	h := sha256.New()
	for _, part := range parts {
		h.Write([]byte(part))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}
