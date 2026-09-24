package ai

import (
	"context"
	"time"
)

// Purpose 表示一次模型调用的用途，由 Runtime 固定并据此选择输入、工具与响应约束。
// 文档 4.1、9.1：未知用途或用途与 Schema 不匹配时不发送，不能根据返回内容临时改用途。
type Purpose string

const (
	// PurposeAgentStep 业务决策：可提出 2.2 的三类业务动作（call_tools / request_input / final_answer）。
	PurposeAgentStep Purpose = "agent_step"
	// PurposeContextSummary 上下文摘要：仅返回内部摘要候选，无业务工具、无用户正文发布权。
	PurposeContextSummary Purpose = "context_summary"
	// PurposeMemoryDerivation 健康记录记忆派生：独立任务身份与来源快照，不伪造 Run。
	PurposeMemoryDerivation Purpose = "memory_derivation"
)

// Valid 报告 purpose 是否为已注册的一期用途。
func (p Purpose) Valid() bool {
	switch p {
	case PurposeAgentStep, PurposeContextSummary, PurposeMemoryDerivation:
		return true
	default:
		return false
	}
}

// FinishReason 是归一化后的模型结束原因，不直接沿用 Provider 的原始字符串驱动 Run。
// 文档 4.3：至少区分正常结束、工具调用、输出截断、拒绝和未识别原因；未知原因不能默认按正常完成。
type FinishReason string

const (
	FinishStop      FinishReason = "stop"       // 正常结束
	FinishToolCalls FinishReason = "tool_calls" // 工具调用结束（本次输出结束，后续工具与答复仍属当前 Run）
	FinishLength    FinishReason = "length"     // 输出长度上限终止（截断）
	FinishRefusal   FinishReason = "refusal"    // 拒绝或能力不足
	FinishUnknown   FinishReason = "unknown"    // 未识别原因
)

// Message 是一段有序模型消息。内容块组合规则（TextBlock / ImageBlock / ToolCallBlock）见 5.2、T5，
// 契约层只承载角色与文本正文，图片与工具通过 Request 的一等字段表达，避免在 T3 越界定稿内容块 Schema。
type Message struct {
	Role    string // system / user / assistant / tool
	Content string // 文本正文
}

// ImageRef 引用已上传并校验的不可变资产。文档 5.2：不把临时访问链接作为图片身份。
type ImageRef struct {
	AssetID     string // asset_id 引用
	Version     string // 不可变内容版本
	Description string // 用户描述（可选）
}

// ToolSpec 描述一个候选工具。文档 4.1：不发送越权或目录外工具；模型仍可能提出非法调用，执行前按第 7 节再校验。
type ToolSpec struct {
	Name        string
	Version     string
	Description string
	Parameters  any // 参数 Schema
}

// Request 是统一模型请求。调用方是 Runtime，输入是已经过权限和上下文筛选的本次模型输入；
// Provider 负责请求转换、发送、取消及响应归一化，不自行检索旧聊天、读取业务数据、执行工具、修改任务项、切模型或增加重试。
// 字段对齐文档 4.1 的输入组。
type Request struct {
	// 调用身份
	SchemaVersion string // request_schema_version
	AttemptID     string // attempt_id，同一 Attempt 至多发出一次请求
	// 调用用途
	Purpose Purpose
	// 固定配置
	ProviderID     string       // provider_id
	Model          string       // 请求模型标识
	ProfileVersion string       // Profile 版本
	AdapterVersion string       // 适配器版本
	Capabilities   Capabilities // 能力快照引用
	// 模型输入
	Messages     []Message  // 有序消息
	Images       []ImageRef // 图片引用（身份/审计引用）
	ImageContent [][]byte   // 与 Images 一一对应的受控版本字节（Runtime 发送前生成，文档 4.1/11.5）
	Tools        []ToolSpec // 候选工具（无工具时传空集合）
	// 响应约束（按用途固定：agent_step / context_summary / memory_derivation 分别绑定对应 Schema）
	ResponseSchema any
	// 响应协议标识（如 record_array_v1）。Provider 据此决定结构化输出模式与工具传递方式。
	ResponseProtocol string
	// 参数配置
	Parameters []Parameter
	// 运行限制
	MaxOutputTokens int           // 本次输出上限
	Timeout         time.Duration // 超时
	Deadline        time.Time     // 调用截止时间（now >= deadline 时不得再启动）
}

// ToolCall 是完整归一化后的工具调用。文档 4.2：收齐名称与参数后才形成候选集合，标识冲突、参数解析失败或动作冲突时不执行。
type ToolCall struct {
	ID        string // 映射到本次 Attempt 内的稳定调用标识
	Name      string
	Arguments string // 完整参数（JSON）
}

// Response 是统一模型响应。文档 4.2：Provider 未报告实际模型版本或用量时分别记未知，
// 不能拿请求模型别名冒充实际版本，也不能把缺失用量记为零。
type Response struct {
	AttemptID         string       // attempt_id
	ProviderRequestID string       // Provider 请求标识
	Model             string       // 实际报告的模型版本
	Text              string       // 文本内容
	ToolCalls         []ToolCall   // 完整工具调用集合
	FinishReason      FinishReason // 归一化结束原因
	Usage             Usage        // 用量（未报告时 Complete=false，未知不记零）
	ParameterOmits    []ParameterOmission
}

// StreamEventType 是内部流事件类型。文档 4.2：内部流序号不同于面向页面的 Session 事件序号。
type StreamEventType string

const (
	StreamResponseStarted  StreamEventType = "response_started"
	StreamContentDelta     StreamEventType = "content_delta"
	StreamToolCallDelta    StreamEventType = "tool_call_delta"
	StreamUsageReported    StreamEventType = "usage_reported"
	StreamResponseFinished StreamEventType = "response_finished"
	StreamResponseError    StreamEventType = "response_error"
)

// ToolCallDelta 是工具调用参数分片。文档 4.2：半截流式参数不能成为可执行调用。
type ToolCallDelta struct {
	ID               string
	Name             string
	ArgumentsPartial string
}

// StreamEvent 定位 Attempt 与内容块或调用标识，并带本地顺序。
type StreamEvent struct {
	Type          StreamEventType
	AttemptID     string
	BlockID       string // 内容块或调用标识
	Sequence      int    // 本地顺序
	ContentDelta  string
	ToolCallDelta ToolCallDelta
	Usage         *Usage
	Error         error
}

// Provider 是统一 Provider 接口。文档 4：Agent 只依赖统一 Provider 接口，Provider 负责不同 SDK、请求格式与响应格式适配。
// Complete 是非流式调用；Stream 是流式调用，增量通过 emit 回调，返回完整归一化响应。
type Provider interface {
	Complete(ctx context.Context, request Request) (Response, error)
	Stream(ctx context.Context, request Request, emit func(StreamEvent) error) (Response, error)
}

// 稳定错误分类。文档 9：至少包括超时、取消、限流、额度耗尽、网络不可用、鉴权失败、请求非法、
// 上下文超限、结构化输出非法和未知错误；额度耗尽、鉴权失败和能力不支持不可盲目重试。
const (
	ErrProviderTimeout               = "provider_timeout"
	ErrProviderCanceled              = "provider_canceled"
	ErrProviderRateLimited           = "provider_rate_limited"
	ErrProviderQuotaExhausted        = "provider_quota_exhausted"
	ErrProviderUnavailable           = "provider_unavailable"
	ErrProviderAuthFailed            = "provider_auth_failed"
	ErrProviderRequestInvalid        = "provider_request_invalid"
	ErrProviderContextTooLong        = "provider_context_too_long"
	ErrProviderOutputInvalid         = "provider_output_invalid"
	ErrProviderCapabilityUnsupported = "provider_capability_unsupported"
	ErrProviderFailed                = "provider_failed"
)

// BlindRetryForbidden 报告该错误类别是否禁止盲目重试。
// 文档 9、4.3：鉴权、必需能力、非法配置及额度错误不盲目重试；只有允许恢复的故障才使用剩余预算。
func BlindRetryForbidden(code string) bool {
	switch code {
	case ErrProviderQuotaExhausted, ErrProviderAuthFailed, ErrProviderRequestInvalid, ErrProviderCapabilityUnsupported:
		return true
	default:
		return false
	}
}
