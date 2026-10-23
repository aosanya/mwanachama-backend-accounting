package accounting_test

import (
	"errors"
	"strings"
	"testing"

	accounting "github.com/aosanya/mwanachama-backend-accounting"
)

func postWith(t *testing.T, r accounting.LedgerRepository, debit, credit accounting.Account, amount int64, dims map[string]string) accounting.Entry {
	t.Helper()
	e, err := r.Post(t.Context(), accounting.Entry{
		DebitAccountID: debit.ID, CreditAccountID: credit.ID, Amount: amount, Dimensions: dims,
		DocumentKind: "charge", DocumentID: "doc", ActorID: "actor-1",
	})
	if err != nil {
		t.Fatalf("Post: %v", err)
	}
	return e
}

func TestDimensionsAreStoredWithTheEntryAndReadBack(t *testing.T) {
	r, ctx := newLedger(t)
	holder := openHolder(t, r, "h1")
	pool := openPool(t, r, "p1")

	postWith(t, r, holder, pool, 100, map[string]string{"area": "north", "year": "2026"})

	entries, err := r.ListEntries(ctx, holder.ID, 0)
	if err != nil {
		t.Fatalf("ListEntries: %v", err)
	}
	if len(entries) != 1 || entries[0].Dimensions["area"] != "north" || entries[0].Dimensions["year"] != "2026" {
		t.Fatalf("entries = %+v, want the dimensions read back", entries)
	}
}

func TestAnEntryWithNoDimensionsReadsBackWithNone(t *testing.T) {
	r, ctx := newLedger(t)
	holder := openHolder(t, r, "h1")
	pool := openPool(t, r, "p1")

	postWith(t, r, holder, pool, 100, nil)

	entries, err := r.ListEntries(ctx, holder.ID, 0)
	if err != nil {
		t.Fatalf("ListEntries: %v", err)
	}
	if len(entries) != 1 || len(entries[0].Dimensions) != 0 {
		t.Fatalf("entries = %+v, want no dimensions", entries)
	}
}

func TestOneAccountSplitsByDimensionInsteadOfOneAccountPerValue(t *testing.T) {
	r, ctx := newLedger(t)
	spend := openHolder(t, r, "spend")
	cash := openPool(t, r, "cash")

	postWith(t, r, spend, cash, 100, map[string]string{"area": "north", "year": "2026"})
	postWith(t, r, spend, cash, 250, map[string]string{"area": "south", "year": "2026"})
	postWith(t, r, spend, cash, 40, map[string]string{"area": "north", "year": "2025"})
	postWith(t, r, spend, cash, 7, nil)

	north, err := r.DimensionBalance(ctx, spend.ID, "area", "north")
	if err != nil {
		t.Fatalf("DimensionBalance: %v", err)
	}
	if north != 140 {
		t.Fatalf("north = %d, want 140", north)
	}
	creditSide, err := r.DimensionBalance(ctx, cash.ID, "area", "south")
	if err != nil {
		t.Fatalf("DimensionBalance: %v", err)
	}
	if creditSide != -250 {
		t.Fatalf("south on the credit side = %d, want -250", creditSide)
	}
	year, err := r.ListEntriesByDimension(ctx, spend.ID, "year", "2026", 0)
	if err != nil {
		t.Fatalf("ListEntriesByDimension: %v", err)
	}
	if len(year) != 2 {
		t.Fatalf("2026 entries = %+v, want two", year)
	}
	capped, err := r.ListEntriesByDimension(ctx, spend.ID, "year", "2026", 1)
	if err != nil || len(capped) != 1 {
		t.Fatalf("capped = %+v, %v, want one", capped, err)
	}
	none, err := r.DimensionBalance(ctx, spend.ID, "area", "east")
	if err != nil || none != 0 {
		t.Fatalf("east = %d, %v, want 0", none, err)
	}
	whole, err := r.Balance(ctx, spend.ID)
	if err != nil || whole != 397 {
		t.Fatalf("whole balance = %d, %v, want 397", whole, err)
	}
}

func TestPostRefusesMalformedDimensions(t *testing.T) {
	r, ctx := newLedger(t)
	holder := openHolder(t, r, "h1")
	pool := openPool(t, r, "p1")

	tooMany := map[string]string{}
	for i := 0; i < accounting.MaxDimensions+1; i++ {
		tooMany["k"+strings.Repeat("a", i+1)] = "v"
	}
	for name, dims := range map[string]map[string]string{
		"upper-case key":         {"Area": "north"},
		"key with a dot":         {"a.b": "north"},
		"key with a digit first": {"1a": "north"},
		"empty value":            {"area": " "},
		"control character":      {"area": "no\nrth"},
		"value too long":         {"area": strings.Repeat("x", accounting.MaxDimensionValue+1)},
		"too many":               tooMany,
	} {
		_, err := r.Post(ctx, accounting.Entry{
			DebitAccountID: holder.ID, CreditAccountID: pool.ID, Amount: 1, Dimensions: dims,
			DocumentKind: "charge", DocumentID: "doc", ActorID: "actor-1",
		})
		if !errors.Is(err, accounting.ErrInvalid) {
			t.Fatalf("%s: want ErrInvalid, got %v", name, err)
		}
	}
	entries, err := r.ListEntries(ctx, holder.ID, 0)
	if err != nil || len(entries) != 0 {
		t.Fatalf("entries = %+v, %v, want nothing posted", entries, err)
	}
}

func TestDimensionReadsRefuseABadQueryAndAnUnknownAccount(t *testing.T) {
	r, ctx := newLedger(t)
	holder := openHolder(t, r, "h1")

	if _, err := r.DimensionBalance(ctx, holder.ID, "", "north"); !errors.Is(err, accounting.ErrInvalid) {
		t.Fatalf("empty key: want ErrInvalid, got %v", err)
	}
	if _, err := r.ListEntriesByDimension(ctx, holder.ID, "area", "", 0); !errors.Is(err, accounting.ErrInvalid) {
		t.Fatalf("empty value: want ErrInvalid, got %v", err)
	}
	if _, err := r.DimensionBalance(ctx, "nope", "area", "north"); !errors.Is(err, accounting.ErrNotFound) {
		t.Fatalf("unknown account: want ErrNotFound, got %v", err)
	}
}
