package ask

import (
	"encoding/json"
	"fmt"
	"sync"
)

// 本文件固定执行层 Loop Guard（对应架构设计 v2 文档 7.4 的 blocked 处置）：
// 相同工具 + 相同规范化参数（指纹相同）时，确定性失败、写入准备、以及已经
// 返回过相同结果的只读查询不再执行；结果发生变化（异步状态推进）或参数被
// 真正修正（指纹变化）时继续放行。最大步数与预算是最后一道保险，不替代本判定。

// toolCallOutcome 是一次已执行调用的可比较结果特征。
type toolCallOutcome struct {
	status     ToolResultStatus
	reason     string
	signature  string
	noProgress bool
}

// LoopGuard 记录单次 Run 内的调用指纹与结果，判定重复调用是否还有信息增量。
type LoopGuard struct {
	mu       sync.Mutex
	outcomes map[string]toolCallOutcome
}

func NewLoopGuard() *LoopGuard {
	return &LoopGuard{outcomes: make(map[string]toolCallOutcome)}
}

// BlockReason 报告该调用是否应被阻断，以及给模型/用户可见的原因。
func (g *LoopGuard) BlockReason(call ToolCall, action ActionType) (string, bool) {
	fingerprint, err := fingerprintForCall(call)
	if err != nil {
		return "", false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	previous, ok := g.outcomes[fingerprint]
	if !ok {
		return "", false
	}
	if !IsReadOnly(action) {
		return "相同参数的写入准备已经完成过，请勿重复请求；需要修改请调整参数后重新调用", true
	}
	if previous.status != ToolResultOK {
		return fmt.Sprintf("相同参数的调用已失败（%s），重复调用不会改变结果", previous.reason), true
	}
	if previous.noProgress {
		return "相同参数的调用已返回相同结果，没有新的信息", true
	}
	return "", false
}

// Record 记录一次真实执行的结果。结果与上次完全一致时标记为无进展，
// 使下一次重复调用被阻断；结果变化时清除标记，保留正常的异步轮询。
func (g *LoopGuard) Record(call ToolCall, result ToolResult) {
	fingerprint, err := fingerprintForCall(call)
	if err != nil {
		return
	}
	signature := resultSignature(result)
	reason := ""
	if result.Error != nil {
		reason = result.Error.Reason
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	previous, ok := g.outcomes[fingerprint]
	g.outcomes[fingerprint] = toolCallOutcome{
		status:     result.Status,
		reason:     reason,
		signature:  signature,
		noProgress: ok && signature != "" && signature == previous.signature,
	}
}

func fingerprintForCall(call ToolCall) (string, error) {
	var arguments any
	if len(call.Arguments) > 0 {
		if err := json.Unmarshal(call.Arguments, &arguments); err != nil {
			return "", err
		}
	}
	return ToolFingerprint(call.ToolName, call.ToolVersion, arguments)
}

// volatileResultKeys 是结果里随读取时间变化的字段，不参与「是否产生新信息」的比较。
var volatileResultKeys = map[string]struct{}{
	"read_at": {}, "queued_at": {}, "started_at": {}, "completed_at": {},
}

func resultSignature(result ToolResult) string {
	reason := ""
	if result.Error != nil {
		reason = result.Error.Reason
	}
	raw, err := json.Marshal(map[string]any{"status": string(result.Status), "reason": reason, "data": result.Data})
	if err != nil {
		return ""
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return ""
	}
	canonical, err := CanonicalJSON(dropVolatileKeys(value))
	if err != nil {
		return ""
	}
	return canonical
}

func dropVolatileKeys(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		for key, item := range typed {
			if _, volatile := volatileResultKeys[key]; volatile {
				delete(typed, key)
				continue
			}
			typed[key] = dropVolatileKeys(item)
		}
		return typed
	case []any:
		for index, item := range typed {
			typed[index] = dropVolatileKeys(item)
		}
		return typed
	default:
		return value
	}
}
