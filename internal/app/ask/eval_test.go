package ask

import "testing"

func TestOfflineAgentQualityEvaluation(t *testing.T) {
	engine := DeterministicRuleEngine{}
	validCompleted := RunDecision{Status: RunCompleted, RiskLevel: RiskYellow, Data: map[string]any{
		"current_assessment":    "目前需要观察状态变化",
		"observations":          []string{"今天食欲下降"},
		"possible_causes":       []string{"饮食变化"},
		"home_actions":          []string{"记录饮水和进食"},
		"escalation_conditions": []string{"持续恶化时联系宠物医院"},
	}}
	validQuestion := RunDecision{Status: RunWaitingInput, RiskLevel: RiskUnknown, Data: map[string]any{"question": "症状从什么时候开始？"}}
	cases := []struct {
		name  string
		pass  bool
		check func() bool
	}{
		{name: "red rule takes priority", pass: true, check: func() bool { return engine.Evaluate(RiskInput{Text: "现在呼吸困难"}).Level == RiskRed }},
		{name: "negated red rule is ignored", pass: true, check: func() bool { return engine.Evaluate(RiskInput{Text: "没有呼吸困难"}).Level != RiskRed }},
		{name: "clarification output is valid", pass: true, check: func() bool { return ValidateAnalysisOutput(validQuestion) == nil }},
		{name: "completed output is valid", pass: true, check: func() bool { return ValidateAnalysisOutput(validCompleted) == nil }},
		{name: "unsafe diagnosis is rejected", pass: true, check: func() bool {
			unsafe := validCompleted
			unsafe.Data = map[string]any{"current_assessment": "已确诊感染", "observations": []string{"观察到异常"}, "possible_causes": []string{"感染"}, "home_actions": []string{"观察"}, "escalation_conditions": []string{"恶化就医"}}
			return ValidateAnalysisOutput(unsafe) != nil
		}},
	}
	passed := 0
	for _, test := range cases {
		if test.check() == test.pass {
			passed++
			continue
		}
		t.Errorf("evaluation case %q failed", test.name)
	}
	if passed != len(cases) {
		t.Fatalf("offline evaluation score = %d/%d", passed, len(cases))
	}
}
