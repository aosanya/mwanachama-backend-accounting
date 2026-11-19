package accounting_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	accounting "github.com/aosanya/mwanachama-backend-accounting"
)

func billedLedger(t *testing.T) (accounting.LedgerRepository, accounting.Bill, accounting.Account) {
	t.Helper()
	repo, _ := procurementLedger(t)
	ctx := context.Background()
	open := func(code, category string, kind accounting.AccountType) accounting.Account {
		a, err := repo.OpenAccount(ctx, accounting.Account{Code: code, Category: accounting.AccountCategory(category), HolderID: "town", Type: kind})
		if err != nil {
			t.Fatal(err)
		}
		return a
	}
	fees := open("4000", "market_fees", accounting.AccountTypeIncome)
	owed := open("1200", "receivable", accounting.AccountTypeAsset)
	cash := open("1000", "cash", accounting.AccountTypeAsset)
	bill, err := repo.CreateBill(ctx, accounting.BillInput{
		IncomeAccountID: fees.ID, ReceivableAccountID: owed.ID, Payer: "stall-7", Amount: 900,
		DueAt: "2099-03-01T00:00:00Z", Narration: "March stall fee", BilledBy: "clerk-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	return repo, bill, cash
}

func TestAnOpenBillIsCorrectedInAmountDueDatePayerAndNarration(t *testing.T) {
	repo, bill, _ := billedLedger(t)
	ctx := context.Background()

	got, err := repo.UpdateBill(ctx, accounting.BillUpdate{ID: bill.ID, Payer: "stall-9", Amount: 950, DueAt: "2099-04-01T00:00:00Z", Narration: "April stall fee"})
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != bill.ID || got.Amount != 950 || got.Outstanding != 950 || got.Payer != "stall-9" || got.DueAt != "2099-04-01T00:00:00Z" || got.Narration != "April stall fee" || got.BilledBy != "clerk-1" {
		t.Fatalf("updated bill = %+v", got)
	}
	read, err := repo.GetBill(ctx, bill.ID)
	if err != nil || read.Amount != 950 {
		t.Fatalf("read back %+v, %v", read, err)
	}
}

func TestABillWithAPaymentKeepsItsAmountButTheRestMayChange(t *testing.T) {
	repo, bill, cash := billedLedger(t)
	ctx := context.Background()
	if _, err := repo.MarkBillPaid(ctx, accounting.MarkPaidInput{BillID: bill.ID, CashAccountID: cash.ID, Amount: 300, PaidBy: "clerk-1"}); err != nil {
		t.Fatal(err)
	}

	_, err := repo.UpdateBill(ctx, accounting.BillUpdate{ID: bill.ID, Payer: "stall-7", Amount: 800, DueAt: bill.DueAt})
	if !errors.Is(err, accounting.ErrLocked) || !strings.Contains(err.Error(), "payment") {
		t.Fatalf("changing the amount of a part-paid bill: %v, want a refusal naming the payment", err)
	}
	got, err := repo.UpdateBill(ctx, accounting.BillUpdate{ID: bill.ID, Payer: "stall-7", Amount: 900, DueAt: "2099-05-01T00:00:00Z", Narration: "extended"})
	if err != nil {
		t.Fatalf("moving the due date of a part-paid bill: %v", err)
	}
	if got.Amount != 900 || got.Settled != 300 || got.DueAt != "2099-05-01T00:00:00Z" {
		t.Fatalf("updated bill = %+v", got)
	}
}

func TestASettledBillCannotBeUpdated(t *testing.T) {
	repo, bill, cash := billedLedger(t)
	ctx := context.Background()
	if _, err := repo.MarkBillPaid(ctx, accounting.MarkPaidInput{BillID: bill.ID, CashAccountID: cash.ID, PaidBy: "clerk-1"}); err != nil {
		t.Fatal(err)
	}

	_, err := repo.UpdateBill(ctx, accounting.BillUpdate{ID: bill.ID, Amount: 900, DueAt: bill.DueAt})
	if !errors.Is(err, accounting.ErrLocked) || !strings.Contains(err.Error(), "settled") {
		t.Fatalf("updating a settled bill: %v, want a refusal that says it is settled", err)
	}
}

func TestABillUpdateRefusesABadAmountABadDateAndAnUnknownBill(t *testing.T) {
	repo, bill, _ := billedLedger(t)
	ctx := context.Background()
	for name, in := range map[string]accounting.BillUpdate{
		"zero amount": {ID: bill.ID, Amount: 0, DueAt: bill.DueAt},
		"bad date":    {ID: bill.ID, Amount: 900, DueAt: "tomorrow"},
		"no date":     {ID: bill.ID, Amount: 900},
	} {
		if _, err := repo.UpdateBill(ctx, in); !errors.Is(err, accounting.ErrInvalid) {
			t.Errorf("%s: %v, want ErrInvalid", name, err)
		}
	}
	if _, err := repo.UpdateBill(ctx, accounting.BillUpdate{ID: "missing", Amount: 900, DueAt: bill.DueAt}); !errors.Is(err, accounting.ErrNotFound) {
		t.Errorf("unknown bill: %v, want ErrNotFound", err)
	}
}

func TestATenderIsCorrectedBeforeItClosesAndRefusedAfter(t *testing.T) {
	repo, line := procurementLedger(t)
	ctx := context.Background()
	open, err := repo.CreateTender(ctx, accounting.Tender{BudgetID: line.ID, Title: "Resurfacing", ClosesAt: "2099-01-01T00:00:00Z", CreatedBy: "clerk"})
	if err != nil {
		t.Fatal(err)
	}
	closed, err := repo.CreateTender(ctx, accounting.Tender{BudgetID: line.ID, Title: "Old tender", ClosesAt: "2020-01-01T00:00:00Z", CreatedBy: "clerk"})
	if err != nil {
		t.Fatal(err)
	}

	got, err := repo.UpdateTender(ctx, accounting.TenderUpdate{ID: open.ID, Title: "Resurfacing of Mill Street", Description: "Both lanes", ClosesAt: "2099-02-01T00:00:00Z"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "Resurfacing of Mill Street" || got.Description != "Both lanes" || got.ClosesAt != "2099-02-01T00:00:00Z" || got.BudgetID != line.ID || got.CreatedBy != "clerk" {
		t.Fatalf("updated tender = %+v", got)
	}
	_, err = repo.UpdateTender(ctx, accounting.TenderUpdate{ID: closed.ID, Title: "Reopened"})
	if !errors.Is(err, accounting.ErrLocked) || !strings.Contains(err.Error(), "closed") {
		t.Fatalf("updating a closed tender: %v, want a refusal that says it closed", err)
	}
	if _, err := repo.UpdateTender(ctx, accounting.TenderUpdate{ID: open.ID, Title: " "}); !errors.Is(err, accounting.ErrInvalid) {
		t.Errorf("blank title: %v, want ErrInvalid", err)
	}
	if _, err := repo.UpdateTender(ctx, accounting.TenderUpdate{ID: "missing", Title: "x"}); !errors.Is(err, accounting.ErrNotFound) {
		t.Errorf("unknown tender: %v, want ErrNotFound", err)
	}
}

func TestAQuoteIsCorrectedUntilItsTenderClosesAndKeepsItsTender(t *testing.T) {
	repo, line := procurementLedger(t)
	ctx := context.Background()
	open, err := repo.CreateTender(ctx, accounting.Tender{BudgetID: line.ID, Title: "Resurfacing", ClosesAt: "2099-01-01T00:00:00Z", CreatedBy: "clerk"})
	if err != nil {
		t.Fatal(err)
	}
	quote, err := repo.CreateQuote(ctx, accounting.Quote{TenderID: open.ID, Supplier: "Road Co", Amount: 64000, QuotedBy: "clerk"})
	if err != nil {
		t.Fatal(err)
	}

	got, err := repo.UpdateQuote(ctx, accounting.QuoteUpdate{ID: quote.ID, Supplier: "Road Co Ltd", Amount: 62000, Note: "incl. kerbs"})
	if err != nil {
		t.Fatal(err)
	}
	if got.TenderID != open.ID || got.Amount != 62000 || got.Supplier != "Road Co Ltd" || got.Note != "incl. kerbs" || got.QuotedBy != "clerk" {
		t.Fatalf("updated quote = %+v", got)
	}
	if _, err := repo.UpdateQuote(ctx, accounting.QuoteUpdate{ID: quote.ID, Supplier: "Road Co", Amount: 0}); !errors.Is(err, accounting.ErrInvalid) {
		t.Errorf("zero amount: %v, want ErrInvalid", err)
	}

	if _, err := repo.UpdateTender(ctx, accounting.TenderUpdate{ID: open.ID, Title: "Resurfacing", ClosesAt: "2099-01-01T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	past, err := repo.CreateTender(ctx, accounting.Tender{BudgetID: line.ID, Title: "Closed", ClosesAt: "2020-01-01T00:00:00Z", CreatedBy: "clerk"})
	if err != nil {
		t.Fatal(err)
	}
	late, err := repo.CreateQuote(ctx, accounting.Quote{TenderID: past.ID, Supplier: "Late Co", Amount: 100, QuotedBy: "clerk"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = repo.UpdateQuote(ctx, accounting.QuoteUpdate{ID: late.ID, Supplier: "Late Co", Amount: 90})
	if !errors.Is(err, accounting.ErrLocked) || !strings.Contains(err.Error(), "closed") {
		t.Fatalf("updating a quote of a closed tender: %v, want a refusal that says the tender closed", err)
	}
}

func TestSettingABudgetLineAgainIsTheEditAndKeepsItsId(t *testing.T) {
	repo, line := procurementLedger(t)
	ctx := context.Background()

	edited, err := repo.SetBudget(ctx, accounting.Budget{
		AccountID: line.AccountID, Name: "Road repair and kerbs", Department: "Public works",
		PeriodStart: line.PeriodStart, PeriodEnd: line.PeriodEnd, Amount: 120000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if edited.ID != line.ID || edited.Amount != 120000 || edited.Name != "Road repair and kerbs" {
		t.Fatalf("edited line = %+v, want the same id with the new name and amount", edited)
	}
}
