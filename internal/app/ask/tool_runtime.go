package ask

import (
	"encoding/json"
	"time"
)

// 本文件固定工具 Runtime 的调用身份、执行状态与结果封装契约
// （对应架构设计 v2 文档 7.1、7.3、7.6）。只承载类型与状态机，不承载
// HTTP 调度与真实模型交互（在 T5 接入）。

// ===== 7.1 调用身份、输入与权限 =====

// ToolCall：模型提出的一项工具调用。一次 call_tools 响应形成一个不可变
// 调用批次；tool_call_id 表示模型提出的一项调用，恢复不能重复分配。
type ToolCall struct {
	ToolCallID  string          // 模型提出的一项调用标识
	CallIndex   int             // 批次内调用顺序（由完整模型响应固定）
	AttemptID   string          // 提出调用的模型 Attempt
	ToolName    string          // 目录工具名
	ToolVersion string          // 目录/Schema 版本
	Arguments   json.RawMessage // 完整 arguments，服务端最终校验后才生效
	DependsOn   []string        // 依赖的前置 tool_call_id（真实数据表达依赖）
}

// ToolExecutionStatus：一次实际执行尝试的生命周期（7.6），终态不回改；
// 需要重试时创建新的执行尝试，保留原调用及重试关联。
type ToolExecutionStatus string

const (
	ToolExecQueued    ToolExecutionStatus = "queued"
	ToolExecRunning   ToolExecutionStatus = "running"
	ToolExecBlocked   ToolExecutionStatus = "blocked"
	ToolExecSucceeded ToolExecutionStatus = "succeeded"
	ToolExecFailed    ToolExecutionStatus = "failed"
	ToolExecCanceled  ToolExecutionStatus = "canceled"
	ToolExecUnknown   ToolExecutionStatus = "unknown"
)

// IsTerminal 判断是否终态。
func (s ToolExecutionStatus) IsTerminal() bool {
	switch s {
	case ToolExecSucceeded, ToolExecFailed, ToolExecCanceled, ToolExecUnknown, ToolExecBlocked:
		return true
	default:
		return false
	}
}

// CanTransition 判断是否允许状态转移。终态不再进入运行状态。
func (s ToolExecutionStatus) CanTransition(to ToolExecutionStatus) bool {
	switch s {
	case ToolExecQueued:
		return to == ToolExecRunning || to == ToolExecBlocked || to == ToolExecCanceled
	case ToolExecRunning:
		return to == ToolExecSucceeded || to == ToolExecFailed || to == ToolExecCanceled || to == ToolExecUnknown
	default:
		return false
	}
}

// ToolExecution：Runtime 对一次调用的一次实际执行尝试。工具重试创建新的
// 执行标识，不创建模型 Attempt。
type ToolExecution struct {
	ID             string
	RunID          string
	ToolCallID     string
	InputRevision  int
	ExecutionEpoch int
	Status         ToolExecutionStatus
}

// ===== 7.3 工具结果封装与可用性 =====

// ToolResultStatus：单项工具结果分类。单项结果不决定 Run 成功，
// 未知不当作无数据或执行成功。
type ToolResultStatus string

const (
	ToolResultOK          ToolResultStatus = "ok"
	ToolResultError       ToolResultStatus = "error"
	ToolResultNotExecuted ToolResultStatus = "not_executed"
	ToolResultCanceled    ToolResultStatus = "canceled"
	ToolResultUnknown     ToolResultStatus = "unknown"
)

// Completeness：相对于该工具声明的查询范围的覆盖完整性。
// 执行错误通常不具备数据完整性结论。
type Completeness string

const (
	CompletenessComplete      Completeness = "complete"
	CompletenessPartial       Completeness = "partial"
	CompletenessNotApplicable Completeness = "not_applicable"
)

// ToolErrorCategory：稳定错误类别（7.3 错误字段组），供 Runtime 决定是否重试。
type ToolErrorCategory string

const (
	ToolErrInvalidTool     ToolErrorCategory = "invalid_tool"
	ToolErrInvalidArgument ToolErrorCategory = "invalid_argument"
	ToolErrUnauthorized    ToolErrorCategory = "unauthorized"
	ToolErrPrecondition    ToolErrorCategory = "precondition_unmet"
	ToolErrTimeout         ToolErrorCategory = "timeout"
	ToolErrServiceError    ToolErrorCategory = "service_error"
	ToolErrOutputInvalid   ToolErrorCategory = "output_invalid"
)

// ToolError：稳定的可公开错误信息。模型不能只凭 retryable 直接循环重试。
type ToolError struct {
	Category  ToolErrorCategory
	Stage     string // 发生阶段（validate / dispatch / execute / verify）
	Reason    string // 允许公开的原因
	Retryable bool   // 是否具备恢复条件
}

// ToolResult：统一工具结果。空数组只是一种数据值，不能同时表达无记录、
// 未执行和故障。模型只能读取筛选后的封装，不能自行改状态。
type ToolResult struct {
	ToolCallID   string
	ExecutionID  string // 实际执行标识；空表示明确无执行
	CallIndex    int
	Status       ToolResultStatus
	Completeness Completeness
	Data         any        // 通过对应输出 Schema 的业务数据
	Source       ReadSource // 来源与时效
	Error        *ToolError
	HasMore      bool
	NextCursor   string
	ResultHandle string // 受控结果句柄（补取已有大结果用）
	QueuedAt     time.Time
	StartedAt    time.Time
	CompletedAt  time.Time
	Reused       bool // 是否复用既有结果（关联原执行，不伪造新执行）
}

// ===== 7.4 整批调用 =====

// ToolBatch：一次 call_tools 响应形成的不可变调用批次。批次是步骤内的
// 关联范围，不新增可独立调度的 Run。
type ToolBatch struct {
	BatchID   string
	RunID     string
	AttemptID string
	Calls     []ToolCall
}
