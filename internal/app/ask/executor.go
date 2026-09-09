package ask

import "context"

type DeterministicExecutor struct{}

func (DeterministicExecutor) Execute(context.Context, RunInput) (RunDecision, error) {
	return RunDecision{
		Status:    RunWaitingInput,
		RiskLevel: RiskUnknown,
		EventType: "assistant.question",
		Data: map[string]any{
			"question": "请补充宠物目前最明显的一个症状，以及症状从什么时候开始。",
		},
	}, nil
}
