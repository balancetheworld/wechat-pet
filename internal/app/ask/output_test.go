package ask

import (
	"errors"
	"testing"
)

func TestValidateAnalysisOutput(t *testing.T) {
	valid := RunDecision{
		Status:    RunCompleted,
		RiskLevel: RiskGreen,
		Data: map[string]any{
			"current_assessment":    "目前可以先观察宠物状态",
			"observations":          []string{"今天出现一次呕吐"},
			"possible_causes":       []string{"饮食变化"},
			"home_actions":          []string{"提供清水并观察精神状态"},
			"escalation_conditions": []string{"持续呕吐或精神明显变差时就医"},
		},
	}
	if err := ValidateAnalysisOutput(valid); err != nil {
		t.Fatalf("valid output error = %v", err)
	}
}

func TestValidateAnalysisOutputRejectsUnsafeOrIncompleteData(t *testing.T) {
	tests := []RunDecision{
		{Status: RunCompleted, RiskLevel: RiskGreen, Data: map[string]any{}},
		{Status: RunCompleted, RiskLevel: RiskUnknown, Data: map[string]any{"current_assessment": "观察", "observations": []string{"情况"}, "possible_causes": []string{"方向"}, "home_actions": []string{"建议"}, "escalation_conditions": []string{"升级条件"}}},
		{Status: RunCompleted, RiskLevel: RiskYellow, Data: map[string]any{"current_assessment": "已经确诊", "observations": []string{"情况"}, "possible_causes": []string{"方向"}, "home_actions": []string{"建议"}, "escalation_conditions": []string{"升级条件"}}},
		{Status: RunCompleted, RiskLevel: RiskGreen, Data: map[string]any{"current_assessment": "观察", "observations": []string{"情况"}, "possible_causes": []string{"一", "二", "三", "四"}, "home_actions": []string{"建议"}, "escalation_conditions": []string{"升级条件"}}},
	}
	for index, decision := range tests {
		if err := ValidateAnalysisOutput(decision); !errors.Is(err, ErrInvalidAnalysisOutput) {
			t.Fatalf("case %d error = %v, want invalid output", index, err)
		}
	}
}

func TestValidateAnalysisOutputIgnoresNonCompletedDecision(t *testing.T) {
	if err := ValidateAnalysisOutput(RunDecision{Status: RunWaitingInput, Data: map[string]any{"question": "什么时候开始？"}}); err != nil {
		t.Fatalf("waiting input error = %v", err)
	}
	if err := ValidateAnalysisOutput(RunDecision{Status: RunWaitingInput, Data: map[string]any{}}); !errors.Is(err, ErrInvalidAnalysisOutput) {
		t.Fatalf("invalid question error = %v", err)
	}
}
