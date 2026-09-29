package calendar

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

func (r *SQLRepository) RecordSubscriptionGrant(ctx context.Context, userID, templateID string, accepted bool) (int, error) {
	userID = strings.TrimSpace(userID)
	templateID = strings.TrimSpace(templateID)
	if userID == "" || templateID == "" {
		return 0, errors.New("calendar subscription grant requires user and template")
	}
	if accepted {
		var openid string
		if err := r.db.QueryRowContext(ctx, r.query("SELECT COALESCE(openid, '') FROM users WHERE id = ?"), userID).Scan(&openid); err != nil {
			return 0, err
		}
		id, err := newID()
		if err != nil {
			return 0, err
		}
		now := time.Now().UTC()
		if _, err := r.db.ExecContext(ctx, r.query("INSERT INTO wechat_subscribe_grants (id, user_id, openid, template_id, remaining, created_at, updated_at) VALUES (?, ?, ?, ?, 1, ?, ?) ON CONFLICT (user_id, template_id) DO UPDATE SET remaining = wechat_subscribe_grants.remaining + 1, openid = excluded.openid, updated_at = excluded.updated_at"), id, userID, openid, templateID, now, now); err != nil {
			return 0, err
		}
	}
	return r.SubscriptionRemaining(ctx, userID, templateID)
}

func (r *SQLRepository) SubscriptionRemaining(ctx context.Context, userID, templateID string) (int, error) {
	var remaining int
	err := r.db.QueryRowContext(ctx, r.query("SELECT remaining FROM wechat_subscribe_grants WHERE user_id = ? AND template_id = ?"), userID, templateID).Scan(&remaining)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return remaining, nil
}

func (r *SQLRepository) ConsumeSubscriptionGrant(ctx context.Context, userID, templateID string) (bool, error) {
	result, err := r.db.ExecContext(ctx, r.query("UPDATE wechat_subscribe_grants SET remaining = remaining - 1, updated_at = ? WHERE user_id = ? AND template_id = ? AND remaining > 0"), time.Now().UTC(), userID, templateID)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

func (r *SQLRepository) ListDuePushReminders(ctx context.Context, until string, staleBefore time.Time, limit int) ([]PushReminder, error) {
	if limit < 1 || limit > 500 {
		limit = reminderNotifyBatchLimit
	}
	rows, err := r.db.QueryContext(ctx, r.query("SELECT m.id, m.created_by, COALESCE(u.openid, ''), p.name, r.content, m.reminder_date, m.advance_days, m.notify_attempts FROM calendar_reminders m JOIN calendar_records r ON r.id = m.source_record_id AND r.family_id = m.family_id AND r.deleted_at IS NULL JOIN pets p ON p.id = m.pet_id AND p.family_id = m.family_id JOIN users u ON u.id = m.created_by WHERE m.status = 'pending' AND (m.notify_status = 'pending' OR (m.notify_status = 'sending' AND m.updated_at <= ?)) AND m.notification_channels LIKE ? AND m.reminder_date <= ? ORDER BY m.reminder_date, m.created_at, m.id LIMIT ?"), staleBefore.UTC(), `%"push"%`, until, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]PushReminder, 0)
	for rows.Next() {
		var value PushReminder
		if err := rows.Scan(&value.ReminderID, &value.UserID, &value.OpenID, &value.PetName, &value.Content, &value.ReminderDate, &value.AdvanceDays, &value.NotifyAttempts); err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (r *SQLRepository) ClaimReminderNotification(ctx context.Context, reminderID string, now, staleBefore time.Time) (bool, error) {
	result, err := r.db.ExecContext(ctx, r.query("UPDATE calendar_reminders SET notify_status = 'sending', notify_attempts = notify_attempts + 1, updated_at = ? WHERE id = ? AND status = 'pending' AND (notify_status = 'pending' OR (notify_status = 'sending' AND updated_at <= ?))"), now.UTC(), reminderID, staleBefore.UTC())
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

func (r *SQLRepository) FinishReminderNotification(ctx context.Context, reminderID, status string, notifiedAt *time.Time) error {
	var delivered any
	if notifiedAt != nil {
		delivered = notifiedAt.UTC()
	}
	_, err := r.db.ExecContext(ctx, r.query("UPDATE calendar_reminders SET notify_status = ?, notified_at = ?, updated_at = ? WHERE id = ?"), status, delivered, time.Now().UTC(), reminderID)
	return err
}
