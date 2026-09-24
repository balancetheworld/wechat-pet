package ask

import "testing"

func TestCanTransitionRun(t *testing.T) {
	tests := []struct {
		from RunStatus
		to   RunStatus
		want bool
	}{
		{RunQueued, RunRunning, true},
		{RunQueued, RunFailed, true},
		{RunQueued, RunCanceled, true},
		{RunQueued, RunWaitingInput, false},
		{RunQueued, RunCompleted, false},
		{RunRunning, RunWaitingInput, true},
		{RunRunning, RunCompleted, true},
		{RunRunning, RunCanceling, true},
		{RunRunning, RunFailed, true},
		{RunRunning, RunCanceled, true},
		{RunRunning, RunInterrupted, true},
		{RunRunning, RunQueued, true},
		{RunWaitingInput, RunQueued, true},
		{RunWaitingInput, RunCanceled, true},
		{RunWaitingInput, RunCompleted, false},
		{RunCanceling, RunCanceled, true},
		{RunCanceling, RunCompleted, false},
		{RunCompleted, RunRunning, false},
		{RunCompleted, RunQueued, false},
		{RunFailed, RunRunning, false},
		{RunCanceled, RunRunning, false},
		{RunInterrupted, RunRunning, false},
	}

	for _, test := range tests {
		if got := CanTransitionRun(test.from, test.to); got != test.want {
			t.Fatalf("CanTransitionRun(%q, %q) = %v, want %v", test.from, test.to, got, test.want)
		}
	}
}

func TestTransitionRun(t *testing.T) {
	run := &Run{Status: RunQueued}
	if err := TransitionRun(run, RunRunning); err != nil {
		t.Fatal(err)
	}
	if run.Status != RunRunning {
		t.Fatalf("status = %q, want %q", run.Status, RunRunning)
	}
	if err := TransitionRun(run, RunCompleted); err != nil {
		t.Fatal(err)
	}
	if err := TransitionRun(run, RunRunning); err != ErrInvalidRunTransition {
		t.Fatalf("error = %v, want %v", err, ErrInvalidRunTransition)
	}
}

func TestTransitionRunCanceling(t *testing.T) {
	run := &Run{Status: RunRunning}
	if err := TransitionRun(run, RunCanceling); err != nil {
		t.Fatal(err)
	}
	if err := TransitionRun(run, RunCanceled); err != nil {
		t.Fatal(err)
	}
	if run.Status != RunCanceled {
		t.Fatalf("status = %q, want %q", run.Status, RunCanceled)
	}
	// canceled 是终态，不允许重新运行
	if err := TransitionRun(run, RunQueued); err != ErrInvalidRunTransition {
		t.Fatalf("error = %v, want %v", err, ErrInvalidRunTransition)
	}
}

func TestTransitionSession(t *testing.T) {
	session := &Session{Status: SessionActive}
	if err := TransitionSession(session, SessionClosed); err != nil {
		t.Fatal(err)
	}
	if session.Status != SessionClosed {
		t.Fatalf("status = %q, want %q", session.Status, SessionClosed)
	}
	// closed 是终态，不允许重新激活
	if err := TransitionSession(session, SessionActive); err != ErrInvalidSessionTransition {
		t.Fatalf("error = %v, want %v", err, ErrInvalidSessionTransition)
	}
}

func TestTransitionTurn(t *testing.T) {
	turn := &Turn{Status: TurnReceived}
	if err := TransitionTurn(turn, TurnAttached); err != nil {
		t.Fatal(err)
	}
	// attached 是终态
	if err := TransitionTurn(turn, TurnSuperseded); err != ErrInvalidTurnTransition {
		t.Fatalf("error = %v, want %v", err, ErrInvalidTurnTransition)
	}
	other := &Turn{Status: TurnReceived}
	if err := TransitionTurn(other, TurnSuperseded); err != nil {
		t.Fatal(err)
	}
	if other.Status != TurnSuperseded {
		t.Fatalf("status = %q, want %q", other.Status, TurnSuperseded)
	}
}

func TestTransitionAttempt(t *testing.T) {
	attempt := &Attempt{Status: AttemptQueued}
	if err := TransitionAttempt(attempt, AttemptRunning); err != nil {
		t.Fatal(err)
	}
	if err := TransitionAttempt(attempt, AttemptSucceeded); err != nil {
		t.Fatal(err)
	}
	if err := TransitionAttempt(attempt, AttemptRunning); err != ErrInvalidAttemptTransition {
		t.Fatalf("error = %v, want %v", err, ErrInvalidAttemptTransition)
	}
	unknown := &Attempt{Status: AttemptRunning}
	if err := TransitionAttempt(unknown, AttemptUnknown); err != nil {
		t.Fatal(err)
	}
	if unknown.Status != AttemptUnknown {
		t.Fatalf("status = %q, want %q", unknown.Status, AttemptUnknown)
	}
}

func TestTransitionRejectsNil(t *testing.T) {
	if err := TransitionRun(nil, RunRunning); err != ErrInvalidRunTransition {
		t.Fatalf("run error = %v, want %v", err, ErrInvalidRunTransition)
	}
	if err := TransitionSession(nil, SessionClosed); err != ErrInvalidSessionTransition {
		t.Fatalf("session error = %v, want %v", err, ErrInvalidSessionTransition)
	}
	if err := TransitionTurn(nil, TurnAttached); err != ErrInvalidTurnTransition {
		t.Fatalf("turn error = %v, want %v", err, ErrInvalidTurnTransition)
	}
	if err := TransitionAttempt(nil, AttemptRunning); err != ErrInvalidAttemptTransition {
		t.Fatalf("attempt error = %v, want %v", err, ErrInvalidAttemptTransition)
	}
}
