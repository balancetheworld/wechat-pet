package ask

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestAttemptLifecycle(t *testing.T) {
	repository := newBudgetTestRepository(t)
	ctx := context.Background()
	now := time.Unix(100, 0).UTC()

	attempt := Attempt{ID: "attempt-1", SessionID: "session-1", RunID: "run-1", Purpose: "agent_step", InputSnapshot: `{}`, StartedAt: now}
	if err := repository.CreateAttempt(ctx, attempt); err != nil {
		t.Fatal(err)
	}
	if err := repository.TransitionAttempt(ctx, attempt.ID, AttemptQueued, AttemptRunning, "", now); err != nil {
		t.Fatal(err)
	}
	if err := repository.TransitionAttempt(ctx, attempt.ID, AttemptRunning, AttemptSucceeded, "", now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	loaded, err := repository.GetAttempt(ctx, attempt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Status != AttemptSucceeded || loaded.Sequence != 1 {
		t.Fatalf("attempt = %+v, want succeeded sequence=1", loaded)
	}
	if loaded.CompletedAt == nil {
		t.Fatal("completed_at should be set on terminal state")
	}
}

func TestAttemptInvalidTransitionRejected(t *testing.T) {
	repository := newBudgetTestRepository(t)
	ctx := context.Background()
	now := time.Unix(100, 0).UTC()

	attempt := Attempt{ID: "attempt-1", SessionID: "session-1", RunID: "run-1", StartedAt: now}
	if err := repository.CreateAttempt(ctx, attempt); err != nil {
		t.Fatal(err)
	}
	// queued -> succeeded 非法（必须先 running）
	if err := repository.TransitionAttempt(ctx, attempt.ID, AttemptQueued, AttemptSucceeded, "", now); !errors.Is(err, ErrInvalidAttemptTransition) {
		t.Fatalf("error = %v, want %v", err, ErrInvalidAttemptTransition)
	}
}

func TestAttemptTerminalImmutable(t *testing.T) {
	repository := newBudgetTestRepository(t)
	ctx := context.Background()
	now := time.Unix(100, 0).UTC()

	attempt := Attempt{ID: "attempt-1", SessionID: "session-1", RunID: "run-1", StartedAt: now}
	if err := repository.CreateAttempt(ctx, attempt); err != nil {
		t.Fatal(err)
	}
	if err := repository.TransitionAttempt(ctx, attempt.ID, AttemptQueued, AttemptRunning, "", now); err != nil {
		t.Fatal(err)
	}
	if err := repository.TransitionAttempt(ctx, attempt.ID, AttemptRunning, AttemptSucceeded, "", now); err != nil {
		t.Fatal(err)
	}
	// 终态不可回开
	if err := repository.TransitionAttempt(ctx, attempt.ID, AttemptSucceeded, AttemptRunning, "", now); !errors.Is(err, ErrInvalidAttemptTransition) {
		t.Fatalf("error = %v, want %v", err, ErrInvalidAttemptTransition)
	}
}

func TestAttemptSequenceAutoIncrement(t *testing.T) {
	repository := newBudgetTestRepository(t)
	ctx := context.Background()
	now := time.Unix(100, 0).UTC()

	for i := 1; i <= 3; i++ {
		attempt := Attempt{ID: "attempt-" + string(rune('0'+i)), SessionID: "session-1", RunID: "run-1", StartedAt: now}
		if err := repository.CreateAttempt(ctx, attempt); err != nil {
			t.Fatal(err)
		}
	}
	list, err := repository.ListAttempts(ctx, "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 {
		t.Fatalf("attempts = %d, want 3", len(list))
	}
	for i, value := range list {
		if value.Sequence != i+1 {
			t.Fatalf("attempt[%d].sequence = %d, want %d", i, value.Sequence, i+1)
		}
	}
}

func TestAttemptNotFound(t *testing.T) {
	repository := newBudgetTestRepository(t)
	ctx := context.Background()

	if _, err := repository.GetAttempt(ctx, "missing"); !errors.Is(err, ErrAttemptNotFound) {
		t.Fatalf("error = %v, want %v", err, ErrAttemptNotFound)
	}
}
