package accounting_test

import (
	"testing"

	accounting "github.com/aosanya/mwanachama-backend-accounting"
)

func TestPostingTheSameDocumentLineTwiceGivesTheFirstEntryAndOneBalance(t *testing.T) {
	f := newStoreFixture(t)
	first := f.postAt(t, f.cash, f.sales, 100, accounting.Entry{DocumentKind: "x", DocumentID: "d1"})
	again := f.postAt(t, f.cash, f.sales, 100, accounting.Entry{DocumentKind: "x", DocumentID: "d1"})
	if again.ID != first.ID {
		t.Fatalf("second post made entry %s, first was %s", again.ID, first.ID)
	}
	cash, _ := f.repo.Balance(f.bg, f.cash.ID)
	sales, _ := f.repo.Balance(f.bg, f.sales.ID)
	if cash != 100 || sales != -100 {
		t.Fatalf("cash %d sales %d", cash, sales)
	}
	listed, err := f.repo.ListEntries(f.bg, f.cash.ID, 0)
	if err != nil || len(listed) != 1 {
		t.Fatalf("listed %d %v", len(listed), err)
	}
}

func TestOneDocumentMayCarryDifferentLines(t *testing.T) {
	f := newStoreFixture(t)
	f.postAt(t, f.cash, f.sales, 100, accounting.Entry{DocumentKind: "x", DocumentID: "d1"})
	f.postAt(t, f.cash, f.rent, 100, accounting.Entry{DocumentKind: "x", DocumentID: "d1"})
	f.postAt(t, f.cash, f.sales, 40, accounting.Entry{DocumentKind: "x", DocumentID: "d1"})
	f.postAt(t, f.cash, f.sales, 100, accounting.Entry{DocumentKind: "x", DocumentID: "d2"})
	f.postAt(t, f.cash, f.sales, 100, accounting.Entry{DocumentKind: "y", DocumentID: "d1"})
	cash, _ := f.repo.Balance(f.bg, f.cash.ID)
	if cash != 440 {
		t.Fatalf("cash %d", cash)
	}
}

func TestReversingTheSameEntryTwiceWithTheSameDocumentGivesTheFirstReversal(t *testing.T) {
	f := newStoreFixture(t)
	original := f.postAt(t, f.cash, f.sales, 100, accounting.Entry{DocumentKind: "x", DocumentID: "d1"})
	reversal := accounting.Entry{DebitAccountID: f.sales.ID, CreditAccountID: f.cash.ID, Amount: 100, DocumentKind: "x", DocumentID: "r1", ActorID: "actor-1", ReversesEntryID: original.ID}
	first, err := f.repo.Post(f.bg, reversal)
	if err != nil {
		t.Fatalf("first reversal: %v", err)
	}
	again, err := f.repo.Post(f.bg, reversal)
	if err != nil || again.ID != first.ID {
		t.Fatalf("again %+v %v", again, err)
	}
}
