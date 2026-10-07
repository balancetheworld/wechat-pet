package knowledge

import (
	"fmt"
	"strings"
	"time"
)

// 本文件固定知识条目的领域契约（对应 RAG 设计文档第四节「知识库数据结构」）。
// 知识库是「已审核、可版本化、可撤回」的背景参考资料，只影响回答内容，
// 不影响权限、写入确认、风险升级条件与工具调用权限。

// Status 是知识条目的发布状态。只有 published 参与检索；
// draft 未审核、withdrawn 已撤回，均不得进入 Run。
type Status string

const (
	StatusDraft     Status = "draft"
	StatusPublished Status = "published"
	StatusWithdrawn Status = "withdrawn"
)

func (s Status) valid() bool {
	switch s {
	case StatusDraft, StatusPublished, StatusWithdrawn:
		return true
	default:
		return false
	}
}

// Species 是知识条目适用的物种；all 表示不限物种。
type Species string

const (
	SpeciesAll Species = "all"
	SpeciesCat Species = "cat"
	SpeciesDog Species = "dog"
)

func (s Species) valid() bool {
	switch s {
	case SpeciesAll, SpeciesCat, SpeciesDog:
		return true
	default:
		return false
	}
}

// LifeStage 是知识条目适用的生命周期；all 表示不限阶段。
type LifeStage string

const (
	LifeStageAll    LifeStage = "all"
	LifeStageYoung  LifeStage = "young"  // 幼猫期 / 幼犬期
	LifeStageAdult  LifeStage = "adult"  // 青年期
	LifeStageMature LifeStage = "mature" // 熟龄期（猫）/ 壮年期（狗）
	LifeStageSenior LifeStage = "senior" // 老年期
)

func (l LifeStage) valid() bool {
	switch l {
	case LifeStageAll, LifeStageYoung, LifeStageAdult, LifeStageMature, LifeStageSenior:
		return true
	default:
		return false
	}
}

// RiskLevel 是知识条目自身的风险档位，取值与 ask 层 green/yellow/red 对齐。
// 它描述该条目适合回答的风险场景，不替代 Runtime 的确定性风险规则。
type RiskLevel string

const (
	RiskGreen  RiskLevel = "green"
	RiskYellow RiskLevel = "yellow"
	RiskRed    RiskLevel = "red"
)

func (r RiskLevel) valid() bool {
	switch r {
	case RiskGreen, RiskYellow, RiskRed:
		return true
	default:
		return false
	}
}

// SourceTypeReviewedKnowledge 是参考块中固定的来源类型（设计文档第八节）。
// 它是服务端写死的标识，不取自媒体原文，避免知识正文伪造来源身份。
const SourceTypeReviewedKnowledge = "reviewed_knowledge"

// Chunk 是一条可检索的知识条目。一条 chunk 只解决一个主要问题；
// 红旗症状与就医条件必须能被单独识别，不能只埋在正文里。
type Chunk struct {
	ReferenceID string // 引用标识，同时也是最终回答的 evidence source_id
	DocumentID  string
	ChunkID     string
	Title       string
	Content     string
	Species     Species
	LifeStage   []LifeStage
	Topics      []string
	Keywords    []string
	Symptoms    []string
	RiskLevel   RiskLevel
	TriageLevel string
	SourceName  string
	SourceDate  time.Time
	ReviewedAt  time.Time
	ExpiresAt   time.Time
	Version     string
	Status      Status

	Supports             []string
	DoesNotSupport       []string
	ObservationItems     []string
	EscalationConditions []string
	Contraindications    []string
	RelatedTopics        []string
	// ChunkKey 是条目文件级的稳定后缀（来自文件名），用于生成不随条目增减变化的
	// reference_id；为空时按文档内顺序编号。
	ChunkKey string
}

// Validate 校验条目的必需字段与枚举；不合法时返回可定位错误，不静默补猜。
func (c Chunk) Validate() error {
	if c.ReferenceID == "" {
		return fmt.Errorf("knowledge: chunk %q has empty reference_id", c.ChunkID)
	}
	if c.DocumentID == "" {
		return fmt.Errorf("knowledge: chunk %q has empty document_id", c.ReferenceID)
	}
	if c.ChunkID == "" {
		return fmt.Errorf("knowledge: chunk %q has empty chunk_id", c.ReferenceID)
	}
	if strings.TrimSpace(c.Title) == "" {
		return fmt.Errorf("knowledge: chunk %q has empty title", c.ReferenceID)
	}
	if strings.TrimSpace(c.Content) == "" {
		return fmt.Errorf("knowledge: chunk %q has empty content", c.ReferenceID)
	}
	if !c.Species.valid() {
		return fmt.Errorf("knowledge: chunk %q has unknown species %q", c.ReferenceID, c.Species)
	}
	for _, stage := range c.LifeStage {
		if !stage.valid() {
			return fmt.Errorf("knowledge: chunk %q has unknown life_stage %q", c.ReferenceID, stage)
		}
	}
	if !c.RiskLevel.valid() {
		return fmt.Errorf("knowledge: chunk %q has unknown risk_level %q", c.ReferenceID, c.RiskLevel)
	}
	if !c.Status.valid() {
		return fmt.Errorf("knowledge: chunk %q has unknown status %q", c.ReferenceID, c.Status)
	}
	if c.Version == "" {
		return fmt.Errorf("knowledge: chunk %q has empty version", c.ReferenceID)
	}
	return nil
}

// HasEscalationConditions 报告条目是否携带红旗症状或就医条件。
// Runtime 据此判断高风险提问是否取到了能支撑升级提示的知识条目。
func (c Chunk) HasEscalationConditions() bool {
	for _, condition := range c.EscalationConditions {
		if strings.TrimSpace(condition) != "" {
			return true
		}
	}
	return false
}

// Expired 报告条目在给定时间是否已过期。零值 expires_at 表示不设过期时间。
func (c Chunk) Expired(now time.Time) bool {
	if c.ExpiresAt.IsZero() {
		return false
	}
	return now.After(c.ExpiresAt)
}

// sortUnique 去空、去重并保持首次出现顺序，供检索字段与元数据归一化使用。
func sortUnique(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
