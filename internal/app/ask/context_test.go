package ask

import (
	"strings"
	"testing"
)

func TestSymptomTagAndContextSummary(t *testing.T) {
	if symptomTag("最近已经吐了两次") != "symptom_vomiting" {
		t.Fatal("vomiting tag mismatch")
	}
	if symptomTag("没有咳嗽") != "symptom_cough" {
		t.Fatal("symptom tag should only normalize, not evaluate risk")
	}
	value := contextSummary(strings.Repeat("很长的记录", 80))
	if len([]rune(value)) != 200 {
		t.Fatalf("summary length = %d", len([]rune(value)))
	}
}
