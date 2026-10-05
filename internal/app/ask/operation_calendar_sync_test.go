package ask

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	calendarapp "github.com/balancetheworld/wechat-pet/internal/app/calendar"
)

type recordingCalendarWriter struct {
	created    []calendarapp.CreateRecordRequest
	occurredAt string
}

func (w *recordingCalendarWriter) CreateRecord(_ context.Context, _, _ string, request calendarapp.CreateRecordRequest) (calendarapp.RecordDTO, error) {
	w.created = append(w.created, request)
	occurredAt := w.occurredAt
	if occurredAt == "" {
		occurredAt = request.OccurredAt
	}
	return calendarapp.RecordDTO{ID: "record-1", OccurredAt: occurredAt}, nil
}

func (w *recordingCalendarWriter) UpdateRecord(context.Context, string, string, string, calendarapp.UpdateRecordRequest) (calendarapp.RecordDTO, error) {
	return calendarapp.RecordDTO{}, nil
}

func (w *recordingCalendarWriter) CompleteReminder(context.Context, string, string, string, calendarapp.CompleteReminderRequest) (calendarapp.CompleteReminderDTO, error) {
	return calendarapp.CompleteReminderDTO{}, nil
}

type failingPetProfileWriter struct{}

func (failingPetProfileWriter) Resource(context.Context, string, string, string, string, string, map[string]any) (any, error) {
	return nil, errors.New("profile sync unavailable")
}

// executeCalendarRecordSyncOperation 走完整的“准备 -> 确认 -> 执行”链路，返回执行结果。
// confirmTargets 为用户在确认卡片上的同步选择：nil 表示没有单独选择（沿用准备阶段的建议）。
func executeCalendarRecordSyncOperation(t *testing.T, key, arguments string, confirmTargets *[]string, petWriter petProfileWriter) (Operation, *recordingCalendarWriter, *recordingPetProfileWriter) {
	t.Helper()
	model := &scriptedModel{responses: [][]ProtocolRecord{prepareCallRecords("create_calendar_record", arguments), finalAnswerRecords()}}
	service, repository := newV2Service(t, model, &fakeBusinessRead{})
	calendar := &recordingCalendarWriter{}
	service.SetCalendarWriter(calendar)
	recorder, _ := petWriter.(*recordingPetProfileWriter)
	service.SetPetProfileWriter(petWriter)
	created, err := service.CreateSession(context.Background(), "family-1", "user-1", "pet-1", "帮我记一下并同步到档案", key)
	if err != nil {
		t.Fatal(err)
	}
	processed, err := service.ProcessRun(context.Background(), "family-1", created.Session.ID, created.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if processed.Run.Status != RunCompleted {
		t.Fatalf("run status = %s (error %s), want completed", processed.Run.Status, processed.Run.ErrorCode)
	}
	operations, err := repository.ListOperations(context.Background(), created.Session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(operations) != 1 {
		t.Fatalf("operations = %d, want 1", len(operations))
	}
	operation := operations[0]
	confirmed, err := service.ConfirmOperation(context.Background(), "family-1", "user-1", created.Session.ID, operation.ID, operation.Version, operation.Preview, confirmTargets)
	if err != nil {
		t.Fatal(err)
	}
	executed, err := service.ExecuteOperation(context.Background(), "family-1", "user-1", created.Session.ID, confirmed.ID, confirmed.Version)
	if err != nil {
		t.Fatal(err)
	}
	return executed, calendar, recorder
}

func TestExecuteCalendarRecordCreateSyncsToProfile(t *testing.T) {
	arguments := `{"pet_id":"pet-1","category":"medical","medical_type":"vaccine","content":"接种狂犬疫苗","occurred_at":"2026-09-23T20:00:00+08:00","sync_targets":["growth"]}`
	executed, calendar, writer := executeCalendarRecordSyncOperation(t, "create-sync-growth", arguments, nil, &recordingPetProfileWriter{})
	if executed.Status != OperationSucceeded {
		t.Fatalf("operation = %+v, want succeeded", executed)
	}
	if len(calendar.created) != 1 || calendar.created[0].Content != "接种狂犬疫苗" {
		t.Fatalf("calendar writes = %+v, want one vaccine record", calendar.created)
	}
	if len(writer.writes) != 1 {
		t.Fatalf("profile writes = %+v, want 1", writer.writes)
	}
	growth := writer.writes[0]
	if growth.PetID != "pet-1" || growth.Resource != "growth-events" || growth.Method != "POST" {
		t.Fatalf("growth write = %+v", growth)
	}
	if growth.Fields["type"] != "疫苗" || growth.Fields["occurred_at"] != "2026-09-23" || growth.Fields["content"] != "接种狂犬疫苗" {
		t.Fatalf("growth fields = %+v", growth.Fields)
	}

	var result struct {
		RecordID   string   `json:"record_id"`
		Synced     []string `json:"synced"`
		SyncFailed []string `json:"sync_failed"`
	}
	if err := json.Unmarshal([]byte(executed.Result), &result); err != nil {
		t.Fatalf("result = %s: %v", executed.Result, err)
	}
	if result.RecordID != "record-1" || len(result.Synced) != 1 || result.Synced[0] != "growth" || len(result.SyncFailed) != 0 {
		t.Fatalf("result = %+v, want record-1 with growth synced", result)
	}
}

func TestExecuteCalendarRecordCreateKeepsRecordWhenSyncFails(t *testing.T) {
	arguments := `{"pet_id":"pet-1","category":"daily","content":"洗澡","occurred_at":"2026-09-23T20:00:00+08:00","sync_targets":["growth"]}`
	executed, calendar, _ := executeCalendarRecordSyncOperation(t, "create-sync-fail", arguments, nil, failingPetProfileWriter{})
	if executed.Status != OperationSucceeded {
		t.Fatalf("operation = %+v, want succeeded (profile sync is best-effort)", executed)
	}
	if len(calendar.created) != 1 {
		t.Fatalf("calendar writes = %+v, want one record", calendar.created)
	}
	var result struct {
		RecordID   string   `json:"record_id"`
		Synced     []string `json:"synced"`
		SyncFailed []string `json:"sync_failed"`
	}
	if err := json.Unmarshal([]byte(executed.Result), &result); err != nil {
		t.Fatalf("result = %s: %v", executed.Result, err)
	}
	if result.RecordID != "record-1" || len(result.Synced) != 0 || len(result.SyncFailed) != 1 || result.SyncFailed[0] != "growth" {
		t.Fatalf("result = %+v, want growth in sync_failed", result)
	}
}

func TestExecuteCalendarRecordCreateWithoutSyncTargetsSkipsProfile(t *testing.T) {
	arguments := `{"pet_id":"pet-1","category":"daily","content":"洗澡","occurred_at":"2026-09-23T20:00:00+08:00"}`
	executed, calendar, writer := executeCalendarRecordSyncOperation(t, "create-sync-none", arguments, nil, &recordingPetProfileWriter{})
	if executed.Status != OperationSucceeded {
		t.Fatalf("operation = %+v, want succeeded", executed)
	}
	if len(calendar.created) != 1 || len(writer.writes) != 0 {
		t.Fatalf("calendar writes = %+v, profile writes = %+v, want calendar only", calendar.created, writer.writes)
	}
	if strings.Contains(executed.Result, "growth") {
		t.Fatalf("result = %s, want no sync targets", executed.Result)
	}
}

// TestExecuteCalendarRecordCreateHonorsUserOptOut 用户在卡片上取消勾选时不写档案，
// 但日历记录仍然正常写入。
func TestExecuteCalendarRecordCreateHonorsUserOptOut(t *testing.T) {
	arguments := `{"pet_id":"pet-1","category":"daily","content":"学会握手","occurred_at":"2026-10-04T19:29:00+08:00","sync_targets":["growth"]}`
	choice := []string{}
	executed, calendar, writer := executeCalendarRecordSyncOperation(t, "create-sync-user-optout", arguments, &choice, &recordingPetProfileWriter{})
	if executed.Status != OperationSucceeded {
		t.Fatalf("operation = %+v, want succeeded", executed)
	}
	if len(calendar.created) != 1 || len(writer.writes) != 0 {
		t.Fatalf("calendar writes = %+v, profile writes = %+v, want calendar only", calendar.created, writer.writes)
	}
	if synced := syncedTargetsOf(t, executed.Result); len(synced) != 0 {
		t.Fatalf("synced = %v, want empty", synced)
	}
}

// TestExecuteCalendarRecordCreateSyncsWhenUserOptsIn 模型没有建议同步，但用户在卡片上勾选后仍要写入档案。
func TestExecuteCalendarRecordCreateSyncsWhenUserOptsIn(t *testing.T) {
	arguments := `{"pet_id":"pet-1","category":"daily","content":"学会握手","occurred_at":"2026-10-04T19:29:00+08:00"}`
	choice := []string{"growth"}
	executed, calendar, writer := executeCalendarRecordSyncOperation(t, "create-sync-user-optin", arguments, &choice, &recordingPetProfileWriter{})
	if executed.Status != OperationSucceeded {
		t.Fatalf("operation = %+v, want succeeded", executed)
	}
	if len(calendar.created) != 1 || len(writer.writes) != 1 || writer.writes[0].Resource != "growth-events" {
		t.Fatalf("calendar writes = %+v, profile writes = %+v, want calendar + growth", calendar.created, writer.writes)
	}
}

// TestConfirmOperationRejectsSyncTargetsForOtherTargets 非日历写入不接受档案同步目标。
func TestConfirmOperationRejectsSyncTargetsForOtherTargets(t *testing.T) {
	service, _ := newV2Service(t, &scriptedModel{}, &fakeBusinessRead{})
	created, err := service.CreateSession(context.Background(), "family-1", "user-1", "pet-1", "啾啾是边牧", "confirm-sync-unsupported")
	if err != nil {
		t.Fatal(err)
	}
	operation, err := service.PrepareOperation(context.Background(), created.Session.ID, created.Run.ID, OperationPreviewInput{
		FamilyID: "family-1",
		UserID:   "user-1",
		Target:   operationTargetPetProfileUpdate,
		Summary:  "修改宠物档案（pet-1）：breed=边牧",
		Payload:  map[string]any{"pet_id": "pet-1", "fields": map[string]any{"breed": "边牧"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	targets := []string{"growth"}
	if _, err := service.ConfirmOperation(context.Background(), "family-1", "user-1", created.Session.ID, operation.ID, operation.Version, operation.Preview, &targets); err == nil {
		t.Fatal("sync_targets should be rejected for non calendar operations")
	}
}

func syncedTargetsOf(t *testing.T, result string) []string {
	t.Helper()
	var parsed struct {
		Synced []string `json:"synced"`
	}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("result = %s: %v", result, err)
	}
	return parsed.Synced
}

// TestOperationDTOSyncTargets 卡片依赖 DTO 的 sync_targets：确认前是建议，确认后是用户选择。
func TestOperationDTOSyncTargets(t *testing.T) {
	service, _ := newV2Service(t, &scriptedModel{}, &fakeBusinessRead{})
	created, err := service.CreateSession(context.Background(), "family-1", "user-1", "pet-1", "帮我记一下并同步", "dto-sync-targets")
	if err != nil {
		t.Fatal(err)
	}
	operation, err := service.PrepareOperation(context.Background(), created.Session.ID, created.Run.ID, OperationPreviewInput{
		FamilyID: "family-1",
		UserID:   "user-1",
		Target:   operationTargetCalendarRecordCreate,
		Summary:  "新增日常记录：2026-10-04 19:29 学会握手",
		Payload: calendarRecordCreatePayload{
			Request:     calendarapp.CreateRecordRequest{PetID: "pet-1", Category: "daily", Content: "学会握手", OccurredAt: "2026-10-04T19:29:00+08:00"},
			SyncTargets: []string{"growth"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if targets := NewOperationDTO(operation).SyncTargets; len(targets) != 1 || targets[0] != "growth" {
		t.Fatalf("pending sync_targets = %v, want [growth]", targets)
	}

	choice := []string{}
	confirmed, err := service.ConfirmOperation(context.Background(), "family-1", "user-1", created.Session.ID, operation.ID, operation.Version, operation.Preview, &choice)
	if err != nil {
		t.Fatal(err)
	}
	if targets := NewOperationDTO(confirmed).SyncTargets; len(targets) != 0 {
		t.Fatalf("confirmed sync_targets = %v, want empty after opt-out", targets)
	}
}
