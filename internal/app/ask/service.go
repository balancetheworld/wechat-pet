package ask

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	calendarapp "github.com/balancetheworld/wechat-pet/internal/app/calendar"
	petapp "github.com/balancetheworld/wechat-pet/internal/app/pet"
	appErrors "github.com/balancetheworld/wechat-pet/internal/pkg/errors"
)

const (
	DefaultPromptVersion    = "ask-prompt-v1"
	DefaultRuleVersion      = "ask-rules-v1"
	DefaultKnowledgeVersion = "ask-knowledge-v1"
	MaxInputLength          = 4000
	MaxTurns                = 3
	ContextTurnLimit        = 5
	ContextMaxChars         = 6000
	DefaultContextVersion   = "ask-context-v4"
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

func (s *Service) CreateSession(ctx context.Context, familyID, userID, petID, input string) (ExecutionResult, error) {
	familyID = strings.TrimSpace(familyID)
	userID = strings.TrimSpace(userID)
	petID = strings.TrimSpace(petID)
	input = strings.TrimSpace(input)
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
	if _, err := s.pets.Get(ctx, familyID, petID); err != nil {
		if errors.Is(err, petapp.ErrNotFound) {
			return ExecutionResult{}, appErrors.NotFound("宠物不存在")
		}
		return ExecutionResult{}, appErrors.Internal(err)
	}
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
	session := Session{ID: sessionID, FamilyID: familyID, CreatedBy: userID, PetID: petID, Status: SessionActive, RiskLevel: RiskUnknown, TurnCount: 1, PromptVersion: s.versions.PromptVersion, RuleVersion: s.versions.RuleVersion, KnowledgeVersion: s.versions.KnowledgeVersion, CreatedAt: now, UpdatedAt: now}
	turn := Turn{ID: turnID, SessionID: sessionID, TurnIndex: 0, Status: RunQueued, Input: input, SelectedRunID: runID, CreatedAt: now}
	run := Run{ID: runID, SessionID: sessionID, TurnID: turnID, RunIndex: 0, Status: RunQueued, RiskLevel: RiskUnknown, RuleVersion: s.versions.RuleVersion, PromptVersion: s.versions.PromptVersion, CreatedAt: now}
	event := Event{ID: eventID, SessionID: sessionID, TurnID: turnID, RunID: runID, Sequence: 1, Type: "run.queued", Data: `{}`, CreatedAt: now}
	if err := s.repository.CreateSessionRun(ctx, session, turn, run, event); err != nil {
		return ExecutionResult{}, appErrors.Internal(err)
	}
	return ExecutionResult{Session: session, Run: run, Events: []Event{event}}, nil
}

func (s *Service) ProcessRun(ctx context.Context, familyID, sessionID, runID string) (ExecutionResult, error) {
	session, err := s.repository.GetSession(ctx, strings.TrimSpace(familyID), strings.TrimSpace(sessionID))
	if err != nil {
		if errors.Is(err, ErrSessionNotFound) {
			return ExecutionResult{}, appErrors.NotFound("问问会话不存在")
		}
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
	contextSnapshot := ContextSnapshot{Version: DefaultContextVersion, CapturedAt: s.now().UTC(), Pet: PetContext{ID: pet.ID, Name: pet.Name}, RecentTurns: recentTurns, Sources: []ContextSource{{Name: "pet_base", Version: "pet-base-v1", ItemCount: 1}}}
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
	if err := s.repository.TransitionRun(ctx, run.ID, RunQueued, RunRunning, RiskUnknown, "", now, startedEvent); err != nil {
		if errors.Is(err, ErrRunStateConflict) {
			return ExecutionResult{}, appErrors.Conflict("问问执行状态已改变")
		}
		return ExecutionResult{}, appErrors.Internal(err)
	}
	run.Status = RunRunning
	run.StartedAt = &now
	turn.Status = RunRunning
	events = append(events, startedEvent)
	risk := s.rules.Evaluate(RiskInput{Text: turn.Input})
	var decision RunDecision
	if risk.Level == RiskRed {
		decision = RunDecision{Status: RunEscalated, RiskLevel: RiskRed, EventType: "risk.escalated", Data: map[string]any{"trigger_code": risk.TriggerCode, "message": risk.Message, "action": risk.Action}}
	} else {
		decision, err = s.executor.Execute(ctx, RunInput{Session: session, Turn: turn, Run: run, Context: contextSnapshot})
		if err != nil {
			decision = RunDecision{Status: RunFailed, ErrorCode: "executor_failed", EventType: "run.failed", Data: map[string]any{"message": "当前暂时无法完成分析"}}
		} else if err := ValidateAnalysisOutput(decision); err != nil {
			decision = RunDecision{Status: RunFailed, ErrorCode: "invalid_analysis_output", EventType: "run.failed", Data: map[string]any{"message": "当前暂时无法完成分析"}}
		}
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
	if err := s.repository.TransitionRun(ctx, run.ID, RunRunning, decision.Status, decision.RiskLevel, decision.ErrorCode, finishedAt, finishedEvent); err != nil {
		if errors.Is(err, ErrRunStateConflict) {
			return ExecutionResult{}, appErrors.Conflict("问问执行状态已改变")
		}
		return ExecutionResult{}, appErrors.Internal(err)
	}
	run.Status = decision.Status
	run.RiskLevel = decision.RiskLevel
	run.ErrorCode = decision.ErrorCode
	run.CompletedAt = &finishedAt
	session.Status = sessionStatusForRun(decision.Status)
	session.RiskLevel = decision.RiskLevel
	session.UpdatedAt = finishedAt
	if decision.Status == RunCompleted || decision.Status == RunEscalated {
		session.CompletedAt = &finishedAt
	}
	events = append(events, finishedEvent)
	return ExecutionResult{Session: session, Run: run, Events: events}, nil
}

func (s *Service) Reply(ctx context.Context, familyID, sessionID, runID, input string) (ExecutionResult, error) {
	familyID = strings.TrimSpace(familyID)
	sessionID = strings.TrimSpace(sessionID)
	runID = strings.TrimSpace(runID)
	input = strings.TrimSpace(input)
	if familyID == "" || sessionID == "" || runID == "" {
		return ExecutionResult{}, appErrors.InvalidParam("问问参数无效")
	}
	if input == "" {
		return ExecutionResult{}, appErrors.InvalidParam("回答内容不能为空")
	}
	if len([]rune(input)) > MaxInputLength {
		return ExecutionResult{}, appErrors.InvalidParam("回答内容不能超过 4000 个字符")
	}
	session, err := s.repository.GetSession(ctx, familyID, sessionID)
	if err != nil {
		if errors.Is(err, ErrSessionNotFound) {
			return ExecutionResult{}, appErrors.NotFound("问问会话不存在")
		}
		return ExecutionResult{}, appErrors.Internal(err)
	}
	if session.Status != SessionActive {
		return ExecutionResult{}, appErrors.Conflict("问问会话已结束")
	}
	if session.TurnCount >= MaxTurns {
		return ExecutionResult{}, appErrors.Conflict("问问追问轮次已达上限")
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
	now := s.now().UTC()
	turnID, err := newID()
	if err != nil {
		return ExecutionResult{}, appErrors.Internal(err)
	}
	newRunID, err := newID()
	if err != nil {
		return ExecutionResult{}, appErrors.Internal(err)
	}
	eventID, err := newID()
	if err != nil {
		return ExecutionResult{}, appErrors.Internal(err)
	}
	turn := Turn{ID: turnID, SessionID: session.ID, TurnIndex: session.TurnCount, Status: RunQueued, Input: input, SelectedRunID: newRunID, CreatedAt: now}
	newRun := Run{ID: newRunID, SessionID: session.ID, TurnID: turnID, RunIndex: 0, Status: RunQueued, RiskLevel: RiskUnknown, RuleVersion: s.versions.RuleVersion, PromptVersion: s.versions.PromptVersion, CreatedAt: now}
	event := Event{ID: eventID, SessionID: session.ID, TurnID: turnID, RunID: newRunID, Sequence: 1, Type: "run.queued", Data: `{}`, CreatedAt: now}
	if err := s.repository.CreateFollowUpTurnRun(ctx, session, turn, newRun, event); err != nil {
		if errors.Is(err, ErrSessionStateConflict) {
			return ExecutionResult{}, appErrors.Conflict("问问会话状态已改变")
		}
		return ExecutionResult{}, appErrors.Internal(err)
	}
	session.TurnCount++
	session.UpdatedAt = now
	return ExecutionResult{Session: session, Run: newRun, Events: []Event{event}}, nil
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

func newID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}
