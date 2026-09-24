package ask

import (
	"context"
	"encoding/json"
	"testing"
)

// 本文件锁定工具注册表（tool_registry.go）与 ToolExecutorAdapter、BusinessReadRepository
// 的一致性：工具名称、动作/资源类型、参数名三处必须一一对应，任何单方面改名都会让测试失败。

// toolByName 在工具集合中按 name 查找工具。
func toolByName(t *testing.T, tools []Tool, name string) Tool {
	t.Helper()
	for _, tool := range tools {
		if tool.Name == name {
			return tool
		}
	}
	t.Fatalf("tool %q not found in DefaultTools", name)
	return Tool{}
}

// assertParamNames 断言工具声明的参数名集合与期望完全一致（契约锁定）。
func assertParamNames(t *testing.T, tool Tool, want ...string) {
	t.Helper()
	got := tool.ParameterNames()
	if len(got) != len(want) {
		t.Fatalf("%s parameters = %v, want %v", tool.Name, got, want)
	}
	wantSet := make(map[string]struct{}, len(want))
	for _, w := range want {
		wantSet[w] = struct{}{}
	}
	for _, g := range got {
		if _, ok := wantSet[g]; !ok {
			t.Fatalf("%s has unexpected parameter %q (want %v)", tool.Name, g, want)
		}
	}
}

func TestDefaultToolsAreValid(t *testing.T) {
	tools := DefaultTools()
	if len(tools) != 14 {
		t.Fatalf("DefaultTools() = %d tools, want 14", len(tools))
	}
	catalog, err := NewCatalog(tools, nil, DefaultToolVersion, nil)
	if err != nil {
		t.Fatalf("NewCatalog(DefaultTools) error: %v", err)
	}
	if catalog.Version() != DefaultToolVersion {
		t.Fatalf("catalog version = %q, want %q", catalog.Version(), DefaultToolVersion)
	}
	if catalog.Digest() == "" {
		t.Fatal("catalog digest should not be empty")
	}
	for _, tool := range tools {
		if err := tool.Validate(); err != nil {
			t.Fatalf("tool %q invalid: %v", tool.Name, err)
		}
		if IsReadOnly(tool.ActionType) {
			if tool.RequiresConfirmation {
				t.Fatalf("read-only tool %q should not require confirmation", tool.Name)
			}
			continue
		}
		// 写入准备工具只形成待确认预览，必须声明需要用户确认且使用新身份幂等策略。
		if !tool.RequiresConfirmation {
			t.Fatalf("write preparation tool %q must require confirmation", tool.Name)
		}
		if tool.IdempotencyPolicy != IdempotencyNewIdentity {
			t.Fatalf("write preparation tool %q idempotency = %q, want %q", tool.Name, tool.IdempotencyPolicy, IdempotencyNewIdentity)
		}
		if len(tool.Parameters) == 0 || !json.Valid(tool.Parameters) {
			t.Fatalf("tool %q parameters must be valid JSON Schema", tool.Name)
		}
		if len(tool.OutputSchema) == 0 || !json.Valid(tool.OutputSchema) {
			t.Fatalf("tool %q output_schema must be valid JSON", tool.Name)
		}
	}
}

// TestDefaultToolParameterNames 锁定工具参数名与 ToolExecutorAdapter 分发一致。
// adapter 分别读取 resolvePet 的 query、readByResource 的 pet_id/record_id、
// searchRecords 的 healthRecordSearchQuery、aggregateRecords 的 healthRecordAggregateQuery。
func TestDefaultToolParameterNames(t *testing.T) {
	tools := DefaultTools()

	assertParamNames(t, toolByName(t, tools, "resolve_pet"), "query")
	assertParamNames(t, toolByName(t, tools, "read_pet_profile"), "pet_id")
	assertParamNames(t, toolByName(t, tools, "read_health_record"), "record_id")
	assertParamNames(t, toolByName(t, tools, "search_health_records"),
		"pet_id", "category", "medical_type", "start_at", "end_at", "limit", "cursor")
	assertParamNames(t, toolByName(t, tools, "aggregate_health_records"),
		"pet_id", "category", "medical_type", "custom_medical_type", "start_at", "end_at")
}

// TestDefaultToolsMatchBusinessReadPorts 锁定工具的动作/资源类型与五方法一一对应。
func TestDefaultToolsMatchBusinessReadPorts(t *testing.T) {
	tools := DefaultTools()

	expect := map[string]struct {
		action   ActionType
		resource ResourceType
	}{
		"list_family_pets":           {ActionRead, ResourcePet},
		"resolve_pet":                {ActionResolve, ResourcePet},
		"read_pet_profile":           {ActionRead, ResourcePetProfile},
		"search_health_records":      {ActionSearch, ResourceHealthRecord},
		"aggregate_health_records":   {ActionAggregate, ResourceHealthRecord},
		"read_health_record":         {ActionRead, ResourceHealthRecord},
		"create_calendar_record":     {ActionPrepareCreate, ResourceHealthRecord},
		"update_calendar_record":     {ActionPrepareUpdate, ResourceHealthRecord},
		"update_pet_profile":         {ActionPrepareUpdate, ResourcePetProfile},
		"complete_calendar_reminder": {ActionPrepareUpdate, ResourceHealthRecord},
		"list_calendar_records":      {ActionRead, ResourceHealthRecord},
		"list_reminders":             {ActionRead, ResourceHealthRecord},
		"update_pet_health":          {ActionPrepareUpdate, ResourcePetProfile},
		"create_pet":                 {ActionPrepareCreate, ResourcePet},
	}
	if len(tools) != len(expect) {
		t.Fatalf("tool count = %d, expect %d", len(tools), len(expect))
	}
	for _, tool := range tools {
		want, ok := expect[tool.Name]
		if !ok {
			t.Fatalf("unexpected tool %q in DefaultTools", tool.Name)
		}
		if tool.ActionType != want.action {
			t.Fatalf("%s action = %q, want %q", tool.Name, tool.ActionType, want.action)
		}
		if tool.ResourceType != want.resource {
			t.Fatalf("%s resource = %q, want %q", tool.Name, tool.ResourceType, want.resource)
		}
	}
}

// TestDefaultCatalogRecall 用确定性分词器验证 BM25F 与别名兜底召回。
func TestDefaultCatalogRecall(t *testing.T) {
	catalog, err := NewCatalog(DefaultTools(), nil, DefaultToolVersion, whitespaceTokenizer)
	if err != nil {
		t.Fatal(err)
	}

	// BM25F 命中：name/operation_id 含 "search"。
	result := catalog.RecallTools("search health records", Filter{}, 0)
	if len(result.Matches) == 0 || result.Matches[0].Tool.Name != "search_health_records" {
		t.Fatalf("search recall = %+v, want search_health_records", result.Matches)
	}

	// 别名兜底：query 包含中文别名子串「识别宠物」。
	result = catalog.RecallTools("帮我识别宠物", Filter{}, 0)
	if !result.UsedFallback {
		t.Fatal("别名命中应触发一次兜底检索")
	}
	if len(result.Matches) == 0 || result.Matches[0].Tool.Name != "resolve_pet" {
		t.Fatalf("alias fallback = %+v, want resolve_pet", result.Matches)
	}
}

// TestDefaultCatalogEmergencySkillsEmpty 一期未注册 Skill，急症安全规则不伪造。
func TestDefaultCatalogEmergencySkillsEmpty(t *testing.T) {
	catalog, err := DefaultCatalog(DefaultToolVersion)
	if err != nil {
		t.Fatal(err)
	}
	if got := catalog.EmergencySkills(); len(got) != 0 {
		t.Fatalf("EmergencySkills() = %d, want 0（Skill 尚未注册）", len(got))
	}
	if got := catalog.RecallSkills("中毒", Filter{}); len(got) != 0 {
		t.Fatalf("RecallSkills() = %d, want 0（Skill 尚未注册）", len(got))
	}
}

// TestDefaultCatalogExecuteBatch 端到端契约：默认目录 + 执行适配器，验证参数名对齐后
// 真实分发到 BusinessReadRepository 的正确方法。
func TestDefaultCatalogExecuteBatch(t *testing.T) {
	catalog, err := DefaultCatalog(DefaultToolVersion)
	if err != nil {
		t.Fatal(err)
	}
	business := &fakeBusinessRead{
		resolveOutcome: PetResolveOutcome{Status: PetResolveResolved, Source: ReadSource{SourceType: "pet", SourceID: "pet-1", Version: "v1"}},
		searchOutcome:  HealthRecordSearchOutcome{Source: ReadSource{SourceType: "record", SourceID: "pet-1", Version: "v1"}},
	}
	adapter := NewToolExecutorAdapter(catalog, business, Filter{}, "family-1")

	batch := ToolBatch{BatchID: "b1", RunID: "r1", AttemptID: "a1", Calls: []ToolCall{
		{ToolCallID: "c1", CallIndex: 0, ToolName: "resolve_pet", ToolVersion: DefaultToolVersion, Arguments: json.RawMessage(`{"query":"旺仔"}`)},
		{ToolCallID: "c2", CallIndex: 1, ToolName: "search_health_records", ToolVersion: DefaultToolVersion, Arguments: json.RawMessage(`{"pet_id":"pet-1","category":"medical"}`)},
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
	}
	resolve, _, search, _ := business.counts()
	if resolve != 1 || search != 1 {
		t.Fatalf("resolve=%d search=%d, want 1 each（参数名必须与 adapter 分发一致）", resolve, search)
	}
}
