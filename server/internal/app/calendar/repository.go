package calendar

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type SQLRepository struct {
	db     *sql.DB
	driver string
}

func NewRepository(db *sql.DB, driver string) (*SQLRepository, error) {
	if db == nil {
		return nil, errors.New("database is required")
	}
	return &SQLRepository{db: db, driver: strings.ToLower(driver)}, nil
}

func (r *SQLRepository) ListMonth(ctx context.Context, familyID, month, petID string) ([]DayMarkerDTO, error) {
	start, _ := time.Parse("2006-01", month)
	end := start.AddDate(0, 1, 0)
	markers := map[string]DayMarkerDTO{}
	args := []any{familyID, start.Format("2006-01-02"), end.Format("2006-01-02")}
	petCondition := ""
	if petID != "" {
		petCondition = " AND pet_id = ?"
		args = append(args, petID)
	}
	rows, err := r.db.QueryContext(ctx, r.query("SELECT occurred_on, MAX(CASE WHEN category = 'medical' THEN 1 ELSE 0 END), MAX(CASE WHEN category = 'daily' THEN 1 ELSE 0 END) FROM calendar_records WHERE family_id = ? AND occurred_on >= ? AND occurred_on < ? AND deleted_at IS NULL"+petCondition+" GROUP BY occurred_on ORDER BY occurred_on"), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var date string
		var medical, daily int
		if err := rows.Scan(&date, &medical, &daily); err != nil {
			return nil, err
		}
		date = normalizeDate(date)
		markers[date] = DayMarkerDTO{Date: date, HasMedicalRecord: medical != 0, HasDailyRecord: daily != 0}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	args = []any{familyID, start.Format("2006-01-02"), end.Format("2006-01-02")}
	if petID != "" {
		args = append(args, petID)
	}
	rows, err = r.db.QueryContext(ctx, r.query("SELECT reminder_date FROM calendar_reminders WHERE family_id = ? AND reminder_date >= ? AND reminder_date < ? AND status = 'pending'"+petCondition+" GROUP BY reminder_date ORDER BY reminder_date"), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var date string
		if err := rows.Scan(&date); err != nil {
			return nil, err
		}
		date = normalizeDate(date)
		marker := markers[date]
		marker.Date = date
		marker.HasPendingReminder = true
		markers[date] = marker
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	result := make([]DayMarkerDTO, 0, len(markers))
	for date := start; date.Before(end); date = date.AddDate(0, 0, 1) {
		if marker, ok := markers[date.Format("2006-01-02")]; ok {
			result = append(result, marker)
		}
	}
	return result, nil
}

func (r *SQLRepository) GetDay(ctx context.Context, familyID, date string) (DayDTO, error) {
	records, err := r.listDayRecords(ctx, familyID, date)
	if err != nil {
		return DayDTO{}, err
	}
	reminders, err := r.listDayReminders(ctx, familyID, date)
	if err != nil {
		return DayDTO{}, err
	}
	return DayDTO{
		Date:      date,
		Summary:   DaySummaryDTO{RecordCount: len(records), PendingReminderCount: len(reminders)},
		Reminders: reminders,
		Records:   records,
	}, nil
}

func (r *SQLRepository) CreateRecord(ctx context.Context, familyID, userID string, request CreateRecordRequest, occurredAt time.Time) (RecordDTO, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return RecordDTO{}, err
	}
	defer tx.Rollback()
	if err := r.requirePet(ctx, tx, familyID, request.PetID); err != nil {
		return RecordDTO{}, err
	}
	recordID, err := newID()
	if err != nil {
		return RecordDTO{}, err
	}
	if err := r.insertRecord(ctx, tx, recordID, familyID, userID, request.PetID, request.Category, request.MedicalType, request.Content, occurredAt); err != nil {
		return RecordDTO{}, err
	}
	if err := r.insertMedia(ctx, tx, familyID, recordID, request.MediaAssetIDs); err != nil {
		return RecordDTO{}, err
	}
	if request.Reminder != nil {
		if _, err := r.insertReminder(ctx, tx, familyID, userID, request.PetID, recordID, "", *request.Reminder); err != nil {
			return RecordDTO{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return RecordDTO{}, err
	}
	return r.getRecord(ctx, familyID, recordID)
}

func (r *SQLRepository) CompleteReminder(ctx context.Context, familyID, userID, reminderID string, request CompleteReminderRequest, completedAt time.Time) (CompleteReminderDTO, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return CompleteReminderDTO{}, err
	}
	defer tx.Rollback()
	var sourceRecordID, petID, sourceContent, medicalType, repeatType, channels string
	var repeatInterval sql.NullInt64
	var advanceDays int
	err = tx.QueryRowContext(ctx, r.query("SELECT m.source_record_id, m.pet_id, r.content, COALESCE(r.medical_type, ''), m.repeat_type, m.repeat_interval_days, m.advance_days, m.notification_channels FROM calendar_reminders m JOIN calendar_records r ON r.id = m.source_record_id AND r.family_id = m.family_id AND r.deleted_at IS NULL WHERE m.id = ? AND m.family_id = ?"), reminderID, familyID).Scan(&sourceRecordID, &petID, &sourceContent, &medicalType, &repeatType, &repeatInterval, &advanceDays, &channels)
	if err != nil {
		return CompleteReminderDTO{}, err
	}
	recordID, err := newID()
	if err != nil {
		return CompleteReminderDTO{}, err
	}
	content := strings.TrimSpace(request.Content)
	if content == "" {
		content = sourceContent
	}
	// 先插入完成记录与媒体，再更新提醒状态——completed_record_id 外键
	// 指向的记录必须先存在（Postgres 事务内外键为立即检查，顺序颠倒
	// 会触发 23503 外键违反；SQLite 默认不启用外键故此前未暴露）。
	if err := r.insertRecord(ctx, tx, recordID, familyID, userID, petID, "medical", medicalType, content, completedAt); err != nil {
		return CompleteReminderDTO{}, err
	}
	if err := r.insertMedia(ctx, tx, familyID, recordID, request.MediaAssetIDs); err != nil {
		return CompleteReminderDTO{}, err
	}
	result, err := tx.ExecContext(ctx, r.query("UPDATE calendar_reminders SET status = 'completed', completed_at = ?, completed_by = ?, completed_record_id = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND family_id = ? AND status = 'pending'"), completedAt, userID, recordID, reminderID, familyID)
	if err != nil {
		return CompleteReminderDTO{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return CompleteReminderDTO{}, err
	}
	if affected == 0 {
		return CompleteReminderDTO{}, ErrReminderCompleted
	}
	var nextReminderID string
	if repeatType != "once" {
		nextDate, err := nextReminderDate(completedAt, repeatType, repeatInterval)
		if err != nil {
			return CompleteReminderDTO{}, err
		}
		channelsValue, err := parseChannels(channels)
		if err != nil {
			return CompleteReminderDTO{}, err
		}
		input := CreateReminderRequest{ReminderDate: nextDate, RepeatType: repeatType, AdvanceDays: &advanceDays, NotificationChannels: channelsValue}
		if repeatInterval.Valid {
			value := int(repeatInterval.Int64)
			input.RepeatIntervalDays = &value
		}
		nextReminderID, err = r.insertReminder(ctx, tx, familyID, userID, petID, recordID, reminderID, input)
		if err != nil {
			return CompleteReminderDTO{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return CompleteReminderDTO{}, err
	}
	completedRecord, err := r.getRecord(ctx, familyID, recordID)
	if err != nil {
		return CompleteReminderDTO{}, err
	}
	completedReminder, err := r.getReminder(ctx, familyID, reminderID)
	if err != nil {
		return CompleteReminderDTO{}, err
	}
	resultDTO := CompleteReminderDTO{CompletedRecord: completedRecord, CompletedReminder: completedReminder}
	if nextReminderID != "" {
		nextReminder, err := r.getReminder(ctx, familyID, nextReminderID)
		if err != nil {
			return CompleteReminderDTO{}, err
		}
		resultDTO.NextReminder = &nextReminder
	}
	return resultDTO, nil
}

func (r *SQLRepository) requirePet(ctx context.Context, tx *sql.Tx, familyID, petID string) error {
	var id string
	return tx.QueryRowContext(ctx, r.query("SELECT id FROM pets WHERE id = ? AND family_id = ? AND deleted_at IS NULL"), petID, familyID).Scan(&id)
}

func (r *SQLRepository) insertRecord(ctx context.Context, tx *sql.Tx, recordID, familyID, userID, petID, category, medicalType, content string, occurredAt time.Time) error {
	_, err := tx.ExecContext(ctx, r.query("INSERT INTO calendar_records (id, family_id, pet_id, category, medical_type, content, occurred_at, occurred_on, created_by, updated_by, created_at, updated_at) VALUES (?, ?, ?, ?, NULLIF(?, ''), ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)"), recordID, familyID, petID, category, medicalType, content, occurredAt, occurredAt.Format("2006-01-02"), userID, userID)
	return err
}

func (r *SQLRepository) insertMedia(ctx context.Context, tx *sql.Tx, familyID, recordID string, assetIDs []string) error {
	for index, assetID := range assetIDs {
		mediaID, err := newID()
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, r.query("INSERT INTO calendar_record_media (id, record_id, family_id, asset_id, sort_order, created_at) VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)"), mediaID, recordID, familyID, strings.TrimSpace(assetID), index+1); err != nil {
			return err
		}
	}
	return nil
}

func (r *SQLRepository) insertReminder(ctx context.Context, tx *sql.Tx, familyID, userID, petID, recordID, previousReminderID string, request CreateReminderRequest) (string, error) {
	id, err := newID()
	if err != nil {
		return "", err
	}
	channels, err := json.Marshal(request.NotificationChannels)
	if err != nil {
		return "", err
	}
	var interval any
	if request.RepeatIntervalDays != nil {
		interval = *request.RepeatIntervalDays
	}
	_, err = tx.ExecContext(ctx, r.query("INSERT INTO calendar_reminders (id, family_id, pet_id, source_record_id, previous_reminder_id, reminder_date, repeat_type, repeat_interval_days, advance_days, notification_channels, status, created_by, created_at, updated_at) VALUES (?, ?, ?, ?, NULLIF(?, ''), ?, ?, ?, ?, ?, 'pending', ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)"), id, familyID, petID, recordID, previousReminderID, request.ReminderDate, request.RepeatType, interval, *request.AdvanceDays, string(channels), userID)
	if err != nil {
		return "", err
	}
	return id, nil
}

func (r *SQLRepository) listDayRecords(ctx context.Context, familyID, date string) ([]RecordDTO, error) {
	rows, err := r.db.QueryContext(ctx, r.query("SELECT r.id, r.category, COALESCE(r.medical_type, ''), r.content, r.occurred_at, p.id, p.name, COALESCE(p.avatar_asset_id, ''), u.id, COALESCE(u.nickname, ''), COALESCE(u.avatar_asset_id, '') FROM calendar_records r JOIN pets p ON p.id = r.pet_id AND p.family_id = r.family_id JOIN users u ON u.id = r.created_by WHERE r.family_id = ? AND r.occurred_on = ? AND r.deleted_at IS NULL ORDER BY r.occurred_at DESC, r.id DESC"), familyID, date)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]RecordDTO, 0)
	for rows.Next() {
		value, err := scanRecord(rows)
		if err != nil {
			return nil, err
		}
		value.Media, err = r.listMedia(ctx, familyID, value.ID)
		if err != nil {
			return nil, err
		}
		value.Reminder, err = r.getRecordReminder(ctx, familyID, value.ID)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (r *SQLRepository) getRecord(ctx context.Context, familyID, recordID string) (RecordDTO, error) {
	row := r.db.QueryRowContext(ctx, r.query("SELECT r.id, r.category, COALESCE(r.medical_type, ''), r.content, r.occurred_at, p.id, p.name, COALESCE(p.avatar_asset_id, ''), u.id, COALESCE(u.nickname, ''), COALESCE(u.avatar_asset_id, '') FROM calendar_records r JOIN pets p ON p.id = r.pet_id AND p.family_id = r.family_id JOIN users u ON u.id = r.created_by WHERE r.id = ? AND r.family_id = ? AND r.deleted_at IS NULL"), recordID, familyID)
	value, err := scanRecord(row)
	if err != nil {
		return RecordDTO{}, err
	}
	value.Media, err = r.listMedia(ctx, familyID, value.ID)
	if err != nil {
		return RecordDTO{}, err
	}
	value.Reminder, err = r.getRecordReminder(ctx, familyID, value.ID)
	if err != nil {
		return RecordDTO{}, err
	}
	return value, nil
}

func (r *SQLRepository) listMedia(ctx context.Context, familyID, recordID string) ([]MediaDTO, error) {
	rows, err := r.db.QueryContext(ctx, r.query("SELECT id, asset_id, sort_order FROM calendar_record_media WHERE record_id = ? AND family_id = ? ORDER BY sort_order"), recordID, familyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]MediaDTO, 0)
	for rows.Next() {
		var value MediaDTO
		if err := rows.Scan(&value.ID, &value.AssetID, &value.SortOrder); err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (r *SQLRepository) listDayReminders(ctx context.Context, familyID, date string) ([]ReminderDTO, error) {
	rows, err := r.db.QueryContext(ctx, r.query(reminderSelect("m.family_id = ? AND m.reminder_date = ? AND m.status = 'pending' ORDER BY m.created_at, m.id")), familyID, date)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]ReminderDTO, 0)
	for rows.Next() {
		value, err := scanReminder(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (r *SQLRepository) getRecordReminder(ctx context.Context, familyID, recordID string) (*ReminderDTO, error) {
	row := r.db.QueryRowContext(ctx, r.query(reminderSelect("m.family_id = ? AND m.source_record_id = ? ORDER BY m.created_at DESC, m.id DESC LIMIT 1")), familyID, recordID)
	value, err := scanReminder(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &value, nil
}

func (r *SQLRepository) getReminder(ctx context.Context, familyID, reminderID string) (ReminderDTO, error) {
	row := r.db.QueryRowContext(ctx, r.query(reminderSelect("m.family_id = ? AND m.id = ?")), familyID, reminderID)
	return scanReminder(row)
}

func reminderSelect(condition string) string {
	return "SELECT m.id, m.source_record_id, m.reminder_date, p.id, p.name, COALESCE(p.avatar_asset_id, ''), r.content, COALESCE(r.medical_type, ''), u.id, COALESCE(u.nickname, ''), COALESCE(u.avatar_asset_id, ''), m.repeat_type, m.repeat_interval_days, m.advance_days, m.notification_channels, m.status FROM calendar_reminders m JOIN calendar_records r ON r.id = m.source_record_id AND r.family_id = m.family_id JOIN pets p ON p.id = m.pet_id AND p.family_id = m.family_id JOIN users u ON u.id = m.created_by WHERE " + condition
}

type scanner interface {
	Scan(...any) error
}

func scanRecord(row scanner) (RecordDTO, error) {
	var value RecordDTO
	var occurredAt time.Time
	err := row.Scan(&value.ID, &value.Category, &value.MedicalType, &value.Content, &occurredAt, &value.Pet.ID, &value.Pet.Name, &value.Pet.AvatarAssetID, &value.CreatedBy.UserID, &value.CreatedBy.Nickname, &value.CreatedBy.AvatarAssetID)
	if err != nil {
		return RecordDTO{}, err
	}
	value.OccurredAt = occurredAt.Format(time.RFC3339)
	return value, nil
}

func scanReminder(row scanner) (ReminderDTO, error) {
	var value ReminderDTO
	var interval sql.NullInt64
	var channels string
	err := row.Scan(&value.ID, &value.SourceRecordID, &value.ReminderDate, &value.Pet.ID, &value.Pet.Name, &value.Pet.AvatarAssetID, &value.Content, &value.MedicalType, &value.CreatedBy.UserID, &value.CreatedBy.Nickname, &value.CreatedBy.AvatarAssetID, &value.RepeatType, &interval, &value.AdvanceDays, &channels, &value.Status)
	if err != nil {
		return ReminderDTO{}, err
	}
	if interval.Valid {
		intervalValue := int(interval.Int64)
		value.RepeatIntervalDays = &intervalValue
	}
	value.ReminderDate = normalizeDate(value.ReminderDate)
	parsedChannels, err := parseChannels(channels)
	if err != nil {
		return ReminderDTO{}, err
	}
	value.NotificationChannels = parsedChannels
	return value, nil
}

func normalizeDate(value string) string {
	if len(value) >= len("2006-01-02") && value[4] == '-' && value[7] == '-' {
		return value[:len("2006-01-02")]
	}
	return value
}

func parseChannels(value string) ([]string, error) {
	var channels []string
	if err := json.Unmarshal([]byte(value), &channels); err != nil {
		return nil, err
	}
	return channels, nil
}

func nextReminderDate(completedAt time.Time, repeatType string, interval sql.NullInt64) (string, error) {
	date := completedAt
	switch repeatType {
	case "monthly":
		date = date.AddDate(0, 1, 0)
	case "yearly":
		date = date.AddDate(1, 0, 0)
	case "custom_days":
		if !interval.Valid || interval.Int64 <= 0 {
			return "", fmt.Errorf("invalid custom reminder interval")
		}
		date = date.AddDate(0, 0, int(interval.Int64))
	default:
		return "", fmt.Errorf("invalid reminder repeat type")
	}
	return date.Format("2006-01-02"), nil
}

func (r *SQLRepository) query(value string) string {
	if r.driver != "postgres" && r.driver != "postgresql" && r.driver != "pgx" {
		return value
	}
	var result strings.Builder
	index := 0
	for _, character := range value {
		if character != '?' {
			result.WriteRune(character)
			continue
		}
		index++
		result.WriteByte('$')
		result.WriteString(strconv.Itoa(index))
	}
	return result.String()
}

func newID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}
