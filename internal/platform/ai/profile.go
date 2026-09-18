package ai

// CapabilityDimension 是 Provider 能力的维度。文档 4：主模型默认同时支持 text_input、image_input、
// tool_calling、streaming_text、streaming_tool_call；这些名称不是一条互斥状态链，维度拆分见 4.4。
type CapabilityDimension string

const (
	CapabilityTextInput          CapabilityDimension = "text_input"
	CapabilityImageInput         CapabilityDimension = "image_input"
	CapabilityToolCalling        CapabilityDimension = "tool_calling"
	CapabilityStreamingText      CapabilityDimension = "streaming_text"
	CapabilityStreamingToolCall  CapabilityDimension = "streaming_tool_call"
	CapabilityStructuredOutput   CapabilityDimension = "structured_output"
	CapabilityCancellation       CapabilityDimension = "cancellation"
	CapabilityUsageNormalization CapabilityDimension = "usage_normalization"
)

// CapabilityState 是单个维度的状态。文档 4.4：区分未配置、禁用与配置错误，不将网络故障报成不支持。
type CapabilityState string

const (
	CapabilityConfigured    CapabilityState = "configured"    // 配置可用（凭据是否可用）
	CapabilitySupported     CapabilityState = "supported"     // 官方契约依据与适配器支持范围
	CapabilityVerified      CapabilityState = "verified"      // 对具体配置组合已验证
	CapabilityAvailable     CapabilityState = "available"     // 当前健康检查与调用结果可用
	CapabilityDegraded      CapabilityState = "degraded"      // 暂时限流或故障
	CapabilityUnsupported   CapabilityState = "unsupported"   // 不支持
	CapabilityMisconfigured CapabilityState = "misconfigured" // 配置错误
)

// Capabilities 是各能力维度的状态快照。文档 4.4：单个能力存在不证明图片、工具与结构化输出
// 可在同一请求组合使用；组合能力须另行验证。
type Capabilities struct {
	TextInput          CapabilityState
	ImageInput         CapabilityState
	ToolCalling        CapabilityState
	StreamingText      CapabilityState
	StreamingToolCall  CapabilityState
	StructuredOutput   CapabilityState
	Cancellation       CapabilityState
	UsageNormalization CapabilityState
}

// State 返回指定维度的状态。
func (c Capabilities) State(dimension CapabilityDimension) CapabilityState {
	switch dimension {
	case CapabilityTextInput:
		return c.TextInput
	case CapabilityImageInput:
		return c.ImageInput
	case CapabilityToolCalling:
		return c.ToolCalling
	case CapabilityStreamingText:
		return c.StreamingText
	case CapabilityStreamingToolCall:
		return c.StreamingToolCall
	case CapabilityStructuredOutput:
		return c.StructuredOutput
	case CapabilityCancellation:
		return c.Cancellation
	case CapabilityUsageNormalization:
		return c.UsageNormalization
	default:
		return ""
	}
}

// Supports 报告该维度当前是否可用（available 或 verified），不含 degraded / unsupported / misconfigured。
func (c Capabilities) Supports(dimension CapabilityDimension) bool {
	switch c.State(dimension) {
	case CapabilityAvailable, CapabilityVerified:
		return true
	default:
		return false
	}
}

// Parameter 是 Profile 中的一个采样参数。文档 4.4：每个参数记录配置值、必需或可选、适用能力条件；
// 必需参数不支持或值非法时阻止调用，可选不代表可以任意改值。
type Parameter struct {
	Name      string
	Value     any
	Required  bool
	AppliesTo []CapabilityDimension // 适用能力条件（空表示无条件）
}

// ParameterOmission 记录一次省略。文档 4.4：省略记录包含参数名、配置值、原因、能力证据版本及实际发送集合，
// 关联本次 Attempt 与输入快照。
type ParameterOmission struct {
	Name       string
	Configured any
	Reason     string
	Evidence   string   // 能力证据版本
	Sent       []string // 实际发送的参数集合
}

// Profile 是一次 Run 固定的服务端配置。文档 4.1、4.4：模型、温度、最大输出、超时、预算和重试策略
// 由开发者维护的服务端 Profile 控制，用户不可调节；Run 固定 Provider/Model 及可用的 Profile 版本集合。
type Profile struct {
	ProviderID     string
	Model          string // 请求模型标识
	Version        string
	AdapterVersion string
	Capabilities   Capabilities
	Parameters     []Parameter
}

// OmitOptional 按能力快照决定是否可省略某个可选参数。文档 4.4：模型不支持可选参数时允许省略，
// 并记录实际生效配置；必需参数不兼容仍阻止调用。
// 返回省略记录与是否允许；可选参数在对应维度不可用时允许省略，必需参数永不允许省略。
func (p Profile) OmitOptional(parameter Parameter, reason string, evidence string, sent []string) (ParameterOmission, bool) {
	if parameter.Required {
		return ParameterOmission{}, false
	}
	return ParameterOmission{
		Name:       parameter.Name,
		Configured: parameter.Value,
		Reason:     reason,
		Evidence:   evidence,
		Sent:       sent,
	}, true
}
