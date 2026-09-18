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
	DefaultPromptVersion      = "ask-prompt-v2"
	DefaultRuleVersion        = "ask-rules-v1"
	DefaultKnowledgeVersion   = "ask-knowledge-v1"
	MaxInputLength            = 4000
	MaxIdempotencyKeyLength   = 128
	ContextTurnLimit          = 5
	ContextMaxChars           = 6000
	DefaultContextVersion     = "ask-context-v5"
	createSessionOperation    = "ask.session.create"
	createPetSessionOperation = "ask.pet_session.create"
	continueSessionOperation  = "ask.session.continue"
	replyRunOperation         = "ask.run.reply"
)

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

var ErrRunNotWaitingInput = errors.New("ask run is not waiting for input")

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
	rules      RuleEngine
	now        func() time.Time
	versions   Versions
	calendar   calendarRecordReader
	// v2 决策循环依赖（文档 4、6、7）：由 main 装配注入，供 processRun 走
	// RunDecisionLoop 使用。尚未注入时 processRun 降级为 provider_unavailable。
	agentModel   AgentModel
	catalog      *Catalog
	businessRead BusinessReadRepository
}

type Versions struct {
	PromptVersion    string
	RuleVersion      string
	KnowledgeVersion string
}

func NewService(repository Repository, pets petapp.Repository, versions ...Versions) (*Service, error) {
	if repository == nil {
		return nil, errors.New("ask service repository is required")
	}
	if pets == nil {
		return nil, errors.New("ask service pet repository is required")
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
	return &Service{repository: repository, pets: pets, rules: DeterministicRuleEngine{}, now: time.Now, versions: value}, nil
}

func (s *Service) SetCalendarRepository(repository any) {
	if value, ok := repository.(calendarRecordReader); ok {
		s.calendar = value
	}
}

// SetAgentModel 注入决策循环模型端口（v2，文档 4）。nil 表示未启用 v2 决策循环。
func (s *Service) SetAgentModel(model AgentModel) {
	s.agentModel = model
}

// SetToolCatalog 注入工具目录（v2，文档 6）。用于候选召回与工具执行。
func (s *Service) SetToolCatalog(catalog *Catalog) {
	s.catalog = catalog
}

// SetBusinessReadRepository 注入业务读取端口（v2 工具执行依赖，文档 7.2）。
func (s *Service) SetBusinessReadRepository(repository BusinessReadRepository) {
	s.businessRead = repository
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
		familyQuery := DetectFamilyQuery(input)
		if len(pets) > 0 {
			return s.createSessionWithPets(ctx, familyID, userID, input, pets[:1], createSessionOperation, idempotencyKey, hash)
		}
		if familyQuery {
			return ExecutionResult{}, resolution, appErrors.InvalidParam("当前家庭暂无宠物")
		}
		return ExecutionResult{}, resolution, appErrors.InvalidParam("请在问题中补充宠物名称")
	}
	if resolution.Status == PetResolveAmbiguous {
		return ExecutionResult{}, resolution, appErrors.Conflict("宠物名称存在歧义，请补充更多信息")
	}
	return s.createSessionWithPets(ctx, familyID, userID, input, resolution.Resolved, createSessionOperation, idempotencyKey, hash)
}

func (s *Service) ContinueSession(ctx context.Context, familyID, userID, sessionID, input, idempotencyKey string) (ExecutionResult, error) {
	familyID = strings.TrimSpace(familyID)
	userID = strings.TrimSpace(userID)
	sessionID = strings.TrimSpace(sessionID)
	input = strings.TrimSpace(input)
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if familyID == "" || userID == "" {
		return ExecutionResult{}, appErrors.Unauthorized()
	}
	if sessionID == "" {
		return ExecutionResult{}, appErrors.InvalidParam("问问会话参数无效")
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
	hash := idempotencyHash(familyID, sessionID, input)
	if result, found, err := s.replayExecution(ctx, userID, continueSessionOperation, idempotencyKey, hash); err != nil || found {
		return result, err
	}
	session, err := s.GetSession(ctx, familyID, sessionID)
	if err != nil {
		return ExecutionResult{}, err
	}
	if session.Status == SessionClosed {
		return ExecutionResult{}, appErrors.Conflict("问问会话已结束")
	}
	turns, err := s.repository.GetSnapshotTurns(ctx, session.ID)
	if err != nil {
		return ExecutionResult{}, appErrors.Internal(err)
	}
	if len(turns) == 0 {
		return ExecutionResult{}, appErrors.Conflict("问问会话暂无可继续的轮次")
	}
	current := turns[len(turns)-1]
	if current.Turn.TurnIndex != session.TurnCount-1 || current.Turn.SelectedRunID != current.Run.ID {
		return ExecutionResult{}, appErrors.Conflict("问问会话状态已改变")
	}
	if current.Run.Status == RunQueued || current.Run.Status == RunRunning || current.Run.Status == RunWaitingInput {
		return ExecutionResult{}, appErrors.Conflict("当前问问还未完成")
	}
	now := s.now().UTC()
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
	turn := Turn{ID: turnID, SessionID: session.ID, TurnIndex: session.TurnCount, Status: TurnReceived, Input: input, SelectedRunID: runID, CreatedAt: now}
	run := Run{ID: runID, SessionID: session.ID, TurnID: turnID, RunIndex: 0, RowVersion: 1, Status: RunQueued, RiskLevel: RiskUnknown, RuleVersion: session.RuleVersion, PromptVersion: session.PromptVersion, CreatedAt: now}
	event := Event{ID: eventID, SessionID: session.ID, TurnID: turnID, RunID: runID, Sequence: 1, Type: "run.queued", Data: `{}`, CreatedAt: now}
	message := Message{ID: messageID, SessionID: session.ID, TurnID: turnID, RunID: runID, Role: "user", Content: input, CreatedAt: now}
	previousSession := session
	session.Status = SessionActive
	session.RiskLevel = RiskUnknown
	session.TurnCount++
	session.UpdatedAt = now
	session.CompletedAt = nil
	result := ExecutionResult{Session: session, Run: run, Events: []Event{event}}
	responseData, err := json.Marshal(result)
	if err != nil {
		return ExecutionResult{}, appErrors.Internal(err)
	}
	idempotency := IdempotencyRecord{ID: idempotencyID, FamilyID: familyID, UserID: userID, Operation: continueSessionOperation, IdempotencyKey: idempotencyKey, RequestHash: hash, ResponseData: string(responseData), SessionID: session.ID, TurnID: turn.ID, RunID: run.ID, CreatedAt: now}
	if err := s.repository.CreateFollowUpTurnRunWithMessageIdempotent(ctx, previousSession, turn, run, event, message, idempotency); err != nil {
		if errors.Is(err, ErrSessionStateConflict) || errors.Is(err, ErrIdempotencyConflict) {
			if replay, found, replayErr := s.replayExecution(ctx, userID, continueSessionOperation, idempotencyKey, hash); replayErr != nil || found {
				return replay, replayErr
			}
			return ExecutionResult{}, appErrors.Conflict("问问会话状态已改变")
		}
		return ExecutionResult{}, appErrors.Internal(err)
	}
	return result, nil
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
	turn := Turn{ID: turnID, SessionID: sessionID, TurnIndex: 0, Status: TurnReceived, Input: input, SelectedRunID: runID, CreatedAt: now}
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
	return s.processRun(ctx, familyID, sessionID, runID, 0, 0)
}

func (s *Service) ProcessRunVersion(ctx context.Context, familyID, sessionID, runID string, expectedVersion, expectedEpoch int) (ExecutionResult, error) {
	return s.processRun(ctx, familyID, sessionID, runID, expectedVersion, expectedEpoch)
}

func (s *Service) processRun(ctx context.Context, familyID, sessionID, runID string, expectedVersion, expectedEpoch int) (ExecutionResult, error) {
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
	if expectedEpoch > 0 && run.ExecutionEpoch != expectedEpoch {
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
	messages, err := s.repository.ListSessionMessages(ctx, session.ID)
	if err != nil {
		return ExecutionResult{}, appErrors.Internal(err)
	}
	currentInput := turn.Input
	for index := len(messages) - 1; index >= 0; index-- {
		if messages[index].Role == "user" && strings.TrimSpace(messages[index].Content) != "" {
			currentInput = messages[index].Content
			break
		}
	}
	contextMessages := make([]ContextMessage, 0, len(messages))
	for _, message := range messages {
		contextMessages = append(contextMessages, ContextMessage{Role: message.Role, Content: message.Content, CreatedAt: message.CreatedAt})
	}
	currentTime := s.now()
	contextSnapshot := compactContextSnapshot(ContextSnapshot{Version: DefaultContextVersion, CapturedAt: currentTime.UTC(), Messages: contextMessages, Sources: []ContextSource{{Name: "ask_messages", Version: "ask-messages-v1", ItemCount: len(contextMessages)}}}, ContextMaxChars)
	now := currentTime.UTC()
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
	turn.Status = TurnAttached
	events = append(events, startedEvent)
	progressEvent, err := s.appendProgressEvent(ctx, session, run, "intent_routing", "正在理解你的问题")
	if err != nil {
		return ExecutionResult{}, appErrors.Internal(err)
	}
	events = append(events, progressEvent)
	riskText := turn.Input
	for _, message := range messages {
		if message.Role == "user" && message.Content != turn.Input {
			riskText += "\n" + message.Content
		}
	}
	risk := s.rules.Evaluate(RiskInput{Text: riskText})
	var decision RunDecision
	if risk.Level == RiskRed {
		progressEvent, err = s.appendProgressEvent(ctx, session, run, "intent_completed", "已理解你的问题")
		if err != nil {
			return ExecutionResult{}, appErrors.Internal(err)
		}
		events = append(events, progressEvent)
		progressEvent, err = s.appendProgressEvent(ctx, session, run, "risk_checking", "正在进行风险初筛")
		if err != nil {
			return ExecutionResult{}, appErrors.Internal(err)
		}
		events = append(events, progressEvent)
		decision = RunDecision{Status: RunCompleted, RiskLevel: RiskRed, EventType: "risk.escalated", Data: map[string]any{"trigger_code": risk.TriggerCode, "message": risk.Message, "action": risk.Action}}
	} else if s.agentModel != nil && s.catalog != nil && s.businessRead != nil {
		// v2 决策循环：组装上下文 -> 决策 -> 工具 -> 降级映射（文档 2、7、8）。
		progressEvent, err = s.appendProgressEvent(ctx, session, run, "intent_completed", "已理解你的问题")
		if err != nil {
			return ExecutionResult{}, appErrors.Internal(err)
		}
		events = append(events, progressEvent)
		progressEvent, err = s.appendProgressEvent(ctx, session, run, "context_ready", "已关联宠物资料和近期记录")
		if err != nil {
			return ExecutionResult{}, appErrors.Internal(err)
		}
		events = append(events, progressEvent)
		contextSnapshot, err = s.loadHealthContext(ctx, session, contextMessages)
		if err != nil {
			return ExecutionResult{}, err
		}
		progressEvent, err = s.appendProgressEvent(ctx, session, run, "response_generating", "正在生成答复")
		if err != nil {
			return ExecutionResult{}, appErrors.Internal(err)
		}
		events = append(events, progressEvent)
		decision, err = s.runV2DecisionLoop(ctx, session, contextMessages, currentInput, contextSnapshot, risk.Level)
		if err != nil {
			errorCode, retryable := ExecutorErrorDetails(err)
			if retryable {
				return ExecutionResult{Session: session, Run: run, Events: events}, err
			}
			decision = providerFailureDecision(errorCode)
		}
	} else {
		// v2 决策循环依赖未注入：降级为 provider_unavailable。生产环境 main.go
		// 已注入完整 v2 依赖（agentModel/catalog/businessRead），此处仅作为未配置
		// AI 能力时的兜底，保证 Run 不会永久停留在 running 状态。
		progressEvent, err = s.appendProgressEvent(ctx, session, run, "intent_completed", "已理解你的问题")
		if err != nil {
			return ExecutionResult{}, appErrors.Internal(err)
		}
		events = append(events, progressEvent)
		decision = providerFailureDecision("provider_unavailable")
	}
	if decision.Status != RunWaitingInput && decision.Status != RunCompleted && decision.Status != RunFailed {
		decision.Status = RunFailed
		decision.ErrorCode = "invalid_executor_status"
		decision.EventType = "run.failed"
		decision.Data = providerFailureDecision(decision.ErrorCode).Data
	}
	if decision.EventType == "" {
		decision.EventType = "run.completed"
	}
	if decision.RiskLevel == "" {
		decision.RiskLevel = RiskUnknown
	}
	data, marshalErr := json.Marshal(decision.Data)
	if marshalErr != nil {
		decision.Status = RunFailed
		decision.ErrorCode = "invalid_event_data"
		decision.EventType = "run.failed"
		decision.Data = providerFailureDecision(decision.ErrorCode).Data
		data, _ = json.Marshal(decision.Data)
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
	if decision.Status == RunCompleted || decision.Status == RunFailed || decision.Status == RunCanceled || decision.Status == RunInterrupted {
		run.CompletedAt = &finishedAt
	}
	session.RiskLevel = decision.RiskLevel
	session.UpdatedAt = finishedAt
	events = append(events, finishedEvent)
	return ExecutionResult{Session: session, Run: run, Events: events}, nil
}

func (s *Service) loadHealthContext(ctx context.Context, session Session, messages []ContextMessage) (ContextSnapshot, error) {
	pet, err := s.pets.Get(ctx, session.FamilyID, session.PetID)
	if err != nil {
		if errors.Is(err, petapp.ErrNotFound) {
			return ContextSnapshot{}, appErrors.NotFound("宠物不存在")
		}
		return ContextSnapshot{}, appErrors.Internal(err)
	}
	var recentTurns []ContextTurn
	if contextRepository, ok := s.repository.(contextTurnRepository); ok {
		recentTurns, err = contextRepository.ListContextTurns(ctx, session.ID, ContextTurnLimit)
		if err != nil {
			return ContextSnapshot{}, appErrors.Internal(err)
		}
	}
	value := ContextSnapshot{Version: DefaultContextVersion, CapturedAt: s.now().UTC(), Pet: PetContext{ID: pet.ID, Name: pet.Name}, RecentTurns: recentTurns, Messages: messages, Sources: []ContextSource{{Name: "pet_base", Version: "pet-base-v1", ItemCount: 1}, {Name: "ask_messages", Version: "ask-messages-v1", ItemCount: len(messages)}}}
	if profileRepository, ok := s.pets.(petProfileReader); ok {
		if profile, profileErr := profileRepository.GetProfile(ctx, session.FamilyID, session.PetID); profileErr == nil {
			value.Sources = append(value.Sources, ContextSource{Name: "pet_profile", Version: "pet-profile-v1", ItemCount: 1})
			value.Pet.Breed = profile.Breed
			value.Pet.Gender = profile.Gender
			value.Pet.Sterilized = profile.Sterilized
			if profile.Birthday != nil {
				value.Pet.Birthday = *profile.Birthday
			}
		}
	}
	if healthRepository, ok := s.pets.(petHealthReader); ok {
		if health, healthErr := healthRepository.GetHealth(ctx, session.FamilyID, session.PetID); healthErr == nil {
			value.Sources = append(value.Sources, ContextSource{Name: "pet_health", Version: "pet-health-v1", ItemCount: 1})
			value.Pet.HealthStatus = health.Status
			value.Pet.Allergies = health.Allergies
			value.Pet.LongTermMedication = health.LongTermMedication
		}
	}
	if s.calendar != nil {
		recentRecords, recordErr := s.calendar.ListRecentRecords(ctx, session.FamilyID, session.PetID, s.now().UTC().AddDate(0, 0, -90), ContextTurnLimit)
		if recordErr == nil {
			value.RecentRecords = recentRecords
			value.Sources = append(value.Sources, ContextSource{Name: "calendar_records", Version: "calendar-records-v1", ItemCount: len(recentRecords), Truncated: len(recentRecords) >= ContextTurnLimit})
		}
	}
	if _, ok := s.repository.(contextTurnRepository); ok {
		value.Sources = append(value.Sources, ContextSource{Name: "ask_turns", Version: "ask-turns-v1", ItemCount: len(recentTurns), Truncated: len(recentTurns) >= ContextTurnLimit})
	}
	value.Events = normalizeContextEvents(recentTurns, value.RecentRecords)
	return compactContextSnapshot(value, ContextMaxChars), nil
}

func providerFailureDecision(errorCode string) RunDecision {
	return RunDecision{Status: RunFailed, RiskLevel: RiskUnknown, ErrorCode: errorCode, EventType: "run.failed", Data: map[string]any{"error_code": errorCode, "message": providerFailureMessage(errorCode)}}
}

func providerFailureMessage(errorCode string) string {
	switch errorCode {
	case "provider_timeout":
		return "AI 服务响应超时，请稍后重试。"
	case "provider_canceled":
		return "AI 请求被中断，请重新发送问题。"
	case "provider_unavailable":
		return "AI 服务暂时不可用，请稍后重试。"
	case "provider_rate_limited":
		return "AI 服务请求过于频繁，请稍后重试。"
	case "provider_quota_exhausted":
		return "AI 服务额度暂时不可用，请检查额度后重试。"
	case "provider_auth_failed":
		return "AI 服务拒绝访问，请检查 API Key、模型 ID 和模型权限。"
	case "provider_request_invalid":
		return "AI 服务请求配置不兼容，请检查模型与接口配置。"
	case "provider_output_invalid":
		return "AI 返回内容格式异常，未能解析健康建议。"
	case "invalid_analysis_output":
		return "AI 返回的健康建议未通过完整性或安全检查。"
	case "invalid_executor_status":
		return "AI 返回了不支持的执行状态。"
	case "invalid_event_data":
		return "AI 返回内容无法保存，请稍后重试。"
	case "provider_failed":
		return "AI 服务返回了无法识别的错误，请检查服务端日志。"
	case "executor_failed":
		return "问问执行器运行失败，请检查服务端日志。"
	case "worker_attempts_exhausted":
		return "多次调用 AI 服务仍未成功，请稍后重试。"
	default:
		return "问问执行失败，错误代码：" + errorCode
	}
}

func (s *Service) appendProgressEvent(ctx context.Context, session Session, run Run, stage, message string) (Event, error) {
	id, err := newID()
	if err != nil {
		return Event{}, err
	}
	data, err := json.Marshal(map[string]string{"stage": stage, "message": message})
	if err != nil {
		return Event{}, err
	}
	return s.repository.AppendRunEvent(ctx, Event{ID: id, SessionID: session.ID, TurnID: run.TurnID, RunID: run.ID, Type: "run.progress", Data: string(data), CreatedAt: s.now().UTC()})
}

func (s *Service) appendAssistantDelta(ctx context.Context, session Session, run Run, delta string) (Event, error) {
	id, err := newID()
	if err != nil {
		return Event{}, err
	}
	data, err := json.Marshal(map[string]string{"delta": delta})
	if err != nil {
		return Event{}, err
	}
	return s.repository.AppendRunEvent(ctx, Event{ID: id, SessionID: session.ID, TurnID: run.TurnID, RunID: run.ID, Type: "assistant.delta", Data: string(data), CreatedAt: s.now().UTC()})
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
