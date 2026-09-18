package ask

import (
	"errors"
	"time"
)

// 预算体系说明（依据 v2 架构文档 9.3）：
// 预算分为聊天 Run、已确认 Operation 执行、Operation 核实、后台记忆派生四个归属，
// 各自独立、不互相重置；都受服务总量及并发限制。执行资格预算与真实供应商用量
// 分别记录：执行资格（本文件各 Scope 的 MaxModelCalls/MaxToolCalls 等）决定能否派发，
// 真实用量（Token/费用）在派发后结算，未知用量不记零、保留到可信用量或受控对账。

// BudgetScope 表示预算的归属类别。
type BudgetScope string

const (
	BudgetRun              BudgetScope = "run"               // 聊天 Run 预算
	BudgetOperationExecute BudgetScope = "operation_exec"    // 已确认 Operation 执行预算
	BudgetOperationVerify  BudgetScope = "operation_verify"  // Operation 核实预算
	BudgetMemoryDerivation BudgetScope = "memory_derivation" // 后台记忆派生预算
)

// BudgetLimits 是预算各维度的上限。零值表示该维度不限制。
type BudgetLimits struct {
	// MaxModelCalls 是执行资格的次数上限，各 Scope 语义不同：
	//   run               —— 模型调用总数上限（含自动重试与内部摘要）
	//   operation_exec    —— 有效提交次数上限（9.3：至多一次有效提交）
	//   operation_verify  —— 核实查询次数上限（9.3：最多 24 次）
	//   memory_derivation —— 派生生成次数上限（9.3：1 次初始 + 2 次恢复）
	MaxModelCalls int
	// MaxToolCalls 是工具实际调用总数上限（仅 Run 使用）。
	MaxToolCalls int
	// MaxTokens 是输入 + 输出 Token 合计上限。
	MaxTokens int
	// MaxCostMicros 是费用上限（百万分之一元）。一期免费不设硬上限，
	// 但用量仍记录供对账。
	MaxCostMicros int64
	// MaxConcurrency 是并发许可上限。
	MaxConcurrency int
}

// BudgetAmount 是预算的计量单位，用于用量与预留。Concurrency 仅用于预留，
// 本地调用结束即释放，不结算为用量。
type BudgetAmount struct {
	ModelCalls  int
	ToolCalls   int
	Tokens      int
	CostMicros  int64
	Concurrency int
}

// BudgetLedger 是预算账本的持久化状态。
type BudgetLedger struct {
	ID        string
	Scope     BudgetScope
	ScopeID   string
	Limits    BudgetLimits
	Used      BudgetAmount
	Reserved  BudgetAmount
	CreatedAt time.Time
	UpdatedAt time.Time
}

// BudgetReservation 是派发前的一次原子预留，也是预留记录的持久化内容。
type BudgetReservation struct {
	ID         string
	BudgetID   string
	Amount     BudgetAmount
	Status     ReservationStatus
	CreatedAt  time.Time
	SettledAt  *time.Time
	ReleasedAt *time.Time
}

// ReservationStatus 是一次预留的生命周期状态。
type ReservationStatus string

const (
	ReservationReserved ReservationStatus = "reserved"
	ReservationSettled  ReservationStatus = "settled"
	ReservationReleased ReservationStatus = "released"
)

var (
	// ErrBudgetNotFound 表示账本不存在（通常由 EnsureBudget 先建）。
	ErrBudgetNotFound = errors.New("ask budget not found")
	// ErrBudgetExceeded 表示预留超出限额，本次派发应被拒绝且不发送。
	ErrBudgetExceeded = errors.New("ask budget exceeded")
	// ErrReservationNotFound 表示预留记录不存在。
	ErrReservationNotFound = errors.New("ask budget reservation not found")
	// ErrReservationConflict 表示预留记录状态与操作不匹配（如重复结算/归还）。
	ErrReservationConflict = errors.New("ask budget reservation state conflict")
)

// validateBudgetAmount 校验预留/用量各维度非负，任何负值都视为非法请求。
func validateBudgetAmount(amount BudgetAmount) error {
	if amount.ModelCalls < 0 || amount.ToolCalls < 0 || amount.Tokens < 0 || amount.CostMicros < 0 || amount.Concurrency < 0 {
		return errors.New("ask budget amount cannot be negative")
	}
	return nil
}

// exceedsBudget 判断 used + reserved + amount 是否超出 limits 任一维度。
// 零值上限表示该维度不限制；Concurrency 只看 reserved（并发许可无 used 语义）。
func exceedsBudget(limits BudgetLimits, used, reserved, amount BudgetAmount) bool {
	if limits.MaxModelCalls > 0 && used.ModelCalls+reserved.ModelCalls+amount.ModelCalls > limits.MaxModelCalls {
		return true
	}
	if limits.MaxToolCalls > 0 && used.ToolCalls+reserved.ToolCalls+amount.ToolCalls > limits.MaxToolCalls {
		return true
	}
	if limits.MaxTokens > 0 && used.Tokens+reserved.Tokens+amount.Tokens > limits.MaxTokens {
		return true
	}
	if limits.MaxCostMicros > 0 && used.CostMicros+reserved.CostMicros+amount.CostMicros > limits.MaxCostMicros {
		return true
	}
	if limits.MaxConcurrency > 0 && reserved.Concurrency+amount.Concurrency > limits.MaxConcurrency {
		return true
	}
	return false
}

// DefaultBudgetLimits 返回一期各 Scope 的默认限额。
// 数值依据 9.3 的硬约束与保守上界给出，可由版本化服务端配置覆盖。
func DefaultBudgetLimits(scope BudgetScope) BudgetLimits {
	switch scope {
	case BudgetRun:
		// 9.3：模型恢复累计最多 2 次；8 次覆盖正常决策 + 追问 + 摘要 + 重试的保守上界。
		return BudgetLimits{
			MaxModelCalls:  8,
			MaxToolCalls:   20,
			MaxTokens:      12000,
			MaxCostMicros:  0, // 一期免费，不设硬上限，用量单独对账
			MaxConcurrency: 1, // 一期单 Worker 串行
		}
	case BudgetOperationExecute:
		// 9.3：已确认业务变更至多一次有效提交。
		return BudgetLimits{MaxModelCalls: 1, MaxConcurrency: 1}
	case BudgetOperationVerify:
		// 9.3：自动核实最多 24 次（24 小时内）。
		return BudgetLimits{MaxModelCalls: 24, MaxConcurrency: 1}
	case BudgetMemoryDerivation:
		// 9.3：同一来源版本任务一次初始生成 + 临时失败最多 2 次恢复。
		return BudgetLimits{MaxModelCalls: 3, MaxTokens: 6000, MaxConcurrency: 1}
	default:
		return BudgetLimits{}
	}
}
