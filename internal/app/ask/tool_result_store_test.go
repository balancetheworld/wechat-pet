package ask

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func newToolResultRepository(t *testing.T) *SQLRepository {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	createAskSchema(t, db)
	repository, err := NewRepository(db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	return repository
}

func toolResultTestCall(callID string) ToolCall {
	return ToolCall{ToolCallID: callID, CallIndex: 0, ToolName: petRosterToolName, ToolVersion: DefaultToolVersion, Arguments: json.RawMessage(`{}`)}
}

func toolResultTestRecord(t *testing.T, sessionID, runID, callID string) ToolResultRecord {
	t.Helper()
	call := toolResultTestCall(callID)
	result := ToolResult{
		ToolCallID:   callID,
		Status:       ToolResultOK,
		Completeness: CompletenessComplete,
		Data:         PetListOutcome{Pets: []PetListItem{{PetID: "pet-1", Name: "团子"}}},
		Source:       ReadSource{SourceType: "pet", SourceID: "family-1", Version: "v1"},
	}
	record, err := newToolResultRecord(sessionID, runID, call, result, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return record
}

func TestToolResultStoreRoundTrip(t *testing.T) {
	repository := newToolResultRepository(t)
	record := toolResultTestRecord(t, "session-1", "run-1", "c1")
	if err := repository.SaveToolResults(context.Background(), []ToolResultRecord{record}); err != nil {
		t.Fatal(err)
	}
	loaded, err := repository.GetToolResult(context.Background(), record.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ToolCallID != "c1" || loaded.Status != ToolResultOK || loaded.Fingerprint != record.Fingerprint || loaded.CreatedAt.IsZero() {
		t.Fatalf("loaded record = %+v", loaded)
	}
	if len(loaded.Evidence) != 1 || loaded.Evidence[0].SourceType != "pet" {
		t.Fatalf("loaded evidence = %+v", loaded.Evidence)
	}
	if !strings.Contains(loaded.Text, "团子") {
		t.Fatalf("loaded text = %q", loaded.Text)
	}
	if block := loaded.block(); block.Kind != "tool_result" || block.ObjectID != "c1" {
		t.Fatalf("block = %+v", block)
	}
	listed, err := repository.ListRunToolResults(context.Background(), "run-1")
	if err != nil || len(listed) != 1 {
		t.Fatalf("listed = %+v, error = %v", listed, err)
	}
	found, err := repository.FindReusableToolResult(context.Background(), "session-1", record.Fingerprint)
	if err != nil || found.ID != record.ID {
		t.Fatalf("reusable = %+v, error = %v", found, err)
	}
	if _, err := repository.GetToolResult(context.Background(), "missing"); err != ErrToolResultNotFound {
		t.Fatalf("missing record error = %v", err)
	}
}

func TestReusableToolResultRequiresUnchangedSource(t *testing.T) {
	repository := newToolResultRepository(t)
	record := toolResultTestRecord(t, "session-1", "run-1", "c1")
	if err := repository.SaveToolResults(context.Background(), []ToolResultRecord{record}); err != nil {
		t.Fatal(err)
	}
	call := toolResultTestCall("c2")
	ctx := context.Background()
	// 来源集合版本未登记：无法证明仍有效，必须重查。
	if _, reused, err := reusableToolResult(ctx, repository, repository, "session-1", call, ActionRead); err != nil || reused {
		t.Fatalf("unregistered source should not be reused: reused=%v error=%v", reused, err)
	}
	if err := repository.UpsertSourceVersion(ctx, "pet", "family-1", "v1"); err != nil {
		t.Fatal(err)
	}
	if _, reused, err := reusableToolResult(ctx, repository, repository, "session-1", call, ActionRead); err != nil || !reused {
		t.Fatalf("unchanged source should be reused: reused=%v error=%v", reused, err)
	}
	// 来源变化：既有结果失效，重新读取。
	if err := repository.UpsertSourceVersion(ctx, "pet", "family-1", "v2"); err != nil {
		t.Fatal(err)
	}
	if _, reused, err := reusableToolResult(ctx, repository, repository, "session-1", call, ActionRead); err != nil || reused {
		t.Fatalf("changed source should not be reused: reused=%v error=%v", reused, err)
	}
	// 跨 Session 与写入准备一律不复用。
	if _, reused, _ := reusableToolResult(ctx, repository, repository, "session-2", call, ActionRead); reused {
		t.Fatal("cross-session result must not be reused")
	}
	if _, reused, _ := reusableToolResult(ctx, repository, repository, "session-1", call, ActionPrepareCreate); reused {
		t.Fatal("write preparation result must not be reused")
	}
}

func TestRunDecisionLoopReplaysPersistedToolResults(t *testing.T) {
	repository := newToolResultRepository(t)
	call := callRecordsWithoutCoverage(petRosterToolName, `{}`)
	ctx := context.Background()
	first := &scriptedModel{responses: [][]ProtocolRecord{call, requestInputRecords()}}
	outcome, err := runDecisionLoop(ctx, first, &fakeToolExecutor{resultData: PetListOutcome{Pets: []PetListItem{{PetID: "pet-1", Name: "团子"}}}}, StepInput{SessionID: "session-1", RunID: "run-1"}, 4, repository, nil, "", nil, nil, nil)
	if err != nil {
		t.Fatalf("first loop failed: %v", err)
	}
	if outcome.Action != ActionRequestInput {
		t.Fatalf("first loop action = %s, want request_input", outcome.Action)
	}
	records, err := repository.ListRunToolResults(ctx, "run-1")
	if err != nil || len(records) != 1 {
		t.Fatalf("records = %+v, error = %v", records, err)
	}
	if len(records[0].TaskKeys) != 1 || records[0].TaskKeys[0] != stableTaskItemID("run-1", "t1") {
		t.Fatalf("record task keys = %+v", records[0].TaskKeys)
	}
	// 同一 Run 重新进入循环（用户回答追问后恢复）：未终结任务仍需要的工具结果按原正文重放，
	// 且作为「仍有效的工具结果」进入参考数据层，不冒充当前步骤的工具交互。
	resumed := &scriptedModel{responses: [][]ProtocolRecord{finalAnswerRecords()}}
	if _, err := runDecisionLoop(ctx, resumed, &fakeToolExecutor{}, StepInput{SessionID: "session-1", RunID: "run-1"}, 2, repository, outcome.TaskItems, "", nil, nil, nil); err != nil {
		t.Fatalf("resumed loop failed: %v", err)
	}
	found := false
	for _, block := range resumed.inputs[0].Blocks {
		if block.Kind == "tool_result" && block.Layer == LayerReferenceData && strings.Contains(block.Text, "团子") {
			found = true
		}
	}
	if !found {
		t.Fatalf("resumed context missing persisted tool result: %+v", resumed.inputs[0].Blocks)
	}
}

func TestRunDecisionLoopToleratesMissingToolResultTable(t *testing.T) {
	repository := newToolResultRepository(t)
	// 模拟「代码先发布、迁移未执行」的部署环境：结果持久化依赖的库表缺失时，
	// 读写降级为尽力而为，决策循环必须与新增持久化之前一样正常完成。
	if _, err := repository.db.Exec("DROP TABLE ask_tool_results"); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.ListRunToolResults(context.Background(), "run-1"); err == nil {
		t.Fatal("tool result store is still reachable after dropping the table")
	}
	model := &scriptedModel{responses: [][]ProtocolRecord{finalAnswerRecords()}}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	outcome, err := runDecisionLoop(context.Background(), model, &fakeToolExecutor{}, StepInput{SessionID: "session-1", RunID: "run-1"}, 3, repository, nil, "", nil, nil, logger)
	if err != nil {
		t.Fatalf("loop failed without tool result table: %v", err)
	}
	if outcome.Action != ActionFinalAnswer {
		t.Fatalf("action = %s, want final_answer", outcome.Action)
	}
}

// reuseToolExecutor 让批次内每个调用都命中既有结果复用，返回携带结果句柄的复用结果。
type reuseToolExecutor struct {
	record ToolResultRecord
}

func (e reuseToolExecutor) ExecuteBatch(_ context.Context, batch ToolBatch) ([]ToolResult, error) {
	results := make([]ToolResult, 0, len(batch.Calls))
	for _, call := range batch.Calls {
		results = append(results, reusedToolResult(call, e.record))
	}
	return results, nil
}

func TestRunDecisionLoopReusesStoredResultWithoutDuplicateBlock(t *testing.T) {
	repository := newToolResultRepository(t)
	// 已落库的旧结果：正文与证据来自上一次运行，记录里的 tool_call_id 是旧调用。
	record := toolResultTestRecord(t, "session-1", "run-1", "c-old")
	if err := repository.SaveToolResults(context.Background(), []ToolResultRecord{record}); err != nil {
		t.Fatal(err)
	}
	model := &scriptedModel{responses: [][]ProtocolRecord{callRecordsWithoutCoverage(petRosterToolName, `{}`), finalAnswerRecords()}}
	outcome, err := runDecisionLoop(context.Background(), model, reuseToolExecutor{record: record}, StepInput{SessionID: "session-1", RunID: "run-1"}, 3, repository, nil, "", nil, nil, nil)
	if err != nil {
		t.Fatalf("loop failed: %v", err)
	}
	if outcome.Action != ActionFinalAnswer {
		t.Fatalf("action = %s, want final_answer", outcome.Action)
	}
	matches := 0
	for _, block := range model.inputs[len(model.inputs)-1].Blocks {
		if block.Kind != "tool_result" || !strings.Contains(block.Text, "团子") {
			continue
		}
		matches++
		if block.ObjectID != "c1" || block.Layer != LayerToolInteractions {
			t.Fatalf("reused block = %+v, want current call key in tool interactions layer", block)
		}
	}
	if matches != 1 {
		t.Fatalf("reused tool result blocks = %d, want 1", matches)
	}
}

func TestRunDecisionLoopDoesNotChargeToolBudgetForReusedCalls(t *testing.T) {
	repository := newToolResultRepository(t)
	record := toolResultTestRecord(t, "session-1", "run-1", "c-old")
	if err := repository.SaveToolResults(context.Background(), []ToolResultRecord{record}); err != nil {
		t.Fatal(err)
	}
	if err := repository.EnsureBudget(context.Background(), BudgetRun, "run-1", BudgetLimits{MaxModelCalls: 4, MaxToolCalls: 20, MaxTokens: 100000, MaxDurationMillis: 10000, MaxConcurrency: 1}); err != nil {
		t.Fatal(err)
	}
	model := &scriptedModel{responses: [][]ProtocolRecord{callRecordsWithoutCoverage(petRosterToolName, `{}`), finalAnswerRecords()}}
	if _, err := runDecisionLoop(context.Background(), model, reuseToolExecutor{record: record}, StepInput{SessionID: "session-1", RunID: "run-1"}, 3, repository, nil, "", repository, nil, nil); err != nil {
		t.Fatal(err)
	}
	ledger, err := repository.GetBudget(context.Background(), BudgetRun, "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if ledger.Used.ToolCalls != 0 || ledger.Reserved.ToolCalls != 0 {
		t.Fatalf("tool budget = used %d reserved %d, want no charge for reused calls", ledger.Used.ToolCalls, ledger.Reserved.ToolCalls)
	}
}

func TestReplayableToolResultsSkipTerminalTasks(t *testing.T) {
	records := []ToolResultRecord{
		{ToolCallID: "c1", TaskKeys: []string{"run-1:t1"}},
		{ToolCallID: "c2", TaskKeys: []string{"run-1:t2"}},
		{ToolCallID: "c3"},
	}
	taskItems := []TaskItem{
		{TaskItemID: "run-1:t1", Outcome: OutcomeAnswered},
		{TaskItemID: "run-1:t2", Outcome: OutcomeNeedsInput},
	}
	kept := replayableToolResults(records, taskItems)
	if len(kept) != 2 {
		t.Fatalf("kept = %+v, want only unfinished task result and unlinked result", kept)
	}
	if kept[0].ToolCallID != "c2" || kept[1].ToolCallID != "c3" {
		t.Fatalf("kept tools = %s, %s", kept[0].ToolCallID, kept[1].ToolCallID)
	}
	// 任务清单为空时无法判断关联，保守全部保留。
	if all := replayableToolResults(records, nil); len(all) != 3 {
		t.Fatalf("unknown task scope should keep every record, got %d", len(all))
	}
}

// countingListBusiness 记录宠物清单读取次数，并让每次读取结果不同，
// 用于区分「真实执行」与「复用既有结果」。
type countingListBusiness struct {
	fakeBusinessRead
	listCalls int
}

func (c *countingListBusiness) ListFamilyPets(context.Context, string) (PetListOutcome, error) {
	c.listCalls++
	return PetListOutcome{
		Pets:   []PetListItem{{PetID: "pet-1", Name: fmt.Sprintf("第%d次读取", c.listCalls)}},
		Source: ReadSource{SourceType: "pet", SourceID: "family-1", Version: "v1"},
	}, nil
}

func TestToolExecutorAdapterReusesStoredResult(t *testing.T) {
	repository := newToolResultRepository(t)
	catalog, err := DefaultCatalog(DefaultToolVersion)
	if err != nil {
		t.Fatal(err)
	}
	business := &countingListBusiness{}
	ctx := context.Background()
	scope := func(runID string) ToolExecutionScope {
		return ToolExecutionScope{SessionID: "session-1", RunID: runID, Results: repository, Sources: repository}
	}
	call := toolResultTestCall("c1")
	first := NewToolExecutorAdapter(catalog, business, prepareFilter(), "family-1", scope("run-1"))
	results, err := first.ExecuteBatch(ctx, ToolBatch{Calls: []ToolCall{call}})
	if err != nil {
		t.Fatalf("first batch: %v", err)
	}
	if results[0].Reused {
		t.Fatal("first execution must not be a reuse")
	}
	record, err := newToolResultRecord("session-1", "run-1", call, results[0], time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.SaveToolResults(ctx, []ToolResultRecord{record}); err != nil {
		t.Fatal(err)
	}
	if err := repository.UpsertSourceVersion(ctx, record.Source.SourceType, record.Source.SourceID, record.Source.Version); err != nil {
		t.Fatal(err)
	}
	// 同一 Session 的新 Run、相同调用：直接复用，不再读业务库。
	second := NewToolExecutorAdapter(catalog, business, prepareFilter(), "family-1", scope("run-2"))
	results, err = second.ExecuteBatch(ctx, ToolBatch{Calls: []ToolCall{toolResultTestCall("c2")}})
	if err != nil {
		t.Fatalf("second batch: %v", err)
	}
	if !results[0].Reused || results[0].ResultHandle != record.ID {
		t.Fatalf("second batch should reuse stored result: %+v", results[0])
	}
	if business.listCalls != 1 {
		t.Fatalf("business read calls = %d, want 1", business.listCalls)
	}
	// 来源集合版本变化：既有结果失效，必须重新读取。
	if err := repository.UpsertSourceVersion(ctx, record.Source.SourceType, record.Source.SourceID, "v2"); err != nil {
		t.Fatal(err)
	}
	third := NewToolExecutorAdapter(catalog, business, prepareFilter(), "family-1", scope("run-3"))
	results, err = third.ExecuteBatch(ctx, ToolBatch{Calls: []ToolCall{toolResultTestCall("c3")}})
	if err != nil {
		t.Fatalf("third batch: %v", err)
	}
	if results[0].Reused {
		t.Fatalf("stale source must not be reused: %+v", results[0])
	}
	if business.listCalls != 2 {
		t.Fatalf("business read calls = %d, want 2", business.listCalls)
	}
}
