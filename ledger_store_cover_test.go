package accounting_test

import (
	"context"
	"errors"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	accounting "github.com/aosanya/mwanachama-backend-accounting"
)

type storeFixture struct {
	repo   accounting.LedgerRepository
	cash   accounting.Account
	sales  accounting.Account
	rent   accounting.Account
	bg     context.Context
	postAt func(t *testing.T, debit, credit accounting.Account, amount int64, over accounting.Entry) accounting.Entry
}

func newStoreFixture(t *testing.T) storeFixture {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open: %v", err)
	}
	s, err := accounting.SpecFor("test")
	if err != nil {
		t.Fatalf("SpecFor: %v", err)
	}
	if err := accounting.Provision(db, s); err != nil {
		t.Fatalf("Provision: %v", err)
	}
	repo, err := accounting.NewLedgerRepository(db, s)
	if err != nil {
		t.Fatalf("NewLedgerRepository: %v", err)
	}
	bg := context.Background()
	open := func(code, category, holder string, kind accounting.AccountType) accounting.Account {
		a, err := repo.OpenAccount(bg, accounting.Account{Code: code, Category: accounting.AccountCategory(category), HolderID: holder, Type: kind})
		if err != nil {
			t.Fatalf("OpenAccount %s: %v", code, err)
		}
		return a
	}
	f := storeFixture{
		repo:  repo,
		bg:    bg,
		cash:  open("1000", "cash", "hub-1", accounting.AccountTypeAsset),
		sales: open("4000", "sales", "hub-1", accounting.AccountTypeIncome),
		rent:  open("5000", "rent", "hub-1", accounting.AccountTypeExpense),
	}
	f.postAt = func(t *testing.T, debit, credit accounting.Account, amount int64, over accounting.Entry) accounting.Entry {
		t.Helper()
		e := over
		e.DebitAccountID, e.CreditAccountID, e.Amount = debit.ID, credit.ID, amount
		if e.DocumentKind == "" {
			e.DocumentKind = "invoice"
		}
		if e.DocumentID == "" {
			e.DocumentID = "doc-1"
		}
		if e.ActorID == "" {
			e.ActorID = "actor-1"
		}
		posted, err := repo.Post(bg, e)
		if err != nil {
			t.Fatalf("Post: %v", err)
		}
		return posted
	}
	return f
}

func TestOpeningTheSameAccountTwiceGivesTheFirstOne(t *testing.T) {
	f := newStoreFixture(t)
	again, err := f.repo.OpenAccount(f.bg, accounting.Account{Code: "1000", Category: "cash", HolderID: "hub-1", Type: accounting.AccountTypeAsset})
	if err != nil || again.ID != f.cash.ID {
		t.Fatalf("again %+v %v", again, err)
	}
}

func TestACodeAlreadyHeldByAnotherAccountIsRefused(t *testing.T) {
	f := newStoreFixture(t)
	_, err := f.repo.OpenAccount(f.bg, accounting.Account{Code: "1000", Category: "petty", HolderID: "hub-2", Type: accounting.AccountTypeAsset})
	if !errors.Is(err, accounting.ErrCodeTaken) {
		t.Fatalf("err %v", err)
	}
}

func TestAnAccountOfAnUnknownTypeIsRefused(t *testing.T) {
	f := newStoreFixture(t)
	_, err := f.repo.OpenAccount(f.bg, accounting.Account{Code: "9", Category: "x", HolderID: "h", Type: "revenue"})
	if err == nil {
		t.Fatal("an unknown type was opened")
	}
}

func TestGetAndListAccounts(t *testing.T) {
	f := newStoreFixture(t)
	got, err := f.repo.GetAccount(f.bg, f.sales.ID)
	if err != nil || got.Code != "4000" || got.OpenedAt == "" {
		t.Fatalf("got %+v %v", got, err)
	}
	if _, err := f.repo.GetAccount(f.bg, "missing"); !errors.Is(err, accounting.ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
	all, err := f.repo.ListAccounts(f.bg, 0)
	if err != nil || len(all) != 3 {
		t.Fatalf("all %d %v", len(all), err)
	}
	one, err := f.repo.ListAccounts(f.bg, 1)
	if err != nil || len(one) != 1 || one[0].ID != f.cash.ID {
		t.Fatalf("one %+v %v", one, err)
	}
}

func TestPostingMovesTheBalancesOfBothAccounts(t *testing.T) {
	f := newStoreFixture(t)
	posted := f.postAt(t, f.cash, f.sales, 500, accounting.Entry{})
	if posted.ID == "" || posted.PostedAt == "" {
		t.Fatalf("posted %+v", posted)
	}
	cash, _ := f.repo.Balance(f.bg, f.cash.ID)
	sales, _ := f.repo.Balance(f.bg, f.sales.ID)
	if cash != 500 || sales != -500 {
		t.Fatalf("cash %d sales %d", cash, sales)
	}
	if _, err := f.repo.Balance(f.bg, "missing"); !errors.Is(err, accounting.ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
}

func TestPostingRefusals(t *testing.T) {
	f := newStoreFixture(t)
	base := accounting.Entry{DebitAccountID: f.cash.ID, CreditAccountID: f.sales.ID, Amount: 5, DocumentKind: "invoice", DocumentID: "d", ActorID: "a"}
	for name, mutate := range map[string]func(e *accounting.Entry){
		"zero amount":    func(e *accounting.Entry) { e.Amount = 0 },
		"same account":   func(e *accounting.Entry) { e.CreditAccountID = e.DebitAccountID },
		"bad due":        func(e *accounting.Entry) { e.DueAt = "soon" },
		"bad dimensions": func(e *accounting.Entry) { e.Dimensions = map[string]string{"Bad": "x"} },
	} {
		e := base
		mutate(&e)
		if _, err := f.repo.Post(f.bg, e); !errors.Is(err, accounting.ErrInvalid) {
			t.Fatalf("%s: %v", name, err)
		}
	}
	e := base
	e.DebitAccountID = "missing"
	if _, err := f.repo.Post(f.bg, e); !errors.Is(err, accounting.ErrNotFound) {
		t.Fatalf("missing debit: %v", err)
	}
	e = base
	e.CreditAccountID = "missing"
	if _, err := f.repo.Post(f.bg, e); !errors.Is(err, accounting.ErrNotFound) {
		t.Fatalf("missing credit: %v", err)
	}
}

func TestAnAccountWithABalanceCannotBeClosedUntilItIsBackToZero(t *testing.T) {
	f := newStoreFixture(t)
	f.postAt(t, f.cash, f.sales, 100, accounting.Entry{})
	if _, err := f.repo.CloseAccount(f.bg, f.cash.ID); !errors.Is(err, accounting.ErrNonZeroBalance) {
		t.Fatalf("non zero: %v", err)
	}
	f.postAt(t, f.sales, f.cash, 100, accounting.Entry{})
	closed, err := f.repo.CloseAccount(f.bg, f.cash.ID)
	if err != nil || closed.ClosedAt == "" {
		t.Fatalf("closed %+v %v", closed, err)
	}
	if _, err := f.repo.CloseAccount(f.bg, f.cash.ID); !errors.Is(err, accounting.ErrClosed) {
		t.Fatalf("twice: %v", err)
	}
	if _, err := f.repo.CloseAccount(f.bg, "missing"); !errors.Is(err, accounting.ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
}

func TestNothingCanBePostedToAClosedAccount(t *testing.T) {
	f := newStoreFixture(t)
	if _, err := f.repo.CloseAccount(f.bg, f.rent.ID); err != nil {
		t.Fatalf("close: %v", err)
	}
	_, err := f.repo.Post(f.bg, accounting.Entry{DebitAccountID: f.rent.ID, CreditAccountID: f.cash.ID, Amount: 5, DocumentKind: "bill", DocumentID: "d", ActorID: "a"})
	if !errors.Is(err, accounting.ErrClosed) {
		t.Fatalf("err %v", err)
	}
}

func TestAnEntryCanBeReversedOnceAndTheReversalKeepsItsDimensions(t *testing.T) {
	f := newStoreFixture(t)
	original := f.postAt(t, f.rent, f.cash, 80, accounting.Entry{Dimensions: map[string]string{"ward": "3"}})
	reversal := f.postAt(t, f.cash, f.rent, 80, accounting.Entry{ReversesEntryID: original.ID, Dimensions: map[string]string{"ward": "9"}})
	if reversal.Dimensions["ward"] != "3" {
		t.Fatalf("dimensions %v", reversal.Dimensions)
	}
	_, err := f.repo.Post(f.bg, accounting.Entry{DebitAccountID: f.cash.ID, CreditAccountID: f.rent.ID, Amount: 80, DocumentKind: "bill", DocumentID: "d", ActorID: "a", ReversesEntryID: original.ID})
	if !errors.Is(err, accounting.ErrAlreadyReversed) {
		t.Fatalf("second reversal: %v", err)
	}
	_, err = f.repo.Post(f.bg, accounting.Entry{DebitAccountID: f.cash.ID, CreditAccountID: f.rent.ID, Amount: 1, DocumentKind: "bill", DocumentID: "d", ActorID: "a", ReversesEntryID: "missing"})
	if !errors.Is(err, accounting.ErrNotFound) {
		t.Fatalf("reversal of nothing: %v", err)
	}
	rent, _ := f.repo.Balance(f.bg, f.rent.ID)
	if rent != 0 {
		t.Fatalf("rent %d", rent)
	}
}

func TestSettlingAnEntryThatDoesNotExistIsRefused(t *testing.T) {
	f := newStoreFixture(t)
	_, err := f.repo.Post(f.bg, accounting.Entry{DebitAccountID: f.cash.ID, CreditAccountID: f.sales.ID, Amount: 5, DocumentKind: "receipt", DocumentID: "d", ActorID: "a", SettlesEntryID: "missing"})
	if !errors.Is(err, accounting.ErrNotFound) {
		t.Fatalf("err %v", err)
	}
}

func TestEntriesListInTheOrderTheyHappenedAndHonourTheLimit(t *testing.T) {
	f := newStoreFixture(t)
	f.postAt(t, f.cash, f.sales, 3, accounting.Entry{OccurredAt: "2026-03-01T00:00:00Z"})
	f.postAt(t, f.cash, f.sales, 1, accounting.Entry{OccurredAt: "2026-01-01T00:00:00Z"})
	f.postAt(t, f.cash, f.sales, 2, accounting.Entry{OccurredAt: "2026-02-01T00:00:00Z"})
	all, err := f.repo.ListEntries(f.bg, f.cash.ID, 0)
	if err != nil || len(all) != 3 || all[0].Amount != 1 || all[1].Amount != 2 || all[2].Amount != 3 {
		t.Fatalf("all %+v %v", all, err)
	}
	two, err := f.repo.ListEntries(f.bg, f.cash.ID, 2)
	if err != nil || len(two) != 2 {
		t.Fatalf("two %+v %v", two, err)
	}
	none, err := f.repo.ListEntries(f.bg, f.rent.ID, 0)
	if err != nil || len(none) != 0 {
		t.Fatalf("none %+v %v", none, err)
	}
}

func TestEntriesAndBalancesCanBeSlicedByDimension(t *testing.T) {
	f := newStoreFixture(t)
	f.postAt(t, f.rent, f.cash, 40, accounting.Entry{Dimensions: map[string]string{"ward": "3"}})
	f.postAt(t, f.rent, f.cash, 60, accounting.Entry{Dimensions: map[string]string{"ward": "3", "project": "road"}})
	f.postAt(t, f.rent, f.cash, 900, accounting.Entry{Dimensions: map[string]string{"ward": "4"}})
	got, err := f.repo.ListEntriesByDimension(f.bg, f.rent.ID, "ward", "3", 0)
	if err != nil || len(got) != 2 {
		t.Fatalf("got %+v %v", got, err)
	}
	limited, err := f.repo.ListEntriesByDimension(f.bg, f.rent.ID, "ward", "3", 1)
	if err != nil || len(limited) != 1 {
		t.Fatalf("limited %+v %v", limited, err)
	}
	balance, err := f.repo.DimensionBalance(f.bg, f.rent.ID, "ward", "3")
	if err != nil || balance != 100 {
		t.Fatalf("balance %d %v", balance, err)
	}
	if _, err := f.repo.ListEntriesByDimension(f.bg, f.rent.ID, "Ward", "3", 0); !errors.Is(err, accounting.ErrInvalid) {
		t.Fatalf("bad key: %v", err)
	}
	if _, err := f.repo.DimensionBalance(f.bg, "missing", "ward", "3"); !errors.Is(err, accounting.ErrNotFound) {
		t.Fatalf("missing account: %v", err)
	}
}
