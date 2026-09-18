package ask

import "errors"

var (
	ErrInvalidRunTransition     = errors.New("invalid ask run status transition")
	ErrInvalidSessionTransition = errors.New("invalid ask session status transition")
	ErrInvalidTurnTransition    = errors.New("invalid ask turn status transition")
	ErrInvalidAttemptTransition = errors.New("invalid ask attempt status transition")
)

// Session：active -> closed，closed 为终态。
func CanTransitionSession(from, to SessionStatus) bool {
	return from == SessionActive && to == SessionClosed
}

func TransitionSession(session *Session, to SessionStatus) error {
	if session == nil || !CanTransitionSession(session.Status, to) {
		return ErrInvalidSessionTransition
	}
	session.Status = to
	return nil
}

// Turn：received -> attached | superseded。
func CanTransitionTurn(from, to TurnStatus) bool {
	switch from {
	case TurnReceived:
		return to == TurnAttached || to == TurnSuperseded
	default:
		return false
	}
}

func TransitionTurn(turn *Turn, to TurnStatus) error {
	if turn == nil || !CanTransitionTurn(turn.Status, to) {
		return ErrInvalidTurnTransition
	}
	turn.Status = to
	return nil
}

// Run：queued -> running/failed/canceled；
// running -> queued/waiting_input/canceling/completed/failed/interrupted；
// waiting_input -> queued/canceled；canceling -> canceled。
func CanTransitionRun(from, to RunStatus) bool {
	switch from {
	case RunQueued:
		return to == RunRunning || to == RunFailed || to == RunCanceled
	case RunRunning:
		return to == RunQueued || to == RunWaitingInput || to == RunCanceling || to == RunCompleted || to == RunFailed || to == RunInterrupted || to == RunCanceled
	case RunWaitingInput:
		return to == RunQueued || to == RunCanceled
	case RunCanceling:
		return to == RunCanceled
	default:
		return false
	}
}

func TransitionRun(run *Run, to RunStatus) error {
	if run == nil || !CanTransitionRun(run.Status, to) {
		return ErrInvalidRunTransition
	}
	run.Status = to
	return nil
}

// Attempt：queued -> running/failed/canceled；
// running -> succeeded/failed/canceled/unknown。
func CanTransitionAttempt(from, to AttemptStatus) bool {
	switch from {
	case AttemptQueued:
		return to == AttemptRunning || to == AttemptFailed || to == AttemptCanceled
	case AttemptRunning:
		return to == AttemptSucceeded || to == AttemptFailed || to == AttemptCanceled || to == AttemptUnknown
	default:
		return false
	}
}

func TransitionAttempt(attempt *Attempt, to AttemptStatus) error {
	if attempt == nil || !CanTransitionAttempt(attempt.Status, to) {
		return ErrInvalidAttemptTransition
	}
	attempt.Status = to
	return nil
}
