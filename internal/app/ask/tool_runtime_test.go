package ask

import (
	"encoding/json"
	"strings"
	"testing"
)

func testToolRuntimeCatalog(t *testing.T) *Catalog {
	t.Helper()
	tools := []Tool{
		{Name: "resolve_pet", OperationID: "resolve.pet", ResourceType: ResourcePet, ActionType: ActionResolve, Version: "v1"},
		{Name: "search_records", OperationID: "search.health_record", ResourceType: ResourceHealthRecord, ActionType: ActionSearch, Version: "v1"},
		{Name: "read_record", OperationID: "read.health_record", ResourceType: ResourceHealthRecord, ActionType: ActionRead, Version: "v1"},
		{Name: "prepare_create", OperationID: "prepare.create", ResourceType: ResourceHealthRecord, ActionType: ActionPrepareCreate, Version: "v1"},
	}
	catalog, err := NewCatalog(tools, nil, "v1", func(s string) []string { return strings.Fields(s) })
	if err != nil {
		t.Fatal(err)
	}
	return catalog
}

func TestCanonicalJSONSortsKeysAndDistinguishesTypes(t *testing.T) {
	a, err := CanonicalJSON(map[string]any{"a": 1, "b": 2})
	if err != nil {
		t.Fatal(err)
	}
	b, err := CanonicalJSON(map[string]any{"b": 2, "a": 1})
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Fatalf("key order should not matter: %q vs %q", a, b)
	}

	nestedA, err := CanonicalJSON(map[string]any{"a": map[string]any{"x": 1, "y": 2}})
	if err != nil {
		t.Fatal(err)
	}
	nestedB, err := CanonicalJSON(map[string]any{"a": map[string]any{"y": 2, "x": 1}})
	if err != nil {
		t.Fatal(err)
	}
	if nestedA != nestedB {
		t.Fatalf("nested key order should not matter: %q vs %q", nestedA, nestedB)
	}

	// 类型区分：字符串 "1" 与数字 1 不同。
	str, _ := CanonicalJSON(map[string]any{"v": "1"})
	num, _ := CanonicalJSON(map[string]any{"v": 1})
	if str == num {
		t.Fatalf("string vs number should differ: %q", str)
	}

	// 数组顺序保留。
	arrA, _ := CanonicalJSON(map[string]any{"v": []any{1, 2}})
	arrB, _ := CanonicalJSON(map[string]any{"v": []any{2, 1}})
	if arrA == arrB {
		t.Fatalf("array order should matter: %q", arrA)
	}
}

func TestToolFingerprintStableAndSensitive(t *testing.T) {
	a, err := ToolFingerprint("search_records", "v1", map[string]any{"pet_id": "p1", "start": "2026-01-01"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := ToolFingerprint("search_records", "v1", map[string]any{"start": "2026-01-01", "pet_id": "p1"})
	if err != nil {
		t.Fatal(err)
	}
	if a == "" || a != b {
		t.Fatalf("fingerprint not stable: %q vs %q", a, b)
	}

	changedArg, err := ToolFingerprint("search_records", "v1", map[string]any{"pet_id": "p2", "start": "2026-01-01"})
	if err != nil {
		t.Fatal(err)
	}
	if changedArg == a {
		t.Fatalf("fingerprint should change with arguments")
	}
	changedTool, err := ToolFingerprint("read_record", "v1", map[string]any{"pet_id": "p1", "start": "2026-01-01"})
	if err != nil {
		t.Fatal(err)
	}
	if changedTool == a {
		t.Fatalf("fingerprint should change with tool name")
	}
}

func TestToolExecutionStateMachine(t *testing.T) {
	cases := []struct {
		from ToolExecutionStatus
		to   ToolExecutionStatus
		want bool
	}{
		{ToolExecQueued, ToolExecRunning, true},
		{ToolExecQueued, ToolExecBlocked, true},
		{ToolExecQueued, ToolExecCanceled, true},
		{ToolExecQueued, ToolExecSucceeded, false},
		{ToolExecRunning, ToolExecSucceeded, true},
		{ToolExecRunning, ToolExecFailed, true},
		{ToolExecRunning, ToolExecCanceled, true},
		{ToolExecRunning, ToolExecUnknown, true},
		{ToolExecRunning, ToolExecRunning, false},
		{ToolExecSucceeded, ToolExecRunning, false},
		{ToolExecFailed, ToolExecQueued, false},
		{ToolExecUnknown, ToolExecRunning, false},
		{ToolExecBlocked, ToolExecRunning, false},
	}
	for _, c := range cases {
		if got := c.from.CanTransition(c.to); got != c.want {
			t.Fatalf("CanTransition(%s, %s) = %v, want %v", c.from, c.to, got, c.want)
		}
	}
	for _, status := range []ToolExecutionStatus{ToolExecSucceeded, ToolExecFailed, ToolExecCanceled, ToolExecUnknown, ToolExecBlocked} {
		if !status.IsTerminal() {
			t.Fatalf("%s should be terminal", status)
		}
	}
	for _, status := range []ToolExecutionStatus{ToolExecQueued, ToolExecRunning} {
		if status.IsTerminal() {
			t.Fatalf("%s should not be terminal", status)
		}
	}
}

func validArguments() json.RawMessage {
	return json.RawMessage(`{"pet_id":"p1"}`)
}

func TestValidateBatchRejectsInvalidTools(t *testing.T) {
	catalog := testToolRuntimeCatalog(t)

	valid := ToolBatch{BatchID: "b1", RunID: "r1", AttemptID: "a1", Calls: []ToolCall{
		{ToolCallID: "c1", CallIndex: 0, ToolName: "resolve_pet", ToolVersion: "v1", Arguments: validArguments()},
	}}
	if err := ValidateBatch(valid, catalog, Filter{}); err != nil {
		t.Fatalf("valid batch rejected: %v", err)
	}

	// 非法工具。
	unknown := ToolBatch{BatchID: "b2", RunID: "r1", AttemptID: "a1", Calls: []ToolCall{
		{ToolCallID: "c1", CallIndex: 0, ToolName: "not_a_tool", ToolVersion: "v1", Arguments: validArguments()},
	}}
	if _, ok := ValidateBatch(unknown, catalog, Filter{}).(*BatchValidationError); !ok {
		t.Fatalf("unknown tool should fail batch validation")
	}

	// 版本不匹配。
	versionMismatch := ToolBatch{BatchID: "b3", RunID: "r1", AttemptID: "a1", Calls: []ToolCall{
		{ToolCallID: "c1", CallIndex: 0, ToolName: "resolve_pet", ToolVersion: "v2", Arguments: validArguments()},
	}}
	if _, ok := ValidateBatch(versionMismatch, catalog, Filter{}).(*BatchValidationError); !ok {
		t.Fatalf("version mismatch should fail batch validation")
	}

	// 禁用工具。
	disabled := ToolBatch{BatchID: "b4", RunID: "r1", AttemptID: "a1", Calls: []ToolCall{
		{ToolCallID: "c1", CallIndex: 0, ToolName: "resolve_pet", ToolVersion: "v1", Arguments: validArguments()},
	}}
	if _, ok := ValidateBatch(disabled, catalog, Filter{DisabledTools: map[string]struct{}{"resolve.pet": {}}}).(*BatchValidationError); !ok {
		t.Fatalf("disabled tool should fail batch validation")
	}
}

func TestValidateBatchRejectsInvalidArgumentsAndDuplicates(t *testing.T) {
	catalog := testToolRuntimeCatalog(t)

	// 非法参数（空）。
	emptyArgs := ToolBatch{BatchID: "b1", RunID: "r1", AttemptID: "a1", Calls: []ToolCall{
		{ToolCallID: "c1", CallIndex: 0, ToolName: "resolve_pet", ToolVersion: "v1", Arguments: nil},
	}}
	if _, ok := ValidateBatch(emptyArgs, catalog, Filter{}).(*BatchValidationError); !ok {
		t.Fatalf("empty arguments should fail batch validation")
	}

	// 非法参数（非 JSON）。
	badJSON := ToolBatch{BatchID: "b2", RunID: "r1", AttemptID: "a1", Calls: []ToolCall{
		{ToolCallID: "c1", CallIndex: 0, ToolName: "resolve_pet", ToolVersion: "v1", Arguments: json.RawMessage(`{bad`)},
	}}
	if _, ok := ValidateBatch(badJSON, catalog, Filter{}).(*BatchValidationError); !ok {
		t.Fatalf("bad JSON should fail batch validation")
	}

	// 重复 tool_call_id。
	dupID := ToolBatch{BatchID: "b3", RunID: "r1", AttemptID: "a1", Calls: []ToolCall{
		{ToolCallID: "c1", CallIndex: 0, ToolName: "resolve_pet", ToolVersion: "v1", Arguments: validArguments()},
		{ToolCallID: "c1", CallIndex: 1, ToolName: "search_records", ToolVersion: "v1", Arguments: validArguments()},
	}}
	if _, ok := ValidateBatch(dupID, catalog, Filter{}).(*BatchValidationError); !ok {
		t.Fatalf("duplicate tool_call_id should fail batch validation")
	}

	// 重复 call_index。
	dupIndex := ToolBatch{BatchID: "b4", RunID: "r1", AttemptID: "a1", Calls: []ToolCall{
		{ToolCallID: "c1", CallIndex: 0, ToolName: "resolve_pet", ToolVersion: "v1", Arguments: validArguments()},
		{ToolCallID: "c2", CallIndex: 0, ToolName: "search_records", ToolVersion: "v1", Arguments: validArguments()},
	}}
	if _, ok := ValidateBatch(dupIndex, catalog, Filter{}).(*BatchValidationError); !ok {
		t.Fatalf("duplicate call_index should fail batch validation")
	}
}

func TestCanRunInParallel(t *testing.T) {
	catalog := testToolRuntimeCatalog(t)

	resolve := ToolCall{ToolCallID: "c1", CallIndex: 0, ToolName: "resolve_pet", ToolVersion: "v1", Arguments: validArguments()}
	search := ToolCall{ToolCallID: "c2", CallIndex: 1, ToolName: "search_records", ToolVersion: "v1", Arguments: validArguments()}
	prepare := ToolCall{ToolCallID: "c3", CallIndex: 2, ToolName: "prepare_create", ToolVersion: "v1", Arguments: validArguments()}

	if !CanRunInParallel(resolve, search, catalog) {
		t.Fatalf("two read-only calls should run in parallel")
	}
	if CanRunInParallel(resolve, prepare, catalog) {
		t.Fatalf("read-only + prepare_create should not run in parallel")
	}
	if CanRunInParallel(prepare, prepare, catalog) {
		t.Fatalf("two prepare_create should not run in parallel")
	}

	// 依赖关系：search 依赖 resolve 的结果，不并行。
	dependent := search
	dependent.DependsOn = []string{"c1"}
	if CanRunInParallel(resolve, dependent, catalog) {
		t.Fatalf("dependent call should not run in parallel")
	}
}

func TestDecideReuse(t *testing.T) {
	candidate := ReuseCandidate{
		SessionID:   "s1",
		RunID:       "r-old",
		ToolName:    "search_records",
		ToolVersion: "v1",
		Fingerprint: "fp-1",
		Result:      ToolResult{Status: ToolResultOK, Completeness: CompletenessComplete},
	}
	base := ReuseInput{
		SessionID:        "s1",
		ToolName:         "search_records",
		ToolVersion:      "v1",
		Fingerprint:      "fp-1",
		Candidate:        candidate,
		Permitted:        true,
		SourceStillValid: true,
	}
	if got := DecideReuse(base); got != ReuseReusable {
		t.Fatalf("valid reuse = %s, want reusable", got)
	}

	crossSession := base
	crossSession.SessionID = "s2"
	if got := DecideReuse(crossSession); got != ReuseUnavailable {
		t.Fatalf("cross-session = %s, want unavailable", got)
	}

	versionMismatch := base
	versionMismatch.ToolVersion = "v2"
	if got := DecideReuse(versionMismatch); got != ReuseReread {
		t.Fatalf("version mismatch = %s, want reread", got)
	}

	fingerprintChange := base
	fingerprintChange.Fingerprint = "fp-2"
	if got := DecideReuse(fingerprintChange); got != ReuseReread {
		t.Fatalf("fingerprint change = %s, want reread", got)
	}

	notPermitted := base
	notPermitted.Permitted = false
	if got := DecideReuse(notPermitted); got != ReuseUnavailable {
		t.Fatalf("not permitted = %s, want unavailable", got)
	}

	sourceChanged := base
	sourceChanged.SourceStillValid = false
	if got := DecideReuse(sourceChanged); got != ReuseReread {
		t.Fatalf("source changed = %s, want reread", got)
	}
}

func TestDecideLoopGuard(t *testing.T) {
	if got := DecideLoopGuard(LoopGuardInput{}); got != LoopGuardAllow {
		t.Fatalf("no history = %s, want allow", got)
	}
	if got := DecideLoopGuard(LoopGuardInput{PriorFailure: true}); got != LoopGuardBlock {
		t.Fatalf("prior failure = %s, want block", got)
	}
	if got := DecideLoopGuard(LoopGuardInput{PriorResult: &ToolResult{Status: ToolResultOK}}); got != LoopGuardBlock {
		t.Fatalf("prior success = %s, want block", got)
	}
	if got := DecideLoopGuard(LoopGuardInput{PriorResult: &ToolResult{Status: ToolResultError}}); got != LoopGuardAllow {
		t.Fatalf("prior error = %s, want allow (error is not success)", got)
	}
	if got := DecideLoopGuard(LoopGuardInput{IsPoll: true, PollIntervalMet: true, PollStatusActive: true}); got != LoopGuardAllow {
		t.Fatalf("valid poll = %s, want allow", got)
	}
	if got := DecideLoopGuard(LoopGuardInput{IsPoll: true, PollIntervalMet: false, PollStatusActive: true}); got != LoopGuardBlock {
		t.Fatalf("poll interval not met = %s, want block", got)
	}
	if got := DecideLoopGuard(LoopGuardInput{IsPoll: true, PollIntervalMet: true, PollStatusActive: false}); got != LoopGuardBlock {
		t.Fatalf("poll status terminal = %s, want block", got)
	}
}

func TestResultHandleRoundTripAndValidation(t *testing.T) {
	handle := ResultHandle{FamilyID: "family-1", ToolName: "search_records", QueryHash: "query-abc", Version: "v1"}
	encoded := EncodeResultHandle(handle)
	decoded, err := DecodeResultHandle(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if decoded != handle {
		t.Fatalf("round trip = %+v, want %+v", decoded, handle)
	}

	// 篡改拒绝。
	if _, err := DecodeResultHandle(encoded[:len(encoded)-1] + "0"); err == nil {
		t.Fatalf("tampered handle should fail")
	}
	if _, err := DecodeResultHandle("garbage"); err == nil {
		t.Fatalf("garbage handle should fail")
	}

	// 换家庭/工具/查询条件拒绝。
	if err := ValidateResultHandle(handle, "family-1", "search_records", "query-abc"); err != nil {
		t.Fatalf("valid handle rejected: %v", err)
	}
	if err := ValidateResultHandle(handle, "family-2", "search_records", "query-abc"); err == nil {
		t.Fatalf("family mismatch should fail")
	}
	if err := ValidateResultHandle(handle, "family-1", "read_record", "query-abc"); err == nil {
		t.Fatalf("tool mismatch should fail")
	}
	if err := ValidateResultHandle(handle, "family-1", "search_records", "query-other"); err == nil {
		t.Fatalf("query mismatch should fail")
	}
}
