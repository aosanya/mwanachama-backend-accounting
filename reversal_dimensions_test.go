package accounting_test

import (
	"testing"

	accounting "github.com/aosanya/mwanachama-backend-accounting"
)

func TestAReversalKeepsItsOriginalsDimensionsSoADimensionBalanceNetsToZero(t *testing.T) {
	r, ctx := newLedger(t)
	spend := openHolder(t, r, "h1")
	cash := openPool(t, r, "p1")

	original := postWith(t, r, spend, cash, 100, map[string]string{"area": "north"})
	reversal, err := r.Post(ctx, accounting.Entry{
		DebitAccountID: cash.ID, CreditAccountID: spend.ID, Amount: 100,
		DocumentKind: "charge", DocumentID: "rev", ActorID: "actor-1", ReversesEntryID: original.ID,
	})
	if err != nil {
		t.Fatalf("reverse: %v", err)
	}
	if reversal.Dimensions["area"] != "north" {
		t.Fatalf("reversal dimensions = %v, want the original's", reversal.Dimensions)
	}
	north, err := r.DimensionBalance(ctx, spend.ID, "area", "north")
	if err != nil {
		t.Fatalf("DimensionBalance: %v", err)
	}
	if north != 0 {
		t.Fatalf("north = %d, want a reversed amount to net to zero", north)
	}
}

func TestAReversalTakesItsOriginalsDimensionsOverAnyItNames(t *testing.T) {
	r, ctx := newLedger(t)
	spend := openHolder(t, r, "h1")
	cash := openPool(t, r, "p1")

	original := postWith(t, r, spend, cash, 100, map[string]string{"area": "north"})
	reversal, err := r.Post(ctx, accounting.Entry{
		DebitAccountID: cash.ID, CreditAccountID: spend.ID, Amount: 100, Dimensions: map[string]string{"area": "south"},
		DocumentKind: "charge", DocumentID: "rev", ActorID: "actor-1", ReversesEntryID: original.ID,
	})
	if err != nil {
		t.Fatalf("reverse: %v", err)
	}
	if len(reversal.Dimensions) != 1 || reversal.Dimensions["area"] != "north" {
		t.Fatalf("reversal dimensions = %v, want only the original's", reversal.Dimensions)
	}
	entries, err := r.ListEntries(ctx, spend.ID, 0)
	if err != nil {
		t.Fatalf("ListEntries: %v", err)
	}
	for _, e := range entries {
		if e.Dimensions["area"] != "north" {
			t.Fatalf("stored %+v, want area north on both", e)
		}
	}
}
