package ask

import (
	"fmt"
	"sort"
)

// SkillScope：Skill 领域范围，同时决定候选召回后的排序优先级。
// 排序固定为：急症安全规则 > 用户明确业务操作 > 症状 Skill > 一般健康知识 > 闲聊 Skill。
type SkillScope string

const (
	ScopeEmergencySafety SkillScope = "emergency_safety" // 急症安全规则，最高优先级
	ScopeBusinessAction  SkillScope = "business_action"  // 用户明确业务操作
	ScopeSymptom         SkillScope = "symptom"          // 症状 Skill
	ScopeGeneralHealth   SkillScope = "general_health"   // 一般健康知识
	ScopeChitchat        SkillScope = "chitchat"         // 闲聊 Skill
)

// scopeOrder 定义各 SkillScope 的固定排序序数，序数越小优先级越高。
var scopeOrder = map[SkillScope]int{
	ScopeEmergencySafety: 0,
	ScopeBusinessAction:  1,
	ScopeSymptom:         2,
	ScopeGeneralHealth:   3,
	ScopeChitchat:        4,
}

// ValidSkillScope 判断 SkillScope 是否在一期枚举内。
func ValidSkillScope(value SkillScope) bool {
	_, ok := scopeOrder[value]
	return ok
}

// Skill：开发者维护、审核和发布的领域 Skill，不开放运营后台编辑。字段对应 v2 第 6 节。
type Skill struct {
	ID               string     `json:"skill_id"`
	Version          string     `json:"version"`
	Scope            SkillScope `json:"scope"`
	TriggerExamples  []string   `json:"trigger_examples"`
	NegativeExamples []string   `json:"negative_examples"`
	RequiredContext  []string   `json:"required_context"`
	PreferredTools   []string   `json:"preferred_tools"`
	ObservationRules []string   `json:"observation_rules"`
	QuestionPolicy   string     `json:"question_policy"`
	ResponsePolicy   string     `json:"response_policy"`
	RiskTriggers     []string   `json:"risk_triggers"`
	Priority         int        `json:"priority"`
}

// Validate 校验 Skill 目录字段。
func (s Skill) Validate() error {
	if s.ID == "" {
		return fmt.Errorf("skill: empty skill_id")
	}
	if !ValidSkillScope(s.Scope) {
		return fmt.Errorf("skill %q: unknown scope %q", s.ID, s.Scope)
	}
	if s.Version == "" {
		return fmt.Errorf("skill %q: empty version", s.ID)
	}
	return nil
}

// SortSkills 按固定顺序排序：scope 序数升序（急症安全优先），同 scope 内 priority 降序，
// 再按 skill_id 稳定排序保证确定性。
func SortSkills(skills []Skill) {
	sort.SliceStable(skills, func(i, j int) bool {
		oi := scopeOrder[skills[i].Scope]
		oj := scopeOrder[skills[j].Scope]
		if oi != oj {
			return oi < oj
		}
		if skills[i].Priority != skills[j].Priority {
			return skills[i].Priority > skills[j].Priority
		}
		return skills[i].ID < skills[j].ID
	})
}
