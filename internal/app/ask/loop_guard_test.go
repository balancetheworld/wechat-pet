package ask

import (
	"context"
	"encoding/json"
	"testing"
)

func guardCall(callID string, toolName string, arguments string) ToolCall {
	return ToolCall{ToolCallID: callID, CallIndex: 0, ToolName: toolName, ToolVersion: DefaultToolVersion, Arguments: json.RawMessage(arguments)}
}

func okResult(call ToolCall, data any) ToolResult {
	return ToolResult{ToolCallID: call.ToolCallID, CallIndex: call.CallIndex, Status: ToolResultOK, Completeness: CompletenessComplete, Data: data}
}

func failureResult(call ToolCall, reason string) ToolResult {
	return ToolResult{ToolCallID: call.ToolCallID, CallIndex: call.CallIndex, Status: ToolResultError, Error: &ToolError{Category: ToolErrServiceError, Stage: "execute", Reason: reason}}
}

func TestLoopGuardBlocksRepeatedDeterministicFailure(t *testing.T) {
	guard := NewLoopGuard()
	call := guardCall("c1", "search_health_records", `{"pet_id":"pet-1","category":"medical"}`)
	guard.Record(call, failureResult(call, "permission denied"))

	reason, blocked := guard.BlockReason(guardCall("c2", "search_health_records", `{"category":"medical","pet_id":"pet-1"}`), ActionSearch)
	if !blocked || reason == "" {
		t.Fatal("identical failing call should be blocked even with reordered keys")
	}
}

func TestLoopGuardBlocksRepeatedIdenticalRead(t *testing.T) {
	guard := NewLoopGuard()
	call := guardCall("c1", "read_pet_profile", `{"pet_id":"pet-1"}`)
	if _, blocked := guard.BlockReason(call, ActionRead); blocked {
		t.Fatal("first call must not be blocked")
	}
	guard.Record(call, okResult(call, PetProfileOutcome{Source: ReadSource{SourceID: "pet-1", Version: "v1"}}))

	retry := guardCall("c2", "read_pet_profile", `{"pet_id":"pet-1"}`)
	if _, blocked := guard.BlockReason(retry, ActionRead); blocked {
		t.Fatal("second identical call must be allowed once to detect async change")
	}
	guard.Record(retry, okResult(retry, PetProfileOutcome{Source: ReadSource{SourceID: "pet-1", Version: "v1"}}))

	if reason, blocked := guard.BlockReason(guardCall("c3", "read_pet_profile", `{"pet_id":"pet-1"}`), ActionRead); !blocked || reason == "" {
		t.Fatal("third identical call with unchanged result should be blocked")
	}
}

func TestLoopGuardAllowsChangedReadResult(t *testing.T) {
	guard := NewLoopGuard()
	call := guardCall("c1", "read_pet_profile", `{"pet_id":"pet-1"}`)
	guard.Record(call, okResult(call, PetProfileOutcome{Source: ReadSource{SourceID: "pet-1", Version: "v1"}}))
	guard.Record(guardCall("c2", "read_pet_profile", `{"pet_id":"pet-1"}`), okResult(call, PetProfileOutcome{Source: ReadSource{SourceID: "pet-1", Version: "v2"}}))

	if _, blocked := guard.BlockReason(guardCall("c3", "read_pet_profile", `{"pet_id":"pet-1"}`), ActionRead); blocked {
		t.Fatal("changed result must keep polling allowed")
	}
}

func TestLoopGuardBlocksRepeatedWritePreparation(t *testing.T) {
	guard := NewLoopGuard()
	call := guardCall("c1", "create_calendar_record", `{"pet_id":"pet-1","category":"daily","content":"洗澡","occurred_at":"2026-09-23T20:00:00+08:00"}`)
	guard.Record(call, okResult(call, PreparedOperation{OperationID: "op-1", Status: "pending", Preview: "新增日常记录"}))

	if _, blocked := guard.BlockReason(guardCall("c2", "create_calendar_record", `{"pet_id":"pet-1","category":"daily","content":"洗澡","occurred_at":"2026-09-23T20:00:00+08:00"}`), ActionPrepareCreate); !blocked {
		t.Fatal("identical write preparation should be blocked")
	}
	if _, blocked := guard.BlockReason(guardCall("c3", "create_calendar_record", `{"pet_id":"pet-1","category":"daily","content":"喂药","occurred_at":"2026-09-23T20:00:00+08:00"}`), ActionPrepareCreate); blocked {
		t.Fatal("corrected arguments must be allowed")
	}
}

func TestExecuteBatchBlocksSecondIdenticalRead(t *testing.T) {
	catalog, err := DefaultCatalog(DefaultToolVersion)
	if err != nil {
		t.Fatal(err)
	}
	adapter := NewToolExecutorAdapter(catalog, &fakeBusinessRead{}, readOnlyFilter(), "family-1")
	batch := ToolBatch{Calls: []ToolCall{
		guardCall("c1", "read_pet_profile", `{"pet_id":"pet-1"}`),
		{CallIndex: 1, ToolCallID: "c2", ToolName: "read_pet_profile", ToolVersion: DefaultToolVersion, Arguments: json.RawMessage(`{"pet_id":"pet-1"}`)},
	}}
	results, err := adapter.ExecuteBatch(context.Background(), batch)
	if err != nil {
		t.Fatalf("ExecuteBatch: %v", err)
	}
	if results[0].Status != ToolResultOK {
		t.Fatalf("first result = %+v, want ok", results[0])
	}
	if results[1].Status != ToolResultNotExecuted || results[1].Error == nil {
		t.Fatalf("second result = %+v, want blocked by loop guard", results[1])
	}
}
