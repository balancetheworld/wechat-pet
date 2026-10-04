package ask

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func callCalendarCreateRecords() []ProtocolRecord {
	header := headerRecord(ActionCallTools)
	header.Header.TaskUpdates = []TaskUpdate{{TaskKey: "t1", Goal: "帮用户登记一条洗澡记录"}}
	call := ProtocolRecord{Type: RecordCall, Call: &CallRecord{
		Type:           RecordCall,
		CallKey:        "c1",
		TaskKeys:       []string{"t1"},
		ToolName:       "create_calendar_record",
		CatalogVersion: DefaultToolVersion,
		Arguments:      json.RawMessage(`{"pet_id":"pet-1","category":"daily","content":"洗澡","occurred_at":"2026-09-23T20:00:00+08:00"}`),
	}}
	coverage := ProtocolRecord{Type: RecordCoverage, Coverage: &CoverageRecord{
		Type:  RecordCoverage,
		Tasks: []TaskCoverage{{TaskKey: "t1", CallKeys: []string{"c1"}}},
	}}
	return []ProtocolRecord{header, call, coverage, endRecord()}
}

func TestProcessRunPreparesCalendarRecordOperation(t *testing.T) {
	model := &scriptedModel{responses: [][]ProtocolRecord{callCalendarCreateRecords(), finalAnswerRecords()}}
	service, repository := newV2Service(t, model, &fakeBusinessRead{})

	created, err := service.CreateSession(context.Background(), "family-1", "user-1", "pet-1", "帮我记一下今天给团子洗澡", "create-prepare-1")
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
		t.Fatalf("operations = %d, want 1 pending preview", len(operations))
	}
	operation := operations[0]
	if operation.Status != OperationPending || operation.Target != operationTargetCalendarRecordCreate {
		t.Fatalf("operation = %+v, want pending calendar record create", operation)
	}
	if operation.RunID != created.Run.ID || operation.CreatedBy != "user-1" {
		t.Fatalf("operation identity = %+v", operation)
	}
	if operation.Preview == "" || operation.ExpiresAt == nil {
		t.Fatalf("operation preview/expiry missing: %+v", operation)
	}
	var payload struct {
		Request struct {
			PetID      string `json:"pet_id"`
			Category   string `json:"category"`
			Content    string `json:"content"`
			OccurredAt string `json:"occurred_at"`
		} `json:"request"`
		SyncTargets []string `json:"sync_targets"`
	}
	if err := json.Unmarshal([]byte(operation.Payload), &payload); err != nil {
		t.Fatalf("payload = %s: %v", operation.Payload, err)
	}
	if payload.Request.PetID != "pet-1" || payload.Request.Category != "daily" || payload.Request.Content != "洗澡" || payload.Request.OccurredAt != "2026-09-23T20:00:00+08:00" {
		t.Fatalf("payload = %+v", payload)
	}
	if len(payload.SyncTargets) != 0 {
		t.Fatalf("sync_targets = %v, want empty", payload.SyncTargets)
	}
	if len(model.inputs) != 2 {
		t.Fatalf("model calls = %d, want 2 (prepare then final answer)", len(model.inputs))
	}
}

func prepareCallRecords(toolName, arguments string) []ProtocolRecord {
	header := headerRecord(ActionCallTools)
	header.Header.TaskUpdates = []TaskUpdate{{TaskKey: "t1", Goal: "准备写入"}}
	call := ProtocolRecord{Type: RecordCall, Call: &CallRecord{
		Type:           RecordCall,
		CallKey:        "c1",
		TaskKeys:       []string{"t1"},
		ToolName:       toolName,
		CatalogVersion: DefaultToolVersion,
		Arguments:      json.RawMessage(arguments),
	}}
	coverage := ProtocolRecord{Type: RecordCoverage, Coverage: &CoverageRecord{
		Type:  RecordCoverage,
		Tasks: []TaskCoverage{{TaskKey: "t1", CallKeys: []string{"c1"}}},
	}}
	return []ProtocolRecord{header, call, coverage, endRecord()}
}

func prepareOperationFor(t *testing.T, toolName, arguments string) Operation {
	t.Helper()
	model := &scriptedModel{responses: [][]ProtocolRecord{prepareCallRecords(toolName, arguments), finalAnswerRecords()}}
	service, repository := newV2Service(t, model, &fakeBusinessRead{})
	created, err := service.CreateSession(context.Background(), "family-1", "user-1", "pet-1", "帮我改一下", "create-prepare-"+toolName)
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
	return operations[0]
}

func TestProcessRunPreparesCustomMedicalTypeRecord(t *testing.T) {
	operation := prepareOperationFor(t, "create_calendar_record", `{"pet_id":"pet-1","category":"medical","medical_type":"other","custom_medical_type":"过敏复查","content":"皮肤过敏复查","occurred_at":"2026-09-23T20:00:00+08:00"}`)
	if operation.Target != operationTargetCalendarRecordCreate || operation.Status != OperationPending {
		t.Fatalf("operation = %+v, want pending calendar record create", operation)
	}
	if !strings.Contains(operation.Preview, "过敏复查") {
		t.Fatalf("preview = %q, want custom medical type", operation.Preview)
	}
	var payload struct {
		Request struct {
			MedicalType       string `json:"medical_type"`
			CustomMedicalType string `json:"custom_medical_type"`
		} `json:"request"`
	}
	if err := json.Unmarshal([]byte(operation.Payload), &payload); err != nil {
		t.Fatalf("payload = %s: %v", operation.Payload, err)
	}
	if payload.Request.MedicalType != "other" || payload.Request.CustomMedicalType != "过敏复查" {
		t.Fatalf("payload = %+v", payload)
	}
}

func TestPrepareCalendarRecordMedicalType(t *testing.T) {
	base := func(overrides map[string]any) map[string]any {
		args := map[string]any{
			"pet_id":      "pet-1",
			"category":    "medical",
			"content":     "皮肤过敏复查",
			"occurred_at": "2026-09-23T20:00:00+08:00",
		}
		for key, value := range overrides {
			args[key] = value
		}
		return args
	}

	input, err := prepareInputForTool("create_calendar_record", base(map[string]any{"medical_type": "other", "custom_medical_type": "过敏复查"}))
	if err != nil {
		t.Fatalf("prepareInputForTool: %v", err)
	}
	raw, err := json.Marshal(input.Payload)
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Request struct {
			MedicalType       string `json:"medical_type"`
			CustomMedicalType string `json:"custom_medical_type"`
		} `json:"request"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Request.MedicalType != "other" || payload.Request.CustomMedicalType != "过敏复查" {
		t.Fatalf("payload = %s", raw)
	}
	if !strings.Contains(input.Summary, "过敏复查") {
		t.Fatalf("summary = %q, want custom medical type", input.Summary)
	}

	rejected := map[string]map[string]any{
		"非枚举医疗类型":        base(map[string]any{"medical_type": "疫苗"}),
		"other 缺自定义名称":   base(map[string]any{"medical_type": "other"}),
		"非 other 带自定义名称": base(map[string]any{"medical_type": "vaccine", "custom_medical_type": "过敏复查"}),
		"日常记录带医疗类型":      base(map[string]any{"category": "daily", "medical_type": "vaccine"}),
		"自定义名称超长":        base(map[string]any{"medical_type": "other", "custom_medical_type": strings.Repeat("a", 51)}),
	}
	for name, args := range rejected {
		if _, err := prepareInputForTool("create_calendar_record", args); err == nil {
			t.Fatalf("%s should be rejected before freezing the preview", name)
		}
	}

	for name, args := range map[string]map[string]any{
		"枚举医疗类型":   base(map[string]any{"medical_type": "vaccine"}),
		"医疗记录不带类型": base(nil),
	} {
		if _, err := prepareInputForTool("create_calendar_record", args); err != nil {
			t.Fatalf("%s should be accepted: %v", name, err)
		}
	}
}

func TestPrepareCalendarRecordSyncTargets(t *testing.T) {
	base := func(overrides map[string]any) map[string]any {
		args := map[string]any{
			"pet_id":      "pet-1",
			"category":    "daily",
			"content":     "洗澡",
			"occurred_at": "2026-09-23T20:00:00+08:00",
		}
		for key, value := range overrides {
			args[key] = value
		}
		return args
	}
	syncTargetsOf := func(t *testing.T, input OperationPreviewInput) []string {
		t.Helper()
		raw, err := json.Marshal(input.Payload)
		if err != nil {
			t.Fatal(err)
		}
		var payload struct {
			SyncTargets []string `json:"sync_targets"`
		}
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Fatal(err)
		}
		return payload.SyncTargets
	}

	input, err := prepareInputForTool("create_calendar_record", base(map[string]any{"sync_targets": []any{"growth"}}))
	if err != nil {
		t.Fatalf("prepareInputForTool: %v", err)
	}
	if targets := syncTargetsOf(t, input); len(targets) != 1 || targets[0] != "growth" {
		t.Fatalf("sync_targets = %v, want [growth]", targets)
	}
	if strings.Contains(input.Summary, "同步") {
		t.Fatalf("summary = %q, want preview without sync (sync is chosen on the confirm card)", input.Summary)
	}

	input, err = prepareInputForTool("create_calendar_record", base(map[string]any{"sync_targets": []any{"growth", "growth"}}))
	if err != nil {
		t.Fatalf("prepareInputForTool: %v", err)
	}
	if targets := syncTargetsOf(t, input); len(targets) != 1 || targets[0] != "growth" {
		t.Fatalf("sync_targets = %v, want deduped [growth]", targets)
	}

	for name, args := range map[string]map[string]any{
		"非数组":    base(map[string]any{"sync_targets": "growth"}),
		"未知目标":   base(map[string]any{"sync_targets": []any{"calendar"}}),
		"生日纪念页":  base(map[string]any{"sync_targets": []any{"birthday"}}),
		"元素非字符串": base(map[string]any{"sync_targets": []any{1}}),
	} {
		if _, err := prepareInputForTool("create_calendar_record", args); err == nil {
			t.Fatalf("%s should be rejected before freezing the preview", name)
		}
	}
}

func TestProcessRunPreparesCalendarRecordUpdate(t *testing.T) {
	operation := prepareOperationFor(t, "update_calendar_record", `{"record_id":"rec-1","content":"洗澡（已更正）"}`)
	if operation.Target != operationTargetCalendarRecordUpdate || operation.Status != OperationPending {
		t.Fatalf("operation = %+v, want pending calendar record update", operation)
	}
	var payload struct {
		RecordID string `json:"record_id"`
		Request  struct {
			Content *string `json:"content"`
		} `json:"request"`
	}
	if err := json.Unmarshal([]byte(operation.Payload), &payload); err != nil {
		t.Fatalf("payload = %s: %v", operation.Payload, err)
	}
	if payload.RecordID != "rec-1" || payload.Request.Content == nil || *payload.Request.Content != "洗澡（已更正）" {
		t.Fatalf("payload = %+v", payload)
	}
}

func TestProcessRunPreparesPetProfileUpdate(t *testing.T) {
	operation := prepareOperationFor(t, "update_pet_profile", `{"pet_id":"pet-1","breed":"英短","birthday":"2020-05-01","home_date":"2020-06-01"}`)
	if operation.Target != operationTargetPetProfileUpdate || operation.Status != OperationPending {
		t.Fatalf("operation = %+v, want pending pet profile update", operation)
	}
	var payload struct {
		PetID  string         `json:"pet_id"`
		Fields map[string]any `json:"fields"`
	}
	if err := json.Unmarshal([]byte(operation.Payload), &payload); err != nil {
		t.Fatalf("payload = %s: %v", operation.Payload, err)
	}
	if payload.PetID != "pet-1" || payload.Fields["breed"] != "英短" || payload.Fields["birthday"] != "2020-05-01" || payload.Fields["home_date"] != "2020-06-01" {
		t.Fatalf("payload = %+v", payload)
	}
	if _, leaked := payload.Fields["avatar_asset_id"]; leaked {
		t.Fatalf("asset fields must not be accepted from the model: %+v", payload.Fields)
	}
}

func TestPrepareInputForToolRejectsUnsupportedFields(t *testing.T) {
	input, err := prepareInputForTool("update_pet_profile", map[string]any{"pet_id": "pet-1", "breed": "英短", "avatar_asset_id": "asset-x"})
	if err != nil {
		t.Fatalf("prepareInputForTool: %v", err)
	}
	raw, err := json.Marshal(input.Payload)
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		PetID  string         `json:"pet_id"`
		Fields map[string]any `json:"fields"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("payload = %s: %v", raw, err)
	}
	if _, leaked := payload.Fields["avatar_asset_id"]; leaked {
		t.Fatalf("asset fields must be dropped: %+v", payload.Fields)
	}
	if input.Target != operationTargetPetProfileUpdate || input.Summary == "" {
		t.Fatalf("input = %+v", input)
	}

	if _, err := prepareInputForTool("update_pet_profile", map[string]any{"pet_id": "pet-1"}); err == nil {
		t.Fatal("empty profile update should fail")
	}
	if _, err := prepareInputForTool("update_calendar_record", map[string]any{"record_id": "rec-1"}); err == nil {
		t.Fatal("calendar record update without changes should fail")
	}
}

func TestProcessRunPreparesCalendarReminderComplete(t *testing.T) {
	operation := prepareOperationFor(t, "complete_calendar_reminder", `{"reminder_id":"rem-1","content":"已打完疫苗"}`)
	if operation.Target != operationTargetCalendarReminderComplete || operation.Status != OperationPending {
		t.Fatalf("operation = %+v, want pending reminder completion", operation)
	}
	var payload struct {
		ReminderID string `json:"reminder_id"`
		Request    struct {
			Content string `json:"content"`
		} `json:"request"`
	}
	if err := json.Unmarshal([]byte(operation.Payload), &payload); err != nil {
		t.Fatalf("payload = %s: %v", operation.Payload, err)
	}
	if payload.ReminderID != "rem-1" || payload.Request.Content != "已打完疫苗" {
		t.Fatalf("payload = %+v", payload)
	}
}

func TestProcessRunPreparesPetHealthUpdate(t *testing.T) {
	operation := prepareOperationFor(t, "update_pet_health", `{"pet_id":"pet-1","allergies":"鸡肉过敏","long_term_medication":"Apoquel"}`)
	if operation.Target != operationTargetPetHealthUpdate || operation.Status != OperationPending {
		t.Fatalf("operation = %+v, want pending pet health update", operation)
	}
	var payload struct {
		PetID  string         `json:"pet_id"`
		Fields map[string]any `json:"fields"`
	}
	if err := json.Unmarshal([]byte(operation.Payload), &payload); err != nil {
		t.Fatalf("payload = %s: %v", operation.Payload, err)
	}
	if payload.PetID != "pet-1" || payload.Fields["allergies"] != "鸡肉过敏" || payload.Fields["long_term_medication"] != "Apoquel" {
		t.Fatalf("payload = %+v", payload)
	}
	if len(payload.Fields) != 2 {
		t.Fatalf("only whitelisted health fields may be frozen, got %+v", payload.Fields)
	}
}

func TestProcessRunPreparesPetCreate(t *testing.T) {
	operation := prepareOperationFor(t, "create_pet", `{"name":"球球","breed":"英短","birthday":"2024-04-01","home_date":"2024-05-01"}`)
	if operation.Target != operationTargetPetCreate || operation.Status != OperationPending {
		t.Fatalf("operation = %+v, want pending pet create", operation)
	}
	var payload struct {
		Name     string `json:"name"`
		Breed    string `json:"breed"`
		Birthday string `json:"birthday"`
		HomeDate string `json:"home_date"`
	}
	if err := json.Unmarshal([]byte(operation.Payload), &payload); err != nil {
		t.Fatalf("payload = %s: %v", operation.Payload, err)
	}
	if payload.Name != "球球" || payload.Breed != "英短" || payload.Birthday != "2024-04-01" || payload.HomeDate != "2024-05-01" {
		t.Fatalf("payload = %+v", payload)
	}
}
