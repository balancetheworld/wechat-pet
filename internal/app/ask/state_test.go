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
		{RunRunning, RunWaitingInput, true},
		{RunRunning, RunCompleted, true},
		{RunRunning, RunEscalated, true},
		{RunRunning, RunFailed, true},
		{RunRunning, RunCanceled, true},
		{RunRunning, RunInterrupted, true},
		{RunWaitingInput, RunQueued, true},
		{RunWaitingInput, RunCanceled, true},
		{RunCompleted, RunRunning, false},
		{RunWaitingInput, RunCompleted, false},
		{RunFailed, RunRunning, false},
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

func TestTransitionSession(t *testing.T) {
	session := &Session{Status: SessionActive}
	if err := TransitionSession(session, SessionEscalated); err != nil {
		t.Fatal(err)
	}
	if session.Status != SessionEscalated {
		t.Fatalf("status = %q, want %q", session.Status, SessionEscalated)
	}
	if err := TransitionSession(session, SessionCompleted); err != ErrInvalidSessionTransition {
		t.Fatalf("error = %v, want %v", err, ErrInvalidSessionTransition)
	}
}

func TestTransitionRejectsNil(t *testing.T) {
	if err := TransitionRun(nil, RunRunning); err != ErrInvalidRunTransition {
		t.Fatalf("run error = %v, want %v", err, ErrInvalidRunTransition)
	}
	if err := TransitionSession(nil, SessionCompleted); err != ErrInvalidSessionTransition {
		t.Fatalf("session error = %v, want %v", err, ErrInvalidSessionTransition)
	}
}
