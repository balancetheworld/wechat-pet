package ask

import "errors"

var (
	ErrInvalidRunTransition     = errors.New("invalid ask run status transition")
	ErrInvalidSessionTransition = errors.New("invalid ask session status transition")
)

func CanTransitionRun(from, to RunStatus) bool {
	switch from {
	case RunQueued:
		return to == RunRunning || to == RunCanceled
	case RunRunning:
		return to == RunWaitingInput || to == RunCompleted || to == RunEscalated || to == RunFailed || to == RunCanceled || to == RunInterrupted
	case RunWaitingInput:
		return to == RunQueued || to == RunCanceled
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

func CanTransitionSession(from, to SessionStatus) bool {
	switch from {
	case SessionActive:
		return to == SessionCompleted || to == SessionEscalated || to == SessionCanceled
	default:
		return false
	}
}

func TransitionSession(session *Session, to SessionStatus) error {
	if session == nil || !CanTransitionSession(session.Status, to) {
		return ErrInvalidSessionTransition
	}
	session.Status = to
	return nil
}
