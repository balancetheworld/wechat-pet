package ask

import (
	"context"
	"encoding/json"
	"testing"
)

func TestExecuteBatchListsFamilyPets(t *testing.T) {
	catalog, err := DefaultCatalog(DefaultToolVersion)
	if err != nil {
		t.Fatal(err)
	}
	business := &fakeBusinessRead{listOutcome: PetListOutcome{
		Pets:   []PetListItem{{PetID: "pet-1", Name: "团子"}, {PetID: "pet-2", Name: "球球"}},
		Source: ReadSource{SourceType: "pet", SourceID: "family-1", Version: "v1"},
	}}
	adapter := NewToolExecutorAdapter(catalog, business, prepareFilter(), "family-1")
	batch := ToolBatch{Calls: []ToolCall{{
		ToolCallID:  "c1",
		CallIndex:   0,
		ToolName:    "list_family_pets",
		ToolVersion: DefaultToolVersion,
		Arguments:   json.RawMessage(`{}`),
	}}}
	results, err := adapter.ExecuteBatch(context.Background(), batch)
	if err != nil {
		t.Fatalf("ExecuteBatch: %v", err)
	}
	if results[0].Status != ToolResultOK {
		t.Fatalf("result = %+v, want ok", results[0])
	}
	outcome, ok := results[0].Data.(PetListOutcome)
	if !ok || len(outcome.Pets) != 2 || outcome.Pets[0].Name != "团子" {
		t.Fatalf("outcome = %#v", results[0].Data)
	}
}

func TestExecuteBatchListsCalendarRecords(t *testing.T) {
	catalog, err := DefaultCatalog(DefaultToolVersion)
	if err != nil {
		t.Fatal(err)
	}
	business := &fakeBusinessRead{listRecordsOutcome: CalendarRecordListOutcome{
		Records: []CalendarRecordListItem{{RecordID: "rec-1", PetID: "pet-1", PetName: "团子", Category: "daily", Content: "洗澡"}},
		Source:  ReadSource{SourceType: "health_record", SourceID: "family-1", Version: "v1"},
	}}
	adapter := NewToolExecutorAdapter(catalog, business, prepareFilter(), "family-1")
	batch := ToolBatch{Calls: []ToolCall{{
		ToolCallID:  "c1",
		CallIndex:   0,
		ToolName:    "list_calendar_records",
		ToolVersion: DefaultToolVersion,
		Arguments:   json.RawMessage(`{}`),
	}}}
	results, err := adapter.ExecuteBatch(context.Background(), batch)
	if err != nil {
		t.Fatalf("ExecuteBatch: %v", err)
	}
	if results[0].Status != ToolResultOK {
		t.Fatalf("result = %+v, want ok", results[0])
	}
	outcome, ok := results[0].Data.(CalendarRecordListOutcome)
	if !ok || len(outcome.Records) != 1 || outcome.Records[0].PetName != "团子" || outcome.Records[0].Content != "洗澡" {
		t.Fatalf("outcome = %#v", results[0].Data)
	}
}

func TestProcessRunAllowsEmptyArgumentsForNoParameterTool(t *testing.T) {
	model := &scriptedModel{responses: [][]ProtocolRecord{prepareCallRecords("list_family_pets", `{}`), finalAnswerRecords()}}
	service, _ := newV2Service(t, model, &fakeBusinessRead{listOutcome: PetListOutcome{Pets: []PetListItem{{PetID: "pet-1", Name: "团子"}}}})

	created, err := service.CreateSession(context.Background(), "family-1", "user-1", "pet-1", "你知道家里现在有哪些宠物吗", "create-empty-args")
	if err != nil {
		t.Fatal(err)
	}
	processed, err := service.ProcessRun(context.Background(), "family-1", created.Session.ID, created.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if processed.Run.Status != RunCompleted {
		t.Fatalf("run status = %s (error %s: %s), want completed", processed.Run.Status, processed.Run.ErrorCode, lastEvent(processed.Events).Data)
	}
}

func callRecordsWithoutCoverage(toolName, arguments string) []ProtocolRecord {
	header := headerRecord(ActionCallTools)
	header.Header.TaskUpdates = []TaskUpdate{{TaskKey: "t1", Goal: "列出家庭宠物"}}
	call := ProtocolRecord{Type: RecordCall, Call: &CallRecord{
		Type:           RecordCall,
		CallKey:        "c1",
		TaskKeys:       []string{"t1"},
		ToolName:       toolName,
		CatalogVersion: DefaultToolVersion,
		Arguments:      json.RawMessage(arguments),
	}}
	return []ProtocolRecord{header, call, endRecord()}
}

func TestProcessRunDerivesMissingCoverage(t *testing.T) {
	model := &scriptedModel{responses: [][]ProtocolRecord{callRecordsWithoutCoverage("list_family_pets", `{}`), finalAnswerRecords()}}
	service, _ := newV2Service(t, model, &fakeBusinessRead{listOutcome: PetListOutcome{Pets: []PetListItem{{PetID: "pet-1", Name: "团子"}}}})

	created, err := service.CreateSession(context.Background(), "family-1", "user-1", "pet-1", "你知道家里现在有哪些宠物吗", "create-no-coverage")
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
}

func TestExecuteBatchListsReminders(t *testing.T) {
	catalog, err := DefaultCatalog(DefaultToolVersion)
	if err != nil {
		t.Fatal(err)
	}
	business := &fakeBusinessRead{remindersOutcome: ReminderListOutcome{
		Reminders: []ReminderListItem{{ReminderID: "rem-1", PetID: "pet-1", PetName: "团子", ReminderDate: "2026-10-01", Category: "medical", Content: "疫苗"}},
		Source:    ReadSource{SourceType: "health_record", SourceID: "family-1", Version: "v1"},
	}}
	adapter := NewToolExecutorAdapter(catalog, business, prepareFilter(), "family-1")
	batch := ToolBatch{Calls: []ToolCall{{
		ToolCallID:  "c1",
		CallIndex:   0,
		ToolName:    "list_reminders",
		ToolVersion: DefaultToolVersion,
		Arguments:   json.RawMessage(`{}`),
	}}}
	results, err := adapter.ExecuteBatch(context.Background(), batch)
	if err != nil {
		t.Fatalf("ExecuteBatch: %v", err)
	}
	if results[0].Status != ToolResultOK {
		t.Fatalf("result = %+v, want ok", results[0])
	}
	outcome, ok := results[0].Data.(ReminderListOutcome)
	if !ok || len(outcome.Reminders) != 1 || outcome.Reminders[0].ReminderDate != "2026-10-01" {
		t.Fatalf("outcome = %#v", results[0].Data)
	}
}
