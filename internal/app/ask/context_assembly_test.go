package ask

import (
	"strings"
	"testing"
)

func TestEstimateTokens(t *testing.T) {
	if got := EstimateTokens("你好"); got != 2 {
		t.Fatalf("EstimateTokens(你好) = %d, want 2", got)
	}
	if got := EstimateTokens("hello"); got != 5 {
		t.Fatalf("EstimateTokens(hello) = %d, want 5", got)
	}
	if got := EstimateTokens(""); got != 0 {
		t.Fatalf("EstimateTokens(empty) = %d, want 0", got)
	}
}

func TestComputeInputBudget(t *testing.T) {
	// input_budget = min(input_limit, context_limit - output_reserve - margin)
	if got := ComputeInputBudget(10000, 8000, 2048, 512); got != 5440 {
		t.Fatalf("budget = %d, want 5440", got)
	}
	// input_limit 更小时取 input_limit。
	if got := ComputeInputBudget(4000, 8000, 2048, 512); got != 4000 {
		t.Fatalf("budget = %d, want 4000", got)
	}
	// 余量为负时返回 0（不发请求）。
	if got := ComputeInputBudget(10000, 2000, 2048, 512); got != 0 {
		t.Fatalf("budget = %d, want 0", got)
	}
	// 配置缺失返回 0。
	if got := ComputeInputBudget(0, 8000, 2048, 512); got != 0 {
		t.Fatalf("budget = %d, want 0", got)
	}
}

func TestAssembleContextDedupe(t *testing.T) {
	blocks := []ContextBlock{
		{Layer: LayerCurrentTask, Kind: "current_turn", ObjectID: "turn-1", Version: "v1", Text: "旺仔吐了"},
		{Layer: LayerCurrentTask, Kind: "current_turn", ObjectID: "turn-1", Version: "v1", Text: "旺仔吐了"},
		{Layer: LayerHistory, Kind: "history", ObjectID: "msg-1", Version: "v1", Text: "之前查过"},
	}
	asm := AssembleContext(blocks)
	if len(asm.Blocks) != 2 {
		t.Fatalf("dedupe failed, got %d blocks", len(asm.Blocks))
	}
	if len(asm.Rejected) != 1 || asm.Rejected[0].Reason != "duplicate" {
		t.Fatalf("expected 1 duplicate rejection, got %+v", asm.Rejected)
	}
}

func TestAssembleContextDoesNotMergeDifferentObject(t *testing.T) {
	blocks := []ContextBlock{
		{Layer: LayerCurrentTask, Kind: "current_turn", ObjectID: "pet-1", Version: "v1", Text: "旺仔吐了"},
		{Layer: LayerCurrentTask, Kind: "current_turn", ObjectID: "pet-2", Version: "v1", Text: "旺仔吐了"},
	}
	asm := AssembleContext(blocks)
	// 相同文本但不同对象不得合并（文档 5.8）。
	if len(asm.Blocks) != 2 {
		t.Fatalf("different objects must not merge, got %d", len(asm.Blocks))
	}
}

func TestAssembleContextOrdersByLayerThenPriority(t *testing.T) {
	blocks := []ContextBlock{
		{Layer: LayerHistory, Kind: "summary", ObjectID: "sum-1", Version: "v1", Text: "摘要"},
		{Layer: LayerControlInstructions, Kind: "instruction", ObjectID: "rule-1", Version: "v1", Text: "安全规则"},
		{Layer: LayerHistory, Kind: "history", ObjectID: "msg-1", Version: "v1", Text: "历史消息"},
		{Layer: LayerCurrentTask, Kind: "current_turn", ObjectID: "turn-1", Version: "v1", Text: "当前问题"},
	}
	asm := AssembleContext(blocks)
	// 先按逻辑层排序：受控指令(1) 在最前，当前任务(4) 在最后。
	if asm.Blocks[0].Kind != "instruction" {
		t.Fatalf("first should be instruction, got %s", asm.Blocks[0].Kind)
	}
	if asm.Blocks[len(asm.Blocks)-1].Kind != "current_turn" {
		t.Fatalf("current_turn should sort last by layer, got %s", asm.Blocks[len(asm.Blocks)-1].Kind)
	}
	// 层内按优先级：history(priority 2) 在 summary(priority 4) 之前。
	historyIdx, summaryIdx := -1, -1
	for i, b := range asm.Blocks {
		if b.Kind == "history" {
			historyIdx = i
		}
		if b.Kind == "summary" {
			summaryIdx = i
		}
	}
	if historyIdx < 0 || summaryIdx < 0 || historyIdx >= summaryIdx {
		t.Fatalf("history should sort before summary within same layer, history=%d summary=%d", historyIdx, summaryIdx)
	}
}

func TestTrimContextKeepsRequiredAndHighPriority(t *testing.T) {
	blocks := []ContextBlock{
		{Layer: LayerCurrentTask, Kind: "current_turn", ObjectID: "turn-1", Text: "当前问题必须保留", Required: true},
		{Layer: LayerHistory, Kind: "history", ObjectID: "msg-1", Text: "最近消息"},
		{Layer: LayerHistory, Kind: "summary", ObjectID: "sum-1", Text: "低优先级摘要"},
	}
	// 预算只够必须项(8) + 最近消息(4)，摘要(6) 被裁剪。
	kept, err := TrimContext(blocks, 14)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	foundSummary := false
	foundHistory := false
	for _, b := range kept {
		if b.Kind == "summary" {
			foundSummary = true
		}
		if b.Kind == "history" {
			foundHistory = true
		}
	}
	if foundSummary {
		t.Fatal("low-priority summary should be trimmed first")
	}
	if !foundHistory {
		t.Fatal("higher-priority history should be kept")
	}
}

func TestTrimContextErrorsWhenRequiredExceeds(t *testing.T) {
	blocks := []ContextBlock{
		{Layer: LayerCurrentTask, Kind: "current_turn", ObjectID: "turn-1", Text: "当前问题很长很长很长", Required: true},
	}
	if _, err := TrimContext(blocks, 2); err == nil {
		t.Fatal("expected capacity error when required blocks exceed budget")
	}
}

func TestTrimContextKeepsFreshBlocksFirst(t *testing.T) {
	blocks := []ContextBlock{
		{Layer: LayerToolInteractions, Kind: "tool_result", ObjectID: "stale", Text: strings.Repeat("旧", 10)},
		{Layer: LayerToolInteractions, Kind: "tool_result", ObjectID: "fresh", Text: strings.Repeat("新", 10), Fresh: true},
	}
	// 预算只够一块：本步新增的结果先保留，更早的结果先被裁掉。
	kept, err := TrimContext(blocks, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(kept) != 1 || kept[0].ObjectID != "fresh" {
		t.Fatalf("kept = %+v, want the fresh block", kept)
	}
}

func TestTrimContextKeepsInputOrder(t *testing.T) {
	blocks := []ContextBlock{
		{Layer: LayerHistory, Kind: "summary", ObjectID: "sum-1", Text: "摘要"},
		{Layer: LayerHistory, Kind: "history", ObjectID: "msg-1", Text: "历史消息"},
	}
	kept, err := TrimContext(blocks, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 预算充足时两块都保留，输出维持输入顺序：优先级只决定保留哪些内容。
	if len(kept) != 2 || kept[0].ObjectID != "sum-1" || kept[1].ObjectID != "msg-1" {
		t.Fatalf("kept = %+v, want input order preserved", kept)
	}
}
