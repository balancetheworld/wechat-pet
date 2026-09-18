package ask

import "testing"

// TestOfflineAgentQualityEvaluation 是规则引擎的离线质量评估：急症信号识别优先，
// 否定式描述不误报。v2 输出校验（ValidateFinalAnswer/任务项覆盖）由 decision_loop_test
// 与 agent_loop_test 独立覆盖。
func TestOfflineAgentQualityEvaluation(t *testing.T) {
	engine := DeterministicRuleEngine{}
	cases := []struct {
		name  string
		pass  bool
		check func() bool
	}{
		{name: "red rule takes priority", pass: true, check: func() bool { return engine.Evaluate(RiskInput{Text: "现在呼吸困难"}).Level == RiskRed }},
		{name: "negated red rule is ignored", pass: true, check: func() bool { return engine.Evaluate(RiskInput{Text: "没有呼吸困难"}).Level != RiskRed }},
		{name: "green is not a clean bill of health", pass: true, check: func() bool { return engine.Evaluate(RiskInput{Text: "今天精神不错"}).Level != RiskRed }},
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
