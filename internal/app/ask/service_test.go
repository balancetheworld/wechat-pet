package ask

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

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

func (servicePetRepository) Update(context.Context, string, string, string, string) (petapp.Pet, error) {
	return petapp.Pet{}, errors.New("not implemented")
}

func (servicePetRepository) Delete(context.Context, string, string, string) error {
	return errors.New("not implemented")
}

// repeatModel 每次 Step 都返回同一份记录，用于多次追问/继续会话场景。
type repeatModel struct {
	records []ProtocolRecord
	inputs  []StepInput
}

func (m *repeatModel) Step(_ context.Context, input StepInput) ([]ProtocolRecord, error) {
	m.inputs = append(m.inputs, input)
	return m.records, nil
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
	model := &scriptedModel{responses: [][]ProtocolRecord{finalAnswerRecords(), finalAnswerRecords()}}
	service, repository := newV2Service(t, model, &fakeBusinessRead{})

	first, err := service.CreateSession(context.Background(), "family-1", "user-1", "pet-1", "最近没精神", "create-history")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ProcessRun(context.Background(), "family-1", first.Session.ID, first.Run.ID); err != nil {
		t.Fatal(err)
	}

	second, err := service.ContinueSession(context.Background(), "family-1", "user-1", first.Session.ID, "现在好多了", "continue-history")
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
	if len(historyTexts) != 1 || !strings.Contains(historyTexts[0], "最近没精神") || strings.Contains(historyTexts[0], "现在好多了") {
		t.Fatalf("history blocks = %+v", historyTexts)
	}

	replayed, err := service.ContinueSession(context.Background(), "family-1", "user-1", first.Session.ID, "现在好多了", "continue-history")
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
	replied, err := service.Reply(context.Background(), "family-1", "user-1", created.Session.ID, created.Run.ID, "现在呼吸困难", waiting.Run.RowVersion, "reply-1")
	if err != nil {
		t.Fatal(err)
	}
	if replied.Session.TurnCount != 1 || replied.Run.ID != created.Run.ID || replied.Run.TurnID != created.Run.TurnID || replied.Run.Status != RunQueued || replied.Run.ClarificationCount != 1 {
		t.Fatalf("reply result = %+v", replied)
	}
	turn, err := repository.GetTurn(context.Background(), created.Session.ID, created.Run.TurnID)
	if err != nil {
		t.Fatal(err)
	}
	if turn.TurnIndex != 0 || turn.Input != "最近没精神" || turn.Status != TurnAttached {
		t.Fatalf("resumed turn = %+v", turn)
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
	if processed.Run.Status != RunCompleted || processed.Run.RiskLevel != RiskRed {
		t.Fatalf("resumed red result = %+v", processed)
	}
	if lastEvent(processed.Events).Type != "risk.escalated" {
		t.Fatalf("final event = %+v", lastEvent(processed.Events))
	}
}

func TestServiceReplyValidatesStateVersionAndAllowsUnlimitedClarifications(t *testing.T) {
	// 追问无限次：每次 ProcessRun 都返回 request_input（需要补信息）。
	model := &repeatModel{records: requestInputRecords()}
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
	for index := 3; index <= 5; index++ {
		current, err = service.Reply(context.Background(), "family-1", "user-1", created.Session.ID, current.Run.ID, "第"+strconv.Itoa(index)+"轮回答", current.Run.RowVersion, "reply-limit-"+strconv.Itoa(index))
		if err != nil {
			t.Fatal(err)
		}
		processed, processErr := service.ProcessRun(context.Background(), "family-1", created.Session.ID, current.Run.ID)
		if processErr != nil {
			t.Fatal(processErr)
		}
		if processed.Run.Status != RunWaitingInput || processed.Run.ErrorCode != "" || processed.Run.ClarificationCount != index {
			t.Fatalf("unlimited clarification result = %+v", processed)
		}
		current = processed
	}
}

var _ = time.Now
