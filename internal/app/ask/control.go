package ask

import (
	"context"
	"errors"

	appErrors "github.com/balancetheworld/wechat-pet/internal/pkg/errors"
)

func (s *Service) StopRun(ctx context.Context, familyID, userID, sessionID, runID string, expectedVersion int) (ExecutionResult, error) {
	result, err := s.GetExecution(ctx, familyID, sessionID, runID, userID)
	if err != nil {
		return ExecutionResult{}, err
	}
	if result.Run.RowVersion != expectedVersion || (result.Run.Status != RunQueued && result.Run.Status != RunRunning && result.Run.Status != RunWaitingInput) {
		return ExecutionResult{}, appErrors.Conflict("问问执行状态已改变")
	}
	now := s.now().UTC()
	eventID, err := newID()
	if err != nil {
		return ExecutionResult{}, appErrors.Internal(err)
	}
	event := Event{ID: eventID, SessionID: sessionID, TurnID: result.Run.TurnID, RunID: runID, Sequence: nextSequence(result.Events), Type: "run.canceled", Data: `{}`, CreatedAt: now}
	if err := s.repository.TransitionRun(ctx, runID, expectedVersion, result.Run.Status, RunCanceled, result.Run.RiskLevel, "user_canceled", now, event, Message{}); err != nil {
		if errors.Is(err, ErrRunStateConflict) {
			return ExecutionResult{}, appErrors.Conflict("问问执行状态已改变")
		}
		return ExecutionResult{}, appErrors.Internal(err)
	}
	result.Run.Status = RunCanceled
	result.Run.RowVersion++
	result.Run.ErrorCode = "user_canceled"
	result.Run.CompletedAt = &now
	result.Events = append(result.Events, event)
	return result, nil
}

func (s *Service) RetryRun(ctx context.Context, familyID, userID, sessionID, runID string, expectedVersion int) (ExecutionResult, error) {
	result, err := s.GetExecution(ctx, familyID, sessionID, runID, userID)
	if err != nil {
		return ExecutionResult{}, err
	}
	if result.Run.RowVersion != expectedVersion || (result.Run.Status != RunFailed && result.Run.Status != RunCanceled && result.Run.Status != RunInterrupted) {
		return ExecutionResult{}, appErrors.Conflict("仅失败、取消或中断的执行可以重试")
	}
	repository, ok := s.repository.(interface {
		CreateRetryRun(context.Context, Run, Run, Event) error
	})
	if !ok {
		return ExecutionResult{}, appErrors.Internal(errors.New("ask retry repository unavailable"))
	}
	now := s.now().UTC()
	retryID, err := newID()
	if err != nil {
		return ExecutionResult{}, appErrors.Internal(err)
	}
	eventID, err := newID()
	if err != nil {
		return ExecutionResult{}, appErrors.Internal(err)
	}
	originTurnID := result.Run.OriginTurnID
	if originTurnID == "" {
		originTurnID = result.Run.TurnID
	}
	retry := Run{ID: retryID, SessionID: sessionID, TurnID: result.Run.TurnID, OriginTurnID: originTurnID, RunIndex: result.Run.RunIndex + 1, RowVersion: 1, Status: RunQueued, RiskLevel: RiskUnknown, RuleVersion: result.Run.RuleVersion, PromptVersion: result.Run.PromptVersion, CreatedAt: now}
	event := Event{ID: eventID, SessionID: sessionID, TurnID: retry.TurnID, RunID: retry.ID, Sequence: 1, Type: "run.queued", Data: `{}`, CreatedAt: now}
	if err := repository.CreateRetryRun(ctx, result.Run, retry, event); err != nil {
		if errors.Is(err, ErrRunStateConflict) {
			return ExecutionResult{}, appErrors.Conflict("问问执行状态已改变")
		}
		return ExecutionResult{}, appErrors.Internal(err)
	}
	return ExecutionResult{Session: result.Session, Run: retry, Events: []Event{event}}, nil
}

func (s *Service) ListSessionHistory(ctx context.Context, familyID, userID string, limit int) ([]Session, error) {
	if limit < 1 || limit > 100 {
		limit = 20
	}
	repository, ok := s.repository.(interface {
		ListRecentSessions(context.Context, string, string, int) ([]Session, error)
	})
	if !ok {
		return nil, appErrors.Internal(errors.New("ask history repository unavailable"))
	}
	values, err := repository.ListRecentSessions(ctx, familyID, userID, limit)
	if err != nil {
		return nil, appErrors.Internal(err)
	}
	return values, nil
}
