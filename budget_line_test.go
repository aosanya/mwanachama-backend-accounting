package accounting_test

import (
	"errors"
	"reflect"
	"testing"

	accounting "github.com/aosanya/mwanachama-backend-accounting"
)

func (f storeFixture) setLine(t *testing.T, name, department string, planned int64) accounting.Budget {
	t.Helper()
	b, err := f.repo.SetBudget(f.bg, accounting.Budget{
		AccountID: f.rent.ID, PeriodStart: januaryStart, PeriodEnd: februaryStart,
		Name: name, Department: department, Amount: planned,
	})
	if err != nil {
		t.Fatalf("SetBudget: %v", err)
	}
	return b
}

func TestABudgetLineReadsBackWithItsNameDepartmentPlannedAndSpent(t *testing.T) {
	f := newStoreFixture(t)
	f.postAt(t, f.rent, f.cash, 300, accounting.Entry{OccurredAt: "2026-01-10T00:00:00Z"})
	f.postAt(t, f.rent, f.cash, 9000, accounting.Entry{OccurredAt: "2026-02-03T00:00:00Z"})
	set := f.setLine(t, "Road repair", "Public works", 1000)

	got, err := f.repo.GetBudgetLine(f.bg, set.ID)
	if err != nil {
		t.Fatalf("GetBudgetLine: %v", err)
	}
	want := accounting.BudgetLine{
		ID: set.ID, Name: "Road repair", Department: "Public works", AccountID: f.rent.ID,
		PeriodStart: januaryStart, PeriodEnd: februaryStart, Planned: 1000, Spent: 300, Currency: "XXX", SetAt: set.SetAt,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("line %+v, want %+v", got, want)
	}
}

func TestSettingTheSameLineAgainKeepsOneRowWithTheNewName(t *testing.T) {
	f := newStoreFixture(t)
	first := f.setLine(t, "Road repair", "Public works", 1000)
	second := f.setLine(t, "Road and drain repair", "Public works", 1500)
	if second.ID != first.ID {
		t.Fatalf("second set made a new row: %s then %s", first.ID, second.ID)
	}
	lines, err := f.repo.ListBudgetLines(f.bg, accounting.BudgetLineFilter{})
	if err != nil || len(lines) != 1 || lines[0].Name != "Road and drain repair" || lines[0].Planned != 1500 {
		t.Fatalf("lines %+v %v", lines, err)
	}
}

func TestListingBudgetLinesNarrowsByDepartmentAndAccount(t *testing.T) {
	f := newStoreFixture(t)
	f.setLine(t, "Road repair", "Public works", 1000)
	if _, err := f.repo.SetBudget(f.bg, accounting.Budget{
		AccountID: f.sales.ID, PeriodStart: januaryStart, PeriodEnd: februaryStart,
		Name: "Market fees", Department: "Markets", Amount: 400,
	}); err != nil {
		t.Fatalf("SetBudget: %v", err)
	}

	all, err := f.repo.ListBudgetLines(f.bg, accounting.BudgetLineFilter{})
	if err != nil || len(all) != 2 {
		t.Fatalf("all %+v %v", all, err)
	}
	works, err := f.repo.ListBudgetLines(f.bg, accounting.BudgetLineFilter{Department: "Public works"})
	if err != nil || len(works) != 1 || works[0].Name != "Road repair" {
		t.Fatalf("department %+v %v", works, err)
	}
	onSales, err := f.repo.ListBudgetLines(f.bg, accounting.BudgetLineFilter{AccountID: f.sales.ID})
	if err != nil || len(onSales) != 1 || onSales[0].Name != "Market fees" {
		t.Fatalf("account %+v %v", onSales, err)
	}
	capped, err := f.repo.ListBudgetLines(f.bg, accounting.BudgetLineFilter{Limit: 1})
	if err != nil || len(capped) != 1 {
		t.Fatalf("limit %+v %v", capped, err)
	}
}

func TestAMissingBudgetLineIsNotFound(t *testing.T) {
	f := newStoreFixture(t)
	if _, err := f.repo.GetBudgetLine(f.bg, "missing"); !errors.Is(err, accounting.ErrNotFound) {
		t.Fatalf("err %v", err)
	}
}
