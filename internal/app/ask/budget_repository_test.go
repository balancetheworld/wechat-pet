package ask

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func newBudgetTestRepository(t *testing.T) *SQLRepository {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	createAskSchema(t, db)
	repository, err := NewRepository(db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	return repository
}

func TestEnsureBudgetIsIdempotent(t *testing.T) {
	repository := newBudgetTestRepository(t)
	ctx := context.Background()

	first := BudgetLimits{MaxModelCalls: 2, MaxConcurrency: 1}
	if err := repository.EnsureBudget(ctx, BudgetRun, "run-1", first); err != nil {
		t.Fatal(err)
	}
	// 重复建立不覆盖限额
	if err := repository.EnsureBudget(ctx, BudgetRun, "run-1", BudgetLimits{MaxModelCalls: 99}); err != nil {
		t.Fatal(err)
	}
	ledger, err := repository.GetBudget(ctx, BudgetRun, "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if ledger.Limits.MaxModelCalls != 2 {
		t.Fatalf("limits = %+v, want MaxModelCalls=2 (不覆盖)", ledger.Limits)
	}
}

func TestReserveBudgetRejectsExceeded(t *testing.T) {
	repository := newBudgetTestRepository(t)
	ctx := context.Background()

	if err := repository.EnsureBudget(ctx, BudgetRun, "run-1", BudgetLimits{MaxModelCalls: 2}); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.ReserveBudget(ctx, BudgetRun, "run-1", BudgetAmount{ModelCalls: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.ReserveBudget(ctx, BudgetRun, "run-1", BudgetAmount{ModelCalls: 1}); err != nil {
		t.Fatal(err)
	}
	// 第三次超出 MaxModelCalls=2，应拒绝且不派发
	if _, err := repository.ReserveBudget(ctx, BudgetRun, "run-1", BudgetAmount{ModelCalls: 1}); !errors.Is(err, ErrBudgetExceeded) {
		t.Fatalf("error = %v, want %v", err, ErrBudgetExceeded)
	}
}

func TestReserveBudgetConcurrencyLimit(t *testing.T) {
	repository := newBudgetTestRepository(t)
	ctx := context.Background()

	if err := repository.EnsureBudget(ctx, BudgetRun, "run-1", BudgetLimits{MaxConcurrency: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.ReserveBudget(ctx, BudgetRun, "run-1", BudgetAmount{Concurrency: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.ReserveBudget(ctx, BudgetRun, "run-1", BudgetAmount{Concurrency: 1}); !errors.Is(err, ErrBudgetExceeded) {
		t.Fatalf("error = %v, want %v", err, ErrBudgetExceeded)
	}
}

func TestSettleBudgetReleasesOverReserved(t *testing.T) {
	repository := newBudgetTestRepository(t)
	ctx := context.Background()

	if err := repository.EnsureBudget(ctx, BudgetRun, "run-1", BudgetLimits{MaxModelCalls: 10, MaxTokens: 10000}); err != nil {
		t.Fatal(err)
	}
	reservation, err := repository.ReserveBudget(ctx, BudgetRun, "run-1", BudgetAmount{ModelCalls: 5, Tokens: 1000})
	if err != nil {
		t.Fatal(err)
	}
	// 实际用量小于预留量，差额自动归还
	if err := repository.SettleBudget(ctx, reservation.ID, BudgetAmount{ModelCalls: 3, Tokens: 800}); err != nil {
		t.Fatal(err)
	}
	ledger, err := repository.GetBudget(ctx, BudgetRun, "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if ledger.Used.ModelCalls != 3 || ledger.Used.Tokens != 800 {
		t.Fatalf("used = %+v, want ModelCalls=3 Tokens=800", ledger.Used)
	}
	if ledger.Reserved.ModelCalls != 0 || ledger.Reserved.Tokens != 0 {
		t.Fatalf("reserved = %+v, want zero after settle", ledger.Reserved)
	}
}

func TestDurationBudgetSettlesAndRejectsExceeded(t *testing.T) {
	repository := newBudgetTestRepository(t)
	ctx := context.Background()

	if err := repository.EnsureBudget(ctx, BudgetRun, "run-1", BudgetLimits{MaxDurationMillis: 10}); err != nil {
		t.Fatal(err)
	}
	reservation, err := repository.ReserveBudget(ctx, BudgetRun, "run-1", BudgetAmount{DurationMillis: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.SettleBudget(ctx, reservation.ID, BudgetAmount{DurationMillis: 7}); err != nil {
		t.Fatal(err)
	}
	ledger, err := repository.GetBudget(ctx, BudgetRun, "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if ledger.Used.DurationMillis != 7 || ledger.Reserved.DurationMillis != 0 {
		t.Fatalf("duration budget = used %d reserved %d, want used 7 reserved 0", ledger.Used.DurationMillis, ledger.Reserved.DurationMillis)
	}
	if _, err := repository.ReserveBudget(ctx, BudgetRun, "run-1", BudgetAmount{DurationMillis: 4}); !errors.Is(err, ErrBudgetExceeded) {
		t.Fatalf("error = %v, want %v", err, ErrBudgetExceeded)
	}
}

func TestSettleBudgetIsNotIdempotent(t *testing.T) {
	repository := newBudgetTestRepository(t)
	ctx := context.Background()

	if err := repository.EnsureBudget(ctx, BudgetRun, "run-1", BudgetLimits{MaxModelCalls: 10}); err != nil {
		t.Fatal(err)
	}
	reservation, err := repository.ReserveBudget(ctx, BudgetRun, "run-1", BudgetAmount{ModelCalls: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.SettleBudget(ctx, reservation.ID, BudgetAmount{ModelCalls: 1}); err != nil {
		t.Fatal(err)
	}
	if err := repository.SettleBudget(ctx, reservation.ID, BudgetAmount{ModelCalls: 1}); !errors.Is(err, ErrReservationConflict) {
		t.Fatalf("error = %v, want %v", err, ErrReservationConflict)
	}
}

func TestReleaseBudgetOnlyReturnsUnsent(t *testing.T) {
	repository := newBudgetTestRepository(t)
	ctx := context.Background()

	if err := repository.EnsureBudget(ctx, BudgetRun, "run-1", BudgetLimits{MaxModelCalls: 10}); err != nil {
		t.Fatal(err)
	}
	reservation, err := repository.ReserveBudget(ctx, BudgetRun, "run-1", BudgetAmount{ModelCalls: 2, Concurrency: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.ReleaseBudget(ctx, reservation.ID); err != nil {
		t.Fatal(err)
	}
	ledger, err := repository.GetBudget(ctx, BudgetRun, "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if ledger.Reserved.ModelCalls != 0 || ledger.Reserved.Concurrency != 0 {
		t.Fatalf("reserved = %+v, want zero after release", ledger.Reserved)
	}
	if ledger.Used.ModelCalls != 0 {
		t.Fatalf("used = %+v, want zero (未发送不结算)", ledger.Used)
	}
	// 重复释放幂等
	if err := repository.ReleaseBudget(ctx, reservation.ID); err != nil {
		t.Fatalf("重复释放应幂等，got %v", err)
	}
}

func TestReleaseBudgetAfterSettleIsRejected(t *testing.T) {
	repository := newBudgetTestRepository(t)
	ctx := context.Background()

	if err := repository.EnsureBudget(ctx, BudgetRun, "run-1", BudgetLimits{MaxModelCalls: 10}); err != nil {
		t.Fatal(err)
	}
	reservation, err := repository.ReserveBudget(ctx, BudgetRun, "run-1", BudgetAmount{ModelCalls: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.SettleBudget(ctx, reservation.ID, BudgetAmount{ModelCalls: 1}); err != nil {
		t.Fatal(err)
	}
	if err := repository.ReleaseBudget(ctx, reservation.ID); !errors.Is(err, ErrReservationConflict) {
		t.Fatalf("error = %v, want %v", err, ErrReservationConflict)
	}
}

func TestBudgetNotFoundWithoutEnsure(t *testing.T) {
	repository := newBudgetTestRepository(t)
	ctx := context.Background()

	if _, err := repository.GetBudget(ctx, BudgetRun, "missing"); !errors.Is(err, ErrBudgetNotFound) {
		t.Fatalf("GetBudget error = %v, want %v", err, ErrBudgetNotFound)
	}
	if _, err := repository.ReserveBudget(ctx, BudgetRun, "missing", BudgetAmount{ModelCalls: 1}); !errors.Is(err, ErrBudgetNotFound) {
		t.Fatalf("ReserveBudget error = %v, want %v", err, ErrBudgetNotFound)
	}
}

func TestReserveBudgetRejectsNegativeAmount(t *testing.T) {
	repository := newBudgetTestRepository(t)
	ctx := context.Background()

	if err := repository.EnsureBudget(ctx, BudgetRun, "run-1", BudgetLimits{MaxModelCalls: 10}); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.ReserveBudget(ctx, BudgetRun, "run-1", BudgetAmount{ModelCalls: -1}); err == nil {
		t.Fatal("want error for negative amount")
	}
}
