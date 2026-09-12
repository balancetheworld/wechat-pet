package ai

import (
	"testing"

	askapp "github.com/balancetheworld/wechat-pet/internal/app/ask"
)

func TestParseFamilyQueryIntent(t *testing.T) {
	decision, err := parseIntentDecision(`{"intent":"family_query","reply":"","question":""}`)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Intent != askapp.IntentFamilyQuery {
		t.Fatalf("intent = %s", decision.Intent)
	}
}
