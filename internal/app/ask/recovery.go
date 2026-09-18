package ask

import (
	"encoding/json"
	"time"
)

// 有界恢复边界（依据 v2 架构文档 9.3、10.5）：
//   - 模型自动重试累计上限 2 次：模式降级、能力允许的备用切换、失效输入重建、
//     校验修复都计入，按 Run 累计，不因用户补充或 Worker 重新领取清零。
//   - 单步结构修复最多 1 次，且占用上面 2 次额度。
//   - 只读工具每个逻辑调用临时失败最多重试 1 次，同时占用 Run 总工具预算。
const (
	MaxModelAutoRetry    = 2
	MaxSingleStepRepair  = 1
	MaxReadonlyToolRetry = 1
)

// canAutoRetry 判断在已累计 autoRetryCount 次自动重试后是否仍可再发一次模型请求。
// 自动重试次数由 Attempt 记录承载，T3 Provider 接入后按 Run 累计传入。
func canAutoRetry(autoRetryCount int) bool {
	return autoRetryCount >= 0 && autoRetryCount < MaxModelAutoRetry
}

// 以下为 Run 从 running 回到 queued 时写入 termination_reason 的原因码（v2 3.4.9）。
// 该字段同时承载终态终止原因（session_closed、budget_exhausted 等，后阶段落地），
// 一期先固定恢复/退避两类。
const (
	TerminationReasonLeaseRecovered = "lease_recovered" // 崩溃后重新领取
	TerminationReasonRetryBackoff   = "retry_backoff"   // 执行失败后的退避重试
)

// RunCheckpoint 是 running -> queued 时保存的检查点（进度快照）。
// 一期内容为恢复上下文；工具循环落地（T5）后扩充已完成步骤清单。
type RunCheckpoint struct {
	Reason        string     `json:"reason"`
	AttemptCount  int        `json:"attempt_count"`
	NextAttemptAt *time.Time `json:"next_attempt_at,omitempty"`
}

// marshalRunCheckpoint 序列化检查点；序列化失败时回退为只含原因的最小内容。
func marshalRunCheckpoint(checkpoint RunCheckpoint) string {
	data, err := json.Marshal(checkpoint)
	if err != nil {
		return `{"reason":"` + checkpoint.Reason + `"}`
	}
	return string(data)
}
