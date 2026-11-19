package accounting_test

import (
	"errors"
	"testing"

	accounting "github.com/aosanya/mwanachama-backend-accounting"
)

func (f storeFixture) zoldhalomJanuary(t *testing.T) {
	t.Helper()
	f.postAt(t, f.rent, f.cash, 300, accounting.Entry{OccurredAt: "2026-01-10T00:00:00Z"})
	f.postAt(t, f.rent, f.cash, 50, accounting.Entry{OccurredAt: "2026-01-20T00:00:00Z"})
	f.setLineOn(t, f.rent, januaryStart, februaryStart, "Road repair", "Public works", 1000)
	f.setLineOn(t, f.sales, januaryStart, februaryStart, "Market fees", "Markets", 400)
}

func (f storeFixture) send(t *testing.T, kind string) accounting.UpwardReport {
	t.Helper()
	sent, err := f.repo.SendUpwardReport(f.bg, accounting.UpwardReportInput{
		PeriodStart: januaryStart, PeriodEnd: februaryStart, Kind: accounting.UpwardReportKind(kind), SentBy: "officer-zoldhalom",
	})
	if err != nil {
		t.Fatalf("SendUpwardReport %s: %v", kind, err)
	}
	return sent
}

func TestSendingBudgetAndActualsStoresTheFoldedFiguresAndEveryBudgetLine(t *testing.T) {
	f := newStoreFixture(t)
	f.zoldhalomJanuary(t)
	folded, err := f.repo.GetReport(f.bg, januaryStart, februaryStart)
	if err != nil {
		t.Fatal(err)
	}

	sent := f.send(t, "budget_and_actuals")
	if sent.ID == "" || sent.SentAt == "" || sent.SentBy != "officer-zoldhalom" || sent.Kind != "budget_and_actuals" {
		t.Fatalf("sent %+v", sent)
	}
	if sent.PeriodStart != januaryStart || sent.PeriodEnd != februaryStart {
		t.Fatalf("period %s %s", sent.PeriodStart, sent.PeriodEnd)
	}
	if sent.Planned != folded.Figures.Planned || sent.Spent != folded.Figures.Spent || sent.Difference != folded.Figures.Difference {
		t.Fatalf("totals %d %d %d, report %+v", sent.Planned, sent.Spent, sent.Difference, folded.Figures)
	}
	if sent.Planned != 1400 || sent.Spent != 350 || sent.Difference != 1050 {
		t.Fatalf("totals %+v", sent)
	}
	if len(sent.Departments) != 2 || sent.Departments[0].Department != "Markets" || sent.Departments[1].Spent != 350 {
		t.Fatalf("departments %+v", sent.Departments)
	}
	if len(sent.Lines) != 2 {
		t.Fatalf("lines %+v", sent.Lines)
	}
}

func TestSendingSpendingByDepartmentCarriesNoBudgetLines(t *testing.T) {
	f := newStoreFixture(t)
	f.zoldhalomJanuary(t)

	sent := f.send(t, "spending_by_department")
	if len(sent.Lines) != 0 {
		t.Fatalf("lines %+v", sent.Lines)
	}
	if len(sent.Departments) != 2 || sent.Planned != 1400 {
		t.Fatalf("sent %+v", sent)
	}

	listed, err := f.repo.ListUpwardReports(f.bg, accounting.UpwardReportFilter{})
	if err != nil || len(listed) != 1 || len(listed[0].Lines) != 0 || len(listed[0].Departments) != 2 {
		t.Fatalf("listed %+v %v", listed, err)
	}
}

func TestAnUnknownKindIsRefusedAndNothingIsStored(t *testing.T) {
	f := newStoreFixture(t)
	f.zoldhalomJanuary(t)
	for _, kind := range []string{"", "spending_by_neighbourhood", "Budget_And_Actuals"} {
		_, err := f.repo.SendUpwardReport(f.bg, accounting.UpwardReportInput{
			PeriodStart: januaryStart, PeriodEnd: februaryStart, Kind: accounting.UpwardReportKind(kind), SentBy: "officer-zoldhalom",
		})
		if !errors.Is(err, accounting.ErrInvalid) {
			t.Fatalf("kind %q: err %v", kind, err)
		}
	}
	listed, err := f.repo.ListUpwardReports(f.bg, accounting.UpwardReportFilter{})
	if err != nil || len(listed) != 0 {
		t.Fatalf("listed %+v %v", listed, err)
	}
}

func TestAPeriodWithNoBudgetLinesIsRefusedWithTheWordsOfTheReportRead(t *testing.T) {
	f := newStoreFixture(t)
	_, readErr := f.repo.GetReport(f.bg, januaryStart, februaryStart)
	_, err := f.repo.SendUpwardReport(f.bg, accounting.UpwardReportInput{
		PeriodStart: januaryStart, PeriodEnd: februaryStart, Kind: "budget_and_actuals", SentBy: "officer-zoldhalom",
	})
	if !errors.Is(err, accounting.ErrNotFound) || readErr == nil || err.Error() != readErr.Error() {
		t.Fatalf("send err %v, read err %v", err, readErr)
	}
	listed, _ := f.repo.ListUpwardReports(f.bg, accounting.UpwardReportFilter{})
	if len(listed) != 0 {
		t.Fatalf("listed %+v", listed)
	}
}

func TestAnUnreadablePeriodOrAMissingSenderIsRefused(t *testing.T) {
	f := newStoreFixture(t)
	f.zoldhalomJanuary(t)
	refused := []accounting.UpwardReportInput{
		{PeriodStart: "January", PeriodEnd: februaryStart, Kind: "budget_and_actuals", SentBy: "officer-zoldhalom"},
		{PeriodStart: februaryStart, PeriodEnd: januaryStart, Kind: "budget_and_actuals", SentBy: "officer-zoldhalom"},
		{PeriodStart: januaryStart, PeriodEnd: februaryStart, Kind: "budget_and_actuals", SentBy: " "},
	}
	for _, in := range refused {
		if _, err := f.repo.SendUpwardReport(f.bg, in); !errors.Is(err, accounting.ErrInvalid) {
			t.Fatalf("SendUpwardReport(%+v) = %v, want ErrInvalid", in, err)
		}
	}
}

func TestASecondSendOfTheSamePeriodAndKindIsAnotherRecordWithTheFiguresOfThatMoment(t *testing.T) {
	f := newStoreFixture(t)
	f.zoldhalomJanuary(t)
	first := f.send(t, "spending_by_department")
	f.postAt(t, f.rent, f.cash, 100, accounting.Entry{OccurredAt: "2026-01-25T00:00:00Z"})
	second := f.send(t, "spending_by_department")

	if first.ID == second.ID {
		t.Fatalf("one id %s for two sends", first.ID)
	}
	if first.Spent != 350 || second.Spent != 450 {
		t.Fatalf("spent %d then %d", first.Spent, second.Spent)
	}
	listed, err := f.repo.ListUpwardReports(f.bg, accounting.UpwardReportFilter{})
	if err != nil || len(listed) != 2 {
		t.Fatalf("listed %+v %v", listed, err)
	}
	if listed[0].ID != second.ID || listed[1].ID != first.ID || listed[1].Spent != 350 {
		t.Fatalf("order %s %s, first spent %d", listed[0].ID, listed[1].ID, listed[1].Spent)
	}
}

func TestListingUpwardReportsHonoursTheLimitAndIsEmptyWhenNothingWasSent(t *testing.T) {
	f := newStoreFixture(t)
	f.zoldhalomJanuary(t)
	empty, err := f.repo.ListUpwardReports(f.bg, accounting.UpwardReportFilter{})
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("empty %#v %v", empty, err)
	}
	f.send(t, "budget_and_actuals")
	newest := f.send(t, "spending_by_department")
	limited, err := f.repo.ListUpwardReports(f.bg, accounting.UpwardReportFilter{Limit: 1})
	if err != nil || len(limited) != 1 || limited[0].ID != newest.ID {
		t.Fatalf("limited %+v %v", limited, err)
	}
}
