package ask

import "fmt"

// 本文件固定按对象和任务组织的回答结构（文档 8.5）与回答版本（5.3）的领域类型。
// 一个最终回答由多个 group 组成，表达多宠物、多意图；每组有 answer_kind 与 scope，
// scope 表达答复范围而非执行成功标志，不直接更新任务项状态。

// AnswerKind 是回答组类型（文档 8.5）。
type AnswerKind string

const (
	AnswerCasual AnswerKind = "casual"
	AnswerFact   AnswerKind = "fact"
	AnswerHealth AnswerKind = "health"
)

func (k AnswerKind) Valid() bool {
	switch k {
	case AnswerCasual, AnswerFact, AnswerHealth:
		return true
	default:
		return false
	}
}

// AnswerScope 是答复范围（文档 8.5），不是执行成功标志。
type AnswerScope string

const (
	ScopeFull        AnswerScope = "full"
	ScopeLimited     AnswerScope = "limited"
	ScopeDeclined    AnswerScope = "declined"
	ScopeUnavailable AnswerScope = "unavailable"
)

func (s AnswerScope) Valid() bool {
	switch s {
	case ScopeFull, ScopeLimited, ScopeDeclined, ScopeUnavailable:
		return true
	default:
		return false
	}
}

// SubjectKind 区分已明确宠物与未明确对象（文档 8.4、8.5）。
type SubjectKind string

const (
	SubjectPet        SubjectKind = "pet"
	SubjectUnresolved SubjectKind = "unresolved"
)

// AnswerSubject 是组内对象，组内唯一 subject_key。
// 已明确宠物附带宠物句柄；未明确对象附带带输入依据的描述（文档 8.5）。
type AnswerSubject struct {
	SubjectKey    string      `json:"subject_key"`
	Kind          SubjectKind `json:"kind"`
	PetID         string      `json:"pet_id,omitempty"`
	Description   string      `json:"description,omitempty"`
	SourceTurnIDs []string    `json:"source_turn_ids,omitempty"`
}

// BasisKind 区分事实来源（文档 8.5）。推测不能转为事实。
type BasisKind string

const (
	BasisUserStatement    BasisKind = "user_statement"
	BasisImageObservation BasisKind = "image_observation"
	BasisBusinessFact     BasisKind = "business_fact"
	BasisGeneralKnowledge BasisKind = "general_knowledge"
	BasisSpeculation      BasisKind = "speculation"
)

func (b BasisKind) Valid() bool {
	switch b {
	case BasisUserStatement, BasisImageObservation, BasisBusinessFact, BasisGeneralKnowledge, BasisSpeculation:
		return true
	default:
		return false
	}
}

// EvidenceRef 是输入快照内的来源句柄与可核验位置（文档 8.5）。
// 不能凭模型提供的任意 URL 或新造 ID 创建可信引用。
type EvidenceRef struct {
	SourceType string `json:"source_type"`
	SourceID   string `json:"source_id"`
	Version    string `json:"version,omitempty"`
	Position   string `json:"position,omitempty"`
}

// RiskLevel.Valid 供风险枚举合法性校验（定义于 model.go）。
func (r RiskLevel) Valid() bool {
	switch r {
	case RiskUnknown, RiskGreen, RiskYellow, RiskRed:
		return true
	default:
		return false
	}
}

// allowedFieldsForKind 报告某回答类型允许的 field 值（文档 8.5）。
// limitation 是所有受限范围（scope 非 full）可用的补充字段。
func allowedFieldForKind(kind AnswerKind, field string) bool {
	switch kind {
	case AnswerCasual:
		return field == "reply" || field == "limitation"
	case AnswerFact:
		return field == "result" || field == "limitation"
	case AnswerHealth:
		switch field {
		case "observation", "possible_direction", "watch_item", "care_condition", "uncertainty", "risk", "next_action", "limitation":
			return true
		default:
			return false
		}
	default:
		return false
	}
}

// requiredFieldForKind 报告某回答类型的必填 field（文档 8.5）。
// health 类型的“必填”是要求交代该方面是否可判断，不要求生成肯定结论。
func requiredFieldForKind(kind AnswerKind) (field string, ok bool) {
	switch kind {
	case AnswerCasual:
		return "reply", true
	case AnswerFact:
		return "result", true
	default:
		return "", false
	}
}

// AnswerGroup 是从响应记录组装出的回答组（文档 8.5）。
// Segments 按 field 组织，Risk 为每组每对象至多一条的模型风险建议。
type AnswerGroup struct {
	GroupKey   string          `json:"group_key"`
	TaskKeys   []string        `json:"task_keys"`
	AnswerKind AnswerKind      `json:"answer_kind"`
	Subjects   []AnswerSubject `json:"subjects"`
	Scope      AnswerScope     `json:"scope"`
	Segments   []SegmentRecord `json:"segments"`
	Risks      []RiskRecord    `json:"risks"`
}

// hasField 报告该组是否已有某 field 的非空段。
func (g *AnswerGroup) hasField(field string) bool {
	for _, s := range g.Segments {
		if s.Field == field && s.Text != "" {
			return true
		}
	}
	return false
}

// hasLimitation 报告该组是否有非空 limitation 段。
func (g *AnswerGroup) hasLimitation() bool { return g.hasField("limitation") }

// ValidateAnswerGroup 校验一个已组装回答组的分类型字段规则（文档 8.5）。
func ValidateAnswerGroup(g AnswerGroup) error {
	if !g.AnswerKind.Valid() {
		return fmt.Errorf("answer: invalid answer_kind %q", g.AnswerKind)
	}
	if !g.Scope.Valid() {
		return fmt.Errorf("answer: invalid scope %q", g.Scope)
	}
	if len(g.TaskKeys) == 0 {
		return fmt.Errorf("answer: group %q has empty task_keys", g.GroupKey)
	}
	if len(g.Subjects) == 0 {
		return fmt.Errorf("answer: group %q has empty subjects", g.GroupKey)
	}
	for _, segment := range g.Segments {
		if segment.BasisKind == BasisBusinessFact && len(segment.EvidenceRefs) == 0 {
			return fmt.Errorf("answer: business_fact segment %q requires evidence_refs", segment.SegmentKey)
		}
	}
	if req, ok := requiredFieldForKind(g.AnswerKind); ok && !g.hasField(req) {
		return fmt.Errorf("answer: group %q of kind %q requires field %q", g.GroupKey, g.AnswerKind, req)
	}
	if g.Scope != ScopeFull && !g.hasLimitation() {
		return fmt.Errorf("answer: group %q with scope %q requires a limitation", g.GroupKey, g.Scope)
	}
	if g.AnswerKind == AnswerHealth {
		// 健康组的每个对象都须有 risk（文档 8.5：交代该对象风险是否可判断）。
		for _, subj := range g.Subjects {
			if !g.hasRiskFor(subj.SubjectKey) {
				return fmt.Errorf("answer: health group %q subject %q missing risk", g.GroupKey, subj.SubjectKey)
			}
		}
	}
	return nil
}

func (g *AnswerGroup) hasRiskFor(subjectKey string) bool {
	for _, r := range g.Risks {
		if r.SubjectKey == subjectKey {
			return true
		}
	}
	return false
}

// AssembleAnswerGroups 把已通过结构校验的 final_answer 记录组装为回答组。
// 返回组列表；风险记录按 group_key + subject_key 归入对应组。
func AssembleAnswerGroups(records []ProtocolRecord) ([]AnswerGroup, error) {
	order := make([]string, 0)
	groups := make(map[string]*AnswerGroup)
	for _, rec := range records {
		switch rec.Type {
		case RecordGroup:
			g := &AnswerGroup{
				GroupKey:   rec.Group.GroupKey,
				TaskKeys:   rec.Group.TaskKeys,
				AnswerKind: rec.Group.AnswerKind,
				Subjects:   rec.Group.Subjects,
				Scope:      rec.Group.Scope,
			}
			groups[g.GroupKey] = g
			order = append(order, g.GroupKey)
		case RecordSegment:
			g := groups[rec.Segment.GroupKey]
			if g == nil {
				return nil, fmt.Errorf("answer: segment %q references unknown group %q", rec.Segment.SegmentKey, rec.Segment.GroupKey)
			}
			g.Segments = append(g.Segments, *rec.Segment)
		case RecordRisk:
			g := groups[rec.Risk.GroupKey]
			if g == nil {
				return nil, fmt.Errorf("answer: risk references unknown group %q", rec.Risk.GroupKey)
			}
			g.Risks = append(g.Risks, *rec.Risk)
		}
	}
	result := make([]AnswerGroup, 0, len(order))
	for _, key := range order {
		result = append(result, *groups[key])
	}
	return result, nil
}
