package calendar

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// newCalendarQueryTestDB 建出 SearchRecords / AggregateRecords / GetRecord
// 及其 JOIN 依赖所需的最小表结构。表不含外键约束，与既有测试一致。
func newCalendarQueryTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	schema := `
CREATE TABLE calendar_records (
  id TEXT PRIMARY KEY, family_id TEXT NOT NULL, pet_id TEXT NOT NULL,
  category TEXT NOT NULL, medical_type TEXT, custom_medical_type TEXT NOT NULL DEFAULT '',
  content TEXT NOT NULL DEFAULT '', occurred_at TIMESTAMP NOT NULL,
  created_by TEXT NOT NULL DEFAULT '', deleted_at TIMESTAMP
);
CREATE TABLE pets (
  id TEXT PRIMARY KEY, family_id TEXT NOT NULL, name TEXT NOT NULL,
  avatar_asset_id TEXT, created_by TEXT NOT NULL, updated_by TEXT NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  deleted_at TIMESTAMP
);
CREATE TABLE users (id TEXT PRIMARY KEY, nickname TEXT, avatar_asset_id TEXT);
CREATE TABLE calendar_record_media (
  id TEXT PRIMARY KEY, record_id TEXT NOT NULL, family_id TEXT NOT NULL,
  asset_id TEXT NOT NULL, sort_order INTEGER NOT NULL
);
CREATE TABLE calendar_reminders (
  id TEXT PRIMARY KEY, family_id TEXT NOT NULL, pet_id TEXT NOT NULL,
  source_record_id TEXT NOT NULL, reminder_date TEXT, repeat_type TEXT,
  repeat_interval_days INTEGER, advance_days INTEGER, notification_channels TEXT,
  status TEXT, created_at TIMESTAMP, created_by TEXT
);
`
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func insertRecordForTest(t *testing.T, db *sql.DB, id, family, pet, category, medicalType, content string, at time.Time) {
	t.Helper()
	if _, err := db.Exec("INSERT INTO calendar_records (id, family_id, pet_id, category, medical_type, custom_medical_type, content, occurred_at, created_by) VALUES (?, ?, ?, ?, ?, '', ?, ?, 'user-1')", id, family, pet, category, medicalType, content, at); err != nil {
		t.Fatal(err)
	}
}

func TestSearchRecordsOrdersAndFilters(t *testing.T) {
	db := newCalendarQueryTestDB(t)
	base := time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC)
	insertRecordForTest(t, db, "r-old", "family-1", "pet-1", "medical", "vaccine", "打疫苗", base.Add(-4*time.Hour))
	insertRecordForTest(t, db, "r-mid", "family-1", "pet-1", "medical", "deworming", "驱虫", base.Add(-3*time.Hour))
	insertRecordForTest(t, db, "r-new", "family-1", "pet-1", "daily", "", "洗澡", base.Add(-2*time.Hour))
	insertRecordForTest(t, db, "r-other-pet", "family-1", "pet-2", "medical", "vaccine", "其他宠物", base.Add(-1*time.Hour))
	insertRecordForTest(t, db, "r-other-family", "family-2", "pet-1", "medical", "vaccine", "其他家庭", base)
	// 已删除记录不应出现。
	if _, err := db.Exec("INSERT INTO calendar_records (id, family_id, pet_id, category, medical_type, custom_medical_type, content, occurred_at, deleted_at) VALUES ('r-deleted', 'family-1', 'pet-1', 'medical', 'vaccine', '', '已删除', ?, ?)", base.Add(-30*time.Minute), base); err != nil {
		t.Fatal(err)
	}
	repository, err := NewRepository(db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}

	// 家庭 + 宠物范围过滤，时间倒序。
	page, err := repository.SearchRecords(context.Background(), "family-1", "pet-1", RecordSearchQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Records) != 3 {
		t.Fatalf("records = %+v", page.Records)
	}
	if page.Records[0].ID != "r-new" || page.Records[1].ID != "r-mid" || page.Records[2].ID != "r-old" {
		t.Fatalf("order = %+v", page.Records)
	}
	if page.HasMore {
		t.Fatalf("unexpected has_more")
	}

	// 医疗类型过滤。
	medical, err := repository.SearchRecords(context.Background(), "family-1", "pet-1", RecordSearchQuery{Category: "medical"})
	if err != nil {
		t.Fatal(err)
	}
	if len(medical.Records) != 2 || medical.Records[0].ID != "r-mid" || medical.Records[1].ID != "r-old" {
		t.Fatalf("medical records = %+v", medical.Records)
	}

	// 时间区间：起点包含、终点不包含。
	ranged, err := repository.SearchRecords(context.Background(), "family-1", "pet-1", RecordSearchQuery{
		StartAt: base.Add(-3 * time.Hour), // 含 r-mid
		EndAt:   base.Add(-2 * time.Hour), // 不含 r-new
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(ranged.Records) != 1 || ranged.Records[0].ID != "r-mid" {
		t.Fatalf("ranged records = %+v", ranged.Records)
	}
}

func TestSearchRecordsPaginatesAndCursorContinues(t *testing.T) {
	db := newCalendarQueryTestDB(t)
	base := time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC)
	// 按 occurred_at 从旧到新插入 5 条，期望倒序返回。
	for i := 0; i < 5; i++ {
		id := string(rune('a' + i))
		insertRecordForTest(t, db, "r-"+id, "family-1", "pet-1", "medical", "", "内容"+id, base.Add(time.Duration(i-5)*time.Hour))
	}
	repository, err := NewRepository(db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}

	seen := make([]string, 0, 5)
	cursor := ""
	for page := 0; ; page++ {
		result, err := repository.SearchRecords(context.Background(), "family-1", "pet-1", RecordSearchQuery{Limit: 2, Cursor: cursor})
		if err != nil {
			t.Fatal(err)
		}
		for _, record := range result.Records {
			seen = append(seen, record.ID)
		}
		if !result.HasMore {
			break
		}
		if result.NextCursor == "" {
			t.Fatalf("has_more but empty cursor")
		}
		cursor = result.NextCursor
		if page > 5 {
			t.Fatalf("pagination did not terminate")
		}
	}

	if len(seen) != 5 {
		t.Fatalf("seen = %v", seen)
	}
	// 倒序：新在前。
	want := []string{"r-e", "r-d", "r-c", "r-b", "r-a"}
	for i := range want {
		if seen[i] != want[i] {
			t.Fatalf("seen[%d] = %s, want %s (all %v)", i, seen[i], want[i], seen)
		}
	}
}

func TestSearchRecordsCursorTiebreakByID(t *testing.T) {
	db := newCalendarQueryTestDB(t)
	base := time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC)
	// 相同 occurred_at，靠 id DESC 排序。
	insertRecordForTest(t, db, "r-1", "family-1", "pet-1", "medical", "", "一", base)
	insertRecordForTest(t, db, "r-2", "family-1", "pet-1", "medical", "", "二", base)
	insertRecordForTest(t, db, "r-3", "family-1", "pet-1", "medical", "", "三", base)
	repository, err := NewRepository(db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}

	page1, err := repository.SearchRecords(context.Background(), "family-1", "pet-1", RecordSearchQuery{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(page1.Records) != 2 || page1.Records[0].ID != "r-3" || page1.Records[1].ID != "r-2" || !page1.HasMore {
		t.Fatalf("page1 = %+v", page1)
	}
	page2, err := repository.SearchRecords(context.Background(), "family-1", "pet-1", RecordSearchQuery{Limit: 2, Cursor: page1.NextCursor})
	if err != nil {
		t.Fatal(err)
	}
	if len(page2.Records) != 1 || page2.Records[0].ID != "r-1" || page2.HasMore {
		t.Fatalf("page2 = %+v", page2)
	}
}

func TestAggregateRecordsSeparatesCountAndOccurrence(t *testing.T) {
	db := newCalendarQueryTestDB(t)
	base := time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC)
	insertRecordForTest(t, db, "r-1", "family-1", "pet-1", "medical", "vomit", "吐了3次", base.Add(-3*time.Hour))
	insertRecordForTest(t, db, "r-2", "family-1", "pet-1", "medical", "vomit", "早上吐了一次", base.Add(-2*time.Hour))
	insertRecordForTest(t, db, "r-3", "family-1", "pet-1", "medical", "vomit", "今天又吐了，没数", base.Add(-1*time.Hour))
	repository, err := NewRepository(db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}

	aggregate, err := repository.AggregateRecords(context.Background(), "family-1", "pet-1", RecordAggregateQuery{MedicalType: "vomit"})
	if err != nil {
		t.Fatal(err)
	}
	if aggregate.RecordCount != 3 {
		t.Fatalf("record_count = %d", aggregate.RecordCount)
	}
	if aggregate.OccurrenceCount != 4 { // 3 + 1
		t.Fatalf("occurrence_count = %d", aggregate.OccurrenceCount)
	}
	if aggregate.OccurrenceKnown {
		t.Fatalf("occurrence should be partially unknown")
	}
	if aggregate.UnknownRecords != 1 {
		t.Fatalf("unknown_records = %d", aggregate.UnknownRecords)
	}
	if aggregate.LatestAt.IsZero() || aggregate.CoveredStartAt.IsZero() || aggregate.CoveredEndAt.IsZero() {
		t.Fatalf("aggregate = %+v", aggregate)
	}
}

func TestAggregateRecordsAllKnownOccurrence(t *testing.T) {
	db := newCalendarQueryTestDB(t)
	base := time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC)
	insertRecordForTest(t, db, "r-1", "family-1", "pet-1", "daily", "", "一天两次", base.Add(-2*time.Hour))
	insertRecordForTest(t, db, "r-2", "family-1", "pet-1", "daily", "", "一天三次", base.Add(-1*time.Hour))
	repository, err := NewRepository(db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	aggregate, err := repository.AggregateRecords(context.Background(), "family-1", "pet-1", RecordAggregateQuery{Category: "daily"})
	if err != nil {
		t.Fatal(err)
	}
	if aggregate.RecordCount != 2 || aggregate.OccurrenceCount != 5 || !aggregate.OccurrenceKnown || aggregate.UnknownRecords != 0 {
		t.Fatalf("aggregate = %+v", aggregate)
	}
}

func TestGetRecordReturnsErrNoRowsWhenMissing(t *testing.T) {
	db := newCalendarQueryTestDB(t)
	if _, err := db.Exec("INSERT INTO pets (id, family_id, name, created_by, updated_by) VALUES ('pet-1', 'family-1', '旺仔', 'user-1', 'user-1')"); err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC)
	insertRecordForTest(t, db, "r-1", "family-1", "pet-1", "medical", "vaccine", "疫苗", base)
	repository, err := NewRepository(db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := repository.GetRecord(context.Background(), "family-1", "r-missing"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing err = %v", err)
	}
	// 跨家庭不可见：JOIN 条件包含 family_id，返回无行。
	if _, err := repository.GetRecord(context.Background(), "family-2", "r-1"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("cross-family err = %v", err)
	}
}

func TestGetRecordReturnsRecordWithMedia(t *testing.T) {
	db := newCalendarQueryTestDB(t)
	if _, err := db.Exec("INSERT INTO pets (id, family_id, name, avatar_asset_id, created_by, updated_by) VALUES ('pet-1', 'family-1', '旺仔', 'avatar-1', 'user-1', 'user-1')"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO users (id, nickname, avatar_asset_id) VALUES ('user-1', '主人', 'u-avatar')"); err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC)
	insertRecordForTest(t, db, "r-1", "family-1", "pet-1", "medical", "vaccine", "打疫苗", base)
	if _, err := db.Exec("INSERT INTO calendar_record_media (id, record_id, family_id, asset_id, sort_order) VALUES ('media-1', 'r-1', 'family-1', 'asset-1', 1)"); err != nil {
		t.Fatal(err)
	}
	repository, err := NewRepository(db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}

	record, err := repository.GetRecord(context.Background(), "family-1", "r-1")
	if err != nil {
		t.Fatal(err)
	}
	if record.ID != "r-1" || record.Category != "medical" || record.MedicalType != "vaccine" {
		t.Fatalf("record = %+v", record)
	}
	if len(record.Media) != 1 || record.Media[0].AssetID != "asset-1" {
		t.Fatalf("media = %+v", record.Media)
	}
}

func TestParseOccurrence(t *testing.T) {
	cases := []struct {
		content string
		count   int
		ok      bool
	}{
		{"吐了3次", 3, true},
		{"吐了123次", 123, true},
		{"一次", 1, true},
		{"两次", 2, true},
		{"十次", 10, true},
		{"打疫苗", 0, false},
		{"次", 0, false},
		{"吐了 次", 0, false},
		{"多次", 0, false},
	}
	for _, c := range cases {
		count, ok := parseOccurrence(c.content)
		if count != c.count || ok != c.ok {
			t.Fatalf("parseOccurrence(%q) = (%d, %v), want (%d, %v)", c.content, count, ok, c.count, c.ok)
		}
	}
}

func TestRecordCursorRoundTripAndInvalid(t *testing.T) {
	at := time.Date(2026, 9, 8, 10, 30, 0, 0, time.UTC)
	encoded := encodeRecordCursor(at, "r-abc")
	decodedAt, decodedID, err := parseRecordCursor(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if !decodedAt.Equal(at) || decodedID != "r-abc" {
		t.Fatalf("round trip = (%v, %s)", decodedAt, decodedID)
	}

	for _, invalid := range []string{"", "no-separator", "|tail", "bad|", "abc|id", "|"} {
		if _, _, err := parseRecordCursor(invalid); err == nil {
			t.Fatalf("expected error for cursor %q", invalid)
		}
	}
}
