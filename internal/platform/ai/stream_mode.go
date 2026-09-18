package ai

// StreamMode 是模型响应的接收模式。文档 8.7：非流式只改变本次模型响应的接收及正文发布时间，
// 不改变 Provider 切换、预算、写入确认及回答版本规则。
type StreamMode string

const (
	StreamModeStreaming    StreamMode = "streaming"
	StreamModeNonStreaming StreamMode = "non_streaming"
)

// SelectMode 在发送前选择接收模式。文档 8.7：发送前已知所需组合仅支持非流式时直接选择该模式，
// 没有先发流式就不消耗一次重试；没有任何可用组合则报告能力不足。
func SelectMode(nonStreamOnly bool, streamingVerified bool) StreamMode {
	if nonStreamOnly || !streamingVerified {
		return StreamModeNonStreaming
	}
	return StreamModeStreaming
}

// DegradeDecision 是流式故障后的降级裁决。
type DegradeDecision struct {
	Allow  bool
	Reason string
}

// CanDegradeToNonStreaming 判定流式故障后是否允许自动降级为非流式。
// 文档 8.7：已有当前回答正文后不自动降级重发；输入已变、停止、权限撤销、鉴权失败、额度不足
// 或必要能力不支持时，不以降级作为重试理由。
//
// bodyPublished 按持久化发布记录判断（不按某台客户端是否实际看到判断）；
// recoverable 表示故障是否属于可恢复错误分类（见 BlindRetryForbidden）；
// remainingBudget 表示剩余自动重试预算是否允许（Run 累计）。
func CanDegradeToNonStreaming(bodyPublished bool, recoverable bool, remainingBudget bool) DegradeDecision {
	if bodyPublished {
		return DegradeDecision{Allow: false, Reason: "body_already_published"}
	}
	if !recoverable {
		return DegradeDecision{Allow: false, Reason: "not_recoverable"}
	}
	if !remainingBudget {
		return DegradeDecision{Allow: false, Reason: "no_remaining_budget"}
	}
	return DegradeDecision{Allow: true, Reason: ""}
}
