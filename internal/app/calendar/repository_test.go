package calendar

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func TestListRecentRecordsScopesFamilyPetAndTime(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE calendar_records (id TEXT PRIMARY KEY, family_id TEXT NOT NULL, pet_id TEXT NOT NULL, category TEXT NOT NULL, medical_type TEXT, custom_medical_type TEXT, content TEXT NOT NULL, occurred_at TIMESTAMP NOT NULL, deleted_at TIMESTAMP)`); err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC)
	rows := []struct {
		id, family, pet, content string
		at                       time.Time
		deleted                  any
	}{
		{id: "record-1", family: "family-1", pet: "pet-1", content: "近期复查", at: base.Add(-24 * time.Hour)},
		{id: "record-2", family: "family-1", pet: "pet-2", content: "其他宠物", at: base.Add(-12 * time.Hour)},
		{id: "record-3", family: "family-2", pet: "pet-1", content: "其他家庭", at: base.Add(-6 * time.Hour)},
		{id: "record-4", family: "family-1", pet: "pet-1", content: "已删除", at: base.Add(-2 * time.Hour), deleted: base},
		{id: "record-5", family: "family-1", pet: "pet-1", content: "过早记录", at: base.Add(-100 * 24 * time.Hour)},
		{id: "record-6", family: "family-1", pet: "pet-1", content: "未来记录", at: base.Add(time.Hour)},
	}
	for _, row := range rows {
		if _, err := db.Exec("INSERT INTO calendar_records (id, family_id, pet_id, category, medical_type, custom_medical_type, content, occurred_at, deleted_at) VALUES (?, ?, ?, 'medical', '', '', ?, ?, ?)", row.id, row.family, row.pet, row.content, row.at, row.deleted); err != nil {
			t.Fatal(err)
		}
	}
	repository, err := NewRepository(db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	result, err := repository.ListRecentRecords(context.Background(), "family-1", "pet-1", base.Add(-90*24*time.Hour), base, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 || result[0].ID != "record-1" || result[0].Content != "近期复查" {
		t.Fatalf("records = %+v", result)
	}
}

func TestFindLatestFactScopesAndOrdersRecords(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE calendar_records (id TEXT PRIMARY KEY, family_id TEXT NOT NULL, pet_id TEXT NOT NULL, category TEXT NOT NULL, medical_type TEXT, custom_medical_type TEXT, content TEXT NOT NULL, occurred_at TIMESTAMP NOT NULL, deleted_at TIMESTAMP)`); err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC)
	rows := []struct {
		id, family, pet, medicalType, content string
		at                                    time.Time
		deleted                               any
	}{
		{id: "record-old", family: "family-1", pet: "pet-1", content: "旺仔洗澡", at: base.Add(-48 * time.Hour)},
		{id: "record-latest", family: "family-1", pet: "pet-1", content: "旺仔完成洗浴", at: base.Add(-24 * time.Hour)},
		{id: "record-other-pet", family: "family-1", pet: "pet-2", content: "球球洗澡", at: base},
		{id: "record-other-family", family: "family-2", pet: "pet-1", content: "其他家庭洗澡", at: base},
		{id: "record-deleted", family: "family-1", pet: "pet-1", content: "已删除洗澡", at: base, deleted: base},
		{id: "record-vaccine", family: "family-1", pet: "pet-1", medicalType: "vaccine", content: "完成疫苗", at: base.Add(-12 * time.Hour)},
		{id: "record-deworming", family: "family-1", pet: "pet-1", medicalType: "deworming", content: "完成驱虫", at: base.Add(-6 * time.Hour)},
	}
	for _, row := range rows {
		if _, err := db.Exec("INSERT INTO calendar_records (id, family_id, pet_id, category, medical_type, custom_medical_type, content, occurred_at, deleted_at) VALUES (?, ?, ?, 'medical', ?, '', ?, ?, ?)", row.id, row.family, row.pet, row.medicalType, row.content, row.at, row.deleted); err != nil {
			t.Fatal(err)
		}
	}
	repository, err := NewRepository(db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	bath, err := repository.FindLatestFact(context.Background(), "family-1", "pet-1", "bath")
	if err != nil {
		t.Fatal(err)
	}
	if bath.ID != "record-latest" || bath.Content != "旺仔完成洗浴" {
		t.Fatalf("bath = %+v", bath)
	}
	vaccine, err := repository.FindLatestFact(context.Background(), "family-1", "pet-1", "vaccine")
	if err != nil {
		t.Fatal(err)
	}
	if vaccine.ID != "record-vaccine" || vaccine.MedicalType != "vaccine" {
		t.Fatalf("vaccine = %+v", vaccine)
	}
	deworming, err := repository.FindLatestFact(context.Background(), "family-1", "pet-1", "deworming")
	if err != nil {
		t.Fatal(err)
	}
	if deworming.ID != "record-deworming" || deworming.MedicalType != "deworming" {
		t.Fatalf("deworming = %+v", deworming)
	}
}
