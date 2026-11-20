package accounting_test

import (
	"errors"
	"strings"
	"testing"

	accounting "github.com/aosanya/mwanachama-backend-accounting"
)

func (f storeFixture) billOnProject(t *testing.T, receivable accounting.Account, project string, amount int64) accounting.Bill {
	t.Helper()
	b, err := f.repo.CreateBill(f.bg, accounting.BillInput{
		IncomeAccountID: f.sales.ID, ReceivableAccountID: receivable.ID, Payer: "Harbour Savings Union", Project: project,
		Amount: amount, DueAt: billDue, BilledBy: billedByClrk,
	})
	if err != nil {
		t.Fatalf("CreateBill on %q: %v", project, err)
	}
	return b
}

func TestABillRaisedOnAProjectReadsBackWithIt(t *testing.T) {
	f := newStoreFixture(t)
	rec := f.receivable(t)
	made := f.billOnProject(t, rec, "reef_survey", 5000)

	got, err := f.repo.GetBill(f.bg, made.ID)
	if err != nil || got.Project != "reef_survey" || made.Project != "reef_survey" {
		t.Fatalf("bill %+v %v", got, err)
	}
	plain := f.bill(t, rec, "stall-holder-7", 100, billDue)
	if plain.Project != "" {
		t.Fatalf("a bill raised without a project carries %q", plain.Project)
	}
}

func TestBillsAreListedAndTotalledPerProject(t *testing.T) {
	f := newStoreFixture(t)
	rec := f.receivable(t)
	f.billOnProject(t, rec, "reef_survey", 5000)
	f.billOnProject(t, rec, "reef_survey", 2500)
	f.billOnProject(t, rec, "kelp_study", 900)
	f.bill(t, rec, "stall-holder-7", 100, billDue)

	reef, err := f.repo.ListBills(f.bg, accounting.BillFilter{Project: "reef_survey"})
	if err != nil || len(reef) != 2 {
		t.Fatalf("bills of reef_survey: %+v %v", reef, err)
	}
	for _, b := range reef {
		if b.Project != "reef_survey" {
			t.Fatalf("a bill of another project listed: %+v", b)
		}
	}
	everyBill, err := f.repo.ListBills(f.bg, accounting.BillFilter{})
	if err != nil || len(everyBill) != 4 {
		t.Fatalf("every bill: %d %v", len(everyBill), err)
	}

	totals, err := f.repo.DimensionTotals(f.bg, "project", "asset", "", "")
	if err != nil {
		t.Fatal(err)
	}
	byProject := map[string]int64{}
	for _, row := range totals.Totals {
		byProject[row.Value] = row.Total
	}
	if byProject["reef_survey"] != 7500 || byProject["kelp_study"] != 900 || totals.Without.Total != 100 {
		t.Fatalf("invoiced per project: %+v", totals)
	}
}

func TestACorrectedBillKeepsOrChangesItsProjectAsSent(t *testing.T) {
	f := newStoreFixture(t)
	rec := f.receivable(t)
	made := f.billOnProject(t, rec, "reef_survey", 5000)

	kept, err := f.repo.UpdateBill(f.bg, accounting.BillUpdate{ID: made.ID, Payer: made.Payer, Project: "reef_survey", Amount: 5200, DueAt: made.DueAt})
	if err != nil || kept.Project != "reef_survey" || kept.Amount != 5200 {
		t.Fatalf("kept %+v %v", kept, err)
	}
	moved, err := f.repo.UpdateBill(f.bg, accounting.BillUpdate{ID: made.ID, Payer: made.Payer, Project: "kelp_study", Amount: 5200, DueAt: made.DueAt})
	if err != nil || moved.Project != "kelp_study" {
		t.Fatalf("moved %+v %v", moved, err)
	}
	cleared, err := f.repo.UpdateBill(f.bg, accounting.BillUpdate{ID: made.ID, Payer: made.Payer, Amount: 5200, DueAt: made.DueAt})
	if err != nil || cleared.Project != "" {
		t.Fatalf("cleared %+v %v", cleared, err)
	}
}

func TestABillProjectThatIsNotAValidDimensionValueIsRefused(t *testing.T) {
	f := newStoreFixture(t)
	rec := f.receivable(t)
	for name, project := range map[string]string{
		"too long":     strings.Repeat("p", accounting.MaxDimensionValue+1),
		"control char": "reef\nsurvey",
	} {
		_, err := f.repo.CreateBill(f.bg, accounting.BillInput{
			IncomeAccountID: f.sales.ID, ReceivableAccountID: rec.ID, Project: project, Amount: 10, DueAt: billDue, BilledBy: billedByClrk,
		})
		if !errors.Is(err, accounting.ErrInvalid) {
			t.Fatalf("%s: err %v, want ErrInvalid", name, err)
		}
	}
	made := f.bill(t, rec, "stall-holder-7", 100, billDue)
	_, err := f.repo.UpdateBill(f.bg, accounting.BillUpdate{ID: made.ID, Project: strings.Repeat("p", accounting.MaxDimensionValue+1), Amount: 100, DueAt: made.DueAt})
	if !errors.Is(err, accounting.ErrInvalid) {
		t.Fatalf("correction with a long project: %v", err)
	}
}
