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
	SourceType string    // pet / health_record / record
	SourceID   string    // family_id / pet_id / record_id
	Version    string    // 集合版本
	ReadAt     time.Time // 实际读取时间
}

// HealthRecordItem：单条健康记录（业务读取视角）。MediaAssetIDs 只保留资产
// 身份，不携带临时访问链接。
type HealthRecordItem struct {
	ID                string
	Category          string
	MedicalType       string
	CustomMedicalType string
	Content           string
	OccurredAt        time.Time
	MediaAssetIDs     []string
}

// PetResolveOutcome：宠物消歧结果。Status 复用 pet_resolver 的 none/resolved/ambiguous。
type PetResolveOutcome struct {
	Status    PetResolveStatus
	Resolved  []petapp.Pet
	Ambiguous []PetCandidate
	Source    ReadSource
}

// PetProfileOutcome：宠物档案读取结果。
type PetProfileOutcome struct {
	Profile petapp.PetProfile
	Health  petapp.PetHealth
	Source  ReadSource
}

// HealthRecordSearchOutcome：健康记录检索结果。
type HealthRecordSearchOutcome struct {
	Records    []HealthRecordItem
	HasMore    bool
	NextCursor string
	Source     ReadSource
}

// HealthRecordAggregateOutcome：健康记录聚合结果。严格区分记录条数与发生次数，
// 未知次数保留 UnknownRecords 范围，不冒充精确统计。
type HealthRecordAggregateOutcome struct {
	RecordCount     int
	OccurrenceCount int
	OccurrenceKnown bool
	UnknownRecords  int
	LatestAt        time.Time
	CoveredStartAt  time.Time
	CoveredEndAt    time.Time
	Source          ReadSource
}

// HealthRecordOutcome：单条记录读取结果。
type HealthRecordOutcome struct {
	Record HealthRecordItem
	Source ReadSource
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
