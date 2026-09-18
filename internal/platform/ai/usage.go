package ai

// SettlementStatus 是调用用量的结算状态。
type SettlementStatus string

const (
	SettlementUnsettled SettlementStatus = "unsettled" // 未结算
	SettlementSettled   SettlementStatus = "settled"   // 已结算
	SettlementUnknown   SettlementStatus = "unknown"   // 缺少用量或远端完成事实，未知
)

// Usage 是按 Provider 已知语义归一化后的用量。文档 11.4、4.2：
// 真实调用记录输入、输出、缓存命中与未命中 Token、图片输入数量及供应商实际计价单位，
// 关联价格版本、币种、用量来源和结算状态；供应商未提供的细分不伪造，未知不记零。
type Usage struct {
	InputTokens         int64
	OutputTokens        int64
	TotalTokens         int64
	CachedInputTokens   *int64 // 缓存命中 Token（nil = 未报告/未知）
	UncachedInputTokens *int64 // 缓存未命中 Token（nil = 未报告/未知）
	ReasoningTokens     *int64 // 推理 Token（推理模型；nil = 未报告）
	ImageCount          *int   // 图片输入数量（nil = 未报告）
	PriceVersion        string // 价格版本（空 = 未知）
	Currency            string // 币种（空 = 未知）
	Source              string // 用量来源
	SettlementStatus    SettlementStatus
	Complete            bool // 是否完整报告用量；false 表示缺失，不得按零费用处理
}

// Normalize 补齐未显式设置的派生字段与结算状态，返回规范化后的用量。
// 语义：未报告总 Token 时由输入+输出推导；未报告价格版本、币种、来源或结算状态时保持未知，
// 不把缺失用量记为零。
func (u Usage) Normalize() Usage {
	if u.TotalTokens == 0 {
		u.TotalTokens = u.InputTokens + u.OutputTokens
	}
	if u.SettlementStatus == "" {
		u.SettlementStatus = SettlementUnsettled
	}
	if !u.Complete {
		u.SettlementStatus = SettlementUnknown
	}
	return u
}

// Known 报告是否存在任何可结算的用量证据（至少报告了 Token 或图片数量）。
func (u Usage) Known() bool {
	return u.Complete && (u.TotalTokens > 0 || u.ImageCount != nil)
}
