package accounting_test

import (
	"errors"
	"strings"
	"testing"

	accounting "github.com/aosanya/mwanachama-backend-accounting"
)

const (
	reefDonorRow = "5c0e8a17-3d92-4b6f-a1e4-9f7b2c6d8e03"
	kelpDonorRow = "e1a47f90-6b2d-4c83-95f0-3a8d1c7b5e44"
)

func (f storeFixture) twoProjectsInJanuary(t *testing.T) {
	t.Helper()
	travel, err := f.repo.OpenAccount(f.bg, accounting.Account{Code: "5100", Category: "travel", HolderID: "hub-1", Type: accounting.AccountTypeExpense})
	if err != nil {
		t.Fatalf("OpenAccount: %v", err)
	}
	f.setLineOn(t, f.rent, januaryStart, februaryStart, "Reef survey fees", "reef_survey", 1000)
	f.setLineOn(t, travel, januaryStart, februaryStart, "Kelp study travel", "kelp_study", 400)
	f.postAt(t, f.rent, f.cash, 300, accounting.Entry{OccurredAt: "2026-01-10T00:00:00Z", Dimensions: map[string]string{"project": "reef_survey"}})
	f.postAt(t, f.rent, f.cash, 50, accounting.Entry{OccurredAt: "2026-01-12T00:00:00Z", Dimensions: map[string]string{"project": "kelp_study"}})
	f.postAt(t, f.rent, f.cash, 70, accounting.Entry{OccurredAt: "2026-01-15T00:00:00Z"})
	f.postAt(t, travel, f.cash, 120, accounting.Entry{OccurredAt: "2026-01-18T00:00:00Z", Dimensions: map[string]string{"project": "kelp_study"}})
}

func (f storeFixture) sendForProject(t *testing.T, project, recipient, row, reference string) accounting.UpwardReport {
	t.Helper()
	sent, err := f.repo.SendUpwardReport(f.bg, accounting.UpwardReportInput{
		PeriodStart: januaryStart, PeriodEnd: februaryStart, Kind: "budget_and_actuals", SentBy: "officer-zoldhalom",
		Project: project, Recipient: recipient, RecipientRecordRowID: row, Reference: reference,
	})
	if err != nil {
		t.Fatalf("SendUpwardReport for %q: %v", project, err)
	}
	return sent
}

func TestAReportSentForAProjectFoldsOnlyThatProjectsLinesAndSpending(t *testing.T) {
	f := newStoreFixture(t)
	f.twoProjectsInJanuary(t)

	sent := f.sendForProject(t, "reef_survey", "", "", "")
	if sent.Project != "reef_survey" || sent.Planned != 1000 || sent.Spent != 300 || sent.Difference != 700 {
		t.Fatalf("reef report %+v", sent)
	}
	if len(sent.Lines) != 1 || sent.Lines[0].Name != "Reef survey fees" || sent.Lines[0].Spent != 300 {
		t.Fatalf("reef lines %+v", sent.Lines)
	}
	if len(sent.Departments) != 1 || sent.Departments[0].Department != "reef_survey" {
		t.Fatalf("reef departments %+v", sent.Departments)
	}

	kelp := f.sendForProject(t, "kelp_study", "", "", "")
	if kelp.Planned != 400 || kelp.Spent != 120 || len(kelp.Lines) != 1 || kelp.Lines[0].Name != "Kelp study travel" {
		t.Fatalf("kelp report %+v", kelp)
	}

	whole := f.send(t, "budget_and_actuals")
	if whole.Project != "" || whole.Planned != 1400 || whole.Spent != 300+50+70+120 || len(whole.Lines) != 2 {
		t.Fatalf("a report sent without a project covers the whole period: %+v", whole)
	}
}

func TestAProjectWithNoBudgetLineInThePeriodIsRefusedAndNothingIsStored(t *testing.T) {
	f := newStoreFixture(t)
	f.twoProjectsInJanuary(t)

	_, err := f.repo.SendUpwardReport(f.bg, accounting.UpwardReportInput{
		PeriodStart: januaryStart, PeriodEnd: februaryStart, Kind: "budget_and_actuals", SentBy: "officer-zoldhalom", Project: "coral_atlas",
	})
	if !errors.Is(err, accounting.ErrNotFound) {
		t.Fatalf("err %v, want ErrNotFound", err)
	}
	listed, err := f.repo.ListUpwardReports(f.bg, accounting.UpwardReportFilter{})
	if err != nil || len(listed) != 0 {
		t.Fatalf("stored after a refusal: %+v %v", listed, err)
	}
}

func TestAProjectThatIsNotAValidDimensionValueIsRefusedOnAReport(t *testing.T) {
	f := newStoreFixture(t)
	f.twoProjectsInJanuary(t)

	_, err := f.repo.SendUpwardReport(f.bg, accounting.UpwardReportInput{
		PeriodStart: januaryStart, PeriodEnd: februaryStart, Kind: "budget_and_actuals", SentBy: "officer-zoldhalom",
		Project: strings.Repeat("p", accounting.MaxDimensionValue+1),
	})
	if !errors.Is(err, accounting.ErrInvalid) {
		t.Fatalf("err %v, want ErrInvalid", err)
	}
}

func TestReportsAreListedByProjectAndByDonorTogether(t *testing.T) {
	f := newStoreFixture(t)
	f.twoProjectsInJanuary(t)
	reefToReefDonor := f.sendForProject(t, " reef_survey ", "Reef Fund", reefDonorRow, "RF-11")
	reefToKelpDonor := f.sendForProject(t, "reef_survey", "Kelp Trust", kelpDonorRow, "KT-90")
	kelpToKelpDonor := f.sendForProject(t, "kelp_study", "Kelp Trust", kelpDonorRow, "KT-91")
	f.send(t, "budget_and_actuals")

	if reefToReefDonor.Project != "reef_survey" {
		t.Fatalf("the project is kept trimmed: %q", reefToReefDonor.Project)
	}
	byProject, err := f.repo.ListUpwardReports(f.bg, accounting.UpwardReportFilter{Project: "reef_survey"})
	if err != nil || len(byProject) != 2 {
		t.Fatalf("by project: %+v %v", byProject, err)
	}
	byDonor, err := f.repo.ListUpwardReports(f.bg, accounting.UpwardReportFilter{RecipientRecordRowID: kelpDonorRow})
	if err != nil || len(byDonor) != 2 {
		t.Fatalf("by donor: %+v %v", byDonor, err)
	}
	both, err := f.repo.ListUpwardReports(f.bg, accounting.UpwardReportFilter{RecipientRecordRowID: kelpDonorRow, Project: "reef_survey"})
	if err != nil || len(both) != 1 || both[0].ID != reefToKelpDonor.ID {
		t.Fatalf("by donor and project: %+v %v", both, err)
	}
	if kelpToKelpDonor.Reference != "KT-91" {
		t.Fatalf("reference %q", kelpToKelpDonor.Reference)
	}
	everyReport, err := f.repo.ListUpwardReports(f.bg, accounting.UpwardReportFilter{})
	if err != nil || len(everyReport) != 4 {
		t.Fatalf("every report: %d %v", len(everyReport), err)
	}
}
