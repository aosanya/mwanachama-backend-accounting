package accounting_test

import (
	"errors"
	"testing"

	accounting "github.com/aosanya/mwanachama-backend-accounting"
)

const marchStart = "2026-03-01T00:00:00Z"

func (f storeFixture) setLineOn(t *testing.T, account accounting.Account, start, end, name, department string, planned int64) {
	t.Helper()
	if _, err := f.repo.SetBudget(f.bg, accounting.Budget{
		AccountID: account.ID, PeriodStart: start, PeriodEnd: end, Name: name, Department: department, Amount: planned,
	}); err != nil {
		t.Fatalf("SetBudget: %v", err)
	}
}

func TestAReportFoldsEveryBudgetLineOfOnePeriodIntoTotalsAndDepartments(t *testing.T) {
	f := newStoreFixture(t)
	f.postAt(t, f.rent, f.cash, 300, accounting.Entry{OccurredAt: "2026-01-10T00:00:00Z"})
	f.postAt(t, f.rent, f.cash, 50, accounting.Entry{OccurredAt: "2026-01-20T00:00:00Z"})
	f.postAt(t, f.rent, f.cash, 9000, accounting.Entry{OccurredAt: "2026-02-03T00:00:00Z"})
	f.setLineOn(t, f.rent, januaryStart, februaryStart, "Road repair", "Public works", 1000)
	f.setLineOn(t, f.sales, januaryStart, februaryStart, "Market fees", "Markets", 400)

	got, err := f.repo.GetReport(f.bg, januaryStart, februaryStart)
	if err != nil {
		t.Fatalf("GetReport: %v", err)
	}
	if got.Name != "Budget and actuals 2026-01-01 to 2026-02-01" {
		t.Fatalf("name %q", got.Name)
	}
	if got.PeriodStart != januaryStart || got.PeriodEnd != februaryStart {
		t.Fatalf("period %s %s", got.PeriodStart, got.PeriodEnd)
	}
	if len(got.Lines) != 2 {
		t.Fatalf("lines %+v", got.Lines)
	}
	figures := got.Figures
	if figures.Planned != 1400 || figures.Spent != 350 || figures.Difference != 1050 {
		t.Fatalf("figures %+v", figures)
	}
	if len(figures.Departments) != 2 {
		t.Fatalf("departments %+v", figures.Departments)
	}
	works := figures.Departments[1]
	if figures.Departments[0].Department != "Markets" || works.Department != "Public works" || works.Planned != 1000 || works.Spent != 350 || works.Difference != 650 {
		t.Fatalf("public works %+v", works)
	}
}

func TestAReportDifferenceIsPlannedMinusSpentForTheTotalsAndEveryDepartment(t *testing.T) {
	f := newStoreFixture(t)
	f.postAt(t, f.rent, f.cash, 300, accounting.Entry{OccurredAt: "2026-01-10T00:00:00Z"})
	f.postAt(t, f.rent, f.cash, 50, accounting.Entry{OccurredAt: "2026-01-20T00:00:00Z"})
	f.setLineOn(t, f.rent, januaryStart, februaryStart, "Road repair", "Public works", 1000)
	f.setLineOn(t, f.sales, januaryStart, februaryStart, "Market fees", "Markets", 400)

	got, err := f.repo.GetReport(f.bg, januaryStart, februaryStart)
	if err != nil {
		t.Fatalf("GetReport: %v", err)
	}
	figures := got.Figures
	if figures.Planned != 1400 || figures.Spent != 350 || figures.Difference != 1050 {
		t.Fatalf("figures %+v", figures)
	}
	if figures.Planned-figures.Spent != figures.Difference {
		t.Fatalf("planned %d minus spent %d is not difference %d", figures.Planned, figures.Spent, figures.Difference)
	}
	if len(figures.Departments) != 2 {
		t.Fatalf("departments %+v", figures.Departments)
	}
	for _, d := range figures.Departments {
		if d.Planned-d.Spent != d.Difference {
			t.Fatalf("department %s planned %d minus spent %d is not difference %d", d.Department, d.Planned, d.Spent, d.Difference)
		}
		if d.Department == "Public works" && d.Difference != 650 {
			t.Fatalf("public works %+v", d)
		}
	}
}

func TestAReportSpentMatchesTheVarianceOfEachLine(t *testing.T) {
	f := newStoreFixture(t)
	f.postAt(t, f.rent, f.cash, 300, accounting.Entry{OccurredAt: "2026-01-10T00:00:00Z"})
	f.setLineOn(t, f.rent, januaryStart, februaryStart, "Road repair", "Public works", 1000)

	report, err := f.repo.GetReport(f.bg, januaryStart, februaryStart)
	if err != nil {
		t.Fatal(err)
	}
	variance, err := f.repo.Variance(f.bg, f.rent.ID, januaryStart, februaryStart)
	if err != nil {
		t.Fatal(err)
	}
	if report.Figures.Spent != variance.Actual || report.Figures.Difference != -variance.Difference {
		t.Fatalf("report %+v, variance %+v", report.Figures, variance)
	}
}

func TestAReportCarriesTheClosingBalanceOfEachAccountTypeAtThePeriodEnd(t *testing.T) {
	f := newStoreFixture(t)
	f.postAt(t, f.cash, f.sales, 700, accounting.Entry{OccurredAt: "2026-01-05T00:00:00Z"})
	f.postAt(t, f.rent, f.cash, 200, accounting.Entry{OccurredAt: "2026-01-15T00:00:00Z"})
	f.postAt(t, f.rent, f.cash, 5000, accounting.Entry{OccurredAt: "2026-02-15T00:00:00Z"})
	f.setLineOn(t, f.rent, januaryStart, februaryStart, "Road repair", "Public works", 1000)

	got, err := f.repo.GetReport(f.bg, januaryStart, februaryStart)
	if err != nil {
		t.Fatal(err)
	}
	byType := map[accounting.AccountType]int64{}
	for _, b := range got.Figures.Balances {
		byType[b.Type] = b.Balance
	}
	if byType[accounting.AccountTypeAsset] != 500 || byType[accounting.AccountTypeIncome] != -700 || byType[accounting.AccountTypeExpense] != 200 {
		t.Fatalf("balances %+v", got.Figures.Balances)
	}
}

func TestEachDistinctPeriodOfBudgetLinesIsOneReportInPeriodOrder(t *testing.T) {
	f := newStoreFixture(t)
	f.setLineOn(t, f.rent, februaryStart, marchStart, "Road repair", "Public works", 800)
	f.setLineOn(t, f.rent, januaryStart, februaryStart, "Road repair", "Public works", 1000)
	f.setLineOn(t, f.sales, januaryStart, februaryStart, "Market fees", "Markets", 400)

	reports, err := f.repo.ListReports(f.bg, accounting.ReportFilter{})
	if err != nil {
		t.Fatalf("ListReports: %v", err)
	}
	if len(reports) != 2 || reports[0].PeriodStart != januaryStart || reports[1].PeriodStart != februaryStart {
		t.Fatalf("reports %+v", reports)
	}
	if reports[0].Figures.Planned != 1400 || reports[1].Figures.Planned != 800 {
		t.Fatalf("planned %d %d", reports[0].Figures.Planned, reports[1].Figures.Planned)
	}
}

func TestListingReportsHonoursTheLimit(t *testing.T) {
	f := newStoreFixture(t)
	f.setLineOn(t, f.rent, januaryStart, februaryStart, "Road repair", "Public works", 1000)
	f.setLineOn(t, f.rent, februaryStart, marchStart, "Road repair", "Public works", 800)
	reports, err := f.repo.ListReports(f.bg, accounting.ReportFilter{Limit: 1})
	if err != nil || len(reports) != 1 {
		t.Fatalf("reports %+v %v", reports, err)
	}
}

func TestAReportForAPeriodWithNoBudgetLinesIsNotFound(t *testing.T) {
	f := newStoreFixture(t)
	_, err := f.repo.GetReport(f.bg, januaryStart, februaryStart)
	if !errors.Is(err, accounting.ErrNotFound) {
		t.Fatalf("err %v", err)
	}
}

func TestAReportWithAnUnreadablePeriodIsInvalid(t *testing.T) {
	f := newStoreFixture(t)
	if _, err := f.repo.GetReport(f.bg, "January", februaryStart); !errors.Is(err, accounting.ErrInvalid) {
		t.Fatalf("err %v", err)
	}
	if _, err := f.repo.GetReport(f.bg, februaryStart, januaryStart); !errors.Is(err, accounting.ErrInvalid) {
		t.Fatalf("reversed err %v", err)
	}
}
