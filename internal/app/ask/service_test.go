package ask

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	calendarapp "github.com/balancetheworld/wechat-pet/internal/app/calendar"
	petapp "github.com/balancetheworld/wechat-pet/internal/app/pet"
	_ "github.com/mattn/go-sqlite3"
)

type servicePetRepository struct {
	pet petapp.Pet
}

func (r servicePetRepository) List(context.Context, string) ([]petapp.Pet, error) {
	return []petapp.Pet{r.pet}, nil
}

func (r servicePetRepository) Get(context.Context, string, string) (petapp.Pet, error) {
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

func (servicePetRepository) Update(context.Context, string, string, string, string) (petapp.Pet, error) {
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
	result, err := service.CreateSession(context.Background(), "family-1", "user-1", "pet-1", "最近没精神")
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
	if processed.Run.Status != RunWaitingInput || processed.Session.Status != SessionActive || len(processed.Events) != 3 {
		t.Fatalf("processed result = %+v", processed)
	}
	if processed.Events[2].Type != "assistant.question" {
		t.Fatalf("event type = %q", processed.Events[2].Type)
	}
	repeated, err := service.ProcessRun(context.Background(), "family-1", result.Session.ID, result.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(repeated.Events) != 3 || repeated.Run.Status != RunWaitingInput {
		t.Fatalf("repeated result = %+v", repeated)
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
	created, err := service.CreateSession(context.Background(), "family-1", "user-1", "pet-1", "第一轮问题")
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
	if len(executor.input.Context.Sources) != 5 || executor.input.Context.Sources[0].Name != "pet_base" || executor.input.Context.Sources[3].Name != "calendar_records" || executor.input.Context.Sources[4].Name != "ask_turns" {
		t.Fatalf("context sources = %+v", executor.input.Context.Sources)
	}
	if len(executor.input.Context.Events) != 1 || executor.input.Context.Events[0].Tag != "medical_record" {
		t.Fatalf("context events = %+v", executor.input.Context.Events)
	}
	if !strings.Contains(processed.Events[1].Data, `"context_version":"ask-context-v4"`) || !strings.Contains(processed.Events[1].Data, `"char_count":`) || strings.Contains(processed.Events[1].Data, "鸡肉") {
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
	created, err := service.CreateSession(context.Background(), "family-1", "user-1", "pet-1", "呼吸困难")
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
	created, err := service.CreateSession(context.Background(), "family-1", "user-1", "pet-1", "现在呼吸困难")
	if err != nil {
		t.Fatal(err)
	}
	processed, err := service.ProcessRun(context.Background(), "family-1", created.Session.ID, created.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if processed.Run.Status != RunEscalated || processed.Run.RiskLevel != RiskRed || processed.Events[2].Type != "risk.escalated" {
		t.Fatalf("processed result = %+v", processed)
	}
	if strings.Contains(processed.Events[2].Data, "不应执行") {
		t.Fatalf("executor result was used: %s", processed.Events[2].Data)
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
	if _, err := service.CreateSession(context.Background(), "family-1", "user-1", "pet-1", ""); err == nil {
		t.Fatal("empty input error = nil")
	}
	if _, err := service.CreateSession(context.Background(), "family-1", "user-1", "", "问题"); err == nil {
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
	created, err := service.CreateSession(context.Background(), "family-1", "user-1", "pet-1", "问题")
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

func TestServiceReplyCreatesFollowUpTurn(t *testing.T) {
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
	created, err := service.CreateSession(context.Background(), "family-1", "user-1", "pet-1", "最近没精神")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ProcessRun(context.Background(), "family-1", created.Session.ID, created.Run.ID); err != nil {
		t.Fatal(err)
	}
	replied, err := service.Reply(context.Background(), "family-1", created.Session.ID, created.Run.ID, "现在呼吸困难")
	if err != nil {
		t.Fatal(err)
	}
	if replied.Session.TurnCount != 2 || replied.Run.Status != RunQueued || len(replied.Events) != 1 {
		t.Fatalf("reply result = %+v", replied)
	}
	turn, err := repository.GetTurn(context.Background(), created.Session.ID, replied.Run.TurnID)
	if err != nil {
		t.Fatal(err)
	}
	if turn.TurnIndex != 1 || turn.Input == "" {
		t.Fatalf("follow-up turn = %+v", turn)
	}
	loaded, err := repository.GetSession(context.Background(), "family-1", created.Session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.TurnCount != 2 {
		t.Fatalf("session turn count = %d", loaded.TurnCount)
	}
	if _, err := service.Reply(context.Background(), "family-1", created.Session.ID, created.Run.ID, "重复回答"); err == nil {
		t.Fatal("stale run reply error = nil")
	}
	processed, err := service.ProcessRun(context.Background(), "family-1", created.Session.ID, replied.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if processed.Run.Status != RunEscalated || processed.Run.RiskLevel != RiskRed || processed.Events[2].Type != "risk.escalated" {
		t.Fatalf("follow-up red result = %+v", processed)
	}
}

func TestServiceReplyValidatesStateAndTurnLimit(t *testing.T) {
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
	created, err := service.CreateSession(context.Background(), "family-1", "user-1", "pet-1", "问题")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Reply(context.Background(), "family-1", created.Session.ID, created.Run.ID, "回答"); err == nil {
		t.Fatal("queued run reply error = nil")
	}
	if _, err := service.ProcessRun(context.Background(), "family-1", created.Session.ID, created.Run.ID); err != nil {
		t.Fatal(err)
	}
	first, err := service.Reply(context.Background(), "family-1", created.Session.ID, created.Run.ID, "第一轮回答")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ProcessRun(context.Background(), "family-1", created.Session.ID, first.Run.ID); err != nil {
		t.Fatal(err)
	}
	second, err := service.Reply(context.Background(), "family-1", created.Session.ID, first.Run.ID, "第二轮回答")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ProcessRun(context.Background(), "family-1", created.Session.ID, second.Run.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Reply(context.Background(), "family-1", created.Session.ID, second.Run.ID, "超过上限"); err == nil {
		t.Fatal("turn limit error = nil")
	}
}

type failingExecutor struct{}

func (failingExecutor) Execute(context.Context, RunInput) (RunDecision, error) {
	return RunDecision{}, errors.New("provider unavailable")
}
