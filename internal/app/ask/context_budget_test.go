package ask

import (
	"strings"
	"testing"
	"time"

	calendarapp "github.com/balancetheworld/wechat-pet/internal/app/calendar"
)

func TestCompactContextSnapshotBoundsHistory(t *testing.T) {
	turns := make([]ContextTurn, ContextTurnLimit)
	for index := range turns {
		turns[index] = ContextTurn{TurnIndex: index, Input: strings.Repeat("呕吐", 200), CreatedAt: time.Now()}
	}
	records := make([]calendarapp.ContextRecord, ContextTurnLimit)
	for index := range records {
		records[index] = calendarapp.ContextRecord{Category: "medical", Content: strings.Repeat("复查", 200), OccurredAt: time.Now()}
	}
	value := compactContextSnapshot(ContextSnapshot{RecentTurns: turns, RecentRecords: records, Sources: []ContextSource{{Name: "ask_turns"}, {Name: "calendar_records"}}}, 600)
	if value.CharCount > 600 {
		t.Fatalf("context chars = %d", value.CharCount)
	}
	if !value.Sources[0].Truncated && !value.Sources[1].Truncated {
		t.Fatalf("sources = %+v", value.Sources)
	}
}
