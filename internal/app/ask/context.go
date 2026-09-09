package ask

import (
	"encoding/json"
	"strings"
	"unicode/utf8"

	calendarapp "github.com/balancetheworld/wechat-pet/internal/app/calendar"
)

func contextAuditData(value ContextSnapshot) string {
	truncated := false
	for _, source := range value.Sources {
		if source.Truncated {
			truncated = true
			break
		}
	}
	data, err := json.Marshal(map[string]any{
		"context_version": value.Version,
		"captured_at":     value.CapturedAt,
		"source_count":    len(value.Sources),
		"event_count":     len(value.Events),
		"char_count":      value.CharCount,
		"truncated":       truncated,
	})
	if err != nil {
		return `{}`
	}
	return string(data)
}

func compactContextSnapshot(value ContextSnapshot, maxChars int) ContextSnapshot {
	value.Pet.ID = contextSummary(value.Pet.ID)
	value.Pet.Name = contextSummary(value.Pet.Name)
	value.Pet.Breed = contextSummary(value.Pet.Breed)
	value.Pet.Gender = contextSummary(value.Pet.Gender)
	value.Pet.Birthday = contextSummary(value.Pet.Birthday)
	value.Pet.HealthStatus = contextSummary(value.Pet.HealthStatus)
	value.Pet.Allergies = contextSummary(value.Pet.Allergies)
	value.Pet.LongTermMedication = contextSummary(value.Pet.LongTermMedication)
	for index := range value.RecentTurns {
		value.RecentTurns[index].Input = contextSummary(value.RecentTurns[index].Input)
	}
	for index := range value.Messages {
		value.Messages[index].Content = contextSummary(value.Messages[index].Content)
	}
	for index := range value.RecentRecords {
		value.RecentRecords[index].Content = contextSummary(value.RecentRecords[index].Content)
	}
	value.Events = normalizeContextEvents(value.RecentTurns, value.RecentRecords)
	value.CharCount = contextSnapshotChars(value)
	for value.CharCount > maxChars && len(value.RecentTurns) > 0 {
		value.RecentTurns = value.RecentTurns[1:]
		markContextSourceTruncated(&value, "ask_turns")
		value.Events = normalizeContextEvents(value.RecentTurns, value.RecentRecords)
		value.CharCount = contextSnapshotChars(value)
	}
	for value.CharCount > maxChars && len(value.RecentRecords) > 0 {
		value.RecentRecords = value.RecentRecords[:len(value.RecentRecords)-1]
		markContextSourceTruncated(&value, "calendar_records")
		value.Events = normalizeContextEvents(value.RecentTurns, value.RecentRecords)
		value.CharCount = contextSnapshotChars(value)
	}
	for value.CharCount > maxChars && len(value.Messages) > 1 {
		value.Messages = value.Messages[1:]
		markContextSourceTruncated(&value, "ask_messages")
		value.CharCount = contextSnapshotChars(value)
	}
	return value
}

func contextSnapshotChars(value ContextSnapshot) int {
	result := utf8.RuneCountInString(value.Pet.ID) + utf8.RuneCountInString(value.Pet.Name) + utf8.RuneCountInString(value.Pet.Breed) + utf8.RuneCountInString(value.Pet.Gender) + utf8.RuneCountInString(value.Pet.Birthday) + utf8.RuneCountInString(value.Pet.HealthStatus) + utf8.RuneCountInString(value.Pet.Allergies) + utf8.RuneCountInString(value.Pet.LongTermMedication)
	for _, turn := range value.RecentTurns {
		result += utf8.RuneCountInString(turn.Input)
	}
	for _, message := range value.Messages {
		result += utf8.RuneCountInString(message.Role) + utf8.RuneCountInString(message.Content)
	}
	for _, record := range value.RecentRecords {
		result += utf8.RuneCountInString(record.Content) + utf8.RuneCountInString(record.Category) + utf8.RuneCountInString(record.MedicalType)
	}
	for _, event := range value.Events {
		result += utf8.RuneCountInString(event.Tag) + utf8.RuneCountInString(event.Summary)
	}
	return result
}

func markContextSourceTruncated(value *ContextSnapshot, name string) {
	for index := range value.Sources {
		if value.Sources[index].Name == name {
			value.Sources[index].Truncated = true
		}
	}
}

func normalizeContextEvents(turns []ContextTurn, records []calendarapp.ContextRecord) []ContextEvent {
	result := make([]ContextEvent, 0, len(turns)+len(records))
	for _, turn := range turns {
		if tag := symptomTag(turn.Input); tag != "" {
			result = append(result, ContextEvent{Tag: tag, Source: "ask_turn", Summary: contextSummary(turn.Input), OccurredAt: turn.CreatedAt})
		}
	}
	for _, record := range records {
		tag := "daily_record"
		if record.Category == "medical" {
			tag = "medical_record"
		}
		result = append(result, ContextEvent{Tag: tag, Source: "calendar_record", Summary: contextSummary(record.Content), OccurredAt: record.OccurredAt})
	}
	return result
}

func symptomTag(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch {
	case strings.Contains(value, "呕吐") || strings.Contains(value, "吐了"):
		return "symptom_vomiting"
	case strings.Contains(value, "腹泻") || strings.Contains(value, "拉稀"):
		return "symptom_diarrhea"
	case strings.Contains(value, "咳嗽"):
		return "symptom_cough"
	case strings.Contains(value, "打喷嚏"):
		return "symptom_sneezing"
	case strings.Contains(value, "没精神") || strings.Contains(value, "精神不振"):
		return "symptom_low_energy"
	case strings.Contains(value, "食欲") || strings.Contains(value, "不吃"):
		return "symptom_appetite"
	case strings.Contains(value, "皮肤") || strings.Contains(value, "抓挠"):
		return "symptom_skin"
	default:
		return ""
	}
}

func contextSummary(value string) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if utf8.RuneCountInString(value) <= 200 {
		return value
	}
	runes := []rune(value)
	return string(runes[:200])
}
