package pet

import (
	"context"
	"testing"
)

// TestGrowthEventsResolveRecorderNickname 成长足迹的 recorder 存用户 ID,
// 读取时要换成昵称, 否则前端「添加人」会显示一串数字。
func TestGrowthEventsResolveRecorderNickname(t *testing.T) {
	repository, db := newTestRepository(t)
	if _, err := db.Exec(`INSERT INTO users (id, nickname) VALUES ('user-1', '平衡世界的boy')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO pets (id, family_id, name, created_by, updated_by) VALUES ('pet-1', 'family-1', '啾啾', 'user-1', 'user-1')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO pet_growth_events (id, pet_id, family_id, type, occurred_at, recorder, content) VALUES ('event-1', 'pet-1', 'family-1', '日常', '2026-10-04', 'user-1', '学会握手')`); err != nil {
		t.Fatal(err)
	}

	value, err := repository.Resource(context.Background(), "family-1", "pet-1", "growth-events", "GET", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	events, ok := value.([]map[string]any)
	if !ok || len(events) != 1 {
		t.Fatalf("events = %+v, want one growth event", value)
	}
	if events[0]["recorder"] != "平衡世界的boy" {
		t.Fatalf("recorder = %v, want nickname", events[0]["recorder"])
	}
	if events[0]["content"] != "学会握手" || events[0]["type"] != "日常" {
		t.Fatalf("event = %+v", events[0])
	}
}

// TestGrowthEventsKeepUnknownRecorder 查不到对应用户时保留原值, 不改动历史文本。
func TestGrowthEventsKeepUnknownRecorder(t *testing.T) {
	repository, db := newTestRepository(t)
	if _, err := db.Exec(`INSERT INTO pets (id, family_id, name, created_by, updated_by) VALUES ('pet-1', 'family-1', '啾啾', 'user-1', 'user-1')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO pet_growth_events (id, pet_id, family_id, type, occurred_at, recorder, content) VALUES ('event-1', 'pet-1', 'family-1', '日常', '2026-10-04', '妈妈', '学会握手')`); err != nil {
		t.Fatal(err)
	}

	value, err := repository.Resource(context.Background(), "family-1", "pet-1", "growth-events", "GET", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	events := value.([]map[string]any)
	if len(events) != 1 || events[0]["recorder"] != "妈妈" {
		t.Fatalf("recorder = %v, want 妈妈", events[0]["recorder"])
	}
}
