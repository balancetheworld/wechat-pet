package ask

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"time"

	calendarapp "github.com/balancetheworld/wechat-pet/internal/app/calendar"
	petapp "github.com/balancetheworld/wechat-pet/internal/app/pet"
)

const (
	sourceTypePet          = "pet"
	sourceTypeHealthRecord = "health_record"
	sourceTypeRecord       = "record"
)

// businessReadRepository 实现 BusinessReadRepository，复用 pet / calendar 数据层。
// 消歧候选仅来自授权范围（familyID 对应家庭），读取结果记录来源集合版本，
// 供跨 Run 复用前的失效核验。
type businessReadRepository struct {
	pets     *petapp.SQLRepository
	calendar *calendarapp.SQLRepository
	source   *SQLRepository
}

// NewBusinessReadRepository 构造业务读取实现。
func NewBusinessReadRepository(pets *petapp.SQLRepository, calendar *calendarapp.SQLRepository, source *SQLRepository) BusinessReadRepository {
	return &businessReadRepository{pets: pets, calendar: calendar, source: source}
}

var _ BusinessReadRepository = (*businessReadRepository)(nil)

// ResolvePet 在授权范围内解析宠物。候选仅来自当前家庭，不泄露家庭外对象。
func (r *businessReadRepository) ResolvePet(ctx context.Context, familyID, input string) (PetResolveOutcome, error) {
	pets, err := r.pets.List(ctx, familyID)
	if err != nil {
		return PetResolveOutcome{}, err
	}
	resolution := ResolvePets(input, pets)
	source := ReadSource{SourceType: sourceTypePet, SourceID: familyID, ReadAt: time.Now().UTC()}
	source.Version = petCollectionVersion(pets)
	r.recordSourceVersion(ctx, source)
	return PetResolveOutcome{
		Status:    resolution.Status,
		Resolved:  resolution.Resolved,
		Ambiguous: resolution.Ambiguous,
		Source:    source,
	}, nil
}

// ReadPetProfile 读取宠物档案与健康信息。缺资料不填默认病史；无记录返回
// sql.ErrNoRows 映射为可区分错误，读取故障透传。
func (r *businessReadRepository) ReadPetProfile(ctx context.Context, familyID, petID string) (PetProfileOutcome, error) {
	pet, err := r.pets.Get(ctx, familyID, petID)
	if err != nil {
		return PetProfileOutcome{}, err
	}
	profile, err := r.pets.GetProfile(ctx, familyID, petID)
	if err != nil {
		return PetProfileOutcome{}, err
	}
	health, err := r.pets.GetHealth(ctx, familyID, petID)
	if err != nil {
		return PetProfileOutcome{}, err
	}
	source := ReadSource{SourceType: sourceTypePet, SourceID: petID, ReadAt: time.Now().UTC()}
	source.Version = hashSourceVersion(petID, pet.UpdatedAt.UTC().Format(time.RFC3339Nano))
	r.recordSourceVersion(ctx, source)
	return PetProfileOutcome{Profile: profile, Health: health, Source: source}, nil
}

// SearchHealthRecords 检索健康记录。无记录返回空列表（error 为 nil），读取
// 故障返回非 nil error，两者始终分开。
func (r *businessReadRepository) SearchHealthRecords(ctx context.Context, familyID, petID string, query HealthRecordSearchQuery) (HealthRecordSearchOutcome, error) {
	page, err := r.calendar.SearchRecords(ctx, familyID, petID, calendarapp.RecordSearchQuery{
		StartAt:     query.StartAt,
		EndAt:       query.EndAt,
		Category:    query.Category,
		MedicalType: query.MedicalType,
		Limit:       query.Limit,
		Cursor:      query.Cursor,
	})
	if err != nil {
		return HealthRecordSearchOutcome{}, err
	}
	records := make([]HealthRecordItem, 0, len(page.Records))
	for _, record := range page.Records {
		records = append(records, healthRecordItemFromContext(record))
	}
	source := ReadSource{SourceType: sourceTypeHealthRecord, SourceID: petID, ReadAt: time.Now().UTC()}
	source.Version = recordCollectionVersion(page.Records)
	r.recordSourceVersion(ctx, source)
	return HealthRecordSearchOutcome{
		Records:    records,
		HasMore:    page.HasMore,
		NextCursor: page.NextCursor,
		Source:     source,
	}, nil
}

// AggregateHealthRecords 聚合记录条数与发生次数。未知次数保留范围，不冒充精确统计。
func (r *businessReadRepository) AggregateHealthRecords(ctx context.Context, familyID, petID string, query HealthRecordAggregateQuery) (HealthRecordAggregateOutcome, error) {
	aggregate, err := r.calendar.AggregateRecords(ctx, familyID, petID, calendarapp.RecordAggregateQuery{
		StartAt:           query.StartAt,
		EndAt:             query.EndAt,
		Category:          query.Category,
		MedicalType:       query.MedicalType,
		CustomMedicalType: query.CustomMedicalType,
	})
	if err != nil {
		return HealthRecordAggregateOutcome{}, err
	}
	source := ReadSource{SourceType: sourceTypeHealthRecord, SourceID: petID, ReadAt: time.Now().UTC()}
	source.Version = hashSourceVersion(
		petID,
		fmt.Sprintf("%d", aggregate.RecordCount),
		aggregate.CoveredStartAt.UTC().Format(time.RFC3339Nano),
		aggregate.CoveredEndAt.UTC().Format(time.RFC3339Nano),
	)
	r.recordSourceVersion(ctx, source)
	return HealthRecordAggregateOutcome{
		RecordCount:     aggregate.RecordCount,
		OccurrenceCount: aggregate.OccurrenceCount,
		OccurrenceKnown: aggregate.OccurrenceKnown,
		UnknownRecords:  aggregate.UnknownRecords,
		LatestAt:        aggregate.LatestAt,
		CoveredStartAt:  aggregate.CoveredStartAt,
		CoveredEndAt:    aggregate.CoveredEndAt,
		Source:          source,
	}, nil
}

// ReadHealthRecord 读取单条记录及其媒体身份。无记录/已删除返回
// ErrHealthRecordNotFound，与读取故障分开。
func (r *businessReadRepository) ReadHealthRecord(ctx context.Context, familyID, recordID string) (HealthRecordOutcome, error) {
	record, err := r.calendar.GetRecord(ctx, familyID, recordID)
	if errors.Is(err, sql.ErrNoRows) {
		return HealthRecordOutcome{}, ErrHealthRecordNotFound
	}
	if err != nil {
		return HealthRecordOutcome{}, err
	}
	occurredAt, err := parseRecordTime(record.OccurredAt)
	if err != nil {
		return HealthRecordOutcome{}, err
	}
	media := make([]string, 0, len(record.Media))
	for _, item := range record.Media {
		if item.AssetID != "" {
			media = append(media, item.AssetID)
		}
	}
	source := ReadSource{SourceType: sourceTypeRecord, SourceID: recordID, ReadAt: time.Now().UTC()}
	source.Version = hashSourceVersion(recordID, occurredAt.UTC().Format(time.RFC3339Nano))
	r.recordSourceVersion(ctx, source)
	return HealthRecordOutcome{
		Record: HealthRecordItem{
			ID:                record.ID,
			Category:          record.Category,
			MedicalType:       record.MedicalType,
			CustomMedicalType: record.CustomMedicalType,
			Content:           record.Content,
			OccurredAt:        occurredAt,
			MediaAssetIDs:     media,
		},
		Source: source,
	}, nil
}

// recordSourceVersion 尽力持久化来源集合版本；失败不影响读取结果返回。
func (r *businessReadRepository) recordSourceVersion(ctx context.Context, source ReadSource) {
	if r.source == nil || source.Version == "" {
		return
	}
	_ = r.source.UpsertSourceVersion(ctx, source.SourceType, source.SourceID, source.Version)
}

// petCollectionVersion 计算家庭宠物集合的稳定版本。
func petCollectionVersion(pets []petapp.Pet) string {
	sorted := make([]petapp.Pet, len(pets))
	copy(sorted, pets)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })
	parts := make([]string, 0, len(sorted))
	for _, pet := range sorted {
		parts = append(parts, pet.ID, pet.Name, pet.UpdatedAt.UTC().Format(time.RFC3339Nano))
	}
	return hashSourceVersion(parts...)
}

// recordCollectionVersion 计算记录集合的稳定版本。
func recordCollectionVersion(records []calendarapp.ContextRecord) string {
	sorted := make([]calendarapp.ContextRecord, len(records))
	copy(sorted, records)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })
	parts := make([]string, 0, len(sorted))
	for _, record := range sorted {
		parts = append(parts, record.ID, record.OccurredAt.UTC().Format(time.RFC3339Nano))
	}
	return hashSourceVersion(parts...)
}

func healthRecordItemFromContext(record calendarapp.ContextRecord) HealthRecordItem {
	return HealthRecordItem{
		ID:                record.ID,
		Category:          record.Category,
		MedicalType:       record.MedicalType,
		CustomMedicalType: record.CustomMedicalType,
		Content:           record.Content,
		OccurredAt:        record.OccurredAt,
	}
}

func parseRecordTime(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339, value)
}
