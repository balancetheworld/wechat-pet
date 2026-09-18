package ask

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	calendarapp "github.com/balancetheworld/wechat-pet/internal/app/calendar"
	petapp "github.com/balancetheworld/wechat-pet/internal/app/pet"

	_ "github.com/mattn/go-sqlite3"
)

// newBusinessReadTestDB 建出业务读取工具（ResolvePet / ReadPetProfile /
// SearchHealthRecords / AggregateHealthRecords / ReadHealthRecord）及其
// JOIN 依赖所需的最小表结构。pet / calendar / ask 三个数据层共享同一连接。
func newBusinessReadTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	schema := `
CREATE TABLE pets (
  id TEXT PRIMARY KEY, family_id TEXT NOT NULL, name TEXT NOT NULL,
  avatar_asset_id TEXT, cover_asset_id TEXT, breed TEXT, gender TEXT,
  sterilized BOOLEAN NOT NULL DEFAULT FALSE, birthday TEXT, home_date TEXT,
  created_by TEXT NOT NULL, updated_by TEXT NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  deleted_at TIMESTAMP
);
CREATE TABLE pet_health (
  pet_id TEXT PRIMARY KEY, family_id TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT '', allergies TEXT NOT NULL DEFAULT '',
  long_term_medication TEXT NOT NULL DEFAULT '',
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE calendar_records (
  id TEXT PRIMARY KEY, family_id TEXT NOT NULL, pet_id TEXT NOT NULL,
  category TEXT NOT NULL, medical_type TEXT, custom_medical_type TEXT NOT NULL DEFAULT '',
  content TEXT NOT NULL DEFAULT '', occurred_at TIMESTAMP NOT NULL,
  created_by TEXT NOT NULL DEFAULT '', deleted_at TIMESTAMP
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
CREATE TABLE ask_source_versions (
  source_type TEXT NOT NULL, source_id TEXT NOT NULL, version TEXT NOT NULL,
  versioned_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (source_type, source_id)
);
`
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func newBusinessReadRepo(t *testing.T, db *sql.DB) BusinessReadRepository {
	t.Helper()
	pets, err := petapp.NewRepository(db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	calendar, err := calendarapp.NewRepository(db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	source, err := NewRepository(db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	return NewBusinessReadRepository(pets, calendar, source)
}

func insertPetForTest(t *testing.T, db *sql.DB, id, family, name string) {
	t.Helper()
	if _, err := db.Exec("INSERT INTO pets (id, family_id, name, created_by, updated_by) VALUES (?, ?, ?, 'user-1', 'user-1')", id, family, name); err != nil {
		t.Fatal(err)
	}
}

func TestResolvePetScopesToFamily(t *testing.T) {
	db := newBusinessReadTestDB(t)
	insertPetForTest(t, db, "pet-1", "family-1", "旺仔")
	insertPetForTest(t, db, "pet-2", "family-1", "球球")
	insertPetForTest(t, db, "pet-3", "family-2", "旺仔") // 其他家庭同名，不得泄露
	repository := newBusinessReadRepo(t, db)

	outcome, err := repository.ResolvePet(context.Background(), "family-1", "旺仔今天怎么样")
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Status != PetResolveResolved {
		t.Fatalf("status = %s", outcome.Status)
	}
	if len(outcome.Resolved) != 1 || outcome.Resolved[0].ID != "pet-1" {
		t.Fatalf("resolved = %+v", outcome.Resolved)
	}
	if outcome.Source.Version == "" || outcome.Source.SourceType != sourceTypePet {
		t.Fatalf("source = %+v", outcome.Source)
	}
}

func TestResolvePetAmbiguousAndNone(t *testing.T) {
	db := newBusinessReadTestDB(t)
	insertPetForTest(t, db, "pet-1", "family-1", "旺仔")
	insertPetForTest(t, db, "pet-2", "family-1", "旺仔")
	repository := newBusinessReadRepo(t, db)

	ambiguous, err := repository.ResolvePet(context.Background(), "family-1", "旺仔")
	if err != nil {
		t.Fatal(err)
	}
	if ambiguous.Status != PetResolveAmbiguous {
		t.Fatalf("status = %s", ambiguous.Status)
	}

	none, err := repository.ResolvePet(context.Background(), "family-1", "没有匹配的名字")
	if err != nil {
		t.Fatal(err)
	}
	if none.Status != PetResolveNone {
		t.Fatalf("status = %s", none.Status)
	}
}

func TestReadPetProfileReturnsProfileAndHealth(t *testing.T) {
	db := newBusinessReadTestDB(t)
	if _, err := db.Exec("INSERT INTO pets (id, family_id, name, breed, gender, sterilized, created_by, updated_by) VALUES ('pet-1', 'family-1', '旺仔', '金毛', 'male', 1, 'user-1', 'user-1')"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO pet_health (pet_id, family_id, status, allergies, long_term_medication) VALUES ('pet-1', 'family-1', '健康', '无', '无')"); err != nil {
		t.Fatal(err)
	}
	repository := newBusinessReadRepo(t, db)

	outcome, err := repository.ReadPetProfile(context.Background(), "family-1", "pet-1")
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Profile.ID != "pet-1" || outcome.Profile.Breed != "金毛" {
		t.Fatalf("profile = %+v", outcome.Profile)
	}
	if outcome.Health.Status != "健康" {
		t.Fatalf("health = %+v", outcome.Health)
	}
	if outcome.Source.Version == "" {
		t.Fatalf("source version empty")
	}
}

func TestReadPetProfileMissingPetReturnsError(t *testing.T) {
	db := newBusinessReadTestDB(t)
	repository := newBusinessReadRepo(t, db)

	if _, err := repository.ReadPetProfile(context.Background(), "family-1", "pet-missing"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("err = %v", err)
	}
}

func TestSearchHealthRecordsWrapsAndEmpty(t *testing.T) {
	db := newBusinessReadTestDB(t)
	insertPetForTest(t, db, "pet-1", "family-1", "旺仔")
	base := time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC)
	if _, err := db.Exec("INSERT INTO calendar_records (id, family_id, pet_id, category, medical_type, custom_medical_type, content, occurred_at) VALUES ('r-1', 'family-1', 'pet-1', 'medical', 'vaccine', '', '打疫苗', ?)", base); err != nil {
		t.Fatal(err)
	}
	repository := newBusinessReadRepo(t, db)

	outcome, err := repository.SearchHealthRecords(context.Background(), "family-1", "pet-1", HealthRecordSearchQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(outcome.Records) != 1 || outcome.Records[0].ID != "r-1" || outcome.Records[0].MedicalType != "vaccine" {
		t.Fatalf("records = %+v", outcome.Records)
	}
	if outcome.Source.Version == "" {
		t.Fatalf("source version empty")
	}

	// 无记录：返回空列表，error 为 nil。
	empty, err := repository.SearchHealthRecords(context.Background(), "family-1", "pet-1", HealthRecordSearchQuery{StartAt: base.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if len(empty.Records) != 0 || empty.HasMore {
		t.Fatalf("empty = %+v", empty)
	}
}

func TestAggregateHealthRecordsSeparatesCount(t *testing.T) {
	db := newBusinessReadTestDB(t)
	insertPetForTest(t, db, "pet-1", "family-1", "旺仔")
	base := time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC)
	if _, err := db.Exec("INSERT INTO calendar_records (id, family_id, pet_id, category, medical_type, custom_medical_type, content, occurred_at) VALUES ('r-1', 'family-1', 'pet-1', 'medical', 'vomit', '', '吐了3次', ?)", base); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO calendar_records (id, family_id, pet_id, category, medical_type, custom_medical_type, content, occurred_at) VALUES ('r-2', 'family-1', 'pet-1', 'medical', 'vomit', '', '没数', ?)", base.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	repository := newBusinessReadRepo(t, db)

	outcome, err := repository.AggregateHealthRecords(context.Background(), "family-1", "pet-1", HealthRecordAggregateQuery{MedicalType: "vomit"})
	if err != nil {
		t.Fatal(err)
	}
	if outcome.RecordCount != 2 || outcome.OccurrenceCount != 3 || outcome.OccurrenceKnown || outcome.UnknownRecords != 1 {
		t.Fatalf("outcome = %+v", outcome)
	}
}

func TestReadHealthRecordNotFoundAndFound(t *testing.T) {
	db := newBusinessReadTestDB(t)
	if _, err := db.Exec("INSERT INTO pets (id, family_id, name, created_by, updated_by) VALUES ('pet-1', 'family-1', '旺仔', 'user-1', 'user-1')"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO users (id, nickname, avatar_asset_id) VALUES ('user-1', '主人', 'u-avatar')"); err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC)
	if _, err := db.Exec("INSERT INTO calendar_records (id, family_id, pet_id, category, medical_type, custom_medical_type, content, occurred_at, created_by) VALUES ('r-1', 'family-1', 'pet-1', 'medical', 'vaccine', '', '打疫苗', ?, 'user-1')", base); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO calendar_record_media (id, record_id, family_id, asset_id, sort_order) VALUES ('media-1', 'r-1', 'family-1', 'asset-1', 1)"); err != nil {
		t.Fatal(err)
	}
	repository := newBusinessReadRepo(t, db)

	// 不存在 → 可区分错误。
	if _, err := repository.ReadHealthRecord(context.Background(), "family-1", "r-missing"); !errors.Is(err, ErrHealthRecordNotFound) {
		t.Fatalf("missing err = %v", err)
	}
	// 跨家庭 → 同样视为不存在。
	if _, err := repository.ReadHealthRecord(context.Background(), "family-2", "r-1"); !errors.Is(err, ErrHealthRecordNotFound) {
		t.Fatalf("cross-family err = %v", err)
	}

	outcome, err := repository.ReadHealthRecord(context.Background(), "family-1", "r-1")
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Record.ID != "r-1" || outcome.Record.MedicalType != "vaccine" {
		t.Fatalf("record = %+v", outcome.Record)
	}
	if len(outcome.Record.MediaAssetIDs) != 1 || outcome.Record.MediaAssetIDs[0] != "asset-1" {
		t.Fatalf("media = %+v", outcome.Record.MediaAssetIDs)
	}
	if outcome.Source.Version == "" || outcome.Source.SourceType != sourceTypeRecord {
		t.Fatalf("source = %+v", outcome.Source)
	}
}

func TestHashSourceVersionStableAndSensitive(t *testing.T) {
	first := hashSourceVersion("pet-1", "2026-09-08T10:00:00Z")
	second := hashSourceVersion("pet-1", "2026-09-08T10:00:00Z")
	if first == "" || first != second {
		t.Fatalf("version not stable: %q vs %q", first, second)
	}
	changed := hashSourceVersion("pet-1", "2026-09-08T11:00:00Z")
	if changed == first {
		t.Fatalf("version should change with input")
	}
	reordered := hashSourceVersion("2026-09-08T10:00:00Z", "pet-1")
	if reordered == first {
		t.Fatalf("version should change with order")
	}
}
