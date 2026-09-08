package ask

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	calendarapp "github.com/balancetheworld/wechat-pet/internal/app/calendar"
)

type factReader interface {
	FindLatestFact(context.Context, string, string, string) (calendarapp.FactRecord, error)
}

type FactItem struct {
	PetID      string `json:"pet_id"`
	PetName    string `json:"pet_name"`
	Found      bool   `json:"found"`
	OccurredAt string `json:"occurred_at"`
	Content    string `json:"content"`
}

func DetectFactType(input string) FactType {
	input = strings.ToLower(strings.TrimSpace(input))
	switch {
	case strings.Contains(input, "洗澡") || strings.Contains(input, "洗浴"):
		return FactBath
	case strings.Contains(input, "疫苗") || strings.Contains(input, "免疫"):
		return FactVaccine
	case strings.Contains(input, "驱虫") || strings.Contains(input, "驱蟲"):
		return FactDeworming
	case strings.Contains(input, "体检") || strings.Contains(input, "体查") || strings.Contains(input, "健康检查") || strings.Contains(input, "做检查"):
		return FactCheckup
	case strings.Contains(input, "就医") || strings.Contains(input, "看病") || strings.Contains(input, "看诊"):
		return FactVisit
	case strings.Contains(input, "用药") || strings.Contains(input, "吃药") || strings.Contains(input, "服药"):
		return FactMedication
	default:
		return FactUnknown
	}
}

func buildFactResult(ctx context.Context, reader factReader, familyID string, pets []SessionPet, factType FactType) (map[string]any, error) {
	items := make([]FactItem, 0, len(pets))
	for _, pet := range pets {
		item := FactItem{PetID: pet.PetID, PetName: pet.PetName}
		record, err := reader.FindLatestFact(ctx, familyID, pet.PetID, string(factType))
		if err == nil {
			item.Found = true
			item.OccurredAt = record.OccurredAt.Format(time.RFC3339)
			item.Content = record.Content
		} else if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		items = append(items, item)
	}
	return map[string]any{"fact_type": factType, "items": items}, nil
}
