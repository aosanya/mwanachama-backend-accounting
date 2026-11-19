package accounting_test

import (
	"testing"

	accounting "github.com/aosanya/mwanachama-backend-accounting"
)

func (f storeFixture) setLineWith(t *testing.T, planned int64, version string) (accounting.Budget, error) {
	t.Helper()
	return f.repo.SetBudget(f.bg, accounting.Budget{
		AccountID: f.rent.ID, PeriodStart: januaryStart, PeriodEnd: februaryStart,
		Name: "Road repair", Department: "Public works", Amount: planned, Version: version,
	})
}

func TestSettingALineOnTheVersionItWasReadIsAcceptedAndStoresNoVersion(t *testing.T) {
	f := newStoreFixture(t)
	first := f.setLine(t, "Road repair", "Public works", 1000)

	second, err := f.setLineWith(t, 1200, first.SetAt)
	if err != nil {
		t.Fatalf("set on the current version: %v", err)
	}
	if second.Amount != 1200 || second.Version != "" || second.ID != first.ID {
		t.Fatalf("second %+v", second)
	}
}

func TestSettingALineOnAnOldVersionIsRefusedNamingTheAmountAndKeepsTheLine(t *testing.T) {
	f := newStoreFixture(t)
	f.setLine(t, "Road repair", "Public works", 1000)

	_, err := f.setLineWith(t, 1200, "2020-01-01T00:00:00Z")
	if err == nil || err.Error() != "amount changed since you read it" {
		t.Fatalf("refusal %v", err)
	}
	lines, err := f.repo.ListBudgetLines(f.bg, accounting.BudgetLineFilter{})
	if err != nil || len(lines) != 1 || lines[0].Planned != 1000 {
		t.Fatalf("a refused save changed the line: %+v %v", lines, err)
	}
}

func TestSettingANewLineWithAVersionHasNothingToBeStaleAgainst(t *testing.T) {
	f := newStoreFixture(t)
	if _, err := f.setLineWith(t, 1000, "2020-01-01T00:00:00Z"); err != nil {
		t.Fatalf("first set carrying a version: %v", err)
	}
}
