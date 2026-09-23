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

type serviceCalendarRepository struct {
	since  time.Time
	before time.Time
}

func (r *serviceCalendarRepository) ListRecentRecords(_ context.Context, _, _ string, since, before time.Time, _ int) ([]calendarapp.ContextRecord, error) {
	r.since = since
	r.before = before
	return nil, nil
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

// repeatModel 每次 Step 都返回同一份记录，用于多次追问/继续会话场景。
type repeatModel struct {
	records []ProtocolRecord
	inputs  []StepInput
	usage   ModelUsage
}

func (m *repeatModel) Step(_ context.Context, input StepInput) (ModelStepResult, error) {
	m.inputs = append(m.inputs, input)
	usage := m.usage
	if usage == (ModelUsage{}) {
		usage = ModelUsage{Complete: true}
	}
	return ModelStepResult{Records: m.records, Usage: usage}, nil
}

// TestServiceProcessRunWithoutV2DependenciesDegrades 验证未注入 v2 决策循环依赖时，
// 非急症 Run 降级为 provider_unavailable 失败，不会永久停留在 running。
func TestServiceProcessRunWithoutV2DependenciesDegrades(t *testing.T) {
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
	service, err := NewService(repository, servicePetRepository{pet: petapp.Pet{ID: "pet-1", FamilyID: "family-1", Name: "团子"}})
	if err != nil {
		t.Fatal(err)
	}
	created, err := service.CreateSession(context.Background(), "family-1", "user-1", "pet-1", "最近没精神", "create-degrade")
	if err != nil {
		t.Fatal(err)
	}
	processed, err := service.ProcessRun(context.Background(), "family-1", created.Session.ID, created.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if processed.Run.Status != RunFailed || processed.Run.ErrorCode != "provider_unavailable" {
		t.Fatalf("processed = %+v", processed)
	}
	final := lastEvent(processed.Events)
	if final.Type != "run.failed" || !strings.Contains(final.Data, `"error_code":"provider_unavailable"`) {
		t.Fatalf("final event = %+v", final)
	}
}

// TestServiceCreateAndProcessRun 走 v2 决策循环：request_input 降级为 assistant.question。
func TestServiceCreateAndProcessRun(t *testing.T) {
	model := &scriptedModel{responses: [][]ProtocolRecord{requestInputRecords()}}
	service, _ := newV2Service(t, model, &fakeBusinessRead{})

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
	if processed.Run.Status != RunWaitingInput || processed.Run.RowVersion != 3 || processed.Run.CompletedAt != nil || processed.Session.Status != SessionActive {
		t.Fatalf("processed result = %+v", processed)
	}
	if len(model.inputs) == 0 {
		t.Fatal("v2 decision loop should invoke the model")
	}
	final := lastEvent(processed.Events)
	if final.Type != "assistant.question" {
		t.Fatalf("final event = %s, want assistant.question", final.Type)
	}
	repeated, err := service.ProcessRun(context.Background(), "family-1", result.Session.ID, result.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if repeated.Run.Status != RunWaitingInput {
		t.Fatalf("repeated result = %+v", repeated)
	}
}

// TestServiceContinuesSessionWithHistoryAndIdempotency 验证继续会话把历史消息组装进
// 决策循环上下文，且幂等键命中返回相同结果。
func TestServiceContinuesSessionWithHistoryAndIdempotency(t *testing.T) {
	firstAnswer := finalAnswerRecords()
	firstAnswer[2].Segment.Text = "不要自行使用阿莫西林"
	model := &scriptedModel{responses: [][]ProtocolRecord{firstAnswer, finalAnswerRecords()}}
	service, repository := newV2Service(t, model, &fakeBusinessRead{})

	first, err := service.CreateSession(context.Background(), "family-1", "user-1", "pet-1", "最近没精神", "create-history")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ProcessRun(context.Background(), "family-1", first.Session.ID, first.Run.ID); err != nil {
		t.Fatal(err)
	}

	second, err := service.ContinueSession(context.Background(), "family-1", "user-1", first.Session.ID, "你刚才说的药是什么？", "continue-history")
	if err != nil {
		t.Fatal(err)
	}
	if second.Session.TurnCount != 2 || second.Run.TurnID == first.Run.TurnID || second.Session.ID != first.Session.ID {
		t.Fatalf("continued execution = %+v", second)
	}
	if _, err := service.ProcessRun(context.Background(), "family-1", second.Session.ID, second.Run.ID); err != nil {
		t.Fatal(err)
	}
	// 第二轮决策上下文应包含第一轮的历史消息，当前输入从历史层剔除。
	if len(model.inputs) != 2 {
		t.Fatalf("model calls = %d, want 2", len(model.inputs))
	}
	var historyTexts []string
	for _, b := range model.inputs[1].Blocks {
		if b.Layer == LayerHistory {
			historyTexts = append(historyTexts, b.Text)
		}
	}
	if len(historyTexts) != 2 || historyTexts[0] != "user: 最近没精神" || historyTexts[1] != "assistant: 不要自行使用阿莫西林" {
		t.Fatalf("history blocks = %+v", historyTexts)
	}

	replayed, err := service.ContinueSession(context.Background(), "family-1", "user-1", first.Session.ID, "你刚才说的药是什么？", "continue-history")
	if err != nil {
		t.Fatal(err)
	}
	if replayed.Run.ID != second.Run.ID || replayed.Session.TurnCount != second.Session.TurnCount {
		t.Fatalf("replayed execution = %+v, want %+v", replayed, second)
	}
	turns, err := repository.GetSnapshotTurns(context.Background(), first.Session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 2 {
		t.Fatalf("snapshot turns = %d, want 2", len(turns))
	}
}

// TestServiceEscalatesAndCompletesSession 验证急症红风险短路，不进入决策循环。
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
	service, err := NewService(repository, servicePetRepository{pet: petapp.Pet{ID: "pet-1", FamilyID: "family-1", Name: "团子"}})
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
	if processed.Session.Status != SessionActive || processed.Session.RiskLevel != RiskRed || processed.Run.Status != RunCompleted {
		t.Fatalf("processed result = %+v", processed)
	}
	final := lastEvent(processed.Events)
	if final.Type != "risk.escalated" {
		t.Fatalf("final event = %s, want risk.escalated", final.Type)
	}
}

func TestServiceDoesNotRepeatHistoricalRiskEscalation(t *testing.T) {
	model := &scriptedModel{responses: [][]ProtocolRecord{finalAnswerRecords()}}
	service, _ := newV2Service(t, model, &fakeBusinessRead{})

	first, err := service.CreateSession(context.Background(), "family-1", "user-1", "pet-1", "呼吸困难", "create-risk-history")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ProcessRun(context.Background(), "family-1", first.Session.ID, first.Run.ID); err != nil {
		t.Fatal(err)
	}
	second, err := service.ContinueSession(context.Background(), "family-1", "user-1", first.Session.ID, "谢谢", "continue-risk-history")
	if err != nil {
		t.Fatal(err)
	}
	processed, err := service.ProcessRun(context.Background(), "family-1", second.Session.ID, second.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if processed.Run.Status != RunCompleted || processed.Run.RiskLevel == RiskRed || lastEvent(processed.Events).Type != "assistant.completed" {
		t.Fatalf("processed result = %+v", processed)
	}
	if len(model.inputs) != 1 {
		t.Fatalf("model calls = %d, want 1", len(model.inputs))
	}
}

// TestServiceProcessRunVersionRejectsStaleJob 验证状态版本不匹配时拒绝执行。
func TestServiceProcessRunVersionRejectsStaleJob(t *testing.T) {
	model := &scriptedModel{responses: [][]ProtocolRecord{requestInputRecords()}}
	service, _ := newV2Service(t, model, &fakeBusinessRead{})

	created, err := service.CreateSession(context.Background(), "family-1", "user-1", "pet-1", "问题", "create-version")
	if err != nil {
		t.Fatal(err)
	}
	stale, err := service.ProcessRunVersion(context.Background(), "family-1", created.Session.ID, created.Run.ID, created.Run.RowVersion+1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if stale.Run.Status != RunQueued || stale.Run.RowVersion != created.Run.RowVersion || len(stale.Events) != 1 {
		t.Fatalf("stale job result = %+v", stale)
	}
	processed, err := service.ProcessRunVersion(context.Background(), "family-1", created.Session.ID, created.Run.ID, created.Run.RowVersion, 0)
	if err != nil {
		t.Fatal(err)
	}
	if processed.Run.Status != RunWaitingInput {
		t.Fatalf("processed run status = %s", processed.Run.Status)
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
	service, err := NewService(repository, servicePetRepository{pets: []petapp.Pet{{ID: "pet-1", Name: "旺仔"}, {ID: "pet-2", Name: "球球"}}})
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

func TestLoadHealthContextIncludesEverySessionPet(t *testing.T) {
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
	pets := []petapp.Pet{{ID: "pet-1", FamilyID: "family-1", Name: "旺仔"}, {ID: "pet-2", FamilyID: "family-1", Name: "球球"}}
	service, err := NewService(repository, servicePetRepository{pets: pets})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := service.loadHealthContext(context.Background(), Session{ID: "session-1", FamilyID: "family-1", PetID: "pet-1", Pets: []SessionPet{{PetID: "pet-1"}, {PetID: "pet-2"}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Pets) != 2 || snapshot.Pets[0].Name != "旺仔" || snapshot.Pets[1].Name != "球球" {
		t.Fatalf("snapshot pets = %+v", snapshot.Pets)
	}
	blocks := referenceBlocks(snapshot)
	text := ""
	for _, block := range blocks {
		text += block.Text
	}
	if !strings.Contains(text, "旺仔") || !strings.Contains(text, "球球") {
		t.Fatalf("reference blocks = %+v", blocks)
	}
}

func TestLoadHealthContextUsesInjectedQueryWindow(t *testing.T) {
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
	service, err := NewService(repository, servicePetRepository{pet: petapp.Pet{ID: "pet-1", FamilyID: "family-1", Name: "旺仔"}})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 19, 10, 0, 0, 0, time.UTC)
	calendar := &serviceCalendarRepository{}
	service.now = func() time.Time { return now }
	service.SetCalendarRepository(calendar)

	snapshot, err := service.loadHealthContext(context.Background(), Session{ID: "session-1", FamilyID: "family-1", PetID: "pet-1"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !snapshot.CapturedAt.Equal(now) || !calendar.before.Equal(now) || !calendar.since.Equal(now.AddDate(0, 0, -90)) {
		t.Fatalf("snapshot=%s since=%s before=%s", snapshot.CapturedAt, calendar.since, calendar.before)
	}
}

func TestServiceCreateSessionFromInputDoesNotGuessPet(t *testing.T) {
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
	service, err := NewService(repository, servicePetRepository{pets: []petapp.Pet{{ID: "pet-1", Name: "旺仔"}, {ID: "pet-2", Name: "球球"}}})
	if err != nil {
		t.Fatal(err)
	}
	created, resolution, err := service.CreateSessionFromInput(context.Background(), "family-1", "user-1", "最近怎么样？", "create-unresolved")
	if err != nil {
		t.Fatal(err)
	}
	if resolution.Status != PetResolveNone || created.Session.PetID != "" || len(created.Session.Pets) != 2 || created.Run.Status != RunQueued {
		t.Fatalf("unresolved session should enter agent run, resolution=%+v execution=%+v", resolution, created)
	}
}

func TestServiceCreateSessionFromInputSinglePetWithoutNameEntersRun(t *testing.T) {
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
	service, err := NewService(repository, servicePetRepository{pets: []petapp.Pet{{ID: "pet-1", Name: "旺仔"}}})
	if err != nil {
		t.Fatal(err)
	}
	created, resolution, err := service.CreateSessionFromInput(context.Background(), "family-1", "user-1", "最近精神不好", "create-single-unresolved")
	if err != nil {
		t.Fatal(err)
	}
	if resolution.Status != PetResolveNone || created.Session.PetID != "" || len(created.Session.Pets) != 1 || created.Run.Status != RunQueued {
		t.Fatalf("single-pet unresolved session should enter agent run, resolution=%+v execution=%+v", resolution, created)
	}
	stored, err := repository.GetSession(context.Background(), "family-1", created.Session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.PetID != "" {
		t.Fatalf("stored resolved pet id = %q, want empty", stored.PetID)
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
	service, err := NewService(repository, servicePetRepository{pets: []petapp.Pet{{ID: "pet-1", Name: "旺仔"}}})
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

func TestServiceCreateSessionClosesCurrentSession(t *testing.T) {
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
	service, err := NewService(repository, servicePetRepository{pets: []petapp.Pet{{ID: "pet-1", Name: "旺仔"}}})
	if err != nil {
		t.Fatal(err)
	}
	first, _, err := service.CreateSessionFromInput(context.Background(), "family-1", "user-1", "旺仔最近没精神", "first-create")
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := service.CreateSessionFromInput(context.Background(), "family-1", "user-1", "旺仔今天怎么样", "second-create")
	if err != nil {
		t.Fatal(err)
	}
	previous, err := repository.GetSession(context.Background(), "family-1", first.Session.ID)
	if err != nil {
		t.Fatal(err)
	}
	current, err := repository.GetSession(context.Background(), "family-1", second.Session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if previous.Status != SessionClosed || previous.CompletedAt == nil {
		t.Fatalf("previous session = %+v", previous)
	}
	if current.Status != SessionActive || current.CompletedAt != nil {
		t.Fatalf("current session = %+v", current)
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
	service, err := NewService(repository, servicePetRepository{})
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

func TestServiceReplyResumesSameRunAndPersistsMessages(t *testing.T) {
	model := &scriptedModel{responses: [][]ProtocolRecord{requestInputRecords()}}
	service, repository := newV2Service(t, model, &fakeBusinessRead{})

	created, err := service.CreateSession(context.Background(), "family-1", "user-1", "pet-1", "最近没精神", "create-10")
	if err != nil {
		t.Fatal(err)
	}
	waiting, err := service.ProcessRun(context.Background(), "family-1", created.Session.ID, created.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.db.Exec("UPDATE ask_runs SET attempt_count = 3 WHERE id = ?", waiting.Run.ID); err != nil {
		t.Fatal(err)
	}
	waiting.Run, err = repository.GetRun(context.Background(), created.Session.ID, waiting.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	replied, err := service.Reply(context.Background(), "family-1", "user-1", created.Session.ID, created.Run.ID, "现在呼吸困难", waiting.Run.RowVersion, "reply-1")
	if err != nil {
		t.Fatal(err)
	}
	if replied.Session.TurnCount != 2 || replied.Run.ID != created.Run.ID || replied.Run.TurnID == created.Run.TurnID || replied.Run.Status != RunQueued || replied.Run.ClarificationCount != 1 || replied.Run.InputRevision != waiting.Run.InputRevision+1 || replied.Run.AttemptCount != 3 {
		t.Fatalf("reply result = %+v", replied)
	}
	turn, err := repository.GetTurn(context.Background(), created.Session.ID, replied.Run.TurnID)
	if err != nil {
		t.Fatal(err)
	}
	if turn.TurnIndex != 1 || turn.Input != "现在呼吸困难" || turn.Status != TurnReceived {
		t.Fatalf("resumed turn = %+v", turn)
	}
	messages, err := repository.ListMessages(context.Background(), created.Session.ID, created.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 3 || messages[0].Role != "user" || messages[1].Role != "question" || messages[2].Role != "user" || messages[2].Content != "现在呼吸困难" {
		t.Fatalf("messages = %+v", messages)
	}
	turns, err := repository.GetSnapshotTurns(context.Background(), created.Session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 2 || turns[0].Turn.ID != created.Run.TurnID || turns[1].Turn.ID != replied.Run.TurnID || len(turns[0].Messages) != 2 || len(turns[1].Messages) != 1 {
		t.Fatalf("snapshot turns = %+v", turns)
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
	if processed.Run.Status != RunCompleted || processed.Run.RiskLevel != RiskRed {
		t.Fatalf("resumed red result = %+v", processed)
	}
	if lastEvent(processed.Events).Type != "risk.escalated" {
		t.Fatalf("final event = %+v", lastEvent(processed.Events))
	}
}

func TestServiceReplyValidatesStateVersionAndPreservesCumulativeBudget(t *testing.T) {
	model := &repeatModel{records: requestInputRecords(), usage: ModelUsage{InputTokens: 1500, OutputTokens: 1000, TotalTokens: 2500, Complete: true}}
	service, _ := newV2Service(t, model, &fakeBusinessRead{})

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
	current := waiting
	for index := 3; index <= DefaultBudgetLimits(BudgetRun).MaxModelCalls+2; index++ {
		current, err = service.Reply(context.Background(), "family-1", "user-1", created.Session.ID, current.Run.ID, "第"+strconv.Itoa(index)+"轮回答", current.Run.RowVersion, "reply-limit-"+strconv.Itoa(index))
		if err != nil {
			t.Fatal(err)
		}
		processed, processErr := service.ProcessRun(context.Background(), "family-1", created.Session.ID, current.Run.ID)
		if processErr != nil {
			t.Fatal(processErr)
		}
		if processed.Run.Status == RunFailed {
			if processed.Run.ErrorCode != "budget_exhausted" || processed.Run.ClarificationCount != index {
				t.Fatalf("budget exhaustion result = %+v, model calls = %d", processed, len(model.inputs))
			}
			return
		}
		if processed.Run.Status != RunWaitingInput || processed.Run.ErrorCode != "" || processed.Run.ClarificationCount != index {
			t.Fatalf("clarification result = %+v", processed)
		}
		current = processed
	}
	t.Fatal("cumulative run budget was not exhausted")
}

var _ = time.Now
