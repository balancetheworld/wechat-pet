package ai

import (
	askapp "github.com/balancetheworld/wechat-pet/internal/app/ask"
)

// UsageBudgetAmount 把归一化用量换算为预算结算用量。
// 一次模型调用记 ModelCalls=1；Token 取输入+输出合计；工具执行次数单列、费用一期
// 免费不设硬上限，故 ToolCalls 与 CostMicros 记 0，由上层在需要时另行计入。
func UsageBudgetAmount(u Usage) askapp.BudgetAmount {
	u = u.Normalize()
	return askapp.BudgetAmount{
		ModelCalls: 1,
		Tokens:     int(u.TotalTokens),
	}
}

// UsageCallOutcome 根据用量完整性报告本次调用应结算还是保留待对账。
// 完整用量 → CallOutcomeSent（结算）；缺失/未知用量 → CallOutcomeUnknown（保留，
// 不把缺失用量记为零结算）。落实 v2 文档 11.4「未知用量不记零」。
func UsageCallOutcome(u Usage) askapp.CallOutcome {
	if u.Normalize().Complete {
		return askapp.CallOutcomeSent
	}
	return askapp.CallOutcomeUnknown
}
