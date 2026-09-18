package ai

import (
	"testing"

	askapp "github.com/balancetheworld/wechat-pet/internal/app/ask"
)

func TestUsageBudgetAmount(t *testing.T) {
	amount := UsageBudgetAmount(Usage{InputTokens: 100, OutputTokens: 50, Complete: true})
	if amount.ModelCalls != 1 || amount.Tokens != 150 {
		t.Fatalf("amount = %+v, want ModelCalls=1 Tokens=150", amount)
	}
	if amount.ToolCalls != 0 || amount.CostMicros != 0 || amount.Concurrency != 0 {
		t.Fatalf("amount = %+v, want ToolCalls/CostMicros/Concurrency = 0", amount)
	}
	// TotalTokens 未显式设置时由输入+输出推导
	amount = UsageBudgetAmount(Usage{InputTokens: 10, OutputTokens: 20, Complete: true})
	if amount.Tokens != 30 {
		t.Fatalf("Tokens = %d, want 30 (输入+输出推导)", amount.Tokens)
	}
}

func TestUsageCallOutcome(t *testing.T) {
	if got := UsageCallOutcome(Usage{InputTokens: 10, OutputTokens: 5, Complete: true}); got != askapp.CallOutcomeSent {
		t.Fatalf("complete usage outcome = %v, want %v", got, askapp.CallOutcomeSent)
	}
	// 用量缺失（Complete=false）→ 保留待对账，不按零结算
	if got := UsageCallOutcome(Usage{InputTokens: 10, Complete: false}); got != askapp.CallOutcomeUnknown {
		t.Fatalf("incomplete usage outcome = %v, want %v", got, askapp.CallOutcomeUnknown)
	}
	if got := UsageCallOutcome(Usage{}); got != askapp.CallOutcomeUnknown {
		t.Fatalf("empty usage outcome = %v, want %v", got, askapp.CallOutcomeUnknown)
	}
}
