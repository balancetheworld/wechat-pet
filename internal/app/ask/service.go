package ask

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	calendarapp "github.com/balancetheworld/wechat-pet/internal/app/calendar"
	petapp "github.com/balancetheworld/wechat-pet/internal/app/pet"
	appErrors "github.com/balancetheworld/wechat-pet/internal/pkg/errors"
)

const (
	DefaultPromptVersion      = "ask-prompt-v1"
	DefaultRuleVersion        = "ask-rules-v1"
	DefaultKnowledgeVersion   = "ask-knowledge-v1"
	MaxInputLength            = 4000
	MaxIdempotencyKeyLength   = 128
	MaxClarifications         = 3
	ContextTurnLimit          = 5
	ContextMaxChars           = 6000
	DefaultContextVersion     = "ask-context-v5"
	createSessionOperation    = "ask.session.create"
	createPetSessionOperation = "ask.pet_session.create"
	replyRunOperation         = "ask.run.reply"
)

type Executor interface {
	Execute(context.Context, RunInput) (RunDecision, error)
}

type contextTurnRepository interface {
	ListContextTurns(context.Context, string, int) ([]ContextTurn, error)
}

type petProfileReader interface {
	GetProfile(context.Context, string, string) (petapp.PetProfile, error)
}

type petHealthReader interface {
	GetHealth(context.Context, string, string) (petapp.PetHealth, error)
}

type calendarRecordReader interface {
	ListRecentRecords(context.Context, string, string, time.Time, int) ([]calendarapp.ContextRecord, error)
}

type RunInput struct {
	Session Session
	Turn    Turn
	Run     Run
	Context ContextSnapshot
}

type RunDecision struct {
	Status    RunStatus
	RiskLevel RiskLevel
	ErrorCode string
	EventType string
	Data      map[string]any
}

type ExecutionResult struct {
	Session Session
	Run     Run
	Events  []Event
}

func (s *Service) GetSnapshot(ctx context.Context, familyID, sessionID string) (Snapshot, error) {
	session, err := s.GetSession(ctx, strings.TrimSpace(familyID), strings.TrimSpace(sessionID))
	if err != nil {
		return Snapshot{}, err
	}
	turns, err := s.repository.GetSnapshotTurns(ctx, session.ID)
	if err != nil {
		return Snapshot{}, appErrors.Internal(err)
	}
	return Snapshot{Session: session, Turns: turns}, nil
}

var (
	ErrRunNotWaitingInput  = errors.New("ask run is not waiting for input")
	ErrAskTurnLimitReached = errors.New("ask turn limit reached")
)

func (s *Service) GetSession(ctx context.Context, familyID, sessionID string) (Session, error) {
	value, err := s.repository.GetSession(ctx, strings.TrimSpace(familyID), strings.TrimSpace(sessionID))
	if errors.Is(err, ErrSessionNotFound) {
		return Session{}, appErrors.NotFound("问问会话不存在")
	}
	if err != nil {
		return Session{}, appErrors.Internal(err)
	}
	value, err = s.loadSessionPets(ctx, value)
	if err != nil {
		return Session{}, appErrors.Internal(err)
	}
	return value, nil
}

func (s *Service) loadSessionPets(ctx context.Context, value Session) (Session, error) {
	repository, ok := s.repository.(interface {
		ListSessionPets(context.Context, string) ([]SessionPet, error)
	})
	if !ok {
		return value, nil
	}
	pets, err := repository.ListSessionPets(ctx, value.ID)
	if err != nil {
		return Session{}, err
	}
	value.Pets = pets
	return value, nil
}

func (s *Service) GetEvents(ctx context.Context, familyID, sessionID, runID string, after int) ([]Event, error) {
	if strings.TrimSpace(familyID) == "" || strings.TrimSpace(sessionID) == "" || strings.TrimSpace(runID) == "" || after < 0 {
		return nil, appErrors.InvalidParam("事件查询参数无效")
	}
	if _, err := s.GetSession(ctx, familyID, sessionID); err != nil {
		return nil, err
	}
	if _, err := s.repository.GetRun(ctx, strings.TrimSpace(sessionID), strings.TrimSpace(runID)); err != nil {
		if errors.Is(err, ErrRunNotFound) {
			return nil, appErrors.NotFound("问问执行不存在")
		}
		return nil, appErrors.Internal(err)
	}
	values, err := s.repository.ListEvents(ctx, strings.TrimSpace(sessionID), strings.TrimSpace(runID), after)
	if err != nil {
		return nil, appErrors.Internal(err)
	}
	return values, nil
}

func (s *Service) GetExecution(ctx context.Context, familyID, sessionID, runID string) (ExecutionResult, error) {
	session, err := s.GetSession(ctx, strings.TrimSpace(familyID), strings.TrimSpace(sessionID))
	if err != nil {
		return ExecutionResult{}, err
	}
	run, err := s.repository.GetRun(ctx, session.ID, strings.TrimSpace(runID))
	if errors.Is(err, ErrRunNotFound) {
		return ExecutionResult{}, appErrors.NotFound("问问执行不存在")
	}
	if err != nil {
		return ExecutionResult{}, appErrors.Internal(err)
	}
	events, err := s.repository.ListEvents(ctx, session.ID, run.ID, 0)
	if err != nil {
		return ExecutionResult{}, appErrors.Internal(err)
	}
	return ExecutionResult{Session: session, Run: run, Events: events}, nil
}

type Service struct {
	repository Repository
	pets       petapp.Repository
	executor   Executor
	rules      RuleEngine
	now        func() time.Time
	versions   Versions
	calendar   calendarRecordReader
}

type Versions struct {
	PromptVersion    string
	RuleVersion      string
	KnowledgeVersion string
}

func NewService(repository Repository, pets petapp.Repository, executor Executor, versions ...Versions) (*Service, error) {
	if repository == nil {
		return nil, errors.New("ask service repository is required")
	}
	if pets == nil {
		return nil, errors.New("ask service pet repository is required")
	}
	if executor == nil {
		return nil, errors.New("ask service executor is required")
	}
	value := Versions{PromptVersion: DefaultPromptVersion, RuleVersion: DefaultRuleVersion, KnowledgeVersion: DefaultKnowledgeVersion}
	if len(versions) > 0 {
		if versions[0].PromptVersion != "" {
			value.PromptVersion = versions[0].PromptVersion
		}
		if versions[0].RuleVersion != "" {
			value.RuleVersion = versions[0].RuleVersion
		}
		if versions[0].KnowledgeVersion != "" {
			value.KnowledgeVersion = versions[0].KnowledgeVersion
		}
	}
	return &Service{repository: repository, pets: pets, executor: executor, rules: DeterministicRuleEngine{}, now: time.Now, versions: value}, nil
}

func (s *Service) SetCalendarRepository(repository any) {
	if value, ok := repository.(calendarRecordReader); ok {
		s.calendar = value
	}
}

func (s *Service) CreateSession(ctx context.Context, familyID, userID, petID, input, idempotencyKey string) (ExecutionResult, error) {
	familyID = strings.TrimSpace(familyID)
	userID = strings.TrimSpace(userID)
	petID = strings.TrimSpace(petID)
	input = strings.TrimSpace(input)
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if familyID == "" || userID == "" {
		return ExecutionResult{}, appErrors.Unauthorized()
	}
	if petID == "" {
		return ExecutionResult{}, appErrors.InvalidParam("宠物 ID 不能为空")
	}
	if input == "" {
		return ExecutionResult{}, appErrors.InvalidParam("问题内容不能为空")
	}
	if len([]rune(input)) > MaxInputLength {
		return ExecutionResult{}, appErrors.InvalidParam("问题内容不能超过 4000 个字符")
	}
	if err := validateIdempotencyKey(idempotencyKey); err != nil {
		return ExecutionResult{}, err
	}
	hash := idempotencyHash(familyID, petID, input)
	if result, found, err := s.replayExecution(ctx, userID, createPetSessionOperation, idempotencyKey, hash); err != nil || found {
		return result, err
	}
	pet, err := s.pets.Get(ctx, familyID, petID)
	if errors.Is(err, petapp.ErrNotFound) {
		return ExecutionResult{}, appErrors.NotFound("宠物不存在")
	}
	if err != nil {
		return ExecutionResult{}, appErrors.Internal(err)
	}
	result, _, err := s.createSessionWithPets(ctx, familyID, userID, input, []petapp.Pet{pet}, createPetSessionOperation, idempotencyKey, hash)
	return result, err
}

func (s *Service) CreateSessionFromInput(ctx context.Context, familyID, userID, input, idempotencyKey string) (ExecutionResult, PetResolution, error) {
	familyID = strings.TrimSpace(familyID)
	userID = strings.TrimSpace(userID)
	input = strings.TrimSpace(input)
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if familyID == "" || userID == "" {
		return ExecutionResult{}, PetResolution{}, appErrors.Unauthorized()
	}
	if input == "" {
		return ExecutionResult{}, PetResolution{}, appErrors.InvalidParam("问题内容不能为空")
	}
	if len([]rune(input)) > MaxInputLength {
		return ExecutionResult{}, PetResolution{}, appErrors.InvalidParam("问题内容不能超过 4000 个字符")
	}
	if err := validateIdempotencyKey(idempotencyKey); err != nil {
		return ExecutionResult{}, PetResolution{}, err
	}
	hash := idempotencyHash(familyID, input)
	if result, found, err := s.replayExecution(ctx, userID, createSessionOperation, idempotencyKey, hash); err != nil || found {
		return result, PetResolution{Status: PetResolveResolved, Input: input}, err
	}
	pets, err := s.pets.List(ctx, familyID)
	if err != nil {
		return ExecutionResult{}, PetResolution{}, appErrors.Internal(err)
	}
	resolution := ResolvePets(input, pets)
	if resolution.Status == PetResolveNone {
		return ExecutionResult{}, resolution, appErrors.InvalidParam("请在问题中补充宠物名称")
	}
	if resolution.Status == PetResolveAmbiguous {
		return ExecutionResult{}, resolution, appErrors.Conflict("宠物名称存在歧义，请补充更多信息")
	}
	return s.createSessionWithPets(ctx, familyID, userID, input, resolution.Resolved, createSessionOperation, idempotencyKey, hash)
}

func (s *Service) createSessionWithPets(ctx context.Context, familyID, userID, input string, pets []petapp.Pet, operation, idempotencyKey, hash string) (ExecutionResult, PetResolution, error) {
	if len(pets) == 0 {
		return ExecutionResult{}, PetResolution{}, appErrors.InvalidParam("请在问题中补充宠物名称")
	}
	result, err := s.createSession(ctx, familyID, userID, input, pets, operation, idempotencyKey, hash)
	resolution := PetResolution{Status: PetResolveResolved, Resolved: pets, Input: input}
	return result, resolution, err
}

func (s *Service) createSession(ctx context.Context, familyID, userID, input string, pets []petapp.Pet, operation, idempotencyKey, hash string) (ExecutionResult, error) {
	now := s.now().UTC()
	sessionID, err := newID()
	if err != nil {
		return ExecutionResult{}, appErrors.Internal(err)
	}
	turnID, err := newID()
	if err != nil {
		return ExecutionResult{}, appErrors.Internal(err)
	}
	runID, err := newID()
	if err != nil {
		return ExecutionResult{}, appErrors.Internal(err)
	}
	eventID, err := newID()
	if err != nil {
		return ExecutionResult{}, appErrors.Internal(err)
	}
	messageID, err := newID()
	if err != nil {
		return ExecutionResult{}, appErrors.Internal(err)
	}
	idempotencyID, err := newID()
	if err != nil {
		return ExecutionResult{}, appErrors.Internal(err)
	}
	session := Session{ID: sessionID, FamilyID: familyID, CreatedBy: userID, PetID: pets[0].ID, Status: SessionActive, RiskLevel: RiskUnknown, TurnCount: 1, PromptVersion: s.versions.PromptVersion, RuleVersion: s.versions.RuleVersion, KnowledgeVersion: s.versions.KnowledgeVersion, CreatedAt: now, UpdatedAt: now}
	turn := Turn{ID: turnID, SessionID: sessionID, TurnIndex: 0, Status: RunQueued, Input: input, SelectedRunID: runID, CreatedAt: now}
	run := Run{ID: runID, SessionID: sessionID, TurnID: turnID, RunIndex: 0, RowVersion: 1, Status: RunQueued, RiskLevel: RiskUnknown, RuleVersion: s.versions.RuleVersion, PromptVersion: s.versions.PromptVersion, CreatedAt: now}
	event := Event{ID: eventID, SessionID: sessionID, TurnID: turnID, RunID: runID, Sequence: 1, Type: "run.queued", Data: `{}`, CreatedAt: now}
	message := Message{ID: messageID, SessionID: sessionID, TurnID: turnID, RunID: runID, Role: "user", Content: input, CreatedAt: now}
	sessionPets := make([]SessionPet, 0, len(pets))
	for index, pet := range pets {
		sessionPets = append(sessionPets, SessionPet{PetID: pet.ID, PetName: pet.Name, Mention: pet.Name, SortOrder: index})
	}
	session.Pets = sessionPets
	result := ExecutionResult{Session: session, Run: run, Events: []Event{event}}
	responseData, err := json.Marshal(result)
	if err != nil {
		return ExecutionResult{}, appErrors.Internal(err)
	}
	idempotency := IdempotencyRecord{ID: idempotencyID, FamilyID: familyID, UserID: userID, Operation: operation, IdempotencyKey: idempotencyKey, RequestHash: hash, ResponseData: string(responseData), SessionID: sessionID, TurnID: turnID, RunID: runID, CreatedAt: now}
	if err := s.repository.CreateSessionRunIdempotent(ctx, session, sessionPets, turn, run, event, message, idempotency); err != nil {
		if errors.Is(err, ErrIdempotencyConflict) {
			if replay, found, replayErr := s.replayExecution(ctx, userID, operation, idempotencyKey, hash); replayErr != nil || found {
				return replay, replayErr
			}
		}
		return ExecutionResult{}, appErrors.Internal(err)
	}
	return result, nil
}

func (s *Service) ProcessRun(ctx context.Context, familyID, sessionID, runID string) (ExecutionResult, error) {
	return s.processRun(ctx, familyID, sessionID, runID, 0)
}

func (s *Service) ProcessRunVersion(ctx context.Context, familyID, sessionID, runID string, expectedVersion int) (ExecutionResult, error) {
	return s.processRun(ctx, familyID, sessionID, runID, expectedVersion)
}

func (s *Service) processRun(ctx context.Context, familyID, sessionID, runID string, expectedVersion int) (ExecutionResult, error) {
	session, err := s.repository.GetSession(ctx, strings.TrimSpace(familyID), strings.TrimSpace(sessionID))
	if err != nil {
		if errors.Is(err, ErrSessionNotFound) {
			return ExecutionResult{}, appErrors.NotFound("问问会话不存在")
		}
		return ExecutionResult{}, appErrors.Internal(err)
	}
	session, err = s.loadSessionPets(ctx, session)
	if err != nil {
		return ExecutionResult{}, appErrors.Internal(err)
	}
	run, err := s.repository.GetRun(ctx, session.ID, strings.TrimSpace(runID))
	if err != nil {
		if errors.Is(err, ErrRunNotFound) {
			return ExecutionResult{}, appErrors.NotFound("问问执行不存在")
		}
		return ExecutionResult{}, appErrors.Internal(err)
	}
	events, err := s.repository.ListEvents(ctx, session.ID, run.ID, 0)
	if err != nil {
		return ExecutionResult{}, appErrors.Internal(err)
	}
	if expectedVersion > 0 && run.RowVersion != expectedVersion {
		return ExecutionResult{Session: session, Run: run, Events: events}, nil
	}
	if run.Status != RunQueued {
		return ExecutionResult{Session: session, Run: run, Events: events}, nil
	}
	startedEventID, err := newID()
	if err != nil {
		return ExecutionResult{}, appErrors.Internal(err)
	}
	finishedEventID, err := newID()
	if err != nil {
		return ExecutionResult{}, appErrors.Internal(err)
	}
	messageID, err := newID()
	if err != nil {
		return ExecutionResult{}, appErrors.Internal(err)
	}
	turn, err := s.repository.GetTurn(ctx, session.ID, run.TurnID)
	if err != nil {
		if errors.Is(err, ErrTurnNotFound) {
			return ExecutionResult{}, appErrors.NotFound("问问轮次不存在")
		}
		return ExecutionResult{}, appErrors.Internal(err)
	}
	pet, err := s.pets.Get(ctx, session.FamilyID, session.PetID)
	if err != nil {
		if errors.Is(err, petapp.ErrNotFound) {
			return ExecutionResult{}, appErrors.NotFound("宠物不存在")
		}
		return ExecutionResult{}, appErrors.Internal(err)
	}
	var recentTurns []ContextTurn
	if contextRepository, ok := s.repository.(contextTurnRepository); ok {
		recentTurns, err = contextRepository.ListContextTurns(ctx, session.ID, ContextTurnLimit)
		if err != nil {
			return ExecutionResult{}, appErrors.Internal(err)
		}
	}
	messages, err := s.repository.ListMessages(ctx, session.ID, run.ID)
	if err != nil {
		return ExecutionResult{}, appErrors.Internal(err)
	}
	contextMessages := make([]ContextMessage, 0, len(messages))
	for _, message := range messages {
		contextMessages = append(contextMessages, ContextMessage{Role: message.Role, Content: message.Content, CreatedAt: message.CreatedAt})
	}
	contextSnapshot := ContextSnapshot{Version: DefaultContextVersion, CapturedAt: s.now().UTC(), Pet: PetContext{ID: pet.ID, Name: pet.Name}, RecentTurns: recentTurns, Messages: contextMessages, Sources: []ContextSource{{Name: "pet_base", Version: "pet-base-v1", ItemCount: 1}, {Name: "ask_messages", Version: "ask-messages-v1", ItemCount: len(contextMessages)}}}
	if profileRepository, ok := s.pets.(petProfileReader); ok {
		if profile, profileErr := profileRepository.GetProfile(ctx, session.FamilyID, session.PetID); profileErr == nil {
			contextSnapshot.Sources = append(contextSnapshot.Sources, ContextSource{Name: "pet_profile", Version: "pet-profile-v1", ItemCount: 1})
			contextSnapshot.Pet.Breed = profile.Breed
			contextSnapshot.Pet.Gender = profile.Gender
			contextSnapshot.Pet.Sterilized = profile.Sterilized
			if profile.Birthday != nil {
				contextSnapshot.Pet.Birthday = *profile.Birthday
			}
		}
	}
	if healthRepository, ok := s.pets.(petHealthReader); ok {
		if health, healthErr := healthRepository.GetHealth(ctx, session.FamilyID, session.PetID); healthErr == nil {
			contextSnapshot.Sources = append(contextSnapshot.Sources, ContextSource{Name: "pet_health", Version: "pet-health-v1", ItemCount: 1})
			contextSnapshot.Pet.HealthStatus = health.Status
			contextSnapshot.Pet.Allergies = health.Allergies
			contextSnapshot.Pet.LongTermMedication = health.LongTermMedication
		}
	}
	if s.calendar != nil {
		recentRecords, recordErr := s.calendar.ListRecentRecords(ctx, session.FamilyID, session.PetID, s.now().UTC().AddDate(0, 0, -90), ContextTurnLimit)
		if recordErr == nil {
			contextSnapshot.RecentRecords = recentRecords
			contextSnapshot.Sources = append(contextSnapshot.Sources, ContextSource{Name: "calendar_records", Version: "calendar-records-v1", ItemCount: len(recentRecords), Truncated: len(recentRecords) >= ContextTurnLimit})
		}
	}
	if _, ok := s.repository.(contextTurnRepository); ok {
		contextSnapshot.Sources = append(contextSnapshot.Sources, ContextSource{Name: "ask_turns", Version: "ask-turns-v1", ItemCount: len(recentTurns), Truncated: len(recentTurns) >= ContextTurnLimit})
	}
	contextSnapshot.Events = normalizeContextEvents(recentTurns, contextSnapshot.RecentRecords)
	contextSnapshot = compactContextSnapshot(contextSnapshot, ContextMaxChars)
	now := s.now().UTC()
	startedEvent := Event{ID: startedEventID, SessionID: session.ID, TurnID: run.TurnID, RunID: run.ID, Sequence: nextSequence(events), Type: "run.started", Data: contextAuditData(contextSnapshot), CreatedAt: now}
	if err := s.repository.TransitionRun(ctx, run.ID, run.RowVersion, RunQueued, RunRunning, RiskUnknown, "", now, startedEvent, Message{}); err != nil {
		if errors.Is(err, ErrRunStateConflict) {
			return ExecutionResult{}, appErrors.Conflict("问问执行状态已改变")
		}
		return ExecutionResult{}, appErrors.Internal(err)
	}
	run.Status = RunRunning
	run.RowVersion++
	run.StartedAt = &now
	turn.Status = RunRunning
	events = append(events, startedEvent)
	riskText := turn.Input
	for _, message := range messages {
		if message.Role == "user" && message.Content != turn.Input {
			riskText += "\n" + message.Content
		}
	}
	risk := s.rules.Evaluate(RiskInput{Text: riskText})
	var decision RunDecision
	if risk.Level == RiskRed {
		decision = RunDecision{Status: RunEscalated, RiskLevel: RiskRed, EventType: "risk.escalated", Data: map[string]any{"trigger_code": risk.TriggerCode, "message": risk.Message, "action": risk.Action}}
	} else {
		factType := DetectFactType(turn.Input)
		if factType != FactUnknown {
			if reader, ok := s.calendar.(factReader); ok {
				sessionPets := session.Pets
				if len(sessionPets) == 0 {
					sessionPets = []SessionPet{{PetID: pet.ID, PetName: pet.Name, Mention: pet.Name, SortOrder: 0}}
				}
				var data map[string]any
				data, err = buildFactResult(ctx, reader, session.FamilyID, sessionPets, factType)
				if err == nil {
					decision = RunDecision{Status: RunCompleted, RiskLevel: RiskGreen, EventType: "fact.completed", Data: data}
				}
			}
		}
		if decision.Status == "" {
			decision, err = s.executor.Execute(ctx, RunInput{Session: session, Turn: turn, Run: run, Context: contextSnapshot})
		}
		if err != nil {
			decision = RunDecision{Status: RunFailed, ErrorCode: "executor_failed", EventType: "run.failed", Data: map[string]any{"message": "当前暂时无法完成分析"}}
		} else if err := ValidateAnalysisOutput(decision); err != nil {
			decision = RunDecision{Status: RunFailed, ErrorCode: "invalid_analysis_output", EventType: "run.failed", Data: map[string]any{"message": "当前暂时无法完成分析"}}
		}
	}
	if decision.Status == RunWaitingInput && run.ClarificationCount >= MaxClarifications {
		decision = RunDecision{Status: RunFailed, RiskLevel: decision.RiskLevel, ErrorCode: "clarification_limit_reached", EventType: "run.failed", Data: map[string]any{"message": "补充信息次数已达上限，请重新描述问题"}}
	}
	if decision.Status != RunWaitingInput && decision.Status != RunCompleted && decision.Status != RunEscalated && decision.Status != RunFailed {
		decision.Status = RunFailed
		decision.ErrorCode = "invalid_executor_status"
		decision.EventType = "run.failed"
		decision.Data = map[string]any{"message": "当前暂时无法完成分析"}
	}
	if decision.EventType == "" {
		decision.EventType = "run.completed"
	}
	if decision.RiskLevel == "" {
		decision.RiskLevel = RiskUnknown
	}
	data, marshalErr := json.Marshal(decision.Data)
	if marshalErr != nil {
		data = []byte(`{"message":"当前暂时无法完成分析"}`)
	}
	finishedAt := s.now().UTC()
	finishedEvent := Event{ID: finishedEventID, SessionID: session.ID, TurnID: run.TurnID, RunID: run.ID, Sequence: nextSequence(events), Type: decision.EventType, Data: string(data), CreatedAt: finishedAt}
	questionMessage := Message{}
	if decision.Status == RunWaitingInput {
		questionMessage = Message{ID: messageID, SessionID: session.ID, TurnID: run.TurnID, RunID: run.ID, Role: "question", Content: strings.TrimSpace(decision.Data["question"].(string)), CreatedAt: finishedAt}
	}
	if err := s.repository.TransitionRun(ctx, run.ID, run.RowVersion, RunRunning, decision.Status, decision.RiskLevel, decision.ErrorCode, finishedAt, finishedEvent, questionMessage); err != nil {
		if errors.Is(err, ErrRunStateConflict) {
			return ExecutionResult{}, appErrors.Conflict("问问执行状态已改变")
		}
		return ExecutionResult{}, appErrors.Internal(err)
	}
	run.Status = decision.Status
	run.RowVersion++
	run.RiskLevel = decision.RiskLevel
	run.ErrorCode = decision.ErrorCode
	run.LeaseOwner = ""
	run.LeaseExpiresAt = nil
	run.NextAttemptAt = nil
	if decision.Status == RunCompleted || decision.Status == RunEscalated || decision.Status == RunFailed || decision.Status == RunCanceled || decision.Status == RunInterrupted {
		run.CompletedAt = &finishedAt
	}
	session.Status = sessionStatusForRun(decision.Status)
	session.RiskLevel = decision.RiskLevel
	session.UpdatedAt = finishedAt
	if decision.Status == RunCompleted || decision.Status == RunEscalated {
		session.CompletedAt = &finishedAt
	}
	events = append(events, finishedEvent)
	return ExecutionResult{Session: session, Run: run, Events: events}, nil
}

func (s *Service) Reply(ctx context.Context, familyID, userID, sessionID, runID, input string, expectedVersion int, idempotencyKey string) (ExecutionResult, error) {
	familyID = strings.TrimSpace(familyID)
	userID = strings.TrimSpace(userID)
	sessionID = strings.TrimSpace(sessionID)
	runID = strings.TrimSpace(runID)
	input = strings.TrimSpace(input)
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if familyID == "" || userID == "" {
		return ExecutionResult{}, appErrors.Unauthorized()
	}
	if sessionID == "" || runID == "" || expectedVersion < 1 {
		return ExecutionResult{}, appErrors.InvalidParam("问问参数无效")
	}
	if input == "" {
		return ExecutionResult{}, appErrors.InvalidParam("回答内容不能为空")
	}
	if len([]rune(input)) > MaxInputLength {
		return ExecutionResult{}, appErrors.InvalidParam("回答内容不能超过 4000 个字符")
	}
	if err := validateIdempotencyKey(idempotencyKey); err != nil {
		return ExecutionResult{}, err
	}
	hash := idempotencyHash(familyID, sessionID, runID, input, strconv.Itoa(expectedVersion))
	if result, found, err := s.replayExecution(ctx, userID, replyRunOperation, idempotencyKey, hash); err != nil || found {
		return result, err
	}
	session, err := s.repository.GetSession(ctx, familyID, sessionID)
	if err != nil {
		if errors.Is(err, ErrSessionNotFound) {
			return ExecutionResult{}, appErrors.NotFound("问问会话不存在")
		}
		return ExecutionResult{}, appErrors.Internal(err)
	}
	session, err = s.loadSessionPets(ctx, session)
	if err != nil {
		return ExecutionResult{}, appErrors.Internal(err)
	}
	if session.Status != SessionActive {
		return ExecutionResult{}, appErrors.Conflict("问问会话已结束")
	}
	run, err := s.repository.GetRun(ctx, session.ID, runID)
	if err != nil {
		if errors.Is(err, ErrRunNotFound) {
			return ExecutionResult{}, appErrors.NotFound("问问执行不存在")
		}
		return ExecutionResult{}, appErrors.Internal(err)
	}
	if run.Status != RunWaitingInput {
		return ExecutionResult{}, appErrors.Conflict("当前问问执行不需要回答")
	}
	if run.RowVersion != expectedVersion {
		return ExecutionResult{}, appErrors.Conflict("问问执行版本已改变")
	}
	if run.ClarificationCount >= MaxClarifications {
		return ExecutionResult{}, appErrors.Conflict("问问追问次数已达上限")
	}
	currentTurn, err := s.repository.GetTurn(ctx, session.ID, run.TurnID)
	if err != nil {
		if errors.Is(err, ErrTurnNotFound) {
			return ExecutionResult{}, appErrors.NotFound("问问轮次不存在")
		}
		return ExecutionResult{}, appErrors.Internal(err)
	}
	if currentTurn.TurnIndex != session.TurnCount-1 || currentTurn.SelectedRunID != run.ID {
		return ExecutionResult{}, appErrors.Conflict("只能回答当前问问追问")
	}
	events, err := s.repository.ListEvents(ctx, session.ID, run.ID, 0)
	if err != nil {
		return ExecutionResult{}, appErrors.Internal(err)
	}
	now := s.now().UTC()
	messageID, err := newID()
	if err != nil {
		return ExecutionResult{}, appErrors.Internal(err)
	}
	eventID, err := newID()
	if err != nil {
		return ExecutionResult{}, appErrors.Internal(err)
	}
	idempotencyID, err := newID()
	if err != nil {
		return ExecutionResult{}, appErrors.Internal(err)
	}
	event := Event{ID: eventID, SessionID: session.ID, TurnID: run.TurnID, RunID: run.ID, Sequence: nextSequence(events), Type: "run.queued", Data: `{}`, CreatedAt: now}
	message := Message{ID: messageID, SessionID: session.ID, TurnID: run.TurnID, RunID: run.ID, Role: "user", Content: input, CreatedAt: now}
	previousRun := run
	run.Status = RunQueued
	run.RiskLevel = RiskUnknown
	run.RowVersion++
	run.ClarificationCount++
	run.CompletedAt = nil
	run.ErrorCode = ""
	run.LeaseOwner = ""
	run.LeaseExpiresAt = nil
	run.AttemptCount = 0
	run.NextAttemptAt = nil
	session.Status = SessionActive
	session.RiskLevel = RiskUnknown
	session.UpdatedAt = now
	session.CompletedAt = nil
	result := ExecutionResult{Session: session, Run: run, Events: []Event{event}}
	responseData, err := json.Marshal(result)
	if err != nil {
		return ExecutionResult{}, appErrors.Internal(err)
	}
	idempotency := IdempotencyRecord{ID: idempotencyID, FamilyID: familyID, UserID: userID, Operation: replyRunOperation, IdempotencyKey: idempotencyKey, RequestHash: hash, ResponseData: string(responseData), SessionID: session.ID, TurnID: run.TurnID, RunID: run.ID, CreatedAt: now}
	if err := s.repository.ResumeRun(ctx, previousRun, now, event, message, idempotency); err != nil {
		if errors.Is(err, ErrRunStateConflict) {
			if replay, found, replayErr := s.replayExecution(ctx, userID, replyRunOperation, idempotencyKey, hash); replayErr != nil || found {
				return replay, replayErr
			}
			return ExecutionResult{}, appErrors.Conflict("问问执行状态已改变")
		}
		if errors.Is(err, ErrIdempotencyConflict) {
			if replay, found, replayErr := s.replayExecution(ctx, userID, replyRunOperation, idempotencyKey, hash); replayErr != nil || found {
				return replay, replayErr
			}
		}
		return ExecutionResult{}, appErrors.Internal(err)
	}
	return result, nil
}

func sessionStatusForRun(status RunStatus) SessionStatus {
	switch status {
	case RunCompleted:
		return SessionCompleted
	case RunEscalated:
		return SessionEscalated
	case RunCanceled:
		return SessionCanceled
	default:
		return SessionActive
	}
}

func nextSequence(events []Event) int {
	if len(events) == 0 {
		return 1
	}
	return events[len(events)-1].Sequence + 1
}

func validateIdempotencyKey(value string) error {
	if value == "" {
		return appErrors.InvalidParam("Idempotency-Key 不能为空")
	}
	if len([]rune(value)) > MaxIdempotencyKeyLength {
		return appErrors.InvalidParam("Idempotency-Key 不能超过 128 个字符")
	}
	return nil
}

func idempotencyHash(values ...string) string {
	value := sha256.Sum256([]byte(strings.Join(values, "\x00")))
	return hex.EncodeToString(value[:])
}

func (s *Service) replayExecution(ctx context.Context, userID, operation, key, hash string) (ExecutionResult, bool, error) {
	value, err := s.repository.GetIdempotency(ctx, userID, operation, key)
	if errors.Is(err, ErrIdempotencyNotFound) {
		return ExecutionResult{}, false, nil
	}
	if err != nil {
		return ExecutionResult{}, false, appErrors.Internal(err)
	}
	if value.RequestHash != hash {
		return ExecutionResult{}, true, appErrors.Conflict("Idempotency-Key 已用于其他请求")
	}
	var result ExecutionResult
	if err := json.Unmarshal([]byte(value.ResponseData), &result); err != nil {
		return ExecutionResult{}, true, appErrors.Internal(err)
	}
	return result, true, nil
}

func newID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}
