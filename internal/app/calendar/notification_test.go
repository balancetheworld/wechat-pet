package calendar

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"
)

type recordingNotifier struct {
	notices []ReminderNotice
	err     error
}

func (n *recordingNotifier) NotifyReminder(_ context.Context, notice ReminderNotice) error {
	n.notices = append(n.notices, notice)
	return n.err
}

func newNotificationTestService(t *testing.T, notifier ReminderNotifier) (*Service, *sql.DB) {
	t.Helper()
	db := newCalendarQueryTestDB(t)
	repository, err := NewRepository(db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewService(repository)
	if err != nil {
		t.Fatal(err)
	}
	service.SetReminderPush(notifier, "template-1")
	return service, db
}

func seedPushReminder(t *testing.T, db *sql.DB, reminderID, userID, reminderDate string, advanceDays int) {
	t.Helper()
	if _, err := db.Exec("INSERT INTO users (id, openid, nickname) VALUES (?, ?, '成员')", userID, "openid-"+userID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO pets (id, family_id, name, created_by, updated_by) VALUES ('pet-1', 'family-1', '团子', ?, ?)", userID, userID); err != nil {
		t.Fatal(err)
	}
	insertRecordForTest(t, db, "record-1", "family-1", "pet-1", "medical", "vaccine", "接种狂犬疫苗", time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC))
	if _, err := db.Exec("INSERT INTO calendar_reminders (id, family_id, pet_id, source_record_id, reminder_date, repeat_type, advance_days, notification_channels, status, created_by, notify_status, notify_attempts) VALUES (?, 'family-1', 'pet-1', 'record-1', ?, 'once', ?, '[\"in_app\",\"push\"]', 'pending', ?, 'pending', 0)", reminderID, reminderDate, advanceDays, userID); err != nil {
		t.Fatal(err)
	}
}

func reminderNotificationState(t *testing.T, db *sql.DB, reminderID string) (string, int, sql.NullTime) {
	t.Helper()
	var status string
	var attempts int
	var notifiedAt sql.NullTime
	if err := db.QueryRow("SELECT notify_status, notify_attempts, notified_at FROM calendar_reminders WHERE id = ?", reminderID).Scan(&status, &attempts, &notifiedAt); err != nil {
		t.Fatal(err)
	}
	return status, attempts, notifiedAt
}

func TestRecordReminderSubscriptionCountsAcceptedGrants(t *testing.T) {
	service, db := newNotificationTestService(t, &recordingNotifier{})
	if _, err := db.Exec("INSERT INTO users (id, openid, nickname) VALUES ('user-1', 'openid-1', '成员')"); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if remaining, err := service.RecordReminderSubscription(ctx, "family-1", "user-1", true); err != nil || remaining != 1 {
		t.Fatalf("remaining = %d, error = %v, want 1", remaining, err)
	}
	if remaining, err := service.RecordReminderSubscription(ctx, "family-1", "user-1", true); err != nil || remaining != 2 {
		t.Fatalf("remaining = %d, error = %v, want 2", remaining, err)
	}
	if remaining, err := service.RecordReminderSubscription(ctx, "family-1", "user-1", false); err != nil || remaining != 2 {
		t.Fatalf("remaining = %d, error = %v, want unchanged 2", remaining, err)
	}
}

func TestDispatchReminderNotificationSendsAndConsumesGrant(t *testing.T) {
	notifier := &recordingNotifier{}
	service, db := newNotificationTestService(t, notifier)
	seedPushReminder(t, db, "reminder-1", "user-1", "2026-10-22", 3)
	ctx := context.Background()
	if _, err := service.RecordReminderSubscription(ctx, "family-1", "user-1", true); err != nil {
		t.Fatal(err)
	}
	result, err := service.DispatchReminderNotifications(ctx, time.Date(2026, 10, 19, 10, 0, 0, 0, reminderTimezone))
	if err != nil {
		t.Fatal(err)
	}
	if result.Sent != 1 || result.Skipped != 0 || result.Failed != 0 {
		t.Fatalf("result = %+v, want one sent notification", result)
	}
	if len(notifier.notices) != 1 || notifier.notices[0].OpenID != "openid-user-1" || notifier.notices[0].Content != "接种狂犬疫苗" {
		t.Fatalf("notices = %+v", notifier.notices)
	}
	status, attempts, notifiedAt := reminderNotificationState(t, db, "reminder-1")
	if status != reminderNotifySent || attempts != 1 || !notifiedAt.Valid {
		t.Fatalf("reminder state = %s/%d/%v, want sent/1/notified", status, attempts, notifiedAt.Valid)
	}
	remaining, err := service.repository.SubscriptionRemaining(ctx, "user-1", "template-1")
	if err != nil || remaining != 0 {
		t.Fatalf("remaining = %d, error = %v, want consumed", remaining, err)
	}
}

func TestDispatchReminderNotificationWaitsForSendHour(t *testing.T) {
	notifier := &recordingNotifier{}
	service, db := newNotificationTestService(t, notifier)
	seedPushReminder(t, db, "reminder-1", "user-1", "2026-10-22", 3)
	result, err := service.DispatchReminderNotifications(context.Background(), time.Date(2026, 10, 19, 8, 30, 0, 0, reminderTimezone))
	if err != nil {
		t.Fatal(err)
	}
	if result.Scanned != 0 || len(notifier.notices) != 0 {
		t.Fatalf("result = %+v, notices = %+v, want no dispatch before send hour", result, notifier.notices)
	}
}

func TestDispatchReminderNotificationSkipsWithoutGrant(t *testing.T) {
	notifier := &recordingNotifier{}
	service, db := newNotificationTestService(t, notifier)
	seedPushReminder(t, db, "reminder-1", "user-1", "2026-10-22", 3)
	result, err := service.DispatchReminderNotifications(context.Background(), time.Date(2026, 10, 19, 10, 0, 0, 0, reminderTimezone))
	if err != nil {
		t.Fatal(err)
	}
	if result.Skipped != 1 || len(notifier.notices) != 0 {
		t.Fatalf("result = %+v, notices = %+v, want skipped without grant", result, notifier.notices)
	}
	status, _, _ := reminderNotificationState(t, db, "reminder-1")
	if status != reminderNotifySkipped {
		t.Fatalf("status = %s, want skipped", status)
	}
}

func TestDispatchReminderNotificationSkipsUndeliverable(t *testing.T) {
	notifier := &recordingNotifier{err: ErrReminderNotDeliverable}
	service, db := newNotificationTestService(t, notifier)
	seedPushReminder(t, db, "reminder-1", "user-1", "2026-10-22", 3)
	ctx := context.Background()
	if _, err := service.RecordReminderSubscription(ctx, "family-1", "user-1", true); err != nil {
		t.Fatal(err)
	}
	result, err := service.DispatchReminderNotifications(ctx, time.Date(2026, 10, 19, 10, 0, 0, 0, reminderTimezone))
	if err != nil {
		t.Fatal(err)
	}
	if result.Skipped != 1 {
		t.Fatalf("result = %+v, want skipped", result)
	}
	status, _, _ := reminderNotificationState(t, db, "reminder-1")
	if status != reminderNotifySkipped {
		t.Fatalf("status = %s, want skipped", status)
	}
}

func TestDispatchReminderNotificationRetriesUntilAttemptLimit(t *testing.T) {
	notifier := &recordingNotifier{err: errors.New("wechat unavailable")}
	service, db := newNotificationTestService(t, notifier)
	seedPushReminder(t, db, "reminder-1", "user-1", "2026-10-22", 3)
	ctx := context.Background()
	if _, err := service.RecordReminderSubscription(ctx, "family-1", "user-1", true); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 19, 10, 0, 0, 0, reminderTimezone)
	for attempt := 1; attempt <= reminderNotifyMaxAttempts; attempt++ {
		result, err := service.DispatchReminderNotifications(ctx, now)
		if err != nil {
			t.Fatal(err)
		}
		status, attempts, _ := reminderNotificationState(t, db, "reminder-1")
		if attempts != attempt {
			t.Fatalf("attempts = %d, want %d", attempts, attempt)
		}
		if attempt < reminderNotifyMaxAttempts {
			if status != reminderNotifyPending || result.Retried != 1 {
				t.Fatalf("attempt %d status = %s, result = %+v, want retry", attempt, status, result)
			}
			continue
		}
		if status != reminderNotifyFailed || result.Failed != 1 {
			t.Fatalf("attempt %d status = %s, result = %+v, want failed", attempt, status, result)
		}
	}
	remaining, err := service.repository.SubscriptionRemaining(ctx, "user-1", "template-1")
	if err != nil || remaining != 1 {
		t.Fatalf("remaining = %d, error = %v, want unconsumed after failures", remaining, err)
	}
}

func TestDispatchReminderNotificationReclaimsStaleClaim(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 19, 10, 0, 0, 0, reminderTimezone)
	for name, claimedAt := range map[string]time.Time{
		"过期占用可重发": now.UTC().Add(-10 * time.Minute),
		"未过期占不重发": now.UTC(),
	} {
		notifier := &recordingNotifier{}
		service, db := newNotificationTestService(t, notifier)
		seedPushReminder(t, db, "reminder-1", "user-1", "2026-10-22", 3)
		if _, err := service.RecordReminderSubscription(ctx, "family-1", "user-1", true); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec("UPDATE calendar_reminders SET notify_status = 'sending', updated_at = ? WHERE id = 'reminder-1'", claimedAt); err != nil {
			t.Fatal(err)
		}
		result, err := service.DispatchReminderNotifications(ctx, now)
		if err != nil {
			t.Fatal(err)
		}
		wantSent := 0
		if claimedAt.Before(now.UTC().Add(-reminderNotifyStaleAfter)) {
			wantSent = 1
		}
		if result.Sent != wantSent {
			t.Fatalf("%s: result = %+v, want sent %d", name, result, wantSent)
		}
	}
}

func TestDispatchReminderNotificationFinalizesExhaustedStaleClaim(t *testing.T) {
	notifier := &recordingNotifier{}
	service, db := newNotificationTestService(t, notifier)
	seedPushReminder(t, db, "reminder-1", "user-1", "2026-10-22", 3)
	ctx := context.Background()
	if _, err := service.RecordReminderSubscription(ctx, "family-1", "user-1", true); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 19, 10, 0, 0, 0, reminderTimezone)
	if _, err := db.Exec("UPDATE calendar_reminders SET notify_status = 'sending', notify_attempts = ?, updated_at = ? WHERE id = 'reminder-1'", reminderNotifyMaxAttempts, now.UTC().Add(-10*time.Minute)); err != nil {
		t.Fatal(err)
	}
	result, err := service.DispatchReminderNotifications(ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	if result.Failed != 1 || len(notifier.notices) != 0 {
		t.Fatalf("result = %+v, notices = %+v, want failed without resending", result, notifier.notices)
	}
	status, _, _ := reminderNotificationState(t, db, "reminder-1")
	if status != reminderNotifyFailed {
		t.Fatalf("status = %s, want failed", status)
	}
}

func TestDispatchReminderNotificationUsesConfiguredSendHour(t *testing.T) {
	notifier := &recordingNotifier{}
	service, db := newNotificationTestService(t, notifier)
	seedPushReminder(t, db, "reminder-1", "user-1", "2026-10-22", 3)
	ctx := context.Background()
	if _, err := service.RecordReminderSubscription(ctx, "family-1", "user-1", true); err != nil {
		t.Fatal(err)
	}
	service.SetReminderSendHour(6)
	result, err := service.DispatchReminderNotifications(ctx, time.Date(2026, 10, 19, 6, 30, 0, 0, reminderTimezone))
	if err != nil {
		t.Fatal(err)
	}
	if result.Sent != 1 {
		t.Fatalf("result = %+v, want send at configured hour", result)
	}
	service.SetReminderSendHour(30)
	if service.reminderSendHour != 6 {
		t.Fatalf("send hour = %d, want invalid hour ignored", service.reminderSendHour)
	}
}

func TestValidateReminderRejectsExcessiveAdvanceDays(t *testing.T) {
	advance := MaxReminderAdvanceDays
	request := CreateReminderRequest{ReminderDate: "2026-10-22", RepeatType: "once", AdvanceDays: &advance}
	if err := validateReminder(&request); err != nil {
		t.Fatalf("advance days at limit should be accepted: %v", err)
	}
	advance = MaxReminderAdvanceDays + 1
	if err := validateReminder(&request); err == nil {
		t.Fatalf("advance days above %d should be rejected", MaxReminderAdvanceDays)
	}
}
