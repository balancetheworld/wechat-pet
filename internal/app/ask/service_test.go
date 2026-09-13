package ask

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	calendarapp "github.com/balancetheworld/wechat-pet/internal/app/calendar"
	petapp "github.com/balancetheworld/wechat-pet/internal/app/pet"
	_ "github.com/mattn/go-sqlite3"
)

type servicePetRepository struct {
	pet  petapp.Pet
	pets []petapp.Pet
}

func (r servicePetRepository) List(context.Context, string) ([]petapp.Pet, error) {
	if r.pets != nil {
		return r.pets, nil
	}
	return []petapp.Pet{r.pet}, nil
}

func (r servicePetRepository) Get(_ context.Context, _ string, petID string) (petapp.Pet, error) {
	if r.pets != nil {
		for _, pet := range r.pets {
			if pet.ID == petID {
				return pet, nil
			}
		}
		return petapp.Pet{}, petapp.ErrNotFound
	}
	return r.pet, nil
}

func (r servicePetRepository) GetProfile(context.Context, string, string) (petapp.PetProfile, error) {
	return petapp.PetProfile{ID: r.pet.ID, Name: r.pet.Name, Breed: "田园猫", Gender: "female", Sterilized: true}, nil
}

func (r servicePetRepository) GetHealth(context.Context, string, string) (petapp.PetHealth, error) {
	return petapp.PetHealth{Status: "stable", Allergies: "鸡肉", LongTermMedication: "无"}, nil
}

func (servicePetRepository) Create(context.Context, string, string, string) (petapp.Pet, error) {
	return petapp.Pet{}, errors.New("not implemented")
}

func (servicePetRepository) Update(context.Context, string, string, string, petapp.UpdatePetRequest) (petapp.Pet, error) {
	return petapp.Pet{}, errors.New("not implemented")
}

func (servicePetRepository) Delete(context.Context, string, string, string) error {
	return errors.New("not implemented")
}

type serviceExecutor struct {
	decision RunDecision
}

func (e serviceExecutor) Execute(context.Context, RunInput) (RunDecision, error) {
	return e.decision, nil
}

type streamingServiceExecutor struct {
	decision RunDecision
	deltas   []string
}

func (e streamingServiceExecutor) Execute(context.Context, RunInput) (RunDecision, error) {
	return e.decision, nil
}

func (e streamingServiceExecutor) ExecuteStream(_ context.Context, _ RunInput, emit func(string) error) (RunDecision, error) {
	for _, delta := range e.deltas {
		if err := emit(delta); err != nil {
			return RunDecision{}, err
		}
	}
	return e.decision, nil
}

type contextExecutor struct {
	decision RunDecision
	input    RunInput
}

type contextCalendarRepository struct {
	records []calendarapp.ContextRecord
}

func (r contextCalendarRepository) ListRecentRecords(context.Context, string, string, time.Time, int) ([]calendarapp.ContextRecord, error) {
	return r.records, nil
}

type factCalendarRepository struct {
	records map[string]calendarapp.FactRecord
}

func (r factCalendarRepository) ListRecentRecords(context.Context, string, string, time.Time, int) ([]calendarapp.ContextRecord, error) {
	return nil, nil
}

func (r factCalendarRepository) FindLatestFact(_ context.Context, _, petID, factType string) (calendarapp.FactRecord, error) {
	record, ok := r.records[petID+":"+factType]
	if !ok {
		return calendarapp.FactRecord{}, sql.ErrNoRows
	}
	return record, nil
}

type countingExecutor struct {
	called bool
}

func (e *countingExecutor) Execute(context.Context, RunInput) (RunDecision, error) {
	e.called = true
	return RunDecision{Status: RunFailed, EventType: "run.failed", Data: map[string]any{"message": "should not execute"}}, nil
}

type routingServiceExecutor struct {
	intent        IntentDecision
	decision      RunDecision
	routeCalled   bool
	executeCalled bool
}

func (e *routingServiceExecutor) Route(context.Context, IntentInput) (IntentDecision, error) {
	e.routeCalled = true
	return e.intent, nil
}

func (e *routingServiceExecutor) Execute(context.Context, RunInput) (RunDecision, error) {
	e.executeCalled = true
	return e.decision, nil
}

type quotaExecutor struct{}

func (quotaExecutor) Execute(context.Context, RunInput) (RunDecision, error) {
	return RunDecision{}, NewExecutorError("provider_quota_exhausted", false, 0, errors.New("quota exhausted"))
}

func (e *contextExecutor) Execute(_ context.Context, input RunInput) (RunDecision, error) {
	e.input = input
	return e.decision, nil
}

func TestServiceCreateAndProcessRun(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	createAskSchema(t, db)
	repository, err := NewRepository(db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewService(repository, servicePetRepository{pet: petapp.Pet{ID: "pet-1", FamilyID: "family-1", Name: "团子"}}, serviceExecutor{decision: RunDecision{Status: RunWaitingInput, RiskLevel: RiskUnknown, EventType: "assistant.question", Data: map[string]any{"question": "什么时候开始？"}}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.CreateSession(context.Background(), "family-1", "user-1", "pet-1", "最近没精神", "create-1")
	if err != nil {
		t.Fatal(err)
	}
	if result.Session.TurnCount != 1 || result.Run.Status != RunQueued || len(result.Events) != 1 {
		t.Fatalf("created result = %+v", result)
	}
	processed, err := service.ProcessRun(context.Background(), "family-1", result.Session.ID, result.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if processed.Run.Status != RunWaitingInput || processed.Run.RowVersion != 3 || processed.Run.CompletedAt != nil || processed.Session.Status != SessionActive || len(processed.Events) != 8 {
		t.Fatalf("processed result = %+v", processed)
	}
	if processed.Events[2].Type != "run.progress" || !strings.Contains(processed.Events[2].Data, `"stage":"intent_routing"`) || processed.Events[3].Type != "run.progress" || !strings.Contains(processed.Events[3].Data, `"stage":"input_reviewing"`) || processed.Events[4].Type != "run.progress" || !strings.Contains(processed.Events[4].Data, `"stage":"context_ready"`) || processed.Events[5].Type != "run.progress" || !strings.Contains(processed.Events[5].Data, `"stage":"risk_checking"`) || processed.Events[6].Type != "run.progress" || !strings.Contains(processed.Events[6].Data, `"stage":"response_generating"`) || processed.Events[7].Type != "assistant.question" {
		t.Fatalf("events = %+v", processed.Events)
	}
	repeated, err := service.ProcessRun(context.Background(), "family-1", result.Session.ID, result.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(repeated.Events) != 8 || repeated.Run.Status != RunWaitingInput {
		t.Fatalf("repeated result = %+v", repeated)
	}
}

func TestServicePersistsAssistantDeltasFromStreamingExecutor(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	createAskSchema(t, db)
	repository, err := NewRepository(db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	executor := streamingServiceExecutor{deltas: []string{"目前需要", "密切观察"}, decision: RunDecision{Status: RunWaitingInput, RiskLevel: RiskUnknown, EventType: "assistant.question", Data: map[string]any{"question": "什么时候开始？"}}}
	service, err := NewService(repository, servicePetRepository{pet: petapp.Pet{ID: "pet-1", FamilyID: "family-1", Name: "团子"}}, executor)
	if err != nil {
		t.Fatal(err)
	}
	created, err := service.CreateSession(context.Background(), "family-1", "user-1", "pet-1", "最近没精神", "create-stream")
	if err != nil {
		t.Fatal(err)
	}
	processed, err := service.ProcessRun(context.Background(), "family-1", created.Session.ID, created.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(processed.Events) != 10 || processed.Events[7].Type != "assistant.delta" || processed.Events[8].Type != "assistant.delta" || processed.Events[9].Type != "assistant.question" {
		t.Fatalf("events = %+v", processed.Events)
	}
}

func TestServiceCompletesSimpleGreetingWithoutExecutor(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	createAskSchema(t, db)
	repository, err := NewRepository(db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	executor := &countingExecutor{}
	service, err := NewService(repository, servicePetRepository{pet: petapp.Pet{ID: "pet-1", FamilyID: "family-1", Name: "团子"}}, executor)
	if err != nil {
		t.Fatal(err)
	}
	created, err := service.CreateSession(context.Background(), "family-1", "user-1", "pet-1", "你好", "create-greeting")
	if err != nil {
		t.Fatal(err)
	}
	processed, err := service.ProcessRun(context.Background(), "family-1", created.Session.ID, created.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if executor.called || processed.Run.Status != RunCompleted || processed.Events[3].Type != "assistant.completed" || !strings.Contains(processed.Events[3].Data, `"intent":"casual_chat"`) {
		t.Fatalf("processed = %+v, executor called = %t", processed, executor.called)
	}
}

func TestServiceRoutesPetHealthToExecutor(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	createAskSchema(t, db)
	repository, err := NewRepository(db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	executor := &routingServiceExecutor{intent: IntentDecision{Intent: IntentPetHealth}, decision: RunDecision{Status: RunWaitingInput, RiskLevel: RiskUnknown, EventType: "assistant.question", Data: map[string]any{"question": "症状从什么时候开始？"}}}
	service, err := NewService(repository, servicePetRepository{pet: petapp.Pet{ID: "pet-1", FamilyID: "family-1", Name: "团子"}}, executor)
	if err != nil {
		t.Fatal(err)
	}
	created, err := service.CreateSession(context.Background(), "family-1", "user-1", "pet-1", "团子的状态不太对", "create-health-route")
	if err != nil {
		t.Fatal(err)
	}
	processed, err := service.ProcessRun(context.Background(), "family-1", created.Session.ID, created.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !executor.routeCalled || !executor.executeCalled || processed.Run.Status != RunWaitingInput || processed.Events[7].Type != "assistant.question" {
		t.Fatalf("processed = %+v, route called = %t, execute called = %t", processed, executor.routeCalled, executor.executeCalled)
	}
}

func TestServiceRoutesAmbiguousInputToQuestion(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	createAskSchema(t, db)
	repository, err := NewRepository(db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	executor := &routingServiceExecutor{intent: IntentDecision{Intent: IntentAmbiguous, Question: "你想查询宠物记录，还是咨询健康问题？"}}
	service, err := NewService(repository, servicePetRepository{pet: petapp.Pet{ID: "pet-1", FamilyID: "family-1", Name: "团子"}}, executor)
	if err != nil {
		t.Fatal(err)
	}
	created, err := service.CreateSession(context.Background(), "family-1", "user-1", "pet-1", "帮我看看", "create-ambiguous-route")
	if err != nil {
		t.Fatal(err)
	}
	processed, err := service.ProcessRun(context.Background(), "family-1", created.Session.ID, created.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !executor.routeCalled || executor.executeCalled || processed.Run.Status != RunWaitingInput || processed.Events[3].Type != "assistant.question" {
		t.Fatalf("processed = %+v, route called = %t, execute called = %t", processed, executor.routeCalled, executor.executeCalled)
	}
}

func TestServicePersistsQuotaFailureMessage(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	createAskSchema(t, db)
	repository, err := NewRepository(db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewService(repository, servicePetRepository{pet: petapp.Pet{ID: "pet-1", FamilyID: "family-1", Name: "团子"}}, quotaExecutor{})
	if err != nil {
		t.Fatal(err)
	}
	created, err := service.CreateSession(context.Background(), "family-1", "user-1", "pet-1", "最近总是没精神", "create-quota")
	if err != nil {
		t.Fatal(err)
	}
	processed, err := service.ProcessRun(context.Background(), "family-1", created.Session.ID, created.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	lastEvent := processed.Events[len(processed.Events)-1]
	if processed.Run.Status != RunFailed || processed.Run.ErrorCode != "provider_quota_exhausted" || lastEvent.Type != "run.failed" || !strings.Contains(lastEvent.Data, "AI 服务额度暂时不可用") {
		t.Fatalf("processed = %+v", processed)
	}
}

func TestServiceProcessRunVersionRejectsStaleJob(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	createAskSchema(t, db)
	repository, err := NewRepository(db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewService(repository, servicePetRepository{pet: petapp.Pet{ID: "pet-1", FamilyID: "family-1"}}, DeterministicExecutor{})
	if err != nil {
		t.Fatal(err)
	}
	created, err := service.CreateSession(context.Background(), "family-1", "user-1", "pet-1", "问题", "create-version")
	if err != nil {
		t.Fatal(err)
	}
	stale, err := service.ProcessRunVersion(context.Background(), "family-1", created.Session.ID, created.Run.ID, created.Run.RowVersion+1)
	if err != nil {
		t.Fatal(err)
	}
	if stale.Run.Status != RunQueued || stale.Run.RowVersion != created.Run.RowVersion || len(stale.Events) != 1 {
		t.Fatalf("stale job result = %+v", stale)
	}
	processed, err := service.ProcessRunVersion(context.Background(), "family-1", created.Session.ID, created.Run.ID, created.Run.RowVersion)
	if err != nil {
		t.Fatal(err)
	}
	if processed.Run.Status != RunWaitingInput {
		t.Fatalf("processed run status = %s", processed.Run.Status)
	}
}

func TestServiceProcessRunCompletesMultiPetFactWithoutExecutor(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	createAskSchema(t, db)
	repository, err := NewRepository(db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	executor := &countingExecutor{}
	service, err := NewService(repository, servicePetRepository{pets: []petapp.Pet{{ID: "pet-1", Name: "旺仔"}, {ID: "pet-2", Name: "球球"}}}, executor)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	service.SetCalendarRepository(factCalendarRepository{records: map[string]calendarapp.FactRecord{
		"pet-1:bath": {ID: "record-1", Content: "旺仔洗澡", OccurredAt: at},
	}})
	created, _, err := service.CreateSessionFromInput(context.Background(), "family-1", "user-1", "旺仔和球球上次洗澡分别是什么时候", "create-2")
	if err != nil {
		t.Fatal(err)
	}
	processed, err := service.ProcessRun(context.Background(), "family-1", created.Session.ID, created.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if executor.called {
		t.Fatal("executor was called for deterministic fact query")
	}
	if processed.Run.Status != RunCompleted || processed.Run.RiskLevel != RiskGreen || processed.Events[3].Type != "fact.completed" {
		t.Fatalf("processed = %+v", processed)
	}
	if !strings.Contains(processed.Events[3].Data, `"fact_type":"bath"`) || !strings.Contains(processed.Events[3].Data, `"pet_id":"pet-1"`) || !strings.Contains(processed.Events[3].Data, `"pet_id":"pet-2"`) {
		t.Fatalf("fact event data = %s", processed.Events[3].Data)
	}
	if !strings.Contains(processed.Events[3].Data, `"found":false`) {
		t.Fatalf("missing fact was not represented = %s", processed.Events[3].Data)
	}
}

func TestServiceCreateSessionFromInputPersistsMultiplePets(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	createAskSchema(t, db)
	repository, err := NewRepository(db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewService(repository, servicePetRepository{pets: []petapp.Pet{{ID: "pet-1", Name: "旺仔"}, {ID: "pet-2", Name: "球球"}}}, DeterministicExecutor{})
	if err != nil {
		t.Fatal(err)
	}
	created, resolution, err := service.CreateSessionFromInput(context.Background(), "family-1", "user-1", "旺仔和球球上次洗澡分别是什么时候", "create-3")
	if err != nil {
		t.Fatal(err)
	}
	if resolution.Status != PetResolveResolved || len(created.Session.Pets) != 2 || created.Session.Pets[0].PetID != "pet-1" || created.Session.Pets[1].PetID != "pet-2" {
		t.Fatalf("resolution=%+v session=%+v", resolution, created.Session)
	}
	rows, err := db.Query("SELECT pet_id, mention, sort_order FROM ask_session_pets WHERE session_id = ? ORDER BY sort_order", created.Session.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var values []string
	for rows.Next() {
		var petID, mention string
		var sortOrder int
		if err := rows.Scan(&petID, &mention, &sortOrder); err != nil {
			t.Fatal(err)
		}
		values = append(values, petID+":"+mention+":"+strconv.Itoa(sortOrder))
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if strings.Join(values, ",") != "pet-1:旺仔:0,pet-2:球球:1" {
		t.Fatalf("session pets = %v", values)
	}
}

func TestServiceCreateSessionFromInputAllowsSimpleChatWithoutPetName(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	createAskSchema(t, db)
	repository, err := NewRepository(db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	executor := &countingExecutor{}
	service, err := NewService(repository, servicePetRepository{pets: []petapp.Pet{{ID: "pet-1", Name: "旺仔"}, {ID: "pet-2", Name: "球球"}}}, executor)
	if err != nil {
		t.Fatal(err)
	}
	created, resolution, err := service.CreateSessionFromInput(context.Background(), "family-1", "user-1", "你好", "create-chat")
	if err != nil {
		t.Fatal(err)
	}
	if resolution.Status != PetResolveResolved || len(created.Session.Pets) != 1 || created.Session.Pets[0].PetID != "pet-1" {
		t.Fatalf("resolution=%+v session=%+v", resolution, created.Session)
	}
	processed, err := service.ProcessRun(context.Background(), "family-1", created.Session.ID, created.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if executor.called {
		t.Fatal("executor was called for simple chat")
	}
	if processed.Run.Status != RunCompleted || processed.Events[len(processed.Events)-1].Type != "assistant.completed" {
		t.Fatalf("processed = %+v", processed)
	}
}

func TestServiceListsFamilyPetsWithoutPetName(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	createAskSchema(t, db)
	repository, err := NewRepository(db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	executor := &countingExecutor{}
	service, err := NewService(repository, servicePetRepository{pets: []petapp.Pet{{ID: "pet-1", Name: "旺仔"}, {ID: "pet-2", Name: "球球"}}}, executor)
	if err != nil {
		t.Fatal(err)
	}
	created, resolution, err := service.CreateSessionFromInput(context.Background(), "family-1", "user-1", "你知道我家有哪些宠物吗", "create-family-query")
	if err != nil {
		t.Fatal(err)
	}
	if resolution.Status != PetResolveResolved || len(created.Session.Pets) != 1 || created.Session.Pets[0].PetID != "pet-1" {
		t.Fatalf("resolution=%+v session=%+v", resolution, created.Session)
	}
	processed, err := service.ProcessRun(context.Background(), "family-1", created.Session.ID, created.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if executor.called {
		t.Fatal("executor was called for family pet query")
	}
	completed := processed.Events[len(processed.Events)-1]
	if processed.Run.Status != RunCompleted || completed.Type != "family.pets.completed" {
		t.Fatalf("processed = %+v", processed)
	}
	if !strings.Contains(completed.Data, `"pet_id":"pet-1"`) || !strings.Contains(completed.Data, `"pet_name":"旺仔"`) || !strings.Contains(completed.Data, `"pet_id":"pet-2"`) || !strings.Contains(completed.Data, `"pet_name":"球球"`) {
		t.Fatalf("family pets event data = %s", completed.Data)
	}
}

func TestServiceCreateSessionIsIdempotent(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	createAskSchema(t, db)
	repository, err := NewRepository(db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewService(repository, servicePetRepository{pets: []petapp.Pet{{ID: "pet-1", Name: "旺仔"}}}, DeterministicExecutor{})
	if err != nil {
		t.Fatal(err)
	}
	created, _, err := service.CreateSessionFromInput(context.Background(), "family-1", "user-1", "旺仔最近没精神", "same-create")
	if err != nil {
		t.Fatal(err)
	}
	replayed, _, err := service.CreateSessionFromInput(context.Background(), "family-1", "user-1", "旺仔最近没精神", "same-create")
	if err != nil {
		t.Fatal(err)
	}
	if replayed.Session.ID != created.Session.ID || replayed.Run.ID != created.Run.ID {
		t.Fatalf("replayed result = %+v", replayed)
	}
	if _, _, err := service.CreateSessionFromInput(context.Background(), "family-1", "user-1", "旺仔开始呕吐", "same-create"); err == nil {
		t.Fatal("reused create idempotency key error = nil")
	}
	var sessionCount, messageCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM ask_sessions").Scan(&sessionCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM ask_messages").Scan(&messageCount); err != nil {
		t.Fatal(err)
	}
	if sessionCount != 1 || messageCount != 1 {
		t.Fatalf("session count = %d, message count = %d", sessionCount, messageCount)
	}
}

func TestServiceProcessRunBuildsContextSnapshot(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	createAskSchema(t, db)
	repository, err := NewRepository(db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	executor := &contextExecutor{decision: RunDecision{Status: RunWaitingInput, RiskLevel: RiskUnknown, EventType: "assistant.question", Data: map[string]any{"question": "补充信息"}}}
	service, err := NewService(repository, servicePetRepository{pet: petapp.Pet{ID: "pet-1", FamilyID: "family-1", Name: "团子"}}, executor)
	if err != nil {
		t.Fatal(err)
	}
	service.SetCalendarRepository(contextCalendarRepository{records: []calendarapp.ContextRecord{{ID: "record-1", Category: "medical", Content: "完成疫苗接种"}}})
	created, err := service.CreateSession(context.Background(), "family-1", "user-1", "pet-1", "第一轮问题", "create-4")
	if err != nil {
		t.Fatal(err)
	}
	processed, err := service.ProcessRun(context.Background(), "family-1", created.Session.ID, created.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if executor.input.Context.Pet.ID != "pet-1" || executor.input.Context.Pet.Name != "团子" {
		t.Fatalf("pet context = %+v", executor.input.Context.Pet)
	}
	if executor.input.Context.Version != DefaultContextVersion || executor.input.Context.Pet.Breed != "田园猫" || !executor.input.Context.Pet.Sterilized || executor.input.Context.Pet.Allergies != "鸡肉" {
		t.Fatalf("profile context = %+v", executor.input.Context)
	}
	if len(executor.input.Context.RecentTurns) != 1 || executor.input.Context.RecentTurns[0].Input != "第一轮问题" {
		t.Fatalf("recent turns = %+v", executor.input.Context.RecentTurns)
	}
	if len(executor.input.Context.RecentRecords) != 1 || executor.input.Context.RecentRecords[0].Content != "完成疫苗接种" {
		t.Fatalf("recent records = %+v", executor.input.Context.RecentRecords)
	}
	if len(executor.input.Context.Sources) != 6 || executor.input.Context.Sources[0].Name != "pet_base" || executor.input.Context.Sources[1].Name != "ask_messages" || executor.input.Context.Sources[4].Name != "calendar_records" || executor.input.Context.Sources[5].Name != "ask_turns" {
		t.Fatalf("context sources = %+v", executor.input.Context.Sources)
	}
	if len(executor.input.Context.Messages) != 1 || executor.input.Context.Messages[0].Role != "user" || executor.input.Context.Messages[0].Content != "第一轮问题" {
		t.Fatalf("context messages = %+v", executor.input.Context.Messages)
	}
	if len(executor.input.Context.Events) != 1 || executor.input.Context.Events[0].Tag != "medical_record" {
		t.Fatalf("context events = %+v", executor.input.Context.Events)
	}
	if !strings.Contains(processed.Events[1].Data, `"context_version":"ask-context-v5"`) || !strings.Contains(processed.Events[1].Data, `"char_count":`) || strings.Contains(processed.Events[1].Data, "鸡肉") {
		t.Fatalf("context audit data = %s", processed.Events[1].Data)
	}
}

func TestServiceEscalatesAndCompletesSession(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	createAskSchema(t, db)
	repository, err := NewRepository(db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewService(repository, servicePetRepository{pet: petapp.Pet{ID: "pet-1", FamilyID: "family-1"}}, serviceExecutor{decision: RunDecision{Status: RunEscalated, RiskLevel: RiskRed, EventType: "risk.escalated", Data: map[string]any{"action": "立即就医"}}})
	if err != nil {
		t.Fatal(err)
	}
	created, err := service.CreateSession(context.Background(), "family-1", "user-1", "pet-1", "呼吸困难", "create-5")
	if err != nil {
		t.Fatal(err)
	}
	processed, err := service.ProcessRun(context.Background(), "family-1", created.Session.ID, created.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if processed.Session.Status != SessionEscalated || processed.Session.RiskLevel != RiskRed || processed.Run.Status != RunEscalated {
		t.Fatalf("processed result = %+v", processed)
	}
}

func TestServiceRuleEscalationSkipsExecutor(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	createAskSchema(t, db)
	repository, err := NewRepository(db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewService(repository, servicePetRepository{pet: petapp.Pet{ID: "pet-1", FamilyID: "family-1"}}, serviceExecutor{decision: RunDecision{Status: RunWaitingInput, RiskLevel: RiskUnknown, EventType: "assistant.question", Data: map[string]any{"question": "不应执行"}}})
	if err != nil {
		t.Fatal(err)
	}
	created, err := service.CreateSession(context.Background(), "family-1", "user-1", "pet-1", "现在呼吸困难", "create-6")
	if err != nil {
		t.Fatal(err)
	}
	processed, err := service.ProcessRun(context.Background(), "family-1", created.Session.ID, created.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if processed.Run.Status != RunEscalated || processed.Run.RiskLevel != RiskRed || processed.Events[4].Type != "risk.escalated" {
		t.Fatalf("processed result = %+v", processed)
	}
	if strings.Contains(processed.Events[4].Data, "不应执行") {
		t.Fatalf("executor result was used: %s", processed.Events[4].Data)
	}
}

func TestServiceValidatesCreateInput(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	createAskSchema(t, db)
	repository, err := NewRepository(db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewService(repository, servicePetRepository{}, DeterministicExecutor{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateSession(context.Background(), "family-1", "user-1", "pet-1", "", "create-7"); err == nil {
		t.Fatal("empty input error = nil")
	}
	if _, err := service.CreateSession(context.Background(), "family-1", "user-1", "", "问题", "create-8"); err == nil {
		t.Fatal("empty pet error = nil")
	}
}

func TestServiceExecutorFailureIsPersistedAsFailed(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	createAskSchema(t, db)
	repository, err := NewRepository(db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewService(repository, servicePetRepository{pet: petapp.Pet{ID: "pet-1"}}, failingExecutor{})
	if err != nil {
		t.Fatal(err)
	}
	created, err := service.CreateSession(context.Background(), "family-1", "user-1", "pet-1", "问题", "create-9")
	if err != nil {
		t.Fatal(err)
	}
	processed, err := service.ProcessRun(context.Background(), "family-1", created.Session.ID, created.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if processed.Run.Status != RunFailed || processed.Run.ErrorCode != "executor_failed" {
		t.Fatalf("processed result = %+v", processed)
	}
}

func TestServiceRetryableExecutorFailureRemainsRunning(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	createAskSchema(t, db)
	repository, err := NewRepository(db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewService(repository, servicePetRepository{pet: petapp.Pet{ID: "pet-1"}}, retryableExecutor{})
	if err != nil {
		t.Fatal(err)
	}
	created, err := service.CreateSession(context.Background(), "family-1", "user-1", "pet-1", "问题", "create-retryable")
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.ProcessRun(context.Background(), "family-1", created.Session.ID, created.Run.ID)
	code, retryable := ExecutorErrorDetails(err)
	if code != "provider_unavailable" || !retryable {
		t.Fatalf("error = %v, code = %s, retryable = %t", err, code, retryable)
	}
	run, err := repository.GetRun(context.Background(), created.Session.ID, created.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != RunRunning || run.ErrorCode != "" {
		t.Fatalf("run = %+v", run)
	}
}

func TestServiceReplyResumesSameRunAndPersistsMessages(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	createAskSchema(t, db)
	repository, err := NewRepository(db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewService(repository, servicePetRepository{pet: petapp.Pet{ID: "pet-1", FamilyID: "family-1"}}, DeterministicExecutor{})
	if err != nil {
		t.Fatal(err)
	}
	created, err := service.CreateSession(context.Background(), "family-1", "user-1", "pet-1", "最近没精神", "create-10")
	if err != nil {
		t.Fatal(err)
	}
	waiting, err := service.ProcessRun(context.Background(), "family-1", created.Session.ID, created.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	replied, err := service.Reply(context.Background(), "family-1", "user-1", created.Session.ID, created.Run.ID, "现在呼吸困难", waiting.Run.RowVersion, "reply-1")
	if err != nil {
		t.Fatal(err)
	}
	if replied.Session.TurnCount != 1 || replied.Run.ID != created.Run.ID || replied.Run.TurnID != created.Run.TurnID || replied.Run.Status != RunQueued || replied.Run.RowVersion != 4 || replied.Run.ClarificationCount != 1 || len(replied.Events) != 1 || replied.Events[0].Sequence != 9 {
		t.Fatalf("reply result = %+v", replied)
	}
	turn, err := repository.GetTurn(context.Background(), created.Session.ID, created.Run.TurnID)
	if err != nil {
		t.Fatal(err)
	}
	if turn.TurnIndex != 0 || turn.Input != "最近没精神" || turn.Status != RunQueued {
		t.Fatalf("resumed turn = %+v", turn)
	}
	loaded, err := repository.GetSession(context.Background(), "family-1", created.Session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.TurnCount != 1 {
		t.Fatalf("session turn count = %d", loaded.TurnCount)
	}
	messages, err := repository.ListMessages(context.Background(), created.Session.ID, created.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 3 || messages[0].Role != "user" || messages[1].Role != "question" || messages[2].Role != "user" || messages[2].Content != "现在呼吸困难" {
		t.Fatalf("messages = %+v", messages)
	}
	replayed, err := service.Reply(context.Background(), "family-1", "user-1", created.Session.ID, created.Run.ID, "现在呼吸困难", waiting.Run.RowVersion, "reply-1")
	if err != nil {
		t.Fatal(err)
	}
	if replayed.Run.ID != replied.Run.ID || replayed.Run.RowVersion != replied.Run.RowVersion {
		t.Fatalf("replayed result = %+v", replayed)
	}
	if _, err := service.Reply(context.Background(), "family-1", "user-1", created.Session.ID, created.Run.ID, "不同回答", waiting.Run.RowVersion, "reply-1"); err == nil {
		t.Fatal("reused idempotency key error = nil")
	}
	processed, err := service.ProcessRun(context.Background(), "family-1", created.Session.ID, replied.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if processed.Run.Status != RunEscalated || processed.Run.RiskLevel != RiskRed || processed.Events[12].Type != "risk.escalated" {
		t.Fatalf("resumed red result = %+v", processed)
	}
}

func TestServiceReplyValidatesStateVersionAndClarificationLimit(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	createAskSchema(t, db)
	repository, err := NewRepository(db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewService(repository, servicePetRepository{pet: petapp.Pet{ID: "pet-1", FamilyID: "family-1"}}, DeterministicExecutor{})
	if err != nil {
		t.Fatal(err)
	}
	created, err := service.CreateSession(context.Background(), "family-1", "user-1", "pet-1", "问题", "create-11")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Reply(context.Background(), "family-1", "user-1", created.Session.ID, created.Run.ID, "回答", created.Run.RowVersion, "reply-queued"); err == nil {
		t.Fatal("queued run reply error = nil")
	}
	waiting, err := service.ProcessRun(context.Background(), "family-1", created.Session.ID, created.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	first, err := service.Reply(context.Background(), "family-1", "user-1", created.Session.ID, created.Run.ID, "第一轮回答", waiting.Run.RowVersion, "reply-limit-1")
	if err != nil {
		t.Fatal(err)
	}
	waiting, err = service.ProcessRun(context.Background(), "family-1", created.Session.ID, first.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Reply(context.Background(), "family-1", "user-1", created.Session.ID, created.Run.ID, "过期版本", first.Run.RowVersion, "reply-stale"); err == nil {
		t.Fatal("stale version reply error = nil")
	}
	second, err := service.Reply(context.Background(), "family-1", "user-1", created.Session.ID, first.Run.ID, "第二轮回答", waiting.Run.RowVersion, "reply-limit-2")
	if err != nil {
		t.Fatal(err)
	}
	waiting, err = service.ProcessRun(context.Background(), "family-1", created.Session.ID, second.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	third, err := service.Reply(context.Background(), "family-1", "user-1", created.Session.ID, second.Run.ID, "第三轮回答", waiting.Run.RowVersion, "reply-limit-3")
	if err != nil {
		t.Fatal(err)
	}
	processed, err := service.ProcessRun(context.Background(), "family-1", created.Session.ID, third.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if processed.Run.Status != RunFailed || processed.Run.ErrorCode != "clarification_limit_reached" || processed.Run.ClarificationCount != MaxClarifications {
		t.Fatalf("clarification limit result = %+v", processed)
	}
	if _, err := service.Reply(context.Background(), "family-1", "user-1", created.Session.ID, third.Run.ID, "超过上限", processed.Run.RowVersion, "reply-limit-4"); err == nil {
		t.Fatal("clarification limit reply error = nil")
	}
}

type failingExecutor struct{}

func (failingExecutor) Execute(context.Context, RunInput) (RunDecision, error) {
	return RunDecision{}, errors.New("provider unavailable")
}

type retryableExecutor struct{}

func (retryableExecutor) Execute(context.Context, RunInput) (RunDecision, error) {
	return RunDecision{}, NewExecutorError("provider_unavailable", true, 0, errors.New("unavailable"))
}
