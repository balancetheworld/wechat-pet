package ask

import (
	"context"
	"errors"
)

// CallOutcome 描述一次可能产生费用的模型调用结束后的处置依据，
// 决定已经原子预留的预算应结算、归还还是保留（v2 文档 9.3、11.4）。
type CallOutcome int

const (
	// CallOutcomeSent 请求已实际发出并取得可结算用量，应结算。
	CallOutcomeSent CallOutcome = iota
	// CallOutcomeNotSent 有证据请求未发出（如本地校验失败），应归还预留。
	CallOutcomeNotSent
	// CallOutcomeUnknown 请求可能已发出但结果未知（超时/失联），保留预留待对账。
	CallOutcomeUnknown
)

// MeteredCall 封装「派发前原子预留 → 执行 → 按结果结算/归还/保留」的调用编排。
// 调用方（Runtime）在每次可能产生费用的派发前使用它，避免自行复刻预算事务逻辑。
type MeteredCall struct {
	budgets BudgetRepository
}

// NewMeteredCall 构造计量调用编排器。
func NewMeteredCall(budgets BudgetRepository) *MeteredCall {
	return &MeteredCall{budgets: budgets}
}

// InvokeResult 是一次计量调用的结果，记录预留身份与结算/归还结果。
type InvokeResult struct {
	// ReservationID 是本次预留记录 ID，可据此做迟到用量追加对账。
	ReservationID string
	// Outcome 是本次调用的结束结果分类。
	Outcome CallOutcome
	// Settled 报告预留是否已按实际用量结算。
	Settled bool
	// AccountErr 是结算或归还失败的错误；调用成功但账务操作失败时非空。
	AccountErr error
}

// Invoke 先按 estimate 原子预留；预留失败（ErrBudgetExceeded）直接返回、不执行 fn。
// 预留成功后执行 fn，fn 返回本次调用的实际用量与结束结果：
//   - CallOutcomeSent    → 按 actual 结算（实际用量小于预留量时差额自动归还）
//   - CallOutcomeNotSent → 归还未发送的预留（有证据未发出）
//   - CallOutcomeUnknown → 保留预留待对账，不结算不归还
//
// 返回的 error 语义：预留失败返回预留错误；调用失败返回调用错误；调用成功但账务
// 失败返回账务错误；全部成功返回 nil。AccountErr 始终保留账务错误供诊断。
func (m *MeteredCall) Invoke(ctx context.Context, scope BudgetScope, scopeID string, estimate BudgetAmount, fn func(context.Context) (BudgetAmount, CallOutcome, error)) (InvokeResult, error) {
	if m.budgets == nil {
		return InvokeResult{}, errors.New("ask metered call budgets is required")
	}
	reservation, err := m.budgets.ReserveBudget(ctx, scope, scopeID, estimate)
	if err != nil {
		// 预留失败不发送，本次调用未发生。
		return InvokeResult{Outcome: CallOutcomeUnknown}, err
	}

	actual, outcome, callErr := fn(ctx)

	result := InvokeResult{ReservationID: reservation.ID, Outcome: outcome}
	switch outcome {
	case CallOutcomeSent:
		result.AccountErr = m.budgets.SettleBudget(ctx, reservation.ID, actual)
		result.Settled = result.AccountErr == nil
	case CallOutcomeNotSent:
		result.AccountErr = m.budgets.ReleaseBudget(ctx, reservation.ID)
	case CallOutcomeUnknown:
		// 未知占用保留到可信用量或受控对账，不结算不归还。
	}

	if callErr != nil {
		return result, callErr
	}
	return result, result.AccountErr
}

// EstimateReservation 计算一次模型调用的保守预留量：
// 模型调用 1 次 + 预估输入 Token + 最大输出 Token + 工具上限 + 并发许可 1。
// 费用一期免费不设硬上限，CostMicros 记 0，由调用方在可信用量可得时另行计入。
func EstimateReservation(inputTokensEstimate, maxOutputTokens, toolCallUpperBound int) BudgetAmount {
	return BudgetAmount{
		ModelCalls:  1,
		ToolCalls:   toolCallUpperBound,
		Tokens:      inputTokensEstimate + maxOutputTokens,
		Concurrency: 1,
	}
}
