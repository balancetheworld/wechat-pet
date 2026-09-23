package ask

import "testing"

func TestIsTerminalAskEvent(t *testing.T) {
	terminal := []string{"assistant.completed", "assistant.question", "fact.completed", "family.pets.completed", "run.completed", "risk.escalated", "run.failed"}
	for _, eventType := range terminal {
		if !isTerminalAskEvent(eventType) {
			t.Fatalf("event %s should be terminal", eventType)
		}
	}
	ongoing := []string{"run.queued", "run.started", "run.progress", "assistant.delta", "security.injection_detected"}
	for _, eventType := range ongoing {
		if isTerminalAskEvent(eventType) {
			t.Fatalf("event %s should not be terminal", eventType)
		}
	}
}
