package ask

import (
	"context"
	"errors"
	"testing"
)

func TestMeteredCallReserveFailsDoesNotInvoke(t *testing.T) {
	repository := newBudgetTestRepository(t)
	ctx := context.Background()
	if err := repository.EnsureBudget(ctx, BudgetRun, "run-1", BudgetLimits{MaxModelCalls: 1}); err != nil {
		t.Fatal(err)
	}
	// 先占满额度，使后续预留失败
	if _, err := repository.ReserveBudget(ctx, BudgetRun, "run-1", BudgetAmount{ModelCalls: 1}); err != nil {
		t.Fatal(err)
	}

	meter := NewMeteredCall(repository)
	invoked := false
	_, err := meter.Invoke(ctx, BudgetRun, "run-1", BudgetAmount{ModelCalls: 1}, func(context.Context) (BudgetAmount, CallOutcome, error) {
		invoked = true
		return BudgetAmount{}, CallOutcomeSent, nil
	})
	if !errors.Is(err, ErrBudgetExceeded) {
		t.Fatalf("error = %v, want %v", err, ErrBudgetExceeded)
	}
	if invoked {
		t.Fatal("预留失败时不应执行调用")
	}
}

func TestMeteredCallSettlesOnSent(t *testing.T) {
	repository := newBudgetTestRepository(t)
	ctx := context.Background()
	if err := repository.EnsureBudget(ctx, BudgetRun, "run-1", BudgetLimits{MaxModelCalls: 10, MaxTokens: 10000}); err != nil {
		t.Fatal(err)
	}

	meter := NewMeteredCall(repository)
	result, err := meter.Invoke(ctx, BudgetRun, "run-1", BudgetAmount{ModelCalls: 1, Tokens: 2000}, func(context.Context) (BudgetAmount, CallOutcome, error) {
		// 实际用量小于预留量，差额应自动归还
		return BudgetAmount{ModelCalls: 1, Tokens: 800}, CallOutcomeSent, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Settled || result.AccountErr != nil {
		t.Fatalf("result = %+v, want settled", result)
	}
	ledger, err := repository.GetBudget(ctx, BudgetRun, "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if ledger.Used.ModelCalls != 1 || ledger.Used.Tokens != 800 {
		t.Fatalf("used = %+v, want ModelCalls=1 Tokens=800", ledger.Used)
	}
	if ledger.Reserved.ModelCalls != 0 || ledger.Reserved.Tokens != 0 {
		t.Fatalf("reserved = %+v, want zero after settle", ledger.Reserved)
	}
}

func TestMeteredCallReleasesOnNotSent(t *testing.T) {
	repository := newBudgetTestRepository(t)
	ctx := context.Background()
	if err := repository.EnsureBudget(ctx, BudgetRun, "run-1", BudgetLimits{MaxModelCalls: 10}); err != nil {
		t.Fatal(err)
	}

	meter := NewMeteredCall(repository)
	_, err := meter.Invoke(ctx, BudgetRun, "run-1", BudgetAmount{ModelCalls: 1}, func(context.Context) (BudgetAmount, CallOutcome, error) {
		return BudgetAmount{}, CallOutcomeNotSent, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	ledger, err := repository.GetBudget(ctx, BudgetRun, "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if ledger.Used.ModelCalls != 0 || ledger.Reserved.ModelCalls != 0 {
		t.Fatalf("used/reserved = %+v/%+v, want zero (未发送不结算)", ledger.Used, ledger.Reserved)
	}
}

func TestMeteredCallRetainsOnUnknown(t *testing.T) {
	repository := newBudgetTestRepository(t)
	ctx := context.Background()
	if err := repository.EnsureBudget(ctx, BudgetRun, "run-1", BudgetLimits{MaxModelCalls: 10}); err != nil {
		t.Fatal(err)
	}

	meter := NewMeteredCall(repository)
	result, err := meter.Invoke(ctx, BudgetRun, "run-1", BudgetAmount{ModelCalls: 1}, func(context.Context) (BudgetAmount, CallOutcome, error) {
		return BudgetAmount{}, CallOutcomeUnknown, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Settled {
		t.Fatalf("result = %+v, want not settled (未知保留)", result)
	}
	ledger, err := repository.GetBudget(ctx, BudgetRun, "run-1")
	if err != nil {
		t.Fatal(err)
	}
	// 未知占用保留：reserved 仍为 1，used 为 0
	if ledger.Reserved.ModelCalls != 1 || ledger.Used.ModelCalls != 0 {
		t.Fatalf("reserved/used = %+v/%+v, want reserved=1 used=0 (未知保留)", ledger.Reserved, ledger.Used)
	}
}

func TestMeteredCallSurfacesCallError(t *testing.T) {
	repository := newBudgetTestRepository(t)
	ctx := context.Background()
	if err := repository.EnsureBudget(ctx, BudgetRun, "run-1", BudgetLimits{MaxModelCalls: 10}); err != nil {
		t.Fatal(err)
	}

	meter := NewMeteredCall(repository)
	callErr := errors.New("boom")
	result, err := meter.Invoke(ctx, BudgetRun, "run-1", BudgetAmount{ModelCalls: 1}, func(context.Context) (BudgetAmount, CallOutcome, error) {
		return BudgetAmount{}, CallOutcomeNotSent, callErr
	})
	if !errors.Is(err, callErr) {
		t.Fatalf("error = %v, want call error", err)
	}
	if result.AccountErr != nil {
		t.Fatalf("result.AccountErr = %v, want nil (未发送已归还)", result.AccountErr)
	}
}

func TestEstimateReservation(t *testing.T) {
	amount := EstimateReservation(3000, 1200, 4)
	if amount.ModelCalls != 1 || amount.ToolCalls != 4 || amount.Tokens != 4200 || amount.Concurrency != 1 {
		t.Fatalf("estimate = %+v, want ModelCalls=1 ToolCalls=4 Tokens=4200 Concurrency=1", amount)
	}
	if amount.CostMicros != 0 {
		t.Fatalf("estimate.CostMicros = %d, want 0 (一期免费)", amount.CostMicros)
	}
}
