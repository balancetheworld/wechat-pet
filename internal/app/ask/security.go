package ask

import "strings"

// 本文件固定健康安全边界（文档 11.1）与注入/工具/引用防护（文档 11.2）的纯逻辑。
// 服务端确定性急症规则优先于模型：模型和规则冲突时取更高风险；
// 未知不能覆盖已知急症；绿色不表示健康或排除疾病。

// riskRank 返回风险等级的确定性排序权重（文档 11.1）。
// 顺序：unknown(0) < green(1) < yellow(2) < red(3)。
func riskRank(level RiskLevel) int {
	switch level {
	case RiskGreen:
		return 1
	case RiskYellow:
		return 2
	case RiskRed:
		return 3
	default:
		return 0
	}
}

// MergeRisk 合并规则风险与模型风险，取更高者（文档 11.1）。
// 最终结果不能低于已触发的规则风险；规则 unknown 时采用模型评估，
// 模型 unknown 时保持 unknown（未知不能覆盖已知急症）。
func MergeRisk(ruleLevel, modelLevel RiskLevel) RiskLevel {
	if riskRank(ruleLevel) >= riskRank(modelLevel) {
		return ruleLevel
	}
	return modelLevel
}

// MergedRisk 是一次风险合并的完整结果（文档 11.1）。
// 规则输出保存规则版本、触发码、原文依据和最低风险；
// 模型输出保存自身评估与不确定性；最终结果不能低于规则风险。
type MergedRisk struct {
	FinalLevel  RiskLevel
	RuleLevel   RiskLevel
	RuleVersion string
	RuleCode    string // 确定性规则触发码；空表示未命中规则
	RuleBasis   string // 触发规则的原文依据
	RuleMessage string
	RuleAction  string
	ModelLevel  RiskLevel
	ModelUncertainty string
}

// MergeRiskWithDetail 合并规则决策与模型风险，输出结构化结果（文档 11.1）。
func MergeRiskWithDetail(rule RiskDecision, modelLevel RiskLevel, modelUncertainty string) MergedRisk {
	final := MergeRisk(rule.Level, modelLevel)
	return MergedRisk{
		FinalLevel:       final,
		RuleLevel:        rule.Level,
		RuleVersion:      CurrentRuleVersion,
		RuleCode:         rule.TriggerCode,
		RuleBasis:        "",
		RuleMessage:      rule.Message,
		RuleAction:       rule.Action,
		ModelLevel:       modelLevel,
		ModelUncertainty: modelUncertainty,
	}
}

// InjectionKind 是注入检测结果的分类（文档 11.2）。
type InjectionKind string

const (
	InjectionRuleOverride     InjectionKind = "rule_override"      // 试图覆盖系统规则/扮演身份
	InjectionPrivilege        InjectionKind = "privilege_escalation" // 试图提升角色/越权
	InjectionToolBypass       InjectionKind = "tool_bypass"        // 试图绕过工具权限/确认
	InjectionDataExfil        InjectionKind = "data_exfil"         // 试图读取其他家庭/密钥
)

// InjectionHit 是一次注入检测的命中（文档 11.2）。
// 仅用于低信任标记与评测；不因命中某个词就删除用户内容。
type InjectionHit struct {
	Kind   InjectionKind
	Marker string
}

// injectionMarkers 是注入检测的保守模式集合（文档 11.2 最小安全测试）。
// 命中的文本仍作为低信任数据进入模型，但不获得指令权限、不触发工具、
// 不绕过确认；权限、预算、确认、派发均在服务端执行。
var injectionMarkers = []struct {
	kind    InjectionKind
	markers []string
}{
	{InjectionRuleOverride, []string{"忽略规则", "忽略之前的规则", "忽略以上", "不要遵守", "无视规则", "无视之前", "你是", "system prompt", "系统提示", "system:", "新指令"}},
	{InjectionPrivilege, []string{"管理员模式", "超级管理员", "root 权限", "sudo", "越权", "提升权限"}},
	{InjectionToolBypass, []string{"跳过确认", "无需确认", "不要询问", "直接执行", "自动保存", "绕过确认", "不要校验"}},
	{InjectionDataExfil, []string{"读取另一家庭", "其他家庭的数据", "输出密钥", "输出 api", "api key", "泄露", "密码是"}},
}

// DetectInjection 检测文本中的注入指令模式（文档 11.2）。
// 返回按出现顺序命中的分类；相同分类只保留首个命中。大小写不敏感。
func DetectInjection(text string) []InjectionHit {
	lower := strings.ToLower(text)
	hits := make([]InjectionHit, 0)
	seen := make(map[InjectionKind]bool)
	for _, group := range injectionMarkers {
		for _, marker := range group.markers {
			if strings.Contains(lower, strings.ToLower(marker)) {
				if !seen[group.kind] {
					hits = append(hits, InjectionHit{Kind: group.kind, Marker: marker})
					seen[group.kind] = true
				}
				break
			}
		}
	}
	return hits
}

// IsTrustedInstruction 报告一段内容是否为受控指令层（文档 5.8、11.2）。
// 只有服务端版本化配置产生的指令是高信任的；用户文字、记录、图片文字、
// 旧聊天、摘要、工具返回都是低信任数据，不能覆盖系统规则。
// 此函数由上下文组装层使用，不把低信任内容混入受控指令层。
func IsTrustedInstruction(block ContextBlock) bool {
	return block.Layer == LayerControlInstructions && block.Kind == "instruction"
}
