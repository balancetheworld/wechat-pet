package ask

import (
	"strings"
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

type riskRule struct {
	code    string
	phrases []string
	message string
	action  string
}

var redRiskRules = []riskRule{
	{code: "breathing_distress", phrases: []string{"呼吸困难", "呼吸急促", "喘不上气", "喘不过气", "张口呼吸", "呼吸不上来"}, message: "观察到可能存在呼吸异常", action: "请立即联系附近的宠物医院；途中尽量保持安静，避免强行喂食或喂水。"},
	{code: "persistent_seizure", phrases: []string{"持续抽搐", "一直抽搐", "反复抽搐", "不停抽搐", "癫痫发作"}, message: "观察到可能存在持续或反复抽搐", action: "请立即联系附近的宠物医院，移开周围尖锐物品，不要把手伸进宠物口中。"},
	{code: "severe_bleeding", phrases: []string{"大量出血", "止不住血", "一直流血", "喷血", "大出血"}, message: "观察到可能存在严重出血", action: "请立即联系附近的宠物医院；可用干净纱布轻压出血处，不要反复掀开查看。"},
	{code: "suspected_poisoning", phrases: []string{"疑似中毒", "可能中毒", "误食毒", "吃了老鼠药", "吃了杀虫剂", "误食了杀虫剂", "误食药物", "误食清洁剂"}, message: "观察到可能存在误食或中毒风险", action: "请立即联系附近的宠物医院，不要自行催吐或喂药，并保留可能误食物品的信息。"},
	{code: "altered_consciousness", phrases: []string{"意识不清", "失去意识", "昏迷", "叫不醒", "没有反应", "意识异常"}, message: "观察到可能存在意识异常", action: "请立即联系附近的宠物医院，避免摇晃或强行喂食。"},
	{code: "unable_to_stand", phrases: []string{"无法站立", "站不起来", "不能站立", "突然瘫倒", "四肢无力"}, message: "观察到可能存在突发运动障碍", action: "请立即联系附近的宠物医院，减少移动并防止跌落。"},
}

func (DeterministicRuleEngine) Evaluate(input RiskInput) RiskDecision {
	text := normalizeRuleText(input.Text)
	if text == "" {
		return RiskDecision{Level: RiskUnknown}
	}
	for _, rule := range redRiskRules {
		for _, phrase := range rule.phrases {
			if containsActivePhrase(text, phrase) {
				return RiskDecision{Level: RiskRed, TriggerCode: rule.code, Message: rule.message, Action: rule.action}
			}
		}
	}
	return RiskDecision{Level: RiskUnknown}
}

func normalizeRuleText(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	value = strings.Join(strings.Fields(value), "")
	return value
}

func containsActivePhrase(text, phrase string) bool {
	index := strings.Index(text, phrase)
	for index >= 0 {
		before := text[:index]
		if !hasNegationOrHypothetical(before) {
			return true
		}
		next := index + len(phrase)
		remaining := text[next:]
		nextIndex := strings.Index(remaining, phrase)
		if nextIndex < 0 {
			return false
		}
		index = next + nextIndex
	}
	return false
}

func hasNegationOrHypothetical(prefix string) bool {
	for _, marker := range []string{"没有", "无", "未见", "不是", "并非", "如果", "假如", "怎么判断", "如何判断", "担心", "怕"} {
		if strings.HasSuffix(prefix, marker) {
			return true
		}
	}
	return false
}
