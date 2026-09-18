package calendar

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// RecordSearchQuery：健康记录检索条件。
// StartAt 起点包含、EndAt 终点不包含；零值表示该侧不限。
type RecordSearchQuery struct {
	StartAt     time.Time
	EndAt       time.Time
	Category    string // medical / daily，空表示不限
	MedicalType string // vaccine / deworming / checkup / visit / medication / other，空表示不限
	Limit       int    // 每页条数，<=0 取默认
	Cursor      string // 上一页最后一条的游标，空表示第一页
}

// RecordPage：一页检索结果。HasMore 区分「本页结束」与「全部读完」。
type RecordPage struct {
	Records    []ContextRecord
	HasMore    bool
	NextCursor string
}

const (
	defaultRecordPageSize = 20
	maxRecordPageSize     = 100
)

// SearchRecords 按时间区间、分类与医疗类型检索记录，返回稳定排序
// (occurred_at DESC, id DESC) 的游标分页结果。读取故障与无记录分开表达。
func (r *SQLRepository) SearchRecords(ctx context.Context, familyID, petID string, q RecordSearchQuery) (RecordPage, error) {
	limit := q.Limit
	if limit <= 0 {
		limit = defaultRecordPageSize
	}
	if limit > maxRecordPageSize {
		limit = maxRecordPageSize
	}
	args := []any{familyID, petID}
	condition := "family_id = ? AND pet_id = ? AND deleted_at IS NULL"
	if !q.StartAt.IsZero() {
		condition += " AND occurred_at >= ?"
		args = append(args, q.StartAt)
	}
	if !q.EndAt.IsZero() {
		condition += " AND occurred_at < ?"
		args = append(args, q.EndAt)
	}
	if category := strings.TrimSpace(q.Category); category != "" {
		condition += " AND category = ?"
		args = append(args, category)
	}
	if medicalType := strings.TrimSpace(q.MedicalType); medicalType != "" {
		condition += " AND medical_type = ?"
		args = append(args, medicalType)
	}
	if cursor := strings.TrimSpace(q.Cursor); cursor != "" {
		cursorAt, cursorID, err := parseRecordCursor(cursor)
		if err != nil {
			return RecordPage{}, err
		}
		condition += " AND (occurred_at < ? OR (occurred_at = ? AND id < ?))"
		args = append(args, cursorAt, cursorAt, cursorID)
	}
	// 多取一条判断 HasMore。
	query := r.query("SELECT id, category, COALESCE(medical_type, ''), COALESCE(custom_medical_type, ''), content, occurred_at FROM calendar_records WHERE " + condition + " ORDER BY occurred_at DESC, id DESC LIMIT ?")
	args = append(args, limit+1)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return RecordPage{}, err
	}
	defer rows.Close()
	records := make([]ContextRecord, 0, limit+1)
	for rows.Next() {
		var value ContextRecord
		if err := rows.Scan(&value.ID, &value.Category, &value.MedicalType, &value.CustomMedicalType, &value.Content, &value.OccurredAt); err != nil {
			return RecordPage{}, err
		}
		records = append(records, value)
	}
	if err := rows.Err(); err != nil {
		return RecordPage{}, err
	}
	page := RecordPage{}
	if len(records) > limit {
		page.HasMore = true
		records = records[:limit]
	}
	page.Records = records
	if page.HasMore && len(records) > 0 {
		last := records[len(records)-1]
		page.NextCursor = encodeRecordCursor(last.OccurredAt, last.ID)
	}
	return page, nil
}

// RecordAggregateQuery：聚合查询条件。MedicalType 与 CustomMedicalType
// 均为空时按 Category 聚合整类记录。
type RecordAggregateQuery struct {
	StartAt           time.Time
	EndAt             time.Time
	Category          string // medical / daily，空表示不限
	MedicalType       string // 症状或事件类型（结构化 medical_type），空表示不限
	CustomMedicalType string // 仅配合 medical_type=other 使用
}

// RecordAggregate：聚合结果。严格区分「记录条数」与「发生次数」，
// 无法可靠解析发生次数时保留未知条目范围，不把记录条数冒充发生次数。
type RecordAggregate struct {
	RecordCount     int       // 记录条数（可靠）
	OccurrenceCount int       // 已明确的发生次数合计
	OccurrenceKnown bool      // 是否全部记录的发生次数都已明确
	UnknownRecords  int       // 未能解析发生次数的记录条数
	LatestAt        time.Time // 最近发生时间，零值表示无记录
	CoveredStartAt  time.Time // 实际覆盖起点，零值表示无记录
	CoveredEndAt    time.Time // 实际覆盖终点，零值表示无记录
}

// AggregateRecords 按类型聚合记录条数与发生次数。发生次数只累加可从内容
// 明确解析的部分，解析不到的记入 UnknownRecords，OccurrenceKnown 置 false。
func (r *SQLRepository) AggregateRecords(ctx context.Context, familyID, petID string, q RecordAggregateQuery) (RecordAggregate, error) {
	args := []any{familyID, petID}
	condition := "family_id = ? AND pet_id = ? AND deleted_at IS NULL"
	if !q.StartAt.IsZero() {
		condition += " AND occurred_at >= ?"
		args = append(args, q.StartAt)
	}
	if !q.EndAt.IsZero() {
		condition += " AND occurred_at < ?"
		args = append(args, q.EndAt)
	}
	if category := strings.TrimSpace(q.Category); category != "" {
		condition += " AND category = ?"
		args = append(args, category)
	}
	if medicalType := strings.TrimSpace(q.MedicalType); medicalType != "" {
		condition += " AND medical_type = ?"
		args = append(args, medicalType)
	}
	if customType := strings.TrimSpace(q.CustomMedicalType); customType != "" {
		condition += " AND custom_medical_type = ?"
		args = append(args, customType)
	}
	rows, err := r.db.QueryContext(ctx, r.query("SELECT content, occurred_at FROM calendar_records WHERE "+condition+" ORDER BY occurred_at ASC, id ASC"), args...)
	if err != nil {
		return RecordAggregate{}, err
	}
	defer rows.Close()
	result := RecordAggregate{}
	for rows.Next() {
		var content string
		var occurredAt time.Time
		if err := rows.Scan(&content, &occurredAt); err != nil {
			return RecordAggregate{}, err
		}
		result.RecordCount++
		if result.CoveredStartAt.IsZero() || occurredAt.Before(result.CoveredStartAt) {
			result.CoveredStartAt = occurredAt
		}
		if result.CoveredEndAt.IsZero() || occurredAt.After(result.CoveredEndAt) {
			result.CoveredEndAt = occurredAt
		}
		if occurredAt.After(result.LatestAt) {
			result.LatestAt = occurredAt
		}
		if count, ok := parseOccurrence(content); ok {
			result.OccurrenceCount += count
		} else {
			result.UnknownRecords++
		}
	}
	if err := rows.Err(); err != nil {
		return RecordAggregate{}, err
	}
	result.OccurrenceKnown = result.UnknownRecords == 0
	return result, nil
}

// GetRecord 读取单条记录及其媒体、提醒引用。无记录返回 sql.ErrNoRows，
// 与读取故障（其他 error）分开表达。
func (r *SQLRepository) GetRecord(ctx context.Context, familyID, recordID string) (RecordDTO, error) {
	return r.getRecord(ctx, familyID, recordID)
}

// encodeRecordCursor 用 occurred_at 的 RFC3339Nano 表示 + id 组成稳定游标。
// 保留原始时区偏移，保证还原后序列化结果与存储值逐字一致，分页比较才正确。
func encodeRecordCursor(at time.Time, id string) string {
	return at.Format(time.RFC3339Nano) + "|" + id
}

// parseRecordCursor 还原游标。非法游标返回可定位错误，不静默放宽查询范围。
func parseRecordCursor(cursor string) (time.Time, string, error) {
	index := strings.LastIndex(cursor, "|")
	if index <= 0 || index == len(cursor)-1 {
		return time.Time{}, "", fmt.Errorf("calendar: invalid record cursor")
	}
	at, err := time.Parse(time.RFC3339Nano, cursor[:index])
	if err != nil {
		return time.Time{}, "", fmt.Errorf("calendar: invalid record cursor: %w", err)
	}
	return at, cursor[index+1:], nil
}

// occurrenceUnits：内容中表示「发生一次」的计数单位字。
var occurrenceUnits = map[rune]struct{}{
	'次': {},
	'遍': {},
	'回': {},
	'顿': {},
}

// chineseDigitValue：常见中文数字 → 数值，仅覆盖个位数与「十」。
var chineseDigitValue = map[rune]int{
	'一': 1,
	'二': 2,
	'两': 2,
	'三': 3,
	'四': 4,
	'五': 5,
	'六': 6,
	'七': 7,
	'八': 8,
	'九': 9,
	'十': 10,
}

// parseOccurrence 从内容中解析「明确写出的发生次数」。支持阿拉伯数字紧邻
// 计数单位（"吐了3次"）与常见中文数字（"一次"、"两次"）。无法可靠确定时
// 返回 0,false，调用方不得据此把记录条数当作发生次数。
func parseOccurrence(content string) (int, bool) {
	runes := []rune(content)
	for i := 0; i < len(runes); i++ {
		if _, isUnit := occurrenceUnits[runes[i]]; !isUnit {
			continue
		}
		if i == 0 {
			continue
		}
		// 阿拉伯数字：向前读连续数字。
		if runes[i-1] >= '0' && runes[i-1] <= '9' {
			start := i - 1
			for start > 0 && runes[start-1] >= '0' && runes[start-1] <= '9' {
				start--
			}
			value, err := strconv.Atoi(string(runes[start:i]))
			if err != nil || value <= 0 {
				continue
			}
			return value, true
		}
		// 中文数字：紧邻单个中文数字。
		if value, ok := chineseDigitValue[runes[i-1]]; ok {
			return value, true
		}
	}
	return 0, false
}
