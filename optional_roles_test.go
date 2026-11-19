package accounting_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	accounting "github.com/aosanya/mwanachama-backend-accounting"
	"github.com/aosanya/mwanachama-backend-accounting/models"
)

func schoolLedger(t *testing.T) accounting.LedgerRepository {
	t.Helper()
	s, err := accounting.LoadSpec(filepath.Join(".", "spec", "examples", "school.accounting.json"))
	if err != nil {
		t.Fatalf("load spec: %v", err)
	}
	return newLedgerOn(t, openDB(t), s)
}

func TestADomainThatFillsNoProcurementRolesRefusesTendersAndQuotes(t *testing.T) {
	r := schoolLedger(t)
	ctx := context.Background()
	if _, err := r.CreateTender(ctx, accounting.Tender{BudgetID: "b1", Title: "Roof", CreatedBy: "bursar-2"}); !errors.Is(err, accounting.ErrInvalid) {
		t.Fatalf("CreateTender = %v, want ErrInvalid", err)
	}
	if _, err := r.ListTenders(ctx, accounting.TenderFilter{}); !errors.Is(err, accounting.ErrInvalid) {
		t.Fatalf("ListTenders = %v, want ErrInvalid", err)
	}
	if _, err := r.CreateQuote(ctx, accounting.Quote{TenderID: "t1", Supplier: "Quill Roofing", Amount: 5}); !errors.Is(err, accounting.ErrInvalid) {
		t.Fatalf("CreateQuote = %v, want ErrInvalid", err)
	}
	if _, err := r.UpdateTender(ctx, accounting.TenderUpdate{ID: "t1", Title: "Roof"}); !errors.Is(err, accounting.ErrInvalid) {
		t.Fatalf("UpdateTender = %v, want ErrInvalid", err)
	}
}

func TestADomainThatFillsNoUpwardReportRoleRefusesToSendOne(t *testing.T) {
	r := schoolLedger(t)
	ctx := context.Background()
	in := accounting.UpwardReportInput{PeriodStart: januaryStart, PeriodEnd: februaryStart, Kind: models.UpwardSpendingByDepartment, SentBy: "bursar-2"}
	if _, err := r.SendUpwardReport(ctx, in); !errors.Is(err, accounting.ErrInvalid) {
		t.Fatalf("SendUpwardReport = %v, want ErrInvalid", err)
	}
	if _, err := r.ListUpwardReports(ctx, accounting.UpwardReportFilter{}); !errors.Is(err, accounting.ErrInvalid) {
		t.Fatalf("ListUpwardReports = %v, want ErrInvalid", err)
	}
}

func TestADomainThatFillsNoProcurementRolesStillDeletesAnUnspentBudget(t *testing.T) {
	r := schoolLedger(t)
	ctx := context.Background()
	fees := openPool(t, r, "term-1")
	set, err := r.SetBudget(ctx, accounting.Budget{AccountID: fees.ID, PeriodStart: januaryStart, PeriodEnd: februaryStart, Amount: 500})
	if err != nil {
		t.Fatalf("SetBudget: %v", err)
	}
	if _, err := r.DeleteBudget(ctx, set.ID); err != nil {
		t.Fatalf("DeleteBudget: %v", err)
	}
}

func TestABudgetWithATenderDrawingOnItCannotBeDeletedUntilNoTenderDoes(t *testing.T) {
	r, ctx := newLedger(t)
	pool := openPool(t, r, "p1")
	set, err := r.SetBudget(ctx, accounting.Budget{AccountID: pool.ID, PeriodStart: januaryStart, PeriodEnd: februaryStart, Amount: 500})
	if err != nil {
		t.Fatalf("SetBudget: %v", err)
	}
	if _, err := r.CreateTender(ctx, accounting.Tender{BudgetID: set.ID, Title: "Resurfacing", CreatedBy: "clerk-1"}); err != nil {
		t.Fatalf("CreateTender: %v", err)
	}
	if _, err := r.DeleteBudget(ctx, set.ID); !errors.Is(err, accounting.ErrBudgetLineInUse) {
		t.Fatalf("DeleteBudget = %v, want ErrBudgetLineInUse", err)
	}
}
