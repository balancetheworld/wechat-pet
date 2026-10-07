package ask

import (
	"fmt"
	"strings"
)

// 本文件固定「超过 32 轮、保留最近 16 轮」的摘要契约。
// 一轮是一条已接受且属于本次可用历史的用户 Turn；追问回复和补充各计一轮，
// 工具步骤、模型重试、回答版本和确认按钮不增加轮数。
// 摘要仅影响后续模型上下文，不删除或改写聊天原文。
// 当前摘要规划与候选校验尚无生产调用方，属规划中能力。

// SummaryTrigger：摘要触发原因。
type SummaryTrigger string

const (
	SummaryTriggerNone  SummaryTrigger = "none"  // 未触发
	SummaryTriggerCount SummaryTrigger = "count" // 数量触发：超过阈值
	SummaryTriggerToken SummaryTrigger = "token" // Token 不足提前压缩
)

// SummaryThresholds 是 32/16 摘要的固定阈值（文档 5.10）。
// 超过 32 条未压缩可用 Turn 时，把最近 16 条之前的可压缩完整区间生成摘要；
// 恰好 32 条不触发，第 33 条才触发。
const (
	SummaryCompressThreshold = 32
	SummaryKeepTurns         = 16
)

// SummaryPlan 是一次摘要的范围计算（文档 5.10）。
type SummaryPlan struct {
	Triggered       bool // 是否需要压缩
	Trigger         SummaryTrigger
	CompressTurnIDs []string // 可压缩区间内的 Turn ID（最近 keep 条之前）
	KeepTurnIDs     []string // 保留区间内的 Turn ID（最近 keep 条）
	Reason          string   // 触发原因描述
}

// PlanSummarize 计算数量触发的摘要范围（文档 5.10 开头）。
// turns 必须是本次可用历史中未压缩的用户 Turn，且已按时间顺序排列。
// 恰好 threshold 条不触发，第 threshold+1 条才触发；压缩区间为最近 keep 条之前。
func PlanSummarize(turns []Turn, threshold, keep int) SummaryPlan {
	if threshold <= 0 {
		threshold = SummaryCompressThreshold
	}
	if keep <= 0 {
		keep = SummaryKeepTurns
	}
	if len(turns) <= threshold {
		return SummaryPlan{Triggered: false, Trigger: SummaryTriggerNone}
	}
	keepStart := len(turns) - keep
	if keepStart < 0 {
		keepStart = 0
	}
	compress := make([]string, 0, keepStart)
	for _, t := range turns[:keepStart] {
		if t.ID != "" {
			compress = append(compress, t.ID)
		}
	}
	kept := make([]string, 0, len(turns)-keepStart)
	for _, t := range turns[keepStart:] {
		if t.ID != "" {
			kept = append(kept, t.ID)
		}
	}
	// 无可压缩区间（例如全部 Turn 都属于最近 keep 条）时，容量允许则保留原文，
	// 不因数量阈值而失败（文档 5.10）。
	if len(compress) == 0 {
		return SummaryPlan{Triggered: false, Trigger: SummaryTriggerNone}
	}
	return SummaryPlan{
		Triggered:       true,
		Trigger:         SummaryTriggerCount,
		CompressTurnIDs: compress,
		KeepTurnIDs:     kept,
		Reason:          fmt.Sprintf("uncompressed turns %d exceed threshold %d", len(turns), threshold),
	}
}

// PlanSummarizeForToken 在 Token 不足时提前压缩（文档 5.10：Token 不足可提前压缩，
// 但不能拆开必要的追问和回复关系）。turns 同样按时间顺序。
func PlanSummarizeForToken(turns []Turn, keep int) SummaryPlan {
	if keep <= 0 {
		keep = SummaryKeepTurns
	}
	if len(turns) <= keep {
		return SummaryPlan{Triggered: false, Trigger: SummaryTriggerNone}
	}
	keepStart := len(turns) - keep
	compress := make([]string, 0, keepStart)
	for _, t := range turns[:keepStart] {
		if t.ID != "" {
			compress = append(compress, t.ID)
		}
	}
	kept := make([]string, 0, len(turns)-keepStart)
	for _, t := range turns[keepStart:] {
		if t.ID != "" {
			kept = append(kept, t.ID)
		}
	}
	if len(compress) == 0 {
		return SummaryPlan{Triggered: false, Trigger: SummaryTriggerNone}
	}
	return SummaryPlan{
		Triggered:       true,
		Trigger:         SummaryTriggerToken,
		CompressTurnIDs: compress,
		KeepTurnIDs:     kept,
		Reason:          "token budget insufficient, compressing earlier turns",
	}
}

// SummaryStatus：摘要候选使用状态（文档 5.10.3）。被拒绝、过期或替代的候选
// 不恢复为可用；重建产生新版本。
type SummaryStatus string

const (
	SummaryStatusCandidate  SummaryStatus = "candidate"  // 完整候选落库，尚未通过全部核验
	SummaryStatusUsable     SummaryStatus = "usable"     // 全部核验通过，可进入模型上下文
	SummaryStatusRejected   SummaryStatus = "rejected"   // 内容/结构/覆盖/压缩要求未通过
	SummaryStatusStale      SummaryStatus = "stale"      // 来源不可用、版本变化、授权撤回
	SummaryStatusSuperseded SummaryStatus = "superseded" // 被合格新版本原子替代
)

// Valid 报告状态是否为契约内合法值。
func (s SummaryStatus) Valid() bool {
	switch s {
	case SummaryStatusCandidate, SummaryStatusUsable, SummaryStatusRejected, SummaryStatusStale, SummaryStatusSuperseded:
		return true
	default:
		return false
	}
}

// Terminal 报告状态是否为不再参与上下文选择的终态。
func (s SummaryStatus) Terminal() bool {
	switch s {
	case SummaryStatusRejected, SummaryStatusStale, SummaryStatusSuperseded:
		return true
	default:
		return false
	}
}

// CanTransitionSummary 校验摘要候选状态转移（文档 5.10.3）。
func CanTransitionSummary(from, to SummaryStatus) bool {
	switch from {
	case SummaryStatusCandidate:
		return to == SummaryStatusUsable || to == SummaryStatusRejected || to == SummaryStatusStale
	case SummaryStatusUsable:
		return to == SummaryStatusStale || to == SummaryStatusSuperseded
	default:
		// rejected / stale / superseded 是终态，不恢复为可用；重建产生新版本。
		return false
	}
}

// SummaryItemType：摘要条目类型（文档 5.10.1 items 行）。
// 类型区分用户陈述、业务事实、过去建议、缺失、纠正及状态引用；
// 不得新增来源没有的诊断、归属和成功状态。
type SummaryItemType string

const (
	SummaryItemUserStatement SummaryItemType = "user_statement" // 用户陈述
	SummaryItemBusinessFact  SummaryItemType = "business_fact"  // 已保存业务事实
	SummaryItemPastAdvice    SummaryItemType = "past_advice"    // 过去的建议（不作为事实）
	SummaryItemMissing       SummaryItemType = "missing"        // 缺失信息与用户纠正
	SummaryItemCorrection    SummaryItemType = "correction"     // 纠正关系
	SummaryItemStatusRef     SummaryItemType = "status_ref"     // 未完成任务/Operation 引用
)

// Valid 报告条目类型是否合法。
func (t SummaryItemType) Valid() bool {
	switch t {
	case SummaryItemUserStatement, SummaryItemBusinessFact, SummaryItemPastAdvice,
		SummaryItemMissing, SummaryItemCorrection, SummaryItemStatusRef:
		return true
	default:
		return false
	}
}

// SummarySourceRef 是条目到冻结来源清单的引用（文档 5.10.1）。
type SummarySourceRef struct {
	SourceType string
	ObjectID   string
	Version    string
}

// SummarySource 是摘要来源清单中的一项（文档 5.10.1 sources 行）。
// 逐项保存来源类型、对象标识、版本、原始时间、实际受控片段及片段定位。
type SummarySource struct {
	SourceType   string
	ObjectID     string
	Version      string
	OriginalTime string
	Segment      string // 实际受控片段
	Position     string // 片段定位
}

// SummaryItem 是摘要中的一条（文档 5.10.1 items 行）。
type SummaryItem struct {
	Key          string
	Type         SummaryItemType
	Subject      string // 对象或未明确对象描述
	Statement    string // 陈述内容
	EventTime    string // 事件时间（原文时间，不自行归一化）
	SourceRefs   []SummarySourceRef
	RequiredText string // 必要原文或原始字段值
}

// SummaryCoverage 是覆盖说明（文档 5.10.1 coverage 行）。
// 对拟替代的每组来源列出所对应条目、保留原文范围或可省略原因；
// 原因限定为经核验的重复、无关内容或按规则排除的正文。
type SummaryCoverage struct {
	SourceType string
	SourceID   string
	ItemKeys   []string
	KeptText   bool   // 是否保留原文
	OmitReason string // 可省略原因
}

// SummaryCandidate 是一次摘要候选（文档 5.10.1）。
// 服务端分配身份，模型只能返回来源临时键，不能改归属或状态。
type SummaryCandidate struct {
	ID             string
	Version        int
	SessionID      string
	RunID          string
	AttemptID      string
	InputRevision  int
	Status         SummaryStatus
	Sources        []SummarySource
	ReplaceRefs    []SummarySourceRef // 拟从上下文移出的完整来源组
	ConstraintRefs []SummarySourceRef // 关联的纠正、对象与未完成状态（仍按 5.9 保留）
	RequiredItems  []string           // 需保留的已知对象、陈述、否定、未知、纠正、任务关联
	Items          []SummaryItem
	Coverage       []SummaryCoverage
	RejectReason   string
}

// ValidateSummaryCandidate 执行摘要候选的纯逻辑核验（文档 5.10.2）。
// 覆盖可本地判定的部分：结构完整、来源位于冻结清单、覆盖关系成立、
// 不夹带业务动作或新增事实；对象/时间/数值/否定的语义比对由评测承担。
func ValidateSummaryCandidate(c SummaryCandidate) error {
	// 1. 结构核验（5.10.2 第 1 步）。
	if c.ID == "" {
		return fmt.Errorf("summary: empty summary_id")
	}
	if c.Version <= 0 {
		return fmt.Errorf("summary: version must be positive")
	}
	if c.SessionID == "" {
		return fmt.Errorf("summary: empty session_id")
	}
	if !c.Status.Valid() {
		return fmt.Errorf("summary: invalid status %q", c.Status)
	}
	if len(c.Sources) == 0 {
		return fmt.Errorf("summary: empty sources")
	}
	if len(c.Items) == 0 {
		return fmt.Errorf("summary: empty items")
	}

	// 2. 来源核验（5.10.2 第 2 步）：所有条目引用必须位于冻结来源清单内。
	sourceIndex := buildSummarySourceIndex(c.Sources)
	for _, it := range c.Items {
		if !it.Type.Valid() {
			return fmt.Errorf("summary: item %q has invalid type %q", it.Key, it.Type)
		}
		if it.Key == "" {
			return fmt.Errorf("summary: item has empty key")
		}
		if it.Statement == "" {
			return fmt.Errorf("summary: item %q has empty statement", it.Key)
		}
		for _, ref := range it.SourceRefs {
			if !sourceIndex[summarySourceRefKey(ref)] {
				return fmt.Errorf("summary: item %q references source %s/%s not in frozen source list",
					it.Key, ref.SourceType, ref.ObjectID)
			}
		}
	}

	// 3. 覆盖核验（5.10.2 第 4 步）：每个拟替代来源组必须有覆盖说明。
	for _, replace := range c.ReplaceRefs {
		if !sourceIndex[summarySourceRefKey(replace)] {
			return fmt.Errorf("summary: replace_ref %s/%s not in frozen source list",
				replace.SourceType, replace.ObjectID)
		}
		if !summaryCoverageCovers(c.Coverage, replace) {
			return fmt.Errorf("summary: replace source %s/%s has no coverage",
				replace.SourceType, replace.ObjectID)
		}
	}

	// 4. 禁止夹带（5.10.2 第 1 步 + 4.2）：摘要不得包含业务动作或新增事实的指令。
	for _, it := range c.Items {
		if containsBusinessAction(it.Statement) {
			return fmt.Errorf("summary: item %q contains business action, not allowed in summary", it.Key)
		}
	}
	return nil
}

// buildSummarySourceIndex 建立来源清单的查找索引。
func buildSummarySourceIndex(sources []SummarySource) map[string]bool {
	index := make(map[string]bool, len(sources))
	for _, s := range sources {
		index[summarySourceRefKey(SummarySourceRef{SourceType: s.SourceType, ObjectID: s.ObjectID, Version: s.Version})] = true
		// 版本为空时也允许按类型 + 对象匹配（来源清单可能未逐项带版本）。
		index[summarySourceRefKey(SummarySourceRef{SourceType: s.SourceType, ObjectID: s.ObjectID})] = true
	}
	return index
}

func summarySourceRefKey(ref SummarySourceRef) string {
	return ref.SourceType + "|" + ref.ObjectID + "|" + ref.Version
}

// summaryCoverageCovers 报告覆盖说明是否包含给定来源组。
func summaryCoverageCovers(coverage []SummaryCoverage, ref SummarySourceRef) bool {
	for _, cov := range coverage {
		if cov.SourceType == ref.SourceType && cov.SourceID == ref.ObjectID {
			return true
		}
	}
	return false
}

// containsBusinessAction 检查文本是否夹带业务动作（call_tools / request_input /
// final_answer 或工具调用指令），摘要不得包含这些（文档 4.2、5.10.2）。
func containsBusinessAction(text string) bool {
	lower := strings.ToLower(text)
	for _, marker := range []string{"call_tools", "request_input", "final_answer", "<｜DSML", "</｜DSML"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}
