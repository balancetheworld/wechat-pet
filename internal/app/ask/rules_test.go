package ask

import "testing"

func TestDeterministicRuleEngineMatchesRedRisk(t *testing.T) {
	engine := DeterministicRuleEngine{}
	tests := []struct {
		name  string
		input string
		code  string
	}{
		{name: "breathing", input: "刚才突然呼吸困难", code: "breathing_distress"},
		{name: "seizure", input: "已经持续抽搐五分钟", code: "persistent_seizure"},
		{name: "bleeding", input: "伤口止不住血", code: "severe_bleeding"},
		{name: "poisoning", input: "可能误食了杀虫剂", code: "suspected_poisoning"},
		{name: "consciousness", input: "怎么叫都叫不醒", code: "altered_consciousness"},
		{name: "standing", input: "突然站不起来", code: "unable_to_stand"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := engine.Evaluate(RiskInput{Text: test.input})
			if result.Level != RiskRed || result.TriggerCode != test.code || result.Action == "" {
				t.Fatalf("result = %+v", result)
			}
		})
	}
}

func TestDeterministicRuleEngineIgnoresNegatedOrHypotheticalText(t *testing.T) {
	engine := DeterministicRuleEngine{}
	for _, input := range []string{"没有呼吸困难", "如果呼吸困难怎么办", "如何判断呼吸困难", "之前没有抽搐"} {
		result := engine.Evaluate(RiskInput{Text: input})
		if result.Level == RiskRed {
			t.Fatalf("input %q unexpectedly matched red rule: %+v", input, result)
		}
	}
}

func TestDeterministicRuleEngineMatchesActivePhraseAfterNegation(t *testing.T) {
	result := (DeterministicRuleEngine{}).Evaluate(RiskInput{Text: "之前没有呼吸困难，但现在呼吸困难"})
	if result.Level != RiskRed || result.TriggerCode != "breathing_distress" {
		t.Fatalf("result = %+v", result)
	}
}

func TestDeterministicRuleEngineEmptyInput(t *testing.T) {
	result := (DeterministicRuleEngine{}).Evaluate(RiskInput{Text: "   "})
	if result.Level != RiskUnknown || result.TriggerCode != "" {
		t.Fatalf("result = %+v", result)
	}
}
