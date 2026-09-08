package ask

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	calendarapp "github.com/balancetheworld/wechat-pet/internal/app/calendar"
)

type factTestReader struct {
	records map[string]calendarapp.FactRecord
	err     error
}

func (r factTestReader) FindLatestFact(_ context.Context, _, petID, factType string) (calendarapp.FactRecord, error) {
	if r.err != nil {
		return calendarapp.FactRecord{}, r.err
	}
	record, ok := r.records[petID+":"+factType]
	if !ok {
		return calendarapp.FactRecord{}, sql.ErrNoRows
	}
	return record, nil
}

func TestDetectFactType(t *testing.T) {
	tests := []struct {
		input string
		want  FactType
	}{
		{input: "旺仔上次洗澡是什么时候", want: FactBath},
		{input: "球球最近打过疫苗吗", want: FactVaccine},
		{input: "豆豆上次驱虫是哪天", want: FactDeworming},
		{input: "最近检查呕吐怎么办", want: FactUnknown},
		{input: "旺仔上次做检查是什么时候", want: FactCheckup},
		{input: "最近咳嗽怎么办", want: FactUnknown},
	}
	for _, test := range tests {
		if got := DetectFactType(test.input); got != test.want {
			t.Errorf("DetectFactType(%q) = %q, want %q", test.input, got, test.want)
		}
	}
}

func TestBuildFactResultKeepsPetOrderAndMissingRecords(t *testing.T) {
	at := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	reader := factTestReader{records: map[string]calendarapp.FactRecord{
		"pet-1:bath": {ID: "record-1", Content: "旺仔洗澡", OccurredAt: at},
	}}
	pets := []SessionPet{{PetID: "pet-1", PetName: "旺仔", SortOrder: 0}, {PetID: "pet-2", PetName: "球球", SortOrder: 1}}
	result, err := buildFactResult(context.Background(), reader, "family-1", pets, FactBath)
	if err != nil {
		t.Fatal(err)
	}
	items, ok := result["items"].([]FactItem)
	if !ok || len(items) != 2 {
		t.Fatalf("items = %#v", result["items"])
	}
	if items[0].PetID != "pet-1" || !items[0].Found || items[0].OccurredAt != at.Format(time.RFC3339) {
		t.Fatalf("first item = %+v", items[0])
	}
	if items[1].PetID != "pet-2" || items[1].Found || items[1].OccurredAt != "" {
		t.Fatalf("second item = %+v", items[1])
	}
}

func TestBuildFactResultReturnsQueryError(t *testing.T) {
	want := errors.New("calendar unavailable")
	_, err := buildFactResult(context.Background(), factTestReader{err: want}, "family-1", []SessionPet{{PetID: "pet-1"}}, FactBath)
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
}
