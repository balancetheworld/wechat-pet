package ask

import (
	"encoding/json"
	"fmt"
	"sort"
)

// ResourceType：工具涉及的资源类型。一期固定五种，知识检索在 RAG 阶段新增
// knowledge，表示只读背景知识，不属于任何家庭的业务数据。
type ResourceType string

const (
	ResourcePet           ResourceType = "pet"
	ResourcePetProfile    ResourceType = "pet_profile"
	ResourceHealthRecord  ResourceType = "health_record"
	ResourceHealthSummary ResourceType = "health_summary"
	ResourceConversation  ResourceType = "conversation"
	ResourceKnowledge     ResourceType = "knowledge"
)

// ActionType：工具动作类型。一期固定七种。
type ActionType string

const (
	ActionResolve       ActionType = "resolve"
	ActionRead          ActionType = "read"
	ActionSearch        ActionType = "search"
	ActionAggregate     ActionType = "aggregate"
	ActionPrepareCreate ActionType = "prepare_create"
	ActionPrepareUpdate ActionType = "prepare_update"
	ActionVerify        ActionType = "verify"
)

// ToolRiskLevel：工具风险等级，用于候选召回前的风险过滤。
// 与 model.go 的 RiskLevel（Session/Run 健康风险 green/yellow/red）语义不同，
// 故独立命名，取值 low/medium/high。
type ToolRiskLevel string

const (
	ToolRiskLow    ToolRiskLevel = "low"
	ToolRiskMedium ToolRiskLevel = "medium"
	ToolRiskHigh   ToolRiskLevel = "high"
)

// IdempotencyPolicy：工具声明的幂等策略（字符串标识，由 Runtime 裁决）。
type IdempotencyPolicy string

const (
	IdempotencyDeterministic IdempotencyPolicy = "deterministic" // 同参数结果确定，可安全重复/复用
	IdempotencyReadOnly      IdempotencyPolicy = "read_only"     // 只读，重复调用无业务副作用
	IdempotencyNewIdentity   IdempotencyPolicy = "new_identity"  // 新写入必须新身份，禁止按参数复用
)

// VerificationPolicy：工具声明的来源校验方式（字符串标识，由 Runtime 裁决）。
type VerificationPolicy string

const (
	VerificationSourceVersion VerificationPolicy = "source_version" // 依赖来源集合版本核验
	VerificationRecordVersion VerificationPolicy = "record_version" // 依赖单条记录版本核验
	VerificationNone          VerificationPolicy = "none"           // 无持久化来源，仅实时查询
)

var validResourceTypes = map[ResourceType]struct{}{
	ResourcePet:           {},
	ResourcePetProfile:    {},
	ResourceHealthRecord:  {},
	ResourceHealthSummary: {},
	ResourceConversation:  {},
	ResourceKnowledge:     {},
}

var validActionTypes = map[ActionType]struct{}{
	ActionResolve:       {},
	ActionRead:          {},
	ActionSearch:        {},
	ActionAggregate:     {},
	ActionPrepareCreate: {},
	ActionPrepareUpdate: {},
	ActionVerify:        {},
}

// ValidResourceType 判断资源类型是否在一期枚举内。
func ValidResourceType(value ResourceType) bool {
	_, ok := validResourceTypes[value]
	return ok
}

// ValidActionType 判断动作类型是否在一期枚举内。
func ValidActionType(value ActionType) bool {
	_, ok := validActionTypes[value]
	return ok
}

// IsReadOnly 依据目录声明的 action_type 判断是否只读。
// resolve/read/search/aggregate 为只读；prepare_create/prepare_update 会形成
// 写入准备，非只读；verify 只有确实只查询真实操作状态时才按只读处理，由
// Runtime 依据实际参数裁决，此处不静态判定为只读。
func IsReadOnly(action ActionType) bool {
	switch action {
	case ActionResolve, ActionRead, ActionSearch, ActionAggregate:
		return true
	case ActionPrepareCreate, ActionPrepareUpdate:
		return false
	case ActionVerify:
		return false
	default:
		return false
	}
}

// Tool：结构化能力记录（不是裸函数）。字段对应 v2 第 6 节的工具目录契约。
type Tool struct {
	Name                 string             `json:"tool_name"`
	OperationID          string             `json:"operation_id"`
	AliasesZH            []string           `json:"aliases_zh"`
	AliasesEN            []string           `json:"aliases_en"`
	ResourceType         ResourceType       `json:"resource_type"`
	ActionType           ActionType         `json:"action_type"`
	Parameters           json.RawMessage    `json:"parameters"`
	OutputSchema         json.RawMessage    `json:"output_schema"`
	UseCases             []string           `json:"use_cases"`
	NegativeCases        []string           `json:"negative_cases"`
	Preconditions        []string           `json:"preconditions"`
	SideEffects          []string           `json:"side_effects"`
	RiskLevel            ToolRiskLevel      `json:"risk_level"`
	RequiresConfirmation bool               `json:"requires_confirmation"`
	IdempotencyPolicy    IdempotencyPolicy  `json:"idempotency_policy"`
	VerificationPolicy   VerificationPolicy `json:"verification_policy"`
	Version              string             `json:"version"`
}

// Validate 校验工具的目录字段；不合法时返回可定位错误，不静默补猜。
func (t Tool) Validate() error {
	if t.Name == "" {
		return fmt.Errorf("tool %q: empty tool_name", t.Name)
	}
	if t.OperationID == "" {
		return fmt.Errorf("tool %q: empty operation_id", t.Name)
	}
	if !ValidResourceType(t.ResourceType) {
		return fmt.Errorf("tool %q: unknown resource_type %q", t.Name, t.ResourceType)
	}
	if !ValidActionType(t.ActionType) {
		return fmt.Errorf("tool %q: unknown action_type %q", t.Name, t.ActionType)
	}
	if t.Version == "" {
		return fmt.Errorf("tool %q: empty version", t.Name)
	}
	return nil
}

// ParameterNames 从 parameters 的 JSON Schema 中提取顶层属性名，供 BM25F 检索
// 的「参数名」字段使用。parameters 为空或非法时返回空切片，不影响其余字段检索。
func (t Tool) ParameterNames() []string {
	if len(t.Parameters) == 0 {
		return nil
	}
	var schema struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(t.Parameters, &schema); err != nil {
		return nil
	}
	names := make([]string, 0, len(schema.Properties))
	for name := range schema.Properties {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
