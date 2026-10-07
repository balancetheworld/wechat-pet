package ask

import (
	"github.com/balancetheworld/wechat-pet/internal/platform/safety"
)

const CurrentRuleVersion = "ask-rules-v1"

type RiskInput struct {
	Text string
}

type RiskDecision struct {
	Level       RiskLevel
	TriggerCode string
	Message     string
	Action      string
}

type RuleEngine interface {
	Evaluate(RiskInput) RiskDecision
}

type DeterministicRuleEngine struct{}

// Evaluate 使用项目唯一的红旗表判定确定性风险（internal/platform/safety）。
// 命中任一红旗即升级为 red 并直接给出就医提示，不交给模型自由判断；
// 未命中返回 unknown，后续风险等级由模型建议与 Runtime 合并裁决。
func (DeterministicRuleEngine) Evaluate(input RiskInput) RiskDecision {
	if flag, ok := safety.Match(input.Text); ok {
		return RiskDecision{Level: RiskRed, TriggerCode: flag.Code, Message: flag.Message, Action: flag.Action}
	}
	return RiskDecision{Level: RiskUnknown}
}
