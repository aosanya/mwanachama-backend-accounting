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

const (
	lakeshoreRow = "3f2a9c1e-7b44-4d0e-9a51-5c8e2d6f1a10"
	harborRow    = "b7d1e0c4-2a96-4f38-8c05-1e9a4b7d3f22"
)

func (f storeFixture) sendTo(t *testing.T, recipient, row, reference string) accounting.UpwardReport {
	t.Helper()
	sent, err := f.repo.SendUpwardReport(f.bg, accounting.UpwardReportInput{
		PeriodStart: januaryStart, PeriodEnd: februaryStart, Kind: "budget_and_actuals", SentBy: "officer-zoldhalom",
		Recipient: recipient, RecipientRecordRowID: row, Reference: reference,
	})
	if err != nil {
		t.Fatalf("SendUpwardReport to %q: %v", recipient, err)
	}
	return sent
}

func TestASendNamingARecipientKeepsItAsSentAndListsBackWithIt(t *testing.T) {
	f := newStoreFixture(t)
	f.zoldhalomJanuary(t)

	sent := f.sendTo(t, "Lakeshore Fund", lakeshoreRow, "LF-2026-0417")
	if sent.Recipient != "Lakeshore Fund" || sent.RecipientRecordRowID != lakeshoreRow || sent.Reference != "LF-2026-0417" {
		t.Fatalf("sent %+v", sent)
	}
	listed, err := f.repo.ListUpwardReports(f.bg, accounting.UpwardReportFilter{})
	if err != nil || len(listed) != 1 {
		t.Fatalf("listed %+v %v", listed, err)
	}
	if listed[0].Recipient != "Lakeshore Fund" || listed[0].RecipientRecordRowID != lakeshoreRow || listed[0].Reference != "LF-2026-0417" {
		t.Fatalf("listed back %+v", listed[0])
	}
}

func TestASendWithoutARecipientReadsAsBefore(t *testing.T) {
	f := newStoreFixture(t)
	f.zoldhalomJanuary(t)

	sent := f.send(t, "budget_and_actuals")
	if sent.Recipient != "" || sent.RecipientRecordRowID != "" || sent.Reference != "" {
		t.Fatalf("sent %+v", sent)
	}
	listed, err := f.repo.ListUpwardReports(f.bg, accounting.UpwardReportFilter{})
	if err != nil || len(listed) != 1 || listed[0].Recipient != "" || listed[0].RecipientRecordRowID != "" || listed[0].Reference != "" {
		t.Fatalf("listed %+v %v", listed, err)
	}
}

func TestListingByRecipientRecordRowIDKeepsOnlyThatRecipientsReportsNewestFirst(t *testing.T) {
	f := newStoreFixture(t)
	f.zoldhalomJanuary(t)
	first := f.sendTo(t, "Lakeshore Fund", lakeshoreRow, "LF-2026-0417")
	f.sendTo(t, "Harbor Trust", harborRow, "HT-77")
	f.send(t, "spending_by_department")
	second := f.sendTo(t, "Lakeshore Fund", lakeshoreRow, "LF-2026-0418")

	listed, err := f.repo.ListUpwardReports(f.bg, accounting.UpwardReportFilter{RecipientRecordRowID: lakeshoreRow})
	if err != nil || len(listed) != 2 {
		t.Fatalf("listed %+v %v", listed, err)
	}
	if listed[0].ID != second.ID || listed[1].ID != first.ID {
		t.Fatalf("order %s %s", listed[0].ID, listed[1].ID)
	}
	none, err := f.repo.ListUpwardReports(f.bg, accounting.UpwardReportFilter{RecipientRecordRowID: "0d6c5b4a-9e83-4a71-b2f0-6a1c8e3d5b97"})
	if err != nil || none == nil || len(none) != 0 {
		t.Fatalf("none %#v %v", none, err)
	}
	limited, err := f.repo.ListUpwardReports(f.bg, accounting.UpwardReportFilter{RecipientRecordRowID: lakeshoreRow, Limit: 1})
	if err != nil || len(limited) != 1 || limited[0].ID != second.ID {
		t.Fatalf("limited %+v %v", limited, err)
	}
}

func TestABlankRecipientRecordRowIDFilterListsEveryReport(t *testing.T) {
	f := newStoreFixture(t)
	f.zoldhalomJanuary(t)
	f.sendTo(t, "Lakeshore Fund", lakeshoreRow, "")
	f.send(t, "spending_by_department")

	for _, row := range []string{"", "  "} {
		listed, err := f.repo.ListUpwardReports(f.bg, accounting.UpwardReportFilter{RecipientRecordRowID: row})
		if err != nil || len(listed) != 2 {
			t.Fatalf("filter %q: listed %+v %v", row, listed, err)
		}
	}
}

func TestARecipientRecordRowIDThatIsNotARowIDIsRefusedAndNothingIsStored(t *testing.T) {
	f := newStoreFixture(t)
	f.zoldhalomJanuary(t)
	for _, row := range []string{"Lakeshore Fund", "3f2a9c1e", "3f2a9c1e-7b44-4d0e-9a51-5c8e2d6f1a1"} {
		_, err := f.repo.SendUpwardReport(f.bg, accounting.UpwardReportInput{
			PeriodStart: januaryStart, PeriodEnd: februaryStart, Kind: "budget_and_actuals", SentBy: "officer-zoldhalom",
			RecipientRecordRowID: row,
		})
		if !errors.Is(err, accounting.ErrInvalid) {
			t.Fatalf("row %q: err %v", row, err)
		}
	}
	listed, err := f.repo.ListUpwardReports(f.bg, accounting.UpwardReportFilter{})
	if err != nil || len(listed) != 0 {
		t.Fatalf("listed %+v %v", listed, err)
	}
}

func TestTheRecipientAndReferenceAreStoredTrimmed(t *testing.T) {
	f := newStoreFixture(t)
	f.zoldhalomJanuary(t)
	sent := f.sendTo(t, "  Lakeshore Fund  ", lakeshoreRow, " LF-2026-0417 ")
	if sent.Recipient != "Lakeshore Fund" || sent.Reference != "LF-2026-0417" {
		t.Fatalf("sent %+v", sent)
	}
}
