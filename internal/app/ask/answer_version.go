package ask

import "fmt"

// 本文件固定回答版本的两个独立维度（文档 5.3），避免一个状态同时表达「完整」和「正确适用」。
// 每个对话任务通过 origin_turn_id 关联一个回答组，组内版本跨 Run 单调增加。

// AnswerCompleteness 是生成完整性维度（文档 5.3）。
// 常量使用 Answer* 前缀，与工具结果 Completeness（complete/partial/not_applicable）区分。
type AnswerCompleteness string

const (
	AnswerStreaming  AnswerCompleteness = "streaming"  // 正在生成
	AnswerComplete   AnswerCompleteness = "complete"   // 已通过最终校验
	AnswerIncomplete AnswerCompleteness = "incomplete" // 因停止或故障未形成完整有效答复
)

func (c AnswerCompleteness) Valid() bool {
	switch c {
	case AnswerStreaming, AnswerComplete, AnswerIncomplete:
		return true
	default:
		return false
	}
}

// AnswerApplicability 是输入适用性维度（文档 5.3）。
type AnswerApplicability string

const (
	ApplicabilityCurrent    AnswerApplicability = "current"    // 对应当前输入
	ApplicabilitySuperseded AnswerApplicability = "superseded" // 被后续输入或版本替代
	ApplicabilityRejected   AnswerApplicability = "rejected"   // 被最终校验拒绝
)

func (a AnswerApplicability) Valid() bool {
	switch a {
	case ApplicabilityCurrent, ApplicabilitySuperseded, ApplicabilityRejected:
		return true
	default:
		return false
	}
}

// AnswerVersion 定位一个回答版本（文档 5.3、8.6）。
type AnswerVersion struct {
	OriginTurnID string             // 回答组所属任务（origin_turn_id 关联）
	Version      int                // 组内版本，跨 Run 单调增加
	Completeness AnswerCompleteness // 生成完整性
	Applicability AnswerApplicability // 输入适用性
	InputRevision int               // 关联输入版本
}

// Valid 报告回答版本是否满足双维度约束。
func (v AnswerVersion) Valid() bool {
	return v.Completeness.Valid() && v.Applicability.Valid()
}

// IsEffectiveAnswer 报告该版本是否构成有效回答（文档 5.3）。
// current 不等于可以采用，只有完整性、适用性及最终安全检查都满足才是有效回答。
func (v AnswerVersion) IsEffectiveAnswer() bool {
	return v.Completeness == AnswerComplete && v.Applicability == ApplicabilityCurrent
}

// CanPublish 报告该版本当前是否允许发布/继续追加段。
// 只有仍对应当前输入的流式或完整版本可以继续；被替代或被拒绝的版本停止接收增量。
func (v AnswerVersion) CanPublish() bool {
	if v.Applicability != ApplicabilityCurrent {
		return false
	}
	return v.Completeness == AnswerStreaming || v.Completeness == AnswerComplete
}

// SameAnswerGroup 报告两个回答版本是否属于同一对话任务（文档 5.3）。
// 每个对话任务通过 origin_turn_id 关联一个回答组，组内版本跨 Run 单调增加。
func SameAnswerGroup(a, b AnswerVersion) bool {
	return a.OriginTurnID != "" && a.OriginTurnID == b.OriginTurnID
}

// ValidateVersionMonotonic 校验同一回答组内版本跨 Run 单调增加（文档 5.3）。
// 版本不能简单等同于 run_index 或 Attempt 次数；新版本必须严格大于旧版本。
func ValidateVersionMonotonic(prev, next int) error {
	if next <= prev {
		return fmt.Errorf("answer_version: version must be strictly increasing, got %d -> %d", prev, next)
	}
	return nil
}

// ValidateAnswerVersion 校验回答版本字段（文档 5.3）。
func ValidateAnswerVersion(v AnswerVersion) error {
	if v.OriginTurnID == "" {
		return fmt.Errorf("answer_version: missing origin_turn_id")
	}
	if v.Version <= 0 {
		return fmt.Errorf("answer_version: version must be positive")
	}
	if !v.Completeness.Valid() {
		return fmt.Errorf("answer_version: invalid completeness %q", v.Completeness)
	}
	if !v.Applicability.Valid() {
		return fmt.Errorf("answer_version: invalid applicability %q", v.Applicability)
	}
	return nil
}
