package accounting_test

import (
	"errors"
	"testing"

	accounting "github.com/aosanya/mwanachama-backend-accounting"
)

func TestABudgetWithNothingSpentInItsPeriodIsDeletedAndReturned(t *testing.T) {
	r, ctx := newLedger(t)
	pool := openPool(t, r, "p1")
	set, err := r.SetBudget(ctx, accounting.Budget{AccountID: pool.ID, PeriodStart: periodStart, PeriodEnd: periodEnd, Amount: 500})
	if err != nil {
		t.Fatalf("SetBudget: %v", err)
	}

	removed, err := r.DeleteBudget(ctx, set.ID)
	if err != nil {
		t.Fatalf("DeleteBudget: %v", err)
	}
	if removed.ID != set.ID || removed.Amount != 500 {
		t.Fatalf("removed %+v, want the budget %+v", removed, set)
	}
	if _, err := r.Variance(ctx, pool.ID, periodStart, periodEnd); !errors.Is(err, accounting.ErrNotFound) {
		t.Fatalf("the budget is still there: %v", err)
	}
}

func TestADeletedBudgetLeavesRoomToSetTheSamePeriodAgain(t *testing.T) {
	r, ctx := newLedger(t)
	pool := openPool(t, r, "p1")
	set, err := r.SetBudget(ctx, accounting.Budget{AccountID: pool.ID, PeriodStart: periodStart, PeriodEnd: periodEnd, Amount: 500})
	if err != nil {
		t.Fatalf("SetBudget: %v", err)
	}
	if _, err := r.DeleteBudget(ctx, set.ID); err != nil {
		t.Fatalf("DeleteBudget: %v", err)
	}
	again, err := r.SetBudget(ctx, accounting.Budget{AccountID: pool.ID, PeriodStart: periodStart, PeriodEnd: periodEnd, Amount: 800})
	if err != nil || again.Amount != 800 || again.ID == set.ID {
		t.Fatalf("set again %+v %v", again, err)
	}
}

func TestABudgetWithSpendingInItsPeriodIsRefusedAndKept(t *testing.T) {
	r, ctx := newLedger(t)
	pool := openPool(t, r, "p1")
	holder := openHolder(t, r, "h1")
	set, err := r.SetBudget(ctx, accounting.Budget{AccountID: pool.ID, PeriodStart: periodStart, PeriodEnd: periodEnd, Amount: 500})
	if err != nil {
		t.Fatalf("SetBudget: %v", err)
	}
	postAt(t, r, pool, holder, 300, "2026-01-15T12:00:00Z")

	if _, err := r.DeleteBudget(ctx, set.ID); !errors.Is(err, accounting.ErrBudgetLineInUse) {
		t.Fatalf("a budget with spending was removed or refused for another reason: %v", err)
	}
	if v, err := r.Variance(ctx, pool.ID, periodStart, periodEnd); err != nil || v.Budgeted != 500 {
		t.Fatalf("the refused budget changed: %+v %v", v, err)
	}
}

func TestSpendingOutsideTheBudgetsPeriodDoesNotKeepIt(t *testing.T) {
	r, ctx := newLedger(t)
	pool := openPool(t, r, "p1")
	holder := openHolder(t, r, "h1")
	set, err := r.SetBudget(ctx, accounting.Budget{AccountID: pool.ID, PeriodStart: periodStart, PeriodEnd: periodEnd, Amount: 500})
	if err != nil {
		t.Fatalf("SetBudget: %v", err)
	}
	postAt(t, r, pool, holder, 900, "2026-02-01T00:00:00Z")

	if _, err := r.DeleteBudget(ctx, set.ID); err != nil {
		t.Fatalf("spending in another period kept the budget: %v", err)
	}
}

func TestDeletingAMissingBudgetIsNotFound(t *testing.T) {
	r, ctx := newLedger(t)
	if _, err := r.DeleteBudget(ctx, "missing"); !errors.Is(err, accounting.ErrNotFound) {
		t.Fatalf("missing budget: %v", err)
	}
}
