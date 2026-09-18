package ask

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	petapp "github.com/balancetheworld/wechat-pet/internal/app/pet"
	_ "github.com/mattn/go-sqlite3"
)

// 本文件验证 processRun 的 v2 决策循环路径：注入 v2 依赖后，Run 由 RunDecisionLoop
// 驱动，最终结果降级映射为现有 Event。v1 路径（未注入 v2 依赖）由 service_test.go 覆盖。

// newV2Service 构造注入 v2 依赖的 Service。
func newV2Service(t *testing.T, model AgentModel, business BusinessReadRepository) (*Service, *SQLRepository) {
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
	catalog, err := DefaultCatalog(DefaultToolVersion)
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewService(repository, servicePetRepository{pet: petapp.Pet{ID: "pet-1", FamilyID: "family-1", Name: "团子"}})
	if err != nil {
		t.Fatal(err)
	}
	service.SetAgentModel(model)
	service.SetToolCatalog(catalog)
	service.SetBusinessReadRepository(business)
	return service, repository
}

func lastEvent(events []Event) Event {
	if len(events) == 0 {
		return Event{}
	}
	return events[len(events)-1]
}

func eventDataMap(t *testing.T, e Event) map[string]any {
	t.Helper()
	var data map[string]any
	if err := json.Unmarshal([]byte(e.Data), &data); err != nil {
		t.Fatalf("event %s data not JSON: %v (%s)", e.Type, err, e.Data)
	}
	return data
}

func TestProcessRunV2FinalAnswer(t *testing.T) {
	model := &scriptedModel{responses: [][]ProtocolRecord{finalAnswerRecords()}}
	service, _ := newV2Service(t, model, &fakeBusinessRead{})

	created, err := service.CreateSession(context.Background(), "family-1", "user-1", "pet-1", "你好", "create-v2-1")
	if err != nil {
		t.Fatal(err)
	}
	processed, err := service.ProcessRun(context.Background(), "family-1", created.Session.ID, created.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if processed.Run.Status != RunCompleted {
		t.Fatalf("run status = %s, want completed", processed.Run.Status)
	}
	final := lastEvent(processed.Events)
	if final.Type != "assistant.completed" {
		t.Fatalf("last event = %s, want assistant.completed", final.Type)
	}
	data := eventDataMap(t, final)
	if data["answer"] != "你好" {
		t.Fatalf("answer = %v, want 你好", data["answer"])
	}
	if data["intent"] != "casual_chat" {
		t.Fatalf("intent = %v, want casual_chat", data["intent"])
	}
	if len(model.inputs) == 0 {
		t.Fatal("v2 decision loop should invoke the model")
	}
}

func TestProcessRunV2RequestInput(t *testing.T) {
	model := &scriptedModel{responses: [][]ProtocolRecord{requestInputRecords()}}
	service, _ := newV2Service(t, model, &fakeBusinessRead{})

	created, err := service.CreateSession(context.Background(), "family-1", "user-1", "pet-1", "请问是哪只宠物？", "create-v2-2")
	if err != nil {
		t.Fatal(err)
	}
	processed, err := service.ProcessRun(context.Background(), "family-1", created.Session.ID, created.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if processed.Run.Status != RunWaitingInput {
		t.Fatalf("run status = %s, want waiting_input", processed.Run.Status)
	}
	final := lastEvent(processed.Events)
	if final.Type != "assistant.question" {
		t.Fatalf("last event = %s, want assistant.question", final.Type)
	}
	data := eventDataMap(t, final)
	if data["question"] != "请问是哪只宠物？" {
		t.Fatalf("question = %v", data["question"])
	}
}

func TestProcessRunV2ToolThenAnswer(t *testing.T) {
	// 第一步 call_tools 解析宠物，第二步 final_answer。
	call := ProtocolRecord{Type: RecordCall, Call: &CallRecord{
		Type:           RecordCall,
		CallKey:        "c1",
		TaskKeys:       []string{"t1"},
		ToolName:       "resolve_pet",
		CatalogVersion: DefaultToolVersion,
		Arguments:      json.RawMessage(`{"query":"旺仔"}`),
	}}
	business := &fakeBusinessRead{
		resolveOutcome: PetResolveOutcome{Status: PetResolveResolved, Resolved: []petapp.Pet{{ID: "pet-1", Name: "团子"}}, Source: ReadSource{SourceType: "pet", SourceID: "family-1", Version: "v1"}},
	}
	model := &scriptedModel{responses: [][]ProtocolRecord{
		{headerRecord(ActionCallTools), call, coverageRecord(), endRecord()},
		finalAnswerRecords(),
	}}
	service, _ := newV2Service(t, model, business)

	created, err := service.CreateSession(context.Background(), "family-1", "user-1", "pet-1", "旺仔最近怎么样？", "create-v2-3")
	if err != nil {
		t.Fatal(err)
	}
	processed, err := service.ProcessRun(context.Background(), "family-1", created.Session.ID, created.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if processed.Run.Status != RunCompleted {
		t.Fatalf("run status = %s, want completed", processed.Run.Status)
	}
	// 工具执行器应真实调用 resolve。
	if resolve, _, _, _ := business.counts(); resolve != 1 {
		t.Fatalf("resolve calls = %d, want 1", resolve)
	}
	if len(model.inputs) != 2 {
		t.Fatalf("model calls = %d, want 2 (call_tools then final_answer)", len(model.inputs))
	}
	if final := lastEvent(processed.Events); final.Type != "assistant.completed" {
		t.Fatalf("last event = %s, want assistant.completed", final.Type)
	}
}

func TestMapLoopOutcomeToDecision(t *testing.T) {
	// final_answer 降级映射：health 组、模型风险 yellow 与规则 green 合并为 yellow。
	groups := []AnswerGroup{{
		GroupKey: "g1", TaskKeys: []string{"t1"}, AnswerKind: AnswerHealth, Scope: ScopeFull,
		Subjects: []AnswerSubject{{SubjectKey: "s1", Kind: SubjectPet, PetID: "p1"}},
		Segments: []SegmentRecord{
			{Field: "observation", Text: "精神尚可"},
			{Field: "next_action", Text: "建议观察"},
		},
		Risks: []RiskRecord{{SubjectKey: "s1", Level: RiskYellow}},
	}}
	outcome := LoopOutcome{Action: ActionFinalAnswer, Groups: groups}
	decision := mapLoopOutcomeToDecision(outcome, RiskGreen)
	if decision.Status != RunCompleted || decision.EventType != "assistant.completed" {
		t.Fatalf("decision = %+v", decision)
	}
	if decision.RiskLevel != RiskYellow {
		t.Fatalf("risk = %s, want yellow (rule green merged with model yellow)", decision.RiskLevel)
	}
	if decision.Data["answer"] != "精神尚可\n建议观察" {
		t.Fatalf("answer = %v", decision.Data["answer"])
	}
	if decision.Data["intent"] != "pet_health" {
		t.Fatalf("intent = %v, want pet_health", decision.Data["intent"])
	}
}

func TestMapLoopOutcomeToDecisionRequestInput(t *testing.T) {
	outcome := LoopOutcome{Action: ActionRequestInput, Questions: []Question{{Text: "请问是哪只宠物？"}}}
	decision := mapLoopOutcomeToDecision(outcome, RiskUnknown)
	if decision.Status != RunWaitingInput || decision.EventType != "assistant.question" {
		t.Fatalf("decision = %+v", decision)
	}
	if decision.Data["question"] != "请问是哪只宠物？" {
		t.Fatalf("question = %v", decision.Data["question"])
	}
}

func TestJoinGroupTextsAndIntent(t *testing.T) {
	groups := []AnswerGroup{
		{AnswerKind: AnswerHealth, Segments: []SegmentRecord{{Text: " 第一段 "}, {Text: ""}, {Text: "第二段"}}},
	}
	if got := joinGroupTexts(groups); got != "第一段\n第二段" {
		t.Fatalf("joinGroupTexts = %q", got)
	}
	if got := intentForGroups(groups); got != IntentPetHealth {
		t.Fatalf("intent = %s, want pet_health", got)
	}
	if got := intentForGroups(nil); got != IntentCasualChat {
		t.Fatalf("intent for nil = %s, want casual_chat", got)
	}
}
