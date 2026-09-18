package ask

import (
	"strings"
	"testing"
	"time"
)

func TestControlInstructionsCoversRequiredRules(t *testing.T) {
	text := ControlInstructions()
	if strings.TrimSpace(text) == "" {
		t.Fatal("ControlInstructions() should not be empty")
	}
	for _, keyword := range []string{"安全边界", "权限边界", "动作选择", "回答组织"} {
		if !strings.Contains(text, keyword) {
			t.Fatalf("ControlInstructions() missing section %q", keyword)
		}
	}
	// 关键语义必须存在：不得猜测宠物 ID、急症就医、只读不写。
	for _, keyword := range []string{"不得猜测宠物 ID", "立即就医", "只读工具不修改业务状态"} {
		if !strings.Contains(text, keyword) {
			t.Fatalf("ControlInstructions() missing key rule %q", keyword)
		}
	}
}

func TestBuildRunContextLayers(t *testing.T) {
	snapshot := ContextSnapshot{
		Pet:    PetContext{ID: "pet-1", Name: "旺仔", Breed: "金毛"},
		Events: []ContextEvent{{Tag: "bath", Source: "calendar", Summary: "上周洗过澡", OccurredAt: time.Now()}},
	}
	messages := []ContextMessage{
		{Role: "user", Content: "旺仔最近怎么样？", CreatedAt: time.Now()},
		{Role: "assistant", Content: "旺仔状态不错。", CreatedAt: time.Now()},
	}
	assembly := BuildRunContext("旺仔最近怎么样？", messages, snapshot)

	// 首个块必须是受控指令层，且 Required。
	if len(assembly.Blocks) == 0 {
		t.Fatal("expected non-empty blocks")
	}
	first := assembly.Blocks[0]
	if first.Layer != LayerControlInstructions || first.Kind != "instruction" || !first.Required {
		t.Fatalf("first block should be required control instruction, got %+v", first)
	}
	// 层序严格升序（受控指令 < 参考数据 < 历史 < 当前任务）。
	for i := 1; i < len(assembly.Blocks); i++ {
		if assembly.Blocks[i].Layer < assembly.Blocks[i-1].Layer {
			t.Fatalf("blocks out of layer order at %d: %d after %d", i, assembly.Blocks[i].Layer, assembly.Blocks[i-1].Layer)
		}
	}
	// 当前任务层必须存在且 Required。
	foundCurrent := false
	for _, b := range assembly.Blocks {
		if b.Layer == LayerCurrentTask && b.Kind == "current_turn" {
			foundCurrent = true
			if !b.Required {
				t.Fatal("current_turn block should be Required")
			}
			if b.Text != "旺仔最近怎么样？" {
				t.Fatalf("current_turn text = %q", b.Text)
			}
		}
	}
	if !foundCurrent {
		t.Fatal("missing current_turn block")
	}
}

func TestBuildRunContextCurrentInputNotDuplicatedInHistory(t *testing.T) {
	currentInput := "旺仔最近怎么样？"
	messages := []ContextMessage{
		{Role: "user", Content: currentInput, CreatedAt: time.Now()},
		{Role: "assistant", Content: "旺仔状态不错。", CreatedAt: time.Now()},
	}
	assembly := BuildRunContext(currentInput, messages, ContextSnapshot{})
	currentCount := 0
	historyUserCount := 0
	for _, b := range assembly.Blocks {
		switch {
		case b.Layer == LayerCurrentTask:
			currentCount++
		case b.Layer == LayerHistory && strings.HasPrefix(b.Text, "user: "+currentInput):
			historyUserCount++
		}
	}
	if currentCount != 1 {
		t.Fatalf("current input should appear exactly once in current task layer, got %d", currentCount)
	}
	if historyUserCount != 0 {
		t.Fatalf("current input leaked into history layer %d times", historyUserCount)
	}
	// assistant 历史应保留在历史层。
	historyCount := 0
	for _, b := range assembly.Blocks {
		if b.Layer == LayerHistory {
			historyCount++
		}
	}
	if historyCount != 1 {
		t.Fatalf("history blocks = %d, want 1 (assistant reply)", historyCount)
	}
}

func TestBuildRunContextReferenceData(t *testing.T) {
	snapshot := ContextSnapshot{
		Pet: PetContext{ID: "pet-1", Name: "旺仔", Breed: "金毛", Gender: "公"},
		Events: []ContextEvent{
			{Tag: "bath", Source: "calendar", Summary: "上周洗过澡", OccurredAt: time.Now()},
			{Tag: "vaccine", Source: "calendar", Summary: "上月打过疫苗", OccurredAt: time.Now()},
		},
	}
	assembly := BuildRunContext("x", nil, snapshot)

	var profileCount, recordCount int
	for _, b := range assembly.Blocks {
		if b.Layer != LayerReferenceData {
			continue
		}
		switch b.Kind {
		case "profile":
			profileCount++
			if !strings.Contains(b.Text, "旺仔") || !strings.Contains(b.Text, "金毛") {
				t.Fatalf("profile text missing fields: %q", b.Text)
			}
		case "record":
			recordCount++
		}
	}
	if profileCount != 1 {
		t.Fatalf("profile blocks = %d, want 1", profileCount)
	}
	if recordCount != 2 {
		t.Fatalf("record blocks = %d, want 2", recordCount)
	}
}

func TestBuildRunContextDeduplicatesIdenticalBlocks(t *testing.T) {
	// 相同 Layer/Kind/ObjectID/Version/Position 的块应被去重，保留一份并记录 Rejected。
	dup := ContextBlock{Layer: LayerHistory, Kind: "history", ObjectID: "0", Position: "0", Text: "a"}
	blocks := []ContextBlock{dup, dup}
	assembly := AssembleContext(blocks)
	if len(assembly.Blocks) != 1 {
		t.Fatalf("blocks = %d, want 1 after dedup", len(assembly.Blocks))
	}
	if len(assembly.Rejected) != 1 || assembly.Rejected[0].Reason != "duplicate" {
		t.Fatalf("rejected = %+v, want 1 duplicate", assembly.Rejected)
	}
}

func TestPetProfileText(t *testing.T) {
	if got := petProfileText(PetContext{}); got != "" {
		t.Fatalf("empty pet should yield empty text, got %q", got)
	}
	text := petProfileText(PetContext{ID: "p1", Name: "旺仔", Breed: "金毛", Gender: "公", Sterilized: true, HealthStatus: "良好"})
	for _, part := range []string{"旺仔", "金毛", "公", "已绝育", "良好"} {
		if !strings.Contains(text, part) {
			t.Fatalf("petProfileText missing %q: %q", part, text)
		}
	}
	// 缺资料不填默认值：Birthday 未设置时不应出现"生日"。
	if strings.Contains(text, "生日") {
		t.Fatalf("unset birthday should not appear: %q", text)
	}
}
