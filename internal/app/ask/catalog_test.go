package ask

import (
	"encoding/json"
	"strings"
	"testing"
)

// whitespaceTokenizer：按空白切分的简单分词器，用于确定性测试（不依赖 gse 词典）。
func whitespaceTokenizer(text string) []string {
	return strings.Fields(text)
}

func TestValidResourceAndActionType(t *testing.T) {
	validResources := []ResourceType{ResourcePet, ResourcePetProfile, ResourceHealthRecord, ResourceHealthSummary, ResourceConversation}
	for _, r := range validResources {
		if !ValidResourceType(r) {
			t.Fatalf("ValidResourceType(%q) = false, want true", r)
		}
	}
	if ValidResourceType("unknown") {
		t.Fatal("ValidResourceType(unknown) = true, want false")
	}
	validActions := []ActionType{ActionResolve, ActionRead, ActionSearch, ActionAggregate, ActionPrepareCreate, ActionPrepareUpdate, ActionVerify}
	for _, a := range validActions {
		if !ValidActionType(a) {
			t.Fatalf("ValidActionType(%q) = false, want true", a)
		}
	}
	if ValidActionType("delete") {
		t.Fatal("ValidActionType(delete) = true, want false")
	}
}

func TestIsReadOnly(t *testing.T) {
	for _, a := range []ActionType{ActionResolve, ActionRead, ActionSearch, ActionAggregate} {
		if !IsReadOnly(a) {
			t.Fatalf("IsReadOnly(%q) = false, want true", a)
		}
	}
	for _, a := range []ActionType{ActionPrepareCreate, ActionPrepareUpdate, ActionVerify} {
		if IsReadOnly(a) {
			t.Fatalf("IsReadOnly(%q) = true, want false", a)
		}
	}
}

func TestBM25FActionTypeHasHighestWeight(t *testing.T) {
	tools := []Tool{
		{Name: "op_a", OperationID: "op_a", ResourceType: ResourcePet, ActionType: ActionSearch, Version: "1.0"},
		{Name: "op_b", OperationID: "op_b", ResourceType: ResourcePet, ActionType: ActionRead, UseCases: []string{"search"}, Version: "1.0"},
	}
	idx := NewBM25FIndex(tools, whitespaceTokenizer)
	matches := idx.Search("search", 0)
	if len(matches) != 2 {
		t.Fatalf("matches = %d, want 2", len(matches))
	}
	if matches[0].Tool.OperationID != "op_a" {
		t.Fatalf("top match = %s, want op_a（action_type 权重应高于 use_cases）", matches[0].Tool.OperationID)
	}
	if matches[0].Score <= matches[1].Score {
		t.Fatalf("op_a score %v 应大于 op_b score %v", matches[0].Score, matches[1].Score)
	}
}

func TestBM25FChineseTokenizeSmoke(t *testing.T) {
	tokens := DefaultTokenizer()("宠物呕吐次数统计")
	if len(tokens) == 0 {
		t.Fatal("中文分词返回空")
	}
	if len(tokens) < 2 {
		t.Fatalf("期望分词出多个 token，实际 %v", tokens)
	}
}

func TestCatalogPermissionFilter(t *testing.T) {
	tools := []Tool{
		{Name: "read_pet", OperationID: "read_pet", ResourceType: ResourcePet, ActionType: ActionRead, Version: "1.0"},
		{Name: "read_record", OperationID: "read_record", ResourceType: ResourceHealthRecord, ActionType: ActionRead, Version: "1.0"},
	}
	catalog, err := NewCatalog(tools, nil, "1.0", whitespaceTokenizer)
	if err != nil {
		t.Fatal(err)
	}
	filter := Filter{AllowedResources: map[ResourceType]struct{}{ResourcePet: {}}}
	result := catalog.RecallTools("read", filter, 0)
	if result.UsedFallback {
		t.Fatal("不应触发兜底")
	}
	if len(result.Matches) != 1 || result.Matches[0].Tool.OperationID != "read_pet" {
		t.Fatalf("权限过滤后 matches = %+v，应只保留 read_pet", result.Matches)
	}
}

func TestCatalogAbilityFilter(t *testing.T) {
	tools := []Tool{
		{Name: "read_pet", OperationID: "read_pet", ResourceType: ResourcePet, ActionType: ActionRead, Version: "1.0"},
		{Name: "prepare_record", OperationID: "prepare_record", ResourceType: ResourceHealthRecord, ActionType: ActionPrepareCreate, Version: "1.0"},
	}
	catalog, err := NewCatalog(tools, nil, "1.0", whitespaceTokenizer)
	if err != nil {
		t.Fatal(err)
	}
	// 能力只支持只读动作，过滤 prepare_create
	filter := Filter{AllowedActions: map[ActionType]struct{}{ActionRead: {}, ActionSearch: {}, ActionResolve: {}, ActionAggregate: {}}}
	result := catalog.RecallTools("read", filter, 0)
	if len(result.Matches) != 1 || result.Matches[0].Tool.OperationID != "read_pet" {
		t.Fatalf("能力过滤后 matches = %+v，应只保留 read_pet", result.Matches)
	}
}

func TestCatalogFallbackByAlias(t *testing.T) {
	tools := []Tool{
		{Name: "aggregate_symptom", OperationID: "aggregate_symptom", AliasesZH: []string{"呕吐次数统计"}, ResourceType: ResourceHealthRecord, ActionType: ActionAggregate, Version: "1.0"},
	}
	catalog, err := NewCatalog(tools, nil, "1.0", whitespaceTokenizer)
	if err != nil {
		t.Fatal(err)
	}
	// query 分词（whitespace 下整体一个 token）与工具字段 token 无共享，
	// 但 query 包含别名子串 → BM25F 无命中后触发一次兜底命中
	result := catalog.RecallTools("帮我做呕吐次数统计", Filter{}, 0)
	if !result.UsedFallback {
		t.Fatal("期望触发一次兜底检索")
	}
	if len(result.Matches) != 1 {
		t.Fatalf("兜底 matches = %d, want 1", len(result.Matches))
	}
}

func TestCatalogNoMatchReturnsEmpty(t *testing.T) {
	tools := []Tool{
		{Name: "read_pet", OperationID: "read_pet", ResourceType: ResourcePet, ActionType: ActionRead, Version: "1.0"},
	}
	catalog, err := NewCatalog(tools, nil, "1.0", whitespaceTokenizer)
	if err != nil {
		t.Fatal(err)
	}
	result := catalog.RecallTools("完全无关的查询内容", Filter{}, 0)
	if len(result.Matches) != 0 {
		t.Fatalf("无命中应返回空（明确能力受限），实际 %+v", result.Matches)
	}
}

func TestCatalogDigestStableAndVersionSensitive(t *testing.T) {
	tools := []Tool{
		{Name: "read_pet", OperationID: "read_pet", ResourceType: ResourcePet, ActionType: ActionRead, Version: "1.0"},
	}
	c1, err := NewCatalog(tools, nil, "1.0", whitespaceTokenizer)
	if err != nil {
		t.Fatal(err)
	}
	c2, err := NewCatalog(tools, nil, "1.0", whitespaceTokenizer)
	if err != nil {
		t.Fatal(err)
	}
	if c1.Digest() != c2.Digest() {
		t.Fatal("相同内容 digest 应稳定")
	}
	// digest 是目录内容摘要：工具/Skill 内容变化（工具版本变化）时 digest 应变化
	toolsV2 := []Tool{
		{Name: "read_pet", OperationID: "read_pet", ResourceType: ResourcePet, ActionType: ActionRead, Version: "2.0"},
	}
	c3, err := NewCatalog(toolsV2, nil, "2.0", whitespaceTokenizer)
	if err != nil {
		t.Fatal(err)
	}
	if c1.Digest() == c3.Digest() {
		t.Fatal("工具版本变化 digest 应变化")
	}
}

func TestCatalogDuplicateRejected(t *testing.T) {
	tools := []Tool{
		{Name: "read_pet", OperationID: "read_pet", ResourceType: ResourcePet, ActionType: ActionRead, Version: "1.0"},
		{Name: "read_pet_dup", OperationID: "read_pet", ResourceType: ResourcePet, ActionType: ActionRead, Version: "1.0"},
	}
	if _, err := NewCatalog(tools, nil, "1.0", whitespaceTokenizer); err == nil {
		t.Fatal("同名 operation_id 冲突应被拒绝")
	}
}

func TestEmergencySkillsFallback(t *testing.T) {
	skills := []Skill{
		{ID: "chitchat", Version: "1.0", Scope: ScopeChitchat},
		{ID: "emergency", Version: "1.0", Scope: ScopeEmergencySafety},
		{ID: "symptom", Version: "1.0", Scope: ScopeSymptom},
	}
	catalog, err := NewCatalog(nil, skills, "1.0", whitespaceTokenizer)
	if err != nil {
		t.Fatal(err)
	}
	emergency := catalog.EmergencySkills()
	if len(emergency) != 1 || emergency[0].ID != "emergency" {
		t.Fatalf("急症安全兜底 = %+v，应只含 emergency", emergency)
	}
}

func TestDefaultCatalogRecallsVaccineRecordForNaturalChinese(t *testing.T) {
	catalog, err := NewCatalog(DefaultTools(), nil, DefaultToolVersion, nil)
	if err != nil {
		t.Fatal(err)
	}
	result := catalog.RecallTools("上次疫苗什么时候", readOnlyFilter(), 0)
	for _, match := range result.Matches {
		if match.Tool.OperationID == "search.health_record" {
			return
		}
	}
	t.Fatalf("recall result = %+v", result)
}

func TestDefaultCatalogChineseRecallRegression(t *testing.T) {
	catalog, err := DefaultCatalog(DefaultToolVersion)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		query       string
		operationID string
		parameter   string
	}{
		{query: "上次疫苗什么时候", operationID: "search.health_record", parameter: "pet_id"},
		{query: "最近拉肚子几次", operationID: "aggregate.health_record", parameter: "pet_id"},
		{query: "旺仔和球球分别是谁", operationID: "resolve.pet", parameter: "query"},
		{query: "帮我看下档案", operationID: "read.pet_profile", parameter: "pet_id"},
	}
	for _, test := range tests {
		result := catalog.RecallTools(test.query, readOnlyFilter(), 0)
		var found *Tool
		for _, match := range result.Matches {
			if match.Tool.OperationID == test.operationID {
				tool := match.Tool
				found = &tool
				break
			}
		}
		if found == nil {
			t.Fatalf("query=%q operation=%q matches=%+v", test.query, test.operationID, result.Matches)
		}
		var schema struct {
			Properties map[string]json.RawMessage `json:"properties"`
		}
		if !json.Valid(found.Parameters) || json.Unmarshal(found.Parameters, &schema) != nil || schema.Properties[test.parameter] == nil {
			t.Fatalf("query=%q tool=%q parameter schema invalid: %s", test.query, test.operationID, found.Parameters)
		}
	}
}

func TestRunCatalogSnapshotAppend(t *testing.T) {
	tools := []Tool{
		{Name: "read_pet", OperationID: "read_pet", ResourceType: ResourcePet, ActionType: ActionRead, Version: "1.0"},
	}
	catalog, err := NewCatalog(tools, nil, "1.0", whitespaceTokenizer)
	if err != nil {
		t.Fatal(err)
	}
	snap := NewRunCatalogSnapshot(catalog)
	if snap.ToolCatalogVersion != "1.0" || snap.ToolCatalogDigest != catalog.Digest() {
		t.Fatalf("快照版本/摘要不匹配：%+v", snap)
	}
	if !snap.AppendTool("read_pet") {
		t.Fatal("首次追加应成功")
	}
	if snap.AppendTool("read_pet") {
		t.Fatal("重复追加应返回 false")
	}
	if !snap.ContainsTool("read_pet") {
		t.Fatal("ContainsTool 应为 true")
	}
	if snap.ContainsTool("unknown") {
		t.Fatal("ContainsTool(unknown) 应为 false")
	}
}

func TestSkillRecallSortAndNegative(t *testing.T) {
	skills := []Skill{
		{ID: "chitchat", Version: "1.0", Scope: ScopeChitchat, TriggerExamples: []string{"你好"}, Priority: 1},
		{ID: "emergency", Version: "1.0", Scope: ScopeEmergencySafety, TriggerExamples: []string{"中毒"}, Priority: 1},
		{ID: "symptom", Version: "1.0", Scope: ScopeSymptom, TriggerExamples: []string{"呕吐"}, Priority: 1},
		{ID: "symptom_excluded", Version: "1.0", Scope: ScopeSymptom, TriggerExamples: []string{"呕吐"}, NegativeExamples: []string{"症状"}, Priority: 1},
	}
	catalog, err := NewCatalog(nil, skills, "1.0", whitespaceTokenizer)
	if err != nil {
		t.Fatal(err)
	}
	result := catalog.RecallSkills("呕吐 症状", Filter{})
	// symptom 命中 trigger「呕吐」；symptom_excluded 命中 trigger 但被负例「症状」排除
	if len(result) != 1 || result[0].ID != "symptom" {
		t.Fatalf("召回结果 = %+v，应只含 symptom", result)
	}

	// scope 排序：命中多个时，急症安全应排最前
	multi := catalog.RecallSkills("中毒 呕吐", Filter{})
	if len(multi) == 0 || multi[0].ID != "emergency" {
		t.Fatalf("排序首位 = %+v，应急症安全排最前", multi)
	}
}
