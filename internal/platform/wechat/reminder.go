package wechat

import (
	"context"
	"errors"
	"fmt"
	"strings"

	calendarapp "github.com/balancetheworld/wechat-pet/internal/app/calendar"
)

const (
	reminderTemplatePage        = "pages/calendar/index"
	reminderTemplatePetKeyword  = "thing1"
	reminderTemplateItemKeyword = "thing2"
	reminderTemplateDateKeyword = "date2"
	reminderKeywordMaxRunes     = 20
)

type ReminderNotifier struct {
	client     *SubscribeClient
	templateID string
}

func NewReminderNotifier(client *SubscribeClient, templateID string) (*ReminderNotifier, error) {
	if client == nil {
		return nil, errors.New("wechat subscribe client is required")
	}
	templateID = strings.TrimSpace(templateID)
	if templateID == "" {
		return nil, errors.New("wechat reminder template id is required")
	}
	return &ReminderNotifier{client: client, templateID: templateID}, nil
}

func (n *ReminderNotifier) NotifyReminder(ctx context.Context, notice calendarapp.ReminderNotice) error {
	data := map[string]string{
		reminderTemplatePetKeyword:  truncateReminderKeyword(notice.PetName),
		reminderTemplateItemKeyword: truncateReminderKeyword(notice.Content),
		reminderTemplateDateKeyword: strings.TrimSpace(notice.ReminderDate),
	}
	err := n.client.SendSubscribeMessage(ctx, notice.OpenID, n.templateID, reminderTemplatePage, data)
	var subscribeErr *SubscribeError
	if errors.As(err, &subscribeErr) && subscribeErr.Permanent() {
		return fmt.Errorf("wechat reminder not deliverable (errcode %d): %w", subscribeErr.ErrCode, calendarapp.ErrReminderNotDeliverable)
	}
	return err
}

func truncateReminderKeyword(value string) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) <= reminderKeywordMaxRunes {
		return value
	}
	return string(runes[:reminderKeywordMaxRunes])
}
