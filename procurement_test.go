package accounting_test

import (
	"context"
	"errors"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	accounting "github.com/aosanya/mwanachama-backend-accounting"
)

func procurementLedger(t *testing.T) (accounting.LedgerRepository, accounting.Budget) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	s, err := accounting.SpecFor("test")
	if err != nil {
		t.Fatal(err)
	}
	if err := accounting.Provision(db, s); err != nil {
		t.Fatal(err)
	}
	repo, err := accounting.NewLedgerRepository(db, s)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	roads, err := repo.OpenAccount(ctx, accounting.Account{Code: "5100", Category: "roads", HolderID: "town", Type: accounting.AccountTypeExpense})
	if err != nil {
		t.Fatal(err)
	}
	line, err := repo.SetBudget(ctx, accounting.Budget{
		AccountID: roads.ID, Name: "Road repair", Department: "Public works",
		PeriodStart: "2026-01-01T00:00:00Z", PeriodEnd: "2027-01-01T00:00:00Z", Amount: 90000,
	})
	if err != nil {
		t.Fatal(err)
	}
	return repo, line
}

func TestATenderNamesTheBudgetLineItDrawsOnAndRefusesAnUnknownOne(t *testing.T) {
	repo, line := procurementLedger(t)
	ctx := context.Background()

	if _, err := repo.CreateTender(ctx, accounting.Tender{BudgetID: "missing", Title: "Resurfacing", CreatedBy: "clerk"}); !errors.Is(err, accounting.ErrNotFound) {
		t.Fatalf("unknown budget line: %v", err)
	}
	if _, err := repo.CreateTender(ctx, accounting.Tender{BudgetID: line.ID, CreatedBy: "clerk"}); !errors.Is(err, accounting.ErrInvalid) {
		t.Fatalf("no title: %v", err)
	}
	if _, err := repo.CreateTender(ctx, accounting.Tender{BudgetID: line.ID, Title: "Resurfacing", ClosesAt: "tomorrow", CreatedBy: "clerk"}); !errors.Is(err, accounting.ErrInvalid) {
		t.Fatalf("bad closing date: %v", err)
	}
	tender, err := repo.CreateTender(ctx, accounting.Tender{BudgetID: line.ID, Title: "Resurfacing of Mill Street", CreatedBy: "clerk"})
	if err != nil {
		t.Fatal(err)
	}
	if tender.ID == "" || tender.CreatedAt == "" || tender.BudgetID != line.ID {
		t.Fatalf("created %+v", tender)
	}
}

func TestATenderReadsBackWithItsQuoteCountAndLowestAmount(t *testing.T) {
	repo, line := procurementLedger(t)
	ctx := context.Background()
	tender, _ := repo.CreateTender(ctx, accounting.Tender{BudgetID: line.ID, Title: "Resurfacing", CreatedBy: "clerk"})
	other, _ := repo.CreateTender(ctx, accounting.Tender{BudgetID: line.ID, Title: "Bridge paint", CreatedBy: "clerk"})

	empty, err := repo.GetTender(ctx, tender.ID)
	if err != nil || empty.QuoteCount != 0 || empty.LowestAmount != 0 {
		t.Fatalf("before quotes %+v %v", empty, err)
	}
	for _, q := range []accounting.Quote{
		{TenderID: tender.ID, Supplier: "Asphalt Ltd", Amount: 70000, QuotedBy: "clerk"},
		{TenderID: tender.ID, Supplier: "Road Co", Amount: 64000, QuotedBy: "clerk"},
		{TenderID: other.ID, Supplier: "Brush Co", Amount: 5000, QuotedBy: "clerk"},
	} {
		if _, err := repo.CreateQuote(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	got, err := repo.GetTender(ctx, tender.ID)
	if err != nil || got.QuoteCount != 2 || got.LowestAmount != 64000 || got.Title != "Resurfacing" {
		t.Fatalf("after quotes %+v %v", got, err)
	}
	if _, err := repo.GetTender(ctx, "missing"); !errors.Is(err, accounting.ErrNotFound) {
		t.Fatalf("missing tender: %v", err)
	}
}

func TestTendersListByBudgetLine(t *testing.T) {
	repo, line := procurementLedger(t)
	ctx := context.Background()
	for _, title := range []string{"Resurfacing", "Bridge paint"} {
		if _, err := repo.CreateTender(ctx, accounting.Tender{BudgetID: line.ID, Title: title, CreatedBy: "clerk"}); err != nil {
			t.Fatal(err)
		}
	}
	all, err := repo.ListTenders(ctx, accounting.TenderFilter{})
	if err != nil || len(all) != 2 || all[0].Title != "Resurfacing" {
		t.Fatalf("all %+v %v", all, err)
	}
	none, err := repo.ListTenders(ctx, accounting.TenderFilter{BudgetID: "another"})
	if err != nil || len(none) != 0 {
		t.Fatalf("other line %+v %v", none, err)
	}
	one, err := repo.ListTenders(ctx, accounting.TenderFilter{Limit: 1})
	if err != nil || len(one) != 1 {
		t.Fatalf("limit %+v %v", one, err)
	}
}

func TestQuotesAreCheckedAndListedLowestFirst(t *testing.T) {
	repo, line := procurementLedger(t)
	ctx := context.Background()
	tender, _ := repo.CreateTender(ctx, accounting.Tender{BudgetID: line.ID, Title: "Resurfacing", CreatedBy: "clerk"})

	refused := []accounting.Quote{
		{TenderID: "missing", Supplier: "Road Co", Amount: 10, QuotedBy: "clerk"},
		{TenderID: tender.ID, Amount: 10, QuotedBy: "clerk"},
		{TenderID: tender.ID, Supplier: "Road Co", Amount: 0, QuotedBy: "clerk"},
		{TenderID: tender.ID, Supplier: "Road Co", Amount: -5, QuotedBy: "clerk"},
		{TenderID: tender.ID, Supplier: "Road Co", Amount: 10, ValidUntil: "soon", QuotedBy: "clerk"},
	}
	wants := []error{accounting.ErrNotFound, accounting.ErrInvalid, accounting.ErrInvalid, accounting.ErrInvalid, accounting.ErrInvalid}
	for i, q := range refused {
		if _, err := repo.CreateQuote(ctx, q); !errors.Is(err, wants[i]) {
			t.Fatalf("refused %d: %v", i, err)
		}
	}
	for _, q := range []accounting.Quote{
		{TenderID: tender.ID, Supplier: "Asphalt Ltd", Amount: 70000, QuotedBy: "clerk"},
		{TenderID: tender.ID, Supplier: "Road Co", Amount: 64000, Note: "Includes line marking", ValidUntil: futureInstant, QuotedBy: "clerk"},
	} {
		if _, err := repo.CreateQuote(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	quotes, err := repo.ListQuotes(ctx, tender.ID, 0)
	if err != nil || len(quotes) != 2 || quotes[0].Supplier != "Road Co" || quotes[0].Note != "Includes line marking" || quotes[0].QuotedAt == "" {
		t.Fatalf("listed %+v %v", quotes, err)
	}
	if _, err := repo.ListQuotes(ctx, "missing", 0); !errors.Is(err, accounting.ErrNotFound) {
		t.Fatalf("missing tender: %v", err)
	}
}
