package accounting_test

import (
	"errors"
	"testing"

	accounting "github.com/aosanya/mwanachama-backend-accounting"
)

const (
	periodStart = "2026-01-01T00:00:00Z"
	periodEnd   = "2026-02-01T00:00:00Z"
)

func postAt(t *testing.T, r accounting.LedgerRepository, debit, credit accounting.Account, amount int64, at string) {
	t.Helper()
	ctx := t.Context()
	if _, err := r.Post(ctx, accounting.Entry{
		DebitAccountID: debit.ID, CreditAccountID: credit.ID, Amount: amount, OccurredAt: at,
		DocumentKind: "receipt", DocumentID: "doc-" + at, ActorID: "actor-1",
	}); err != nil {
		t.Fatalf("Post: %v", err)
	}
}

func TestVarianceComparesTheBudgetWithEntriesInsideThePeriodOnly(t *testing.T) {
	r, ctx := newLedger(t)
	pool := openPool(t, r, "p1")
	holder := openHolder(t, r, "h1")

	if _, err := r.SetBudget(ctx, accounting.Budget{AccountID: pool.ID, PeriodStart: periodStart, PeriodEnd: periodEnd, Amount: 500}); err != nil {
		t.Fatalf("SetBudget: %v", err)
	}
	postAt(t, r, pool, holder, 100, "2025-12-31T23:59:59Z")
	postAt(t, r, pool, holder, 300, "2026-01-01T00:00:00Z")
	postAt(t, r, pool, holder, 50, "2026-01-15T12:00:00Z")
	postAt(t, r, pool, holder, 900, "2026-02-01T00:00:00Z")

	v, err := r.Variance(ctx, pool.ID, periodStart, periodEnd)
	if err != nil {
		t.Fatalf("Variance: %v", err)
	}
	if v.Budgeted != 500 || v.Actual != 350 || v.Difference != -150 {
		t.Fatalf("variance = %+v, want budgeted 500, actual 350, difference -150", v)
	}
}

func TestSettingABudgetAgainReplacesItsAmount(t *testing.T) {
	r, ctx := newLedger(t)
	pool := openPool(t, r, "p1")
	first, err := r.SetBudget(ctx, accounting.Budget{AccountID: pool.ID, PeriodStart: periodStart, PeriodEnd: periodEnd, Amount: 500})
	if err != nil {
		t.Fatalf("SetBudget: %v", err)
	}
	second, err := r.SetBudget(ctx, accounting.Budget{AccountID: pool.ID, PeriodStart: periodStart, PeriodEnd: periodEnd, Amount: 800})
	if err != nil {
		t.Fatalf("SetBudget again: %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("second set minted %s, want the same budget %s", second.ID, first.ID)
	}
	v, err := r.Variance(ctx, pool.ID, periodStart, periodEnd)
	if err != nil {
		t.Fatalf("Variance: %v", err)
	}
	if v.Budgeted != 800 || v.Difference != -800 {
		t.Fatalf("variance = %+v, want budgeted 800 and difference -800", v)
	}
}

func TestABudgetRefusesAnUnknownAccountAndABackwardsPeriod(t *testing.T) {
	r, ctx := newLedger(t)
	pool := openPool(t, r, "p1")
	if _, err := r.SetBudget(ctx, accounting.Budget{AccountID: "missing", PeriodStart: periodStart, PeriodEnd: periodEnd, Amount: 1}); !errors.Is(err, accounting.ErrNotFound) {
		t.Fatalf("unknown account: err = %v, want ErrNotFound", err)
	}
	if _, err := r.SetBudget(ctx, accounting.Budget{AccountID: pool.ID, PeriodStart: periodEnd, PeriodEnd: periodStart, Amount: 1}); !errors.Is(err, accounting.ErrInvalid) {
		t.Fatalf("backwards period: err = %v, want ErrInvalid", err)
	}
	if _, err := r.SetBudget(ctx, accounting.Budget{AccountID: pool.ID, PeriodStart: "january", PeriodEnd: periodEnd, Amount: 1}); !errors.Is(err, accounting.ErrInvalid) {
		t.Fatalf("unparseable period: err = %v, want ErrInvalid", err)
	}
}

func TestVarianceWithNoBudgetForThePeriodIsNotFound(t *testing.T) {
	r, ctx := newLedger(t)
	pool := openPool(t, r, "p1")
	if _, err := r.Variance(ctx, pool.ID, periodStart, periodEnd); !errors.Is(err, accounting.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}
