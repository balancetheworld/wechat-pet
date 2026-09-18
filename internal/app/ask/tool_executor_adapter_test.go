package ask

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"
)

// fakeBusinessRead 记录各只读方法调用次数，返回可控结果，用于测试 ToolExecutorAdapter
// 的整批校验、并行执行、错误与无记录分别表达。
type fakeBusinessRead struct {
	mu sync.Mutex

	resolveCalls  int
	readCalls     int
	searchCalls   int
	aggregateCalls int

	resolveOutcome  PetResolveOutcome
	resolveErr      error
	readOutcome     HealthRecordOutcome
	readErr         error
	searchOutcome   HealthRecordSearchOutcome
	searchErr       error
	aggregateOutcome HealthRecordAggregateOutcome
	aggregateErr    error
}

func (f *fakeBusinessRead) ResolvePet(context.Context, string, string) (PetResolveOutcome, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.resolveCalls++
	return f.resolveOutcome, f.resolveErr
}

func (f *fakeBusinessRead) ReadPetProfile(context.Context, string, string) (PetProfileOutcome, error) {
	return PetProfileOutcome{}, nil
}

func (f *fakeBusinessRead) SearchHealthRecords(context.Context, string, string, HealthRecordSearchQuery) (HealthRecordSearchOutcome, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.searchCalls++
	return f.searchOutcome, f.searchErr
}

func (f *fakeBusinessRead) AggregateHealthRecords(context.Context, string, string, HealthRecordAggregateQuery) (HealthRecordAggregateOutcome, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.aggregateCalls++
	return f.aggregateOutcome, f.aggregateErr
}

func (f *fakeBusinessRead) ReadHealthRecord(context.Context, string, string) (HealthRecordOutcome, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.readCalls++
	return f.readOutcome, f.readErr
}

func (f *fakeBusinessRead) counts() (resolve, read, search, aggregate int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.resolveCalls, f.readCalls, f.searchCalls, f.aggregateCalls
}

func adapterTestCatalog(t *testing.T) *Catalog {
	t.Helper()
	tools := []Tool{
		{Name: "resolve_pet", OperationID: "resolve.pet", ResourceType: ResourcePet, ActionType: ActionResolve, Version: "v1"},
		{Name: "read_record", OperationID: "read.health_record", ResourceType: ResourceHealthRecord, ActionType: ActionRead, Version: "v1"},
		{Name: "search_records", OperationID: "search.health_record", ResourceType: ResourceHealthRecord, ActionType: ActionSearch, Version: "v1"},
		{Name: "aggregate_records", OperationID: "aggregate.health_record", ResourceType: ResourceHealthRecord, ActionType: ActionAggregate, Version: "v1"},
		{Name: "prepare_create", OperationID: "prepare.create", ResourceType: ResourceHealthRecord, ActionType: ActionPrepareCreate, Version: "v1"},
	}
	catalog, err := NewCatalog(tools, nil, "v1", whitespaceTokenizer)
	if err != nil {
		t.Fatal(err)
	}
	return catalog
}

func newTestAdapter(catalog *Catalog, business BusinessReadRepository) *ToolExecutorAdapter {
	return NewToolExecutorAdapter(catalog, business, Filter{}, "family-1")
}

func TestExecuteBatchValidationFailureDoesNotExecute(t *testing.T) {
	catalog := adapterTestCatalog(t)
	business := &fakeBusinessRead{}
	adapter := newTestAdapter(catalog, business)

	batch := ToolBatch{BatchID: "b1", RunID: "r1", AttemptID: "a1", Calls: []ToolCall{
		{ToolCallID: "c1", CallIndex: 0, ToolName: "not_a_tool", ToolVersion: "v1", Arguments: validArguments()},
	}}
	if _, err := adapter.ExecuteBatch(context.Background(), batch); err == nil {
		t.Fatal("invalid batch should fail validation")
	} else if _, ok := err.(*BatchValidationError); !ok {
		t.Fatalf("error should be BatchValidationError, got %T", err)
	}
	if resolve, read, search, aggregate := business.counts(); resolve+read+search+aggregate != 0 {
		t.Fatal("no business method should be invoked on validation failure")
	}
}

func TestExecuteBatchRunsCallsInParallel(t *testing.T) {
	catalog := adapterTestCatalog(t)
	business := &fakeBusinessRead{
		resolveOutcome: PetResolveOutcome{Status: PetResolveResolved, Source: ReadSource{SourceType: "pet", SourceID: "pet-1", Version: "v1"}},
		searchOutcome:  HealthRecordSearchOutcome{Source: ReadSource{SourceType: "record", SourceID: "pet-1", Version: "v1"}},
	}
	adapter := newTestAdapter(catalog, business)

	batch := ToolBatch{BatchID: "b1", RunID: "r1", AttemptID: "a1", Calls: []ToolCall{
		{ToolCallID: "c1", CallIndex: 0, ToolName: "resolve_pet", ToolVersion: "v1", Arguments: json.RawMessage(`{"query":"旺仔"}`)},
		{ToolCallID: "c2", CallIndex: 1, ToolName: "search_records", ToolVersion: "v1", Arguments: json.RawMessage(`{"pet_id":"pet-1","category":"medical"}`)},
	}}
	results, err := adapter.ExecuteBatch(context.Background(), batch)
	if err != nil {
		t.Fatalf("ExecuteBatch error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("results = %d, want 2", len(results))
	}
	for i, r := range results {
		if r.Status != ToolResultOK {
			t.Fatalf("result[%d] status = %s, want ok (%+v)", i, r.Status, r)
		}
		if r.CallIndex != i {
			t.Fatalf("result[%d] call_index = %d", i, r.CallIndex)
		}
		if r.CompletedAt.Before(r.QueuedAt) {
			t.Fatalf("result[%d] completed before queued", i)
		}
	}
	resolve, _, search, _ := business.counts()
	if resolve != 1 || search != 1 {
		t.Fatalf("resolve=%d search=%d, want 1 each", resolve, search)
	}
	// 检索结果来源透传。
	if results[1].Source.SourceType != "record" {
		t.Fatalf("search source = %+v", results[1].Source)
	}
}

func TestExecuteBatchReadRecordNotFoundReturnsEmpty(t *testing.T) {
	catalog := adapterTestCatalog(t)
	business := &fakeBusinessRead{readErr: ErrHealthRecordNotFound}
	adapter := newTestAdapter(catalog, business)

	batch := ToolBatch{BatchID: "b1", RunID: "r1", AttemptID: "a1", Calls: []ToolCall{
		{ToolCallID: "c1", CallIndex: 0, ToolName: "read_record", ToolVersion: "v1", Arguments: json.RawMessage(`{"record_id":"r-missing"}`)},
	}}
	results, err := adapter.ExecuteBatch(context.Background(), batch)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %d, want 1", len(results))
	}
	r := results[0]
	if r.Status != ToolResultOK || r.Error != nil {
		t.Fatalf("not-found should be empty success, got %+v", r)
	}
	if r.Completeness != CompletenessComplete {
		t.Fatalf("completeness = %s, want complete", r.Completeness)
	}
}

func TestExecuteBatchServiceErrorSeparatedFromEmpty(t *testing.T) {
	catalog := adapterTestCatalog(t)
	business := &fakeBusinessRead{searchErr: errors.New("db down")}
	adapter := newTestAdapter(catalog, business)

	batch := ToolBatch{BatchID: "b1", RunID: "r1", AttemptID: "a1", Calls: []ToolCall{
		{ToolCallID: "c1", CallIndex: 0, ToolName: "search_records", ToolVersion: "v1", Arguments: json.RawMessage(`{"pet_id":"pet-1"}`)},
	}}
	results, err := adapter.ExecuteBatch(context.Background(), batch)
	if err != nil {
		t.Fatal(err)
	}
	r := results[0]
	if r.Status != ToolResultError || r.Error == nil {
		t.Fatalf("service error should produce error result, got %+v", r)
	}
	if r.Error.Category != ToolErrServiceError || !r.Error.Retryable {
		t.Fatalf("error should be retryable service_error, got %+v", r.Error)
	}
}

func TestExecuteBatchUnsupportedActionReturnsLimit(t *testing.T) {
	catalog := adapterTestCatalog(t)
	business := &fakeBusinessRead{}
	adapter := newTestAdapter(catalog, business)

	batch := ToolBatch{BatchID: "b1", RunID: "r1", AttemptID: "a1", Calls: []ToolCall{
		{ToolCallID: "c1", CallIndex: 0, ToolName: "prepare_create", ToolVersion: "v1", Arguments: validArguments()},
	}}
	results, err := adapter.ExecuteBatch(context.Background(), batch)
	if err != nil {
		t.Fatal(err)
	}
	r := results[0]
	if r.Status != ToolResultError || r.Error == nil {
		t.Fatalf("unsupported action should produce error result, got %+v", r)
	}
	if r.Error.Category != ToolErrInvalidTool {
		t.Fatalf("category = %s, want invalid_tool", r.Error.Category)
	}
	if r.Error.Retryable {
		t.Fatal("unsupported action should not be retryable")
	}
}

func TestExecuteBatchAggregateValid(t *testing.T) {
	catalog := adapterTestCatalog(t)
	business := &fakeBusinessRead{
		aggregateOutcome: HealthRecordAggregateOutcome{RecordCount: 3, OccurrenceCount: 3, OccurrenceKnown: true, Source: ReadSource{SourceType: "record"}},
	}
	adapter := newTestAdapter(catalog, business)

	batch := ToolBatch{BatchID: "b1", RunID: "r1", AttemptID: "a1", Calls: []ToolCall{
		{ToolCallID: "c1", CallIndex: 0, ToolName: "aggregate_records", ToolVersion: "v1", Arguments: json.RawMessage(`{"pet_id":"pet-1","medical_type":"vomit","start_at":"2026-09-01T00:00:00Z"}`)},
	}}
	results, err := adapter.ExecuteBatch(context.Background(), batch)
	if err != nil {
		t.Fatal(err)
	}
	r := results[0]
	if r.Status != ToolResultOK || r.Error != nil {
		t.Fatalf("aggregate should succeed, got %+v", r)
	}
	if _, _, _, aggregate := business.counts(); aggregate != 1 {
		t.Fatalf("aggregate calls = %d, want 1", aggregate)
	}
}

func TestOptionalTime(t *testing.T) {
	now := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	parsed, err := optionalTime(map[string]any{"start_at": "2026-09-01T00:00:00Z"}, "start_at")
	if err != nil || !parsed.Equal(now) {
		t.Fatalf("optionalTime = %v, %v", parsed, err)
	}
	if _, err := optionalTime(map[string]any{"start_at": "not-a-time"}, "start_at"); err == nil {
		t.Fatal("invalid time should error")
	}
	if _, err := optionalTime(map[string]any{"start_at": 123}, "start_at"); err == nil {
		t.Fatal("non-string time should error")
	}
	if got, err := optionalTime(map[string]any{}, "start_at"); err != nil || !got.IsZero() {
		t.Fatalf("missing key should yield zero time, got %v %v", got, err)
	}
}
