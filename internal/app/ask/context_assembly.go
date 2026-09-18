package ask

import (
	"fmt"
	"unicode/utf8"
)

// 本文件固定上下文组装顺序、去重与 Token 裁剪（文档 5.8、5.9）。
// 组装输出为不可变调用快照，以及每项内容被选择、替代、裁剪或拒绝的原因。

// ContextLayer 是上下文组装的逻辑层（文档 5.8 表格）。
// 逻辑分层不要求每层各生成一条消息，适配器再转换为所选协议可接受的消息。
type ContextLayer int

const (
	LayerControlInstructions ContextLayer = 1 // 受控指令：核心安全、权限边界、输出协议、已审核 Skill
	LayerReferenceData       ContextLayer = 2 // 执行与参考数据：任务项、未完成状态、问题、档案、记录、摘要
	LayerHistory             ContextLayer = 3 // 选定的历史对话：当前 Session 原文 + 授权旧聊天片段
	LayerCurrentTask         ContextLayer = 4 // 当前任务对话：原始请求、补充、纠正、追问回复
	LayerToolInteractions    ContextLayer = 5 // 当前步骤工具交互：调用与结果成组配对
)

// ContextBlock 是上下文组装中的一个内容块（文档 5.8）。
// 每块携带来源身份，用于去重与裁剪；相同文本但不同对象、时间或版本不得合并。
type ContextBlock struct {
	Layer    ContextLayer
	Kind     string // instruction / task / question / profile / record / summary / history / current_turn / tool_call / tool_result / referenced_chat
	ObjectID string // 来源对象标识（pet_id / turn_id / message_id / 记录 ID / 摘要 ID）
	Version  string // 内容版本
	Position string // 实际片段位置
	Text     string
	Required bool // 必须保留：核心规则、当前输入、急症触发信息、必要输出约束
}

// ContextPriority 是 5.9 已确认的裁剪优先级。数值越小越必须保留；
// 裁剪从数值大的（低优先级）开始。
const (
	ContextPriorityCritical   = 0 // 当前输入、急症触发信息、核心规则、必要输出约束
	ContextPriorityRunState   = 1 // 当前 Run 未完成状态、已确认工具结果、必要档案/记录
	ContextPriorityRecent     = 2 // 当前 Session 最近消息
	ContextPriorityReferenced = 3 // 用户明确引用的旧聊天片段
	ContextPrioritySummary    = 4 // 摘要（最低）
)

// blockPriority 依据 Kind 返回内容块的裁剪优先级。
func blockPriority(block ContextBlock) int {
	switch block.Kind {
	case "instruction", "current_turn", "emergency":
		return ContextPriorityCritical
	case "task", "question", "profile", "record", "tool_result":
		return ContextPriorityRunState
	case "history", "recent_message":
		return ContextPriorityRecent
	case "referenced_chat":
		return ContextPriorityReferenced
	case "summary":
		return ContextPrioritySummary
	default:
		return ContextPriorityRecent
	}
}

// EstimateTokens 保守估算文本 token 数（文档 5.9）。
// 采用「1 字符（rune）= 1 token」的保守上界：中文 1 字约 0.6~1 token，
// 英文 1 词约 1.3 token 但约 4~5 字符/词，用 rune 数不会低估实际用量。
// 预测输入 Token 与 Provider 真实计量分别记录，不把预估当账单。
func EstimateTokens(text string) int {
	return utf8.RuneCountInString(text)
}

// ComputeInputBudget 计算单次输入 Token 预算（文档 5.9）。
// input_budget = min(input_limit, context_limit - output_reserve - margin)。
// 配置缺失或余量为负时返回 0，表示不发请求。
func ComputeInputBudget(inputLimit, contextLimit, outputReserve, margin int) int {
	if inputLimit <= 0 || contextLimit <= 0 {
		return 0
	}
	available := contextLimit - outputReserve - margin
	if available <= 0 {
		return 0
	}
	if inputLimit < available {
		return inputLimit
	}
	return available
}

// ContextAssembly 是一次上下文组装的结果。
type ContextAssembly struct {
	Blocks []ContextBlock // 已按逻辑层与优先级排序、已去重的内容块
	// 每项内容被选择、替代、裁剪或拒绝的原因（审计用途，不含敏感正文）。
	Rejected []ContextRejection
}

// ContextRejection 记录一块内容被裁剪或拒绝的原因（文档 5.8 输出）。
type ContextRejection struct {
	Kind     string
	ObjectID string
	Reason   string
}

// AssembleContext 对内容块做去重与排序（文档 5.8）。
// 去重优先使用来源类型、对象 ID、内容版本和实际片段位置；
// 相同文本但不同宠物、时间或版本不得合并。
func AssembleContext(blocks []ContextBlock) ContextAssembly {
	seen := make(map[string]bool, len(blocks))
	deduped := make([]ContextBlock, 0, len(blocks))
	rejected := make([]ContextRejection, 0)
	for _, b := range blocks {
		key := blockDedupeKey(b)
		if seen[key] {
			rejected = append(rejected, ContextRejection{Kind: b.Kind, ObjectID: b.ObjectID, Reason: "duplicate"})
			continue
		}
		seen[key] = true
		deduped = append(deduped, b)
	}
	// 稳定排序：先按逻辑层，层内按优先级（低数值在前）。
	sortContextBlocks(deduped)
	return ContextAssembly{Blocks: deduped, Rejected: rejected}
}

func blockDedupeKey(b ContextBlock) string {
	return fmt.Sprintf("%d|%s|%s|%s|%s", b.Layer, b.Kind, b.ObjectID, b.Version, b.Position)
}

// sortContextBlocks 稳定排序：先按逻辑层，层内按优先级（低数值优先），保持输入相对顺序。
func sortContextBlocks(blocks []ContextBlock) {
	// 使用简单稳定的插入排序，避免引入 sort 依赖并保证确定性。
	for i := 1; i < len(blocks); i++ {
		cur := blocks[i]
		j := i - 1
		for j >= 0 && blockLess(cur, blocks[j]) {
			blocks[j+1] = blocks[j]
			j--
		}
		blocks[j+1] = cur
	}
}

func blockLess(a, b ContextBlock) bool {
	if a.Layer != b.Layer {
		return a.Layer < b.Layer
	}
	return blockPriority(a) < blockPriority(b)
}

// TrimContext 在给定输入预算内裁剪内容块（文档 5.9）。
// 必须项（Required）与高优先级内容优先保留；超限时先移除低优先级
// （摘要、旧引用），仍超限才压缩当前 Session 历史（由调用方在移除块后
// 用摘要替代）。必须项仍放不下时返回容量错误，不偷偷截断用户输入或删除图片。
func TrimContext(blocks []ContextBlock, inputBudget int) ([]ContextBlock, error) {
	if inputBudget < 0 {
		inputBudget = 0
	}
	required := make([]ContextBlock, 0)
	optional := make([]ContextBlock, 0)
	requiredTokens := 0
	for _, b := range blocks {
		if b.Required {
			required = append(required, b)
			requiredTokens += EstimateTokens(b.Text)
		} else {
			optional = append(optional, b)
		}
	}
	if requiredTokens > inputBudget {
		return nil, fmt.Errorf("context: required blocks (%d tokens) exceed input budget (%d)", requiredTokens, inputBudget)
	}
	// 可选块按优先级从高到低保留（低数值优先）；优先级相同时按原顺序。
	sortOptionalByPriority(optional)
	remaining := inputBudget - requiredTokens
	keptOptional := make([]ContextBlock, 0, len(optional))
	for _, b := range optional {
		cost := EstimateTokens(b.Text)
		if cost <= remaining {
			keptOptional = append(keptOptional, b)
			remaining -= cost
		}
		// 超过剩余预算的块被裁剪（低优先级先被裁掉）。
	}
	result := append(required, keptOptional...)
	return result, nil
}

// sortOptionalByPriority 对可选块按优先级升序（低数值优先）稳定排序。
func sortOptionalByPriority(blocks []ContextBlock) {
	for i := 1; i < len(blocks); i++ {
		cur := blocks[i]
		j := i - 1
		for j >= 0 && blockPriority(cur) < blockPriority(blocks[j]) {
			blocks[j+1] = blocks[j]
			j--
		}
		blocks[j+1] = cur
	}
}
