package ask

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	petapp "github.com/balancetheworld/wechat-pet/internal/app/pet"
	"github.com/balancetheworld/wechat-pet/internal/platform/knowledge"
)

// fakeKnowledgeSource 记录检索请求并返回可控结果，用于测试只读知识工具接线。
type fakeKnowledgeSource struct {
	mu      sync.Mutex
	result  knowledge.Result
	err     error
	queries []knowledge.Query
}

func (f *fakeKnowledgeSource) Search(_ context.Context, query knowledge.Query) (knowledge.Result, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.queries = append(f.queries, query)
	return f.result, f.err
}

func (f *fakeKnowledgeSource) Version() string { return "2026-10-05.v1" }

func (f *fakeKnowledgeSource) lastQuery(t *testing.T) knowledge.Query {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.queries) == 0 {
		t.Fatal("knowledge source received no query")
	}
	return f.queries[len(f.queries)-1]
}

// knowledgeAdapter 构造只含知识检索工具的执行器，familyID 固定为测试家庭。
func knowledgeAdapter(t *testing.T, source KnowledgeSource) *ToolExecutorAdapter {
	t.Helper()
	catalog, err := NewCatalog([]Tool{SearchPetKnowledgeTool()}, nil, DefaultToolVersion, whitespaceTokenizer)
	if err != nil {
		t.Fatal(err)
	}
	return NewToolExecutorAdapter(catalog, nil, Filter{}, "family-1", ToolExecutionScope{
		SessionID: "s1",
		RunID:     "r1",
		Knowledge: source,
		PetFacts:  map[string]PetProfileFacts{"pet-1": {Species: "cat", LifeStage: "adult"}},
	})
}

// knowledgeCall 构造一次知识检索调用。
func knowledgeCall(arguments string) ToolCall {
	return ToolCall{ToolCallID: "c1", CallIndex: 0, ToolName: petKnowledgeToolName, ToolVersion: DefaultToolVersion, Arguments: json.RawMessage(arguments)}
}

func matchedKnowledgeResult() knowledge.Result {
	return knowledge.Result{
		Status:           knowledge.RetrievalMatched,
		Version:          "2026-10-05.v1",
		RedFlagRequired:  true,
		RedFlagSatisfied: true,
		Hits: []knowledge.Hit{{
			RedFlag: true,
			Chunk: knowledge.Chunk{
				ReferenceID:          "pet_health.vomiting.cat.001",
				DocumentID:           "pet_health.vomiting.cat",
				ChunkID:              "pet_health.vomiting.cat.001",
				Title:                "猫咪呕吐后的基础观察",
				Content:              "猫咪偶尔吐出未消化的猫粮，可以先观察。",
				Species:              knowledge.SpeciesCat,
				Topics:               []string{"vomiting"},
				RiskLevel:            knowledge.RiskYellow,
				EscalationConditions: []string{"连续呕吐、无法喝水、精神很差时应尽快联系宠物医生。"},
			},
		}},
	}
}

func TestSearchPetKnowledgeToolContract(t *testing.T) {
	tool := SearchPetKnowledgeTool()
	if err := tool.Validate(); err != nil {
		t.Fatalf("knowledge tool invalid: %v", err)
	}
	if tool.Name != petKnowledgeToolName || tool.OperationID != "search.pet_knowledge" {
		t.Fatalf("knowledge tool identity = %q/%q", tool.Name, tool.OperationID)
	}
	if tool.ResourceType != ResourceKnowledge || tool.ActionType != ActionSearch {
		t.Fatalf("knowledge tool route = %q/%q, want knowledge/search", tool.ResourceType, tool.ActionType)
	}
	if !IsReadOnly(tool.ActionType) {
		t.Fatal("knowledge tool must be read-only")
	}
	if tool.RequiresConfirmation {
		t.Fatal("read-only knowledge tool must not require confirmation")
	}
	assertParamNames(t, tool, "query", "pet_id", "limit")
}

func TestDefaultCatalogWithKnowledgeIncludesTool(t *testing.T) {
	catalog, err := DefaultCatalogWithKnowledge(DefaultToolVersion)
	if err != nil {
		t.Fatalf("DefaultCatalogWithKnowledge error: %v", err)
	}
	if _, ok := catalog.ToolByName(petKnowledgeToolName, prepareFilter()); !ok {
		t.Fatal("knowledge tool should be recallable in the execution filter")
	}
}

func TestKnowledgeToolForcedForHealthQuestion(t *testing.T) {
	catalog, err := NewCatalog([]Tool{SearchPetKnowledgeTool()}, nil, DefaultToolVersion, whitespaceTokenizer)
	if err != nil {
		t.Fatal(err)
	}
	service := &Service{catalog: catalog, knowledge: &fakeKnowledgeSource{}}
	tools := service.withKnowledgeTool(nil, prepareFilter(), "我家猫今天吐了两次，需要去医院吗")
	if len(tools) != 1 || tools[0].Name != petKnowledgeToolName {
		t.Fatalf("health question should carry the knowledge tool, got %v", tools)
	}
	if chitchat := service.withKnowledgeTool(nil, prepareFilter(), "今天天气真好"); len(chitchat) != 0 {
		t.Fatalf("chitchat should not carry the knowledge tool, got %v", chitchat)
	}
}

func TestKnowledgeToolAbsentWithoutRetriever(t *testing.T) {
	catalog, err := NewCatalog([]Tool{SearchPetKnowledgeTool()}, nil, DefaultToolVersion, whitespaceTokenizer)
	if err != nil {
		t.Fatal(err)
	}
	service := &Service{catalog: catalog}
	if tools := service.withKnowledgeTool(nil, prepareFilter(), "我家猫今天吐了两次"); len(tools) != 0 {
		t.Fatalf("retriever not injected, no knowledge tool expected, got %v", tools)
	}
}

func TestExecuteKnowledgeSearchReturnsCitedReferences(t *testing.T) {
	source := &fakeKnowledgeSource{result: matchedKnowledgeResult()}
	adapter := knowledgeAdapter(t, source)

	results, err := adapter.ExecuteBatch(context.Background(), ToolBatch{BatchID: "b1", RunID: "r1", Calls: []ToolCall{
		knowledgeCall(`{"query":"我家猫吐了要不要去医院","pet_id":"pet-1","limit":2}`),
	}})
	if err != nil {
		t.Fatalf("ExecuteBatch error: %v", err)
	}
	if len(results) != 1 || results[0].Status != ToolResultOK {
		t.Fatalf("unexpected tool results: %+v", results)
	}
	query := source.lastQuery(t)
	if query.Text != "我家猫吐了要不要去医院" || query.Species != "cat" || query.LifeStage != "adult" || query.Limit != 2 {
		t.Fatalf("knowledge query not forwarded with injected pet facts: %+v", query)
	}
	outcome, ok := knowledgeOutcomeOf(results[0].Data)
	if !ok {
		t.Fatalf("data is not a knowledge outcome: %T", results[0].Data)
	}
	if len(outcome.References) != 1 || outcome.References[0].ReferenceID != "pet_health.vomiting.cat.001" {
		t.Fatalf("references = %+v", outcome.References)
	}
	if outcome.References[0].SourceType != knowledge.SourceTypeReviewedKnowledge {
		t.Fatalf("source_type = %q", outcome.References[0].SourceType)
	}
	text := toolResultText(results[0])
	for _, want := range []string{"[REFERENCE]", "reference_id: pet_health.vomiting.cat.001", "knowledge_version: 2026-10-05.v1", "[/REFERENCE]"} {
		if !strings.Contains(text, want) {
			t.Fatalf("reference block missing %q: %s", want, text)
		}
	}
	refs := toolEvidenceRefs(results[0])
	if len(refs) != 1 || refs[0].SourceType != petKnowledgeSourceType || refs[0].SourceID != "pet_health.vomiting.cat.001" || refs[0].Version != "2026-10-05.v1" {
		t.Fatalf("evidence refs = %+v", refs)
	}
	// 模型引用本次注入的 reference_id 必须能通过来源校验。
	keys := evidenceKeysForBlocks([]ContextBlock{{Layer: LayerToolInteractions, Kind: "tool_result", EvidenceRefs: refs}})
	if !evidenceExists(keys, refs[0]) {
		t.Fatal("injected knowledge reference should be resolvable evidence")
	}
	if evidenceExists(keys, EvidenceRef{SourceType: petKnowledgeSourceType, SourceID: "cat-vomit-999", Version: "2026-10-05.v1"}) {
		t.Fatal("fabricated reference_id must not pass evidence validation")
	}
}

func TestExecuteKnowledgeSearchWithoutRetrieverFails(t *testing.T) {
	adapter := knowledgeAdapter(t, nil)
	results, err := adapter.ExecuteBatch(context.Background(), ToolBatch{BatchID: "b1", RunID: "r1", Calls: []ToolCall{
		knowledgeCall(`{"query":"我家猫吐了"}`),
	}})
	if err != nil {
		t.Fatalf("ExecuteBatch error: %v", err)
	}
	if results[0].Status != ToolResultError || results[0].Error == nil || results[0].Error.Reason != "knowledge_unavailable" {
		t.Fatalf("missing retriever should fail explicitly, got %+v", results[0])
	}
}

func TestExecuteKnowledgeSearchFailureIsNotNoMatch(t *testing.T) {
	source := &fakeKnowledgeSource{err: errors.New("upstream down")}
	adapter := knowledgeAdapter(t, source)
	results, err := adapter.ExecuteBatch(context.Background(), ToolBatch{BatchID: "b1", RunID: "r1", Calls: []ToolCall{
		knowledgeCall(`{"query":"我家猫吐了"}`),
	}})
	if err != nil {
		t.Fatalf("ExecuteBatch error: %v", err)
	}
	if results[0].Status != ToolResultError {
		t.Fatalf("retrieval failure must not be reported as ok/no_match, got %+v", results[0])
	}
}

func TestExecuteKnowledgeSearchRejectsPetOutsideSessionScope(t *testing.T) {
	source := &fakeKnowledgeSource{result: matchedKnowledgeResult()}
	adapter := knowledgeAdapter(t, source)
	results, err := adapter.ExecuteBatch(context.Background(), ToolBatch{BatchID: "b1", RunID: "r1", Calls: []ToolCall{
		knowledgeCall(`{"query":"我家猫吐了","pet_id":"pet-9"}`),
	}})
	if err != nil {
		t.Fatalf("ExecuteBatch error: %v", err)
	}
	if results[0].Status != ToolResultError || results[0].Error.Category != ToolErrInvalidArgument {
		t.Fatalf("pet outside session scope must be rejected, got %+v", results[0])
	}
	if len(source.queries) != 0 {
		t.Fatal("out-of-scope call must not reach the knowledge source")
	}
}

func TestKnowledgeSpeciesComesFromRuntimeNotModel(t *testing.T) {
	source := &fakeKnowledgeSource{result: matchedKnowledgeResult()}
	adapter := knowledgeAdapter(t, source)
	// 物种不能由模型填写：schema 拒绝未知参数，调用不会执行。
	_, err := adapter.ExecuteBatch(context.Background(), ToolBatch{BatchID: "b1", RunID: "r1", Calls: []ToolCall{
		knowledgeCall(`{"query":"我家猫吐了","species":"dog"}`),
	}})
	if err == nil {
		t.Fatal("model-provided species must be rejected")
	}
	if len(source.queries) != 0 {
		t.Fatal("rejected call must not reach the knowledge source")
	}
	// 生命周期同样由 Runtime 注入，模型填写同样被拒绝。
	if _, err := adapter.ExecuteBatch(context.Background(), ToolBatch{BatchID: "b3", RunID: "r1", Calls: []ToolCall{
		knowledgeCall(`{"query":"我家猫吐了","life_stage":"young"}`),
	}}); err == nil {
		t.Fatal("model-provided life_stage must be rejected")
	}
	// 未指定 pet_id 且会话只有一只宠物时，使用该宠物档案里的物种。
	results, err := adapter.ExecuteBatch(context.Background(), ToolBatch{BatchID: "b2", RunID: "r1", Calls: []ToolCall{
		knowledgeCall(`{"query":"我家猫吐了"}`),
	}})
	if err != nil {
		t.Fatalf("ExecuteBatch error: %v", err)
	}
	if results[0].Status != ToolResultOK {
		t.Fatalf("single-pet session should resolve species, got %+v", results[0])
	}
	if species := source.lastQuery(t).Species; species != "cat" {
		t.Fatalf("injected species = %q, want cat", species)
	}
}

func TestKnowledgeOutcomeMarksUnconfirmedPetFacts(t *testing.T) {
	source := &fakeKnowledgeSource{result: matchedKnowledgeResult()}
	catalog, err := NewCatalog([]Tool{SearchPetKnowledgeTool()}, nil, DefaultToolVersion, whitespaceTokenizer)
	if err != nil {
		t.Fatal(err)
	}
	adapter := NewToolExecutorAdapter(catalog, nil, Filter{}, "family-1", ToolExecutionScope{
		SessionID: "s1",
		RunID:     "r1",
		Knowledge: source,
		PetFacts:  map[string]PetProfileFacts{"pet-1": {}},
	})
	results, err := adapter.ExecuteBatch(context.Background(), ToolBatch{BatchID: "b1", RunID: "r1", Calls: []ToolCall{
		knowledgeCall(`{"query":"昨天吐了"}`),
	}})
	if err != nil {
		t.Fatalf("ExecuteBatch error: %v", err)
	}
	outcome, ok := knowledgeOutcomeOf(results[0].Data)
	if !ok {
		t.Fatalf("data is not a knowledge outcome: %T", results[0].Data)
	}
	if outcome.SpeciesFilter != petFactsUnknown || outcome.LifeStageFilter != petFactsUnknown {
		t.Fatalf("pet facts filter = species:%q life_stage:%q, want unknown/unknown", outcome.SpeciesFilter, outcome.LifeStageFilter)
	}
	if text := outcome.referenceText(); !strings.Contains(text, "pet_facts: species=unknown, life_stage=unknown") {
		t.Fatalf("reference text missing pet facts: %q", text)
	}
}

func TestFactsForRequiresExplicitScope(t *testing.T) {
	multiPet := ToolExecutionScope{PetFacts: map[string]PetProfileFacts{
		"pet-1": {Species: "cat", LifeStage: "adult"},
		"pet-2": {Species: "dog", LifeStage: "senior"},
	}}
	if facts, ok := multiPet.factsFor(""); !ok || facts != (PetProfileFacts{}) {
		t.Fatalf("multi-pet session without pet_id must stay unconfirmed, got %+v ok=%t", facts, ok)
	}
	if facts, ok := multiPet.factsFor("pet-2"); !ok || facts.Species != "dog" || facts.LifeStage != "senior" {
		t.Fatalf("pet-2 facts = %+v ok=%t, want dog/senior", facts, ok)
	}
	if _, ok := multiPet.factsFor("pet-3"); ok {
		t.Fatal("pet outside scope must be rejected")
	}
	unknown := ToolExecutionScope{PetFacts: map[string]PetProfileFacts{"pet-1": {}}}
	if facts, ok := unknown.factsFor("pet-1"); !ok || facts != (PetProfileFacts{}) {
		t.Fatalf("profile without facts must stay unconfirmed, got %+v ok=%t", facts, ok)
	}
}

func TestPetProfileFactsReadsConfirmedProfile(t *testing.T) {
	service := &Service{pets: servicePetRepository{pet: petapp.Pet{ID: "pet-1", Name: "旺仔"}}, now: time.Now}
	facts := service.petProfileFacts(context.Background(), Session{FamilyID: "family-1", PetID: "pet-1", Pets: []SessionPet{{PetID: "pet-1"}}})
	if facts["pet-1"].Species != "cat" {
		t.Fatalf("pet facts = %+v, want pet-1 species cat", facts)
	}
	// 档案没有生日与年龄时生命周期保持未确认，不按品种或用户原话推断。
	if facts["pet-1"].LifeStage != "" {
		t.Fatalf("pet facts = %+v, want unconfirmed life stage", facts)
	}
	// 档案读不到物种时保持未确认，不用品种猜测。
	service = &Service{pets: servicePetRepositoryWithoutSpecies{}, now: time.Now}
	facts = service.petProfileFacts(context.Background(), Session{FamilyID: "family-1", PetID: "pet-1", Pets: []SessionPet{{PetID: "pet-1"}}})
	if value, ok := facts["pet-1"]; !ok || value != (PetProfileFacts{}) {
		t.Fatalf("pet facts = %+v, want pet-1 unconfirmed", facts)
	}
}

func TestPetAgeMonthsFromBirthday(t *testing.T) {
	now := time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC)
	birthday := func(value string) *string { return &value }
	tests := []struct {
		name    string
		profile petapp.PetProfile
		want    int
	}{
		{name: "kitten", profile: petapp.PetProfile{Birthday: birthday("2026-03-01")}, want: 7},
		{name: "senior cat", profile: petapp.PetProfile{Birthday: birthday("2016-10-06")}, want: 120},
		{name: "day not reached", profile: petapp.PetProfile{Birthday: birthday("2025-10-07")}, want: 11},
		{name: "age fallback", profile: petapp.PetProfile{Age: 9}, want: 108},
		{name: "unknown", profile: petapp.PetProfile{}, want: -1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := petAgeMonths(test.profile, now); got != test.want {
				t.Fatalf("petAgeMonths = %d, want %d", got, test.want)
			}
		})
	}
}

// servicePetRepositoryWithoutSpecies 模拟档案里物种为空的宠物。
type servicePetRepositoryWithoutSpecies struct{}

func (servicePetRepositoryWithoutSpecies) List(context.Context, string) ([]petapp.Pet, error) {
	return nil, nil
}

func (servicePetRepositoryWithoutSpecies) Get(context.Context, string, string) (petapp.Pet, error) {
	return petapp.Pet{ID: "pet-1", Name: "旺仔"}, nil
}

func (servicePetRepositoryWithoutSpecies) GetProfile(context.Context, string, string) (petapp.PetProfile, error) {
	return petapp.PetProfile{ID: "pet-1", Name: "旺仔", Breed: "田园猫", Species: "  "}, nil
}

func (servicePetRepositoryWithoutSpecies) Create(context.Context, string, string, string) (petapp.Pet, error) {
	return petapp.Pet{}, errors.New("not implemented")
}

func (servicePetRepositoryWithoutSpecies) Update(context.Context, string, string, string, petapp.UpdatePetRequest) (petapp.Pet, error) {
	return petapp.Pet{}, errors.New("not implemented")
}

func (servicePetRepositoryWithoutSpecies) Delete(context.Context, string, string, string) error {
	return errors.New("not implemented")
}

func TestSearchKnowledgeRejectsEmptyQuery(t *testing.T) {
	source := &fakeKnowledgeSource{result: matchedKnowledgeResult()}
	adapter := knowledgeAdapter(t, source)
	results, err := adapter.ExecuteBatch(context.Background(), ToolBatch{BatchID: "b1", RunID: "r1", Calls: []ToolCall{
		knowledgeCall(`{"query":"   "}`),
	}})
	if err != nil {
		t.Fatalf("ExecuteBatch error: %v", err)
	}
	if results[0].Status != ToolResultError || results[0].Error.Category != ToolErrInvalidArgument {
		t.Fatalf("empty query should be rejected before retrieval, got %+v", results[0])
	}
}

func TestKnowledgeSearchAgainstEmbeddedCatalog(t *testing.T) {
	catalog, err := knowledge.EmbeddedCatalog(nil)
	if err != nil {
		t.Fatalf("EmbeddedCatalog error: %v", err)
	}
	adapter := knowledgeAdapter(t, catalog)
	results, err := adapter.ExecuteBatch(context.Background(), ToolBatch{BatchID: "b1", RunID: "r1", Calls: []ToolCall{
		knowledgeCall(`{"query":"猫咪连续呕吐，无法喝水，精神很差","pet_id":"pet-1"}`),
	}})
	if err != nil {
		t.Fatalf("ExecuteBatch error: %v", err)
	}
	outcome, ok := knowledgeOutcomeOf(results[0].Data)
	if !ok {
		t.Fatalf("data is not a knowledge outcome: %T", results[0].Data)
	}
	if outcome.RetrievalStatus != petKnowledgeRetrievalMatched {
		t.Fatalf("retrieval_status = %q, want matched", outcome.RetrievalStatus)
	}
	if len(outcome.References) == 0 {
		t.Fatalf("references = %+v", outcome.References)
	}
	if !outcome.RedFlagRequired || !outcome.RedFlagSatisfied {
		t.Fatalf("red flag handling = required:%t satisfied:%t", outcome.RedFlagRequired, outcome.RedFlagSatisfied)
	}
	if len(outcome.References[0].EscalationConditions) == 0 {
		t.Fatal("seed reference should carry escalation conditions")
	}
}
