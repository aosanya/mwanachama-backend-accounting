package accounting_test

import (
	"errors"
	"testing"

	accounting "github.com/aosanya/mwanachama-backend-accounting"
)

const (
	januaryStart  = "2026-01-01T00:00:00Z"
	februaryStart = "2026-02-01T00:00:00Z"
)

func TestVarianceComparesWhatWasSpentInThePeriodWithTheBudget(t *testing.T) {
	f := newStoreFixture(t)
	f.postAt(t, f.rent, f.cash, 300, accounting.Entry{OccurredAt: "2026-01-10T00:00:00Z"})
	f.postAt(t, f.rent, f.cash, 200, accounting.Entry{OccurredAt: "2026-01-25T00:00:00Z"})
	f.postAt(t, f.rent, f.cash, 7000, accounting.Entry{OccurredAt: "2026-02-01T00:00:00Z"})
	if _, err := f.repo.SetBudget(f.bg, accounting.Budget{AccountID: f.rent.ID, PeriodStart: januaryStart, PeriodEnd: februaryStart, Amount: 400}); err != nil {
		t.Fatalf("SetBudget: %v", err)
	}
	v, err := f.repo.Variance(f.bg, f.rent.ID, januaryStart, februaryStart)
	if err != nil {
		t.Fatalf("Variance: %v", err)
	}
	if v.Budgeted != 400 || v.Actual != 500 || v.Difference != 100 || v.AccountID != f.rent.ID {
		t.Fatalf("variance %+v", v)
	}
}

func TestSettingTheSameBudgetAgainReplacesItsAmount(t *testing.T) {
	f := newStoreFixture(t)
	first, err := f.repo.SetBudget(f.bg, accounting.Budget{AccountID: f.rent.ID, PeriodStart: januaryStart, PeriodEnd: februaryStart, Amount: 400})
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	second, err := f.repo.SetBudget(f.bg, accounting.Budget{AccountID: f.rent.ID, PeriodStart: januaryStart, PeriodEnd: februaryStart, Amount: 900})
	if err != nil || second.ID != first.ID || second.Amount != 900 {
		t.Fatalf("second %+v %v", second, err)
	}
	v, err := f.repo.Variance(f.bg, f.rent.ID, januaryStart, februaryStart)
	if err != nil || v.Budgeted != 900 {
		t.Fatalf("variance %+v %v", v, err)
	}
}

func TestBudgetAndVarianceRefusals(t *testing.T) {
	f := newStoreFixture(t)
	if _, err := f.repo.SetBudget(f.bg, accounting.Budget{AccountID: f.rent.ID, PeriodStart: februaryStart, PeriodEnd: januaryStart, Amount: 1}); !errors.Is(err, accounting.ErrInvalid) {
		t.Fatalf("backwards period: %v", err)
	}
	if _, err := f.repo.SetBudget(f.bg, accounting.Budget{AccountID: "missing", PeriodStart: januaryStart, PeriodEnd: februaryStart, Amount: 1}); !errors.Is(err, accounting.ErrNotFound) {
		t.Fatalf("missing account: %v", err)
	}
	if _, err := f.repo.Variance(f.bg, f.rent.ID, "January", februaryStart); !errors.Is(err, accounting.ErrInvalid) {
		t.Fatalf("bad period: %v", err)
	}
	if _, err := f.repo.Variance(f.bg, "missing", januaryStart, februaryStart); !errors.Is(err, accounting.ErrNotFound) {
		t.Fatalf("missing account: %v", err)
	}
	if _, err := f.repo.Variance(f.bg, f.rent.ID, januaryStart, februaryStart); !errors.Is(err, accounting.ErrNotFound) {
		t.Fatalf("no budget set: %v", err)
	}
}

func TestOverdueListsUnpaidChargesPastDueAndLeavesOutSettledAndReversedOnes(t *testing.T) {
	f := newStoreFixture(t)
	unpaid := f.postAt(t, f.cash, f.sales, 1000, accounting.Entry{DueAt: "2026-05-01T00:00:00Z", DocumentID: "inv-1"})
	part := f.postAt(t, f.cash, f.sales, 600, accounting.Entry{DueAt: "2026-05-10T00:00:00Z", DocumentID: "inv-2"})
	settled := f.postAt(t, f.cash, f.sales, 300, accounting.Entry{DueAt: "2026-05-05T00:00:00Z", DocumentID: "inv-3"})
	voided := f.postAt(t, f.cash, f.sales, 700, accounting.Entry{DueAt: "2026-05-06T00:00:00Z", DocumentID: "inv-4"})
	f.postAt(t, f.cash, f.sales, 5, accounting.Entry{DueAt: "2027-01-01T00:00:00Z", DocumentID: "inv-5"})
	f.postAt(t, f.sales, f.cash, 700, accounting.Entry{ReversesEntryID: voided.ID, DocumentID: "inv-4"})
	f.postAt(t, f.sales, f.cash, 200, accounting.Entry{SettlesEntryID: part.ID, DocumentID: "rcpt-2"})
	f.postAt(t, f.sales, f.cash, 300, accounting.Entry{SettlesEntryID: settled.ID, DocumentID: "rcpt-3"})

	got, err := f.repo.Overdue(f.bg, f.cash.ID, "2026-06-01T00:00:00Z", "")
	if err != nil {
		t.Fatalf("Overdue: %v", err)
	}
	if got.AgeFrom != accounting.AgeFromDue || got.AsOf != "2026-06-01T00:00:00Z" || got.AccountID != f.cash.ID {
		t.Fatalf("header %+v", got)
	}
	if len(got.Items) != 2 {
		t.Fatalf("items %+v", got.Items)
	}
	if got.Items[0].ID != unpaid.ID || got.Items[0].Outstanding != 1000 || got.Items[0].DaysOverdue != 31 {
		t.Fatalf("first %+v", got.Items[0])
	}
	if got.Items[1].ID != part.ID || got.Items[1].Outstanding != 400 {
		t.Fatalf("second %+v", got.Items[1])
	}
}

func TestOverdueCanAgeFromTheDayAChargeWasRaised(t *testing.T) {
	f := newStoreFixture(t)
	f.postAt(t, f.cash, f.sales, 100, accounting.Entry{DueAt: "2000-01-01T00:00:00Z"})
	byDue, err := f.repo.Overdue(f.bg, f.cash.ID, "2060-01-01T00:00:00Z", "due")
	if err != nil || len(byDue.Items) != 1 || byDue.Items[0].AgeDays != byDue.Items[0].DaysOverdue {
		t.Fatalf("by due %+v %v", byDue, err)
	}
	byRaised, err := f.repo.Overdue(f.bg, f.cash.ID, "2060-01-01T00:00:00Z", "raised")
	if err != nil || byRaised.AgeFrom != accounting.AgeFromRaised || len(byRaised.Items) != 1 {
		t.Fatalf("by raised %+v %v", byRaised, err)
	}
	if got := byRaised.Items[0]; got.AgeDays >= got.DaysOverdue || got.DaysOverdue != byDue.Items[0].DaysOverdue {
		t.Fatalf("raised after due must be younger than overdue: %+v", got)
	}
}

func TestOverdueRefusals(t *testing.T) {
	f := newStoreFixture(t)
	if _, err := f.repo.Overdue(f.bg, f.cash.ID, "2026-06-01T00:00:00Z", "posted"); !errors.Is(err, accounting.ErrInvalid) {
		t.Fatalf("basis: %v", err)
	}
	if _, err := f.repo.Overdue(f.bg, f.cash.ID, "June", ""); !errors.Is(err, accounting.ErrInvalid) {
		t.Fatalf("as_of: %v", err)
	}
	if _, err := f.repo.Overdue(f.bg, "missing", "2026-06-01T00:00:00Z", ""); !errors.Is(err, accounting.ErrNotFound) {
		t.Fatalf("account: %v", err)
	}
	none, err := f.repo.Overdue(f.bg, f.cash.ID, "", "")
	if err != nil || len(none.Items) != 0 || none.AsOf == "" {
		t.Fatalf("empty %+v %v", none, err)
	}
}
