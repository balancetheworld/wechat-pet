package calendar

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	appErrors "github.com/balancetheworld/wechat-pet/internal/pkg/errors"
)

var ErrReminderNotDeliverable = errors.New("reminder notification not deliverable")

const (
	reminderNotifyPending = "pending"
	reminderNotifySent    = "sent"
	reminderNotifySkipped = "skipped"
	reminderNotifyFailed  = "failed"
)

const (
	DefaultReminderSendHour   = 9
	MaxReminderAdvanceDays    = 30
	reminderNotifyMaxAttempts = 3
	reminderNotifyBatchLimit  = 100
	reminderNotifyStaleAfter  = 5 * time.Minute
)

var reminderTimezone = time.FixedZone("UTC+8", 8*60*60)

type ReminderNotice struct {
	OpenID       string
	PetName      string
	Content      string
	ReminderDate string
}

type ReminderNotifier interface {
	NotifyReminder(context.Context, ReminderNotice) error
}

type PushReminder struct {
	ReminderID     string
	UserID         string
	OpenID         string
	PetName        string
	Content        string
	ReminderDate   string
	AdvanceDays    int
	NotifyAttempts int
}

type ReminderDispatchResult struct {
	Scanned int
	Sent    int
	Skipped int
	Failed  int
	Retried int
}

type ReminderSubscriptionRequest struct {
	Accepted bool `json:"accepted"`
}

type ReminderSubscriptionDTO struct {
	Remaining int `json:"remaining"`
}

func (s *Service) SetReminderPush(notifier ReminderNotifier, templateID string) {
	s.reminderNotifier = notifier
	s.reminderTemplateID = strings.TrimSpace(templateID)
}

func (s *Service) SetReminderSendHour(hour int) {
	if hour < 0 || hour > 23 {
		return
	}
	s.reminderSendHour = hour
}

func (s *Service) RecordReminderSubscription(ctx context.Context, familyID, userID string, accepted bool) (int, error) {
	if s.reminderTemplateID == "" {
		return 0, appErrors.InvalidParam("微信提醒未配置")
	}
	remaining, err := s.repository.RecordSubscriptionGrant(ctx, userID, s.reminderTemplateID, accepted)
	if err != nil {
		return 0, mapError(err)
	}
	return remaining, nil
}

func (s *Service) DispatchReminderNotifications(ctx context.Context, now time.Time) (ReminderDispatchResult, error) {
	var result ReminderDispatchResult
	if s.reminderNotifier == nil || s.reminderTemplateID == "" {
		return result, nil
	}
	local := now.In(reminderTimezone)
	if local.Hour() < s.reminderSendHour {
		return result, nil
	}
	today := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, reminderTimezone)
	staleBefore := now.UTC().Add(-reminderNotifyStaleAfter)
	reminders, err := s.repository.ListDuePushReminders(ctx, today.AddDate(0, 0, MaxReminderAdvanceDays).Format("2006-01-02"), staleBefore, reminderNotifyBatchLimit)
	if err != nil {
		return result, mapError(err)
	}
	for _, reminder := range reminders {
		due, err := reminderDueDate(reminder)
		if err != nil || due.After(today) {
			continue
		}
		result.Scanned++
		claimed, err := s.repository.ClaimReminderNotification(ctx, reminder.ReminderID, now.UTC(), staleBefore)
		if err != nil {
			s.logReminderNotificationFailure("claim reminder notification", reminder.ReminderID, mapError(err))
			result.Failed++
			continue
		}
		if !claimed {
			continue
		}
		if reminder.NotifyAttempts >= reminderNotifyMaxAttempts {
			if err := s.repository.FinishReminderNotification(ctx, reminder.ReminderID, reminderNotifyFailed, nil); err != nil {
				s.logReminderNotificationFailure("finish reminder notification", reminder.ReminderID, mapError(err))
			}
			result.Failed++
			continue
		}
		status, deliveredAt, err := s.deliverReminderNotification(ctx, reminder, now)
		if err != nil {
			s.logReminderNotificationFailure("deliver reminder notification", reminder.ReminderID, err)
			result.Failed++
			continue
		}
		if err := s.repository.FinishReminderNotification(ctx, reminder.ReminderID, status, deliveredAt); err != nil {
			s.logReminderNotificationFailure("finish reminder notification", reminder.ReminderID, mapError(err))
			result.Failed++
			continue
		}
		switch status {
		case reminderNotifySent:
			result.Sent++
		case reminderNotifySkipped:
			result.Skipped++
		case reminderNotifyFailed:
			result.Failed++
		default:
			result.Retried++
		}
	}
	return result, nil
}

func (s *Service) deliverReminderNotification(ctx context.Context, reminder PushReminder, now time.Time) (string, *time.Time, error) {
	if strings.TrimSpace(reminder.OpenID) == "" {
		return reminderNotifySkipped, nil, nil
	}
	remaining, err := s.repository.SubscriptionRemaining(ctx, reminder.UserID, s.reminderTemplateID)
	if err != nil {
		return "", nil, mapError(err)
	}
	if remaining <= 0 {
		return reminderNotifySkipped, nil, nil
	}
	notice := ReminderNotice{OpenID: reminder.OpenID, PetName: reminder.PetName, Content: reminder.Content, ReminderDate: reminder.ReminderDate}
	if err := s.reminderNotifier.NotifyReminder(ctx, notice); err != nil {
		if errors.Is(err, ErrReminderNotDeliverable) {
			return reminderNotifySkipped, nil, nil
		}
		if reminder.NotifyAttempts+1 >= reminderNotifyMaxAttempts {
			return reminderNotifyFailed, nil, nil
		}
		return reminderNotifyPending, nil, nil
	}
	// 消息已发出，本地状态必须落定为已发送；配额扣减失败只记异常与待对账，
	// 否则该项会被判为未完成，5 分钟后重新领取并重复推送。
	if _, err := s.repository.ConsumeSubscriptionGrant(ctx, reminder.UserID, s.reminderTemplateID); err != nil {
		s.logReminderNotificationFailure("consume subscription grant", reminder.ReminderID, mapError(err))
	}
	deliveredAt := now.UTC()
	return reminderNotifySent, &deliveredAt, nil
}

// logReminderNotificationFailure 记录提醒推送的单项异常。
// 单项失败不得中断整批派发，其余到期提醒仍要在本次 tick 处理。
func (s *Service) logReminderNotificationFailure(operation, reminderID string, err error) {
	logger := s.debugLogger
	if logger == nil {
		logger = slog.Default()
	}
	logger.Error("calendar reminder notification failed", "operation", operation, "reminder_id", reminderID, "error", err)
}

func reminderDueDate(reminder PushReminder) (time.Time, error) {
	date, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(reminder.ReminderDate), reminderTimezone)
	if err != nil {
		return time.Time{}, err
	}
	return date.AddDate(0, 0, -reminder.AdvanceDays), nil
}
