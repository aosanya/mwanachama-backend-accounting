package accounting_test

import (
	"errors"
	"testing"

	accounting "github.com/aosanya/mwanachama-backend-accounting"
)

func TestABudgetLineWithNothingSpentIsDeletedAndReturned(t *testing.T) {
	f := newStoreFixture(t)
	line := f.setLine(t, "Road repair", "Public works", 1000)

	removed, err := f.repo.DeleteBudgetLine(f.bg, line.ID, "")
	if err != nil {
		t.Fatalf("DeleteBudgetLine: %v", err)
	}
	if removed.ID != line.ID || removed.Name != "Road repair" || removed.Planned != 1000 {
		t.Fatalf("removed %+v", removed)
	}
	if _, err := f.repo.GetBudgetLine(f.bg, line.ID); !errors.Is(err, accounting.ErrNotFound) {
		t.Fatalf("the line is still there: %v", err)
	}
	lines, err := f.repo.ListBudgetLines(f.bg, accounting.BudgetLineFilter{})
	if err != nil || len(lines) != 0 {
		t.Fatalf("lines %+v %v", lines, err)
	}
}

func TestADeletedBudgetLineLeavesRoomToSetTheSamePeriodAgain(t *testing.T) {
	f := newStoreFixture(t)
	line := f.setLine(t, "Road repair", "Public works", 1000)
	if _, err := f.repo.DeleteBudgetLine(f.bg, line.ID, ""); err != nil {
		t.Fatalf("DeleteBudgetLine: %v", err)
	}
	again, err := f.repo.SetBudget(f.bg, accounting.Budget{
		AccountID: f.rent.ID, PeriodStart: januaryStart, PeriodEnd: februaryStart,
		Name: "Road repair", Department: "Public works", Amount: 900, Create: true,
	})
	if err != nil || again.Amount != 900 {
		t.Fatalf("set again %+v %v", again, err)
	}
}

func TestABudgetLineWithSpendingInItsPeriodIsRefusedAndKept(t *testing.T) {
	f := newStoreFixture(t)
	f.postAt(t, f.rent, f.cash, 300, accounting.Entry{OccurredAt: "2026-01-10T00:00:00Z"})
	line := f.setLine(t, "Road repair", "Public works", 1000)

	if _, err := f.repo.DeleteBudgetLine(f.bg, line.ID, ""); !errors.Is(err, accounting.ErrBudgetLineInUse) {
		t.Fatalf("a line with spending was removed or refused for another reason: %v", err)
	}
	if got, err := f.repo.GetBudgetLine(f.bg, line.ID); err != nil || got.Spent != 300 {
		t.Fatalf("the refused line changed: %+v %v", got, err)
	}
}

func TestSpendingOutsideTheLinesPeriodDoesNotKeepTheLine(t *testing.T) {
	f := newStoreFixture(t)
	f.postAt(t, f.rent, f.cash, 9000, accounting.Entry{OccurredAt: "2026-02-03T00:00:00Z"})
	line := f.setLine(t, "Road repair", "Public works", 1000)

	if _, err := f.repo.DeleteBudgetLine(f.bg, line.ID, ""); err != nil {
		t.Fatalf("spending in another period kept the line: %v", err)
	}
}

func TestABudgetLineATenderDrawsOnIsRefusedAndKept(t *testing.T) {
	repo, line := procurementLedger(t)
	if _, err := repo.CreateTender(t.Context(), accounting.Tender{BudgetID: line.ID, Title: "Resurfacing of Mill Street", CreatedBy: "clerk"}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.DeleteBudgetLine(t.Context(), line.ID, ""); !errors.Is(err, accounting.ErrBudgetLineInUse) {
		t.Fatalf("a line a tender draws on was removed or refused for another reason: %v", err)
	}
	if _, err := repo.GetBudgetLine(t.Context(), line.ID); err != nil {
		t.Fatalf("the refused line is gone: %v", err)
	}
}

func TestDeletingAMissingBudgetLineIsNotFound(t *testing.T) {
	f := newStoreFixture(t)
	if _, err := f.repo.DeleteBudgetLine(f.bg, "missing", ""); !errors.Is(err, accounting.ErrNotFound) {
		t.Fatalf("missing line: %v", err)
	}
}

func TestDeletingWithAnOldVersionIsRefusedAndKeepsTheLine(t *testing.T) {
	f := newStoreFixture(t)
	line := f.setLine(t, "Road repair", "Public works", 1000)

	if _, err := f.repo.DeleteBudgetLine(f.bg, line.ID, "2020-01-01T00:00:00Z"); err == nil || err.Error() != "record changed since you read it" {
		t.Fatalf("an old version was accepted or refused with another words: %v", err)
	}
	if _, err := f.repo.GetBudgetLine(f.bg, line.ID); err != nil {
		t.Fatalf("the refused line is gone: %v", err)
	}
	if _, err := f.repo.DeleteBudgetLine(f.bg, line.ID, line.SetAt); err != nil {
		t.Fatalf("the current version was refused: %v", err)
	}
}

func TestTheDeletedBudgetLineKeepsItsMilestoneSwitchAndCostShares(t *testing.T) {
	f := newStoreFixture(t)
	set, err := f.repo.SetBudget(f.bg, accounting.Budget{
		AccountID: f.rent.ID, PeriodStart: januaryStart, PeriodEnd: februaryStart,
		Name: "Road repair", Department: "Public works", Amount: 1000,
		FollowMilestones: true, CostShares: map[string]int64{"Northfield": 60, "Southfield": 40},
	})
	if err != nil {
		t.Fatalf("SetBudget: %v", err)
	}
	removed, err := f.repo.DeleteBudgetLine(f.bg, set.ID, "")
	if err != nil {
		t.Fatalf("DeleteBudgetLine: %v", err)
	}
	if !removed.FollowMilestones || removed.CostShares["Northfield"] != 60 || removed.CostShares["Southfield"] != 40 {
		t.Fatalf("removed %+v", removed)
	}
}
