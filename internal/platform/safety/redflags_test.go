package safety

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMatchReturnsStableCodes(t *testing.T) {
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
		{name: "vomiting blood", input: "呕吐物带血", code: "vomiting_blood"},
		{name: "bloody stool", input: "大便带血", code: "bloody_stool"},
		{name: "unable to drink", input: "猫咪无法喝水", code: "unable_to_drink"},
		{name: "severe weakness", input: "它今天精神很差", code: "severe_weakness"},
		{name: "repeated vomiting", input: "猫咪连续呕吐", code: "repeated_vomiting"},
		{name: "urinary", input: "一直蹲猫砂没尿", code: "urinary_obstruction"},
		{name: "abdominal", input: "肚子胀得很大", code: "severe_abdominal_pain"},
		{name: "deterioration", input: "突然倒地不起", code: "rapid_deterioration"},
		{name: "dehydration", input: "牙龈很干，眼窝凹陷", code: "dehydration"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			flag, ok := Match(test.input)
			if !ok || flag.Code != test.code {
				t.Fatalf("Match(%q) = %+v, ok=%t, want code %q", test.input, flag, ok, test.code)
			}
			if flag.Action == "" || flag.Message == "" {
				t.Fatalf("flag %q must carry message and action: %+v", flag.Code, flag)
			}
		})
	}
}

func TestMatchIgnoresNegatedOrHypotheticalText(t *testing.T) {
	for _, input := range []string{"没有呼吸困难", "如果呼吸困难怎么办", "如何判断呼吸困难", "之前没有抽搐", "没有无法喝水", "如果精神很差怎么办"} {
		if flag, ok := Match(input); ok {
			t.Fatalf("input %q unexpectedly matched %q", input, flag.Code)
		}
	}
}

func TestMatchFindsActivePhraseAfterNegation(t *testing.T) {
	flag, ok := Match("之前没有呼吸困难，但现在呼吸困难")
	if !ok || flag.Code != "breathing_distress" {
		t.Fatalf("Match returned %+v, ok=%t", flag, ok)
	}
}

func TestTermsReturnsMatchedPhrases(t *testing.T) {
	terms := Terms("猫咪呕吐物带血，而且无法喝水")
	if len(terms) == 0 {
		t.Fatal("expected red flag terms")
	}
	for _, want := range []string{"呕吐物带血", "无法喝水"} {
		if !contains(terms, want) {
			t.Fatalf("terms = %v, want %q", terms, want)
		}
	}
	if got := Terms("没有呼吸困难也没有抽搐"); len(got) != 0 {
		t.Fatalf("negated terms should be empty, got %v", got)
	}
}

func TestContainsPhraseIgnoresNegation(t *testing.T) {
	// 知识正文是审核过的参考文本，抽取升级条件按字面判断，
	// 否则「如果无法喝水」这类条件句会被漏掉。
	if !ContainsPhrase("如果猫咪无法喝水，应尽快联系宠物医生。") {
		t.Fatal("knowledge paragraph with escalation condition should match")
	}
	if ContainsPhrase("猫咪今天精神很好，吃饭正常。") {
		t.Fatal("ordinary observation should not match red flag phrases")
	}
}

func TestFlagsAreWellFormed(t *testing.T) {
	flags := Flags()
	if len(flags) == 0 {
		t.Fatal("red flag table must not be empty")
	}
	seen := make(map[string]struct{}, len(flags))
	for _, flag := range flags {
		if flag.Code == "" || flag.Label == "" || flag.Message == "" || flag.Action == "" {
			t.Fatalf("flag has empty field: %+v", flag)
		}
		if len(flag.Phrases) == 0 {
			t.Fatalf("flag %q has no phrases", flag.Code)
		}
		if _, ok := seen[flag.Code]; ok {
			t.Fatalf("duplicate flag code %q", flag.Code)
		}
		seen[flag.Code] = struct{}{}
		if _, ok := FlagByCode(flag.Code); !ok {
			t.Fatalf("FlagByCode(%q) not found", flag.Code)
		}
	}
}

// TestRedFlagTableIsDocumented 锁定红旗表与人工审核文档一致，
// 避免补充词表后忘记更新 docs/red-flags.md。
func TestRedFlagTableIsDocumented(t *testing.T) {
	path := filepath.Join("..", "..", "..", "docs", "red-flags.md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	doc := string(raw)
	for _, flag := range Flags() {
		if !strings.Contains(doc, flag.Code) {
			t.Fatalf("docs/red-flags.md missing code %q", flag.Code)
		}
		if !strings.Contains(doc, flag.Label) {
			t.Fatalf("docs/red-flags.md missing label %q", flag.Label)
		}
	}
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
