package accounting_test

import (
	"errors"
	"reflect"
	"testing"

	accounting "github.com/aosanya/mwanachama-backend-accounting"
)

const (
	januaryOpens  = "2026-01-01T00:00:00Z"
	februaryOpens = "2026-02-01T00:00:00Z"
)

type townLedger struct {
	repo   accounting.LedgerRepository
	wages  accounting.Account
	repair accounting.Account
	cash   accounting.Account
}

func openTownLedger(t *testing.T) townLedger {
	t.Helper()
	r, ctx := newLedger(t)
	open := func(category accounting.AccountCategory, kind accounting.AccountType, holder string) accounting.Account {
		a, err := r.OpenAccount(ctx, accounting.Account{Category: category, Type: kind, HolderID: holder})
		if err != nil {
			t.Fatalf("OpenAccount(%s): %v", holder, err)
		}
		return a
	}
	return townLedger{
		repo:   r,
		wages:  open("wages", accounting.AccountTypeExpense, "zoldhalom"),
		repair: open("repairs", accounting.AccountTypeExpense, "zoldhalom"),
		cash:   open("cash", accounting.AccountTypeAsset, "zoldhalom"),
	}
}

func (l townLedger) spend(t *testing.T, from accounting.Account, amount int64, occurredAt string, dims map[string]string) accounting.Entry {
	t.Helper()
	e, err := l.repo.Post(t.Context(), accounting.Entry{
		DebitAccountID: from.ID, CreditAccountID: l.cash.ID, Amount: amount, OccurredAt: occurredAt, Dimensions: dims,
		DocumentKind: "invoice", DocumentID: "doc-" + occurredAt, ActorID: "actor-1",
	})
	if err != nil {
		t.Fatalf("Post: %v", err)
	}
	return e
}

func (l townLedger) reverse(t *testing.T, original accounting.Entry, occurredAt string) {
	t.Helper()
	if _, err := l.repo.Post(t.Context(), accounting.Entry{
		DebitAccountID: original.CreditAccountID, CreditAccountID: original.DebitAccountID, Amount: original.Amount,
		OccurredAt: occurredAt, DocumentKind: "invoice", DocumentID: "reversal", ActorID: "actor-1", ReversesEntryID: original.ID,
	}); err != nil {
		t.Fatalf("reverse: %v", err)
	}
}

func neighbourhood(name string) map[string]string {
	return map[string]string{"neighbourhood": name}
}

func zoldhalomYear(t *testing.T) townLedger {
	t.Helper()
	l := openTownLedger(t)
	l.spend(t, l.wages, 100, "2026-01-10T09:00:00Z", neighbourhood("Óváros"))
	l.spend(t, l.repair, 50, "2026-01-20T09:00:00Z", neighbourhood("Óváros"))
	l.spend(t, l.wages, 200, januaryOpens, neighbourhood("Kertváros"))
	l.spend(t, l.wages, 30, "2026-01-31T23:59:59Z", neighbourhood("Malomsor"))
	l.spend(t, l.repair, 70, februaryOpens, neighbourhood("Malomsor"))
	reversed := l.spend(t, l.wages, 80, "2026-01-05T09:00:00Z", neighbourhood("Határrét"))
	l.reverse(t, reversed, "2026-01-06T09:00:00Z")
	l.spend(t, l.wages, 999, "2026-01-12T09:00:00Z", nil)
	l.spend(t, l.repair, 5, "2026-01-12T09:00:00Z", map[string]string{"department": "roads"})
	l.spend(t, l.repair, 1000, "", neighbourhood("Óváros"))
	return l
}

func TestDimensionTotalsGivesOneTotalPerValueSortedByValue(t *testing.T) {
	l := zoldhalomYear(t)

	got, err := l.repo.DimensionTotals(t.Context(), "neighbourhood", string(accounting.AccountTypeExpense), "", "")
	if err != nil {
		t.Fatalf("DimensionTotals: %v", err)
	}
	want := []accounting.DimensionTotal{
		{Value: "Határrét", Total: 0, Entries: 2},
		{Value: "Kertváros", Total: 200, Entries: 1},
		{Value: "Malomsor", Total: 100, Entries: 2},
		{Value: "Óváros", Total: 1150, Entries: 3},
	}
	if got.Key != "neighbourhood" || got.AccountType != "expense" {
		t.Fatalf("totals = %+v, want the key and account type echoed", got)
	}
	if !reflect.DeepEqual(got.Totals, want) {
		t.Fatalf("totals = %+v, want %+v", got.Totals, want)
	}
}

func TestDimensionTotalsAcrossEveryAccountNetToZeroBecauseEachEntryHasTwoSides(t *testing.T) {
	l := zoldhalomYear(t)

	got, err := l.repo.DimensionTotals(t.Context(), "neighbourhood", "", "", "")
	if err != nil {
		t.Fatalf("DimensionTotals: %v", err)
	}
	want := []accounting.DimensionTotal{
		{Value: "Határrét", Total: 0, Entries: 2},
		{Value: "Kertváros", Total: 0, Entries: 1},
		{Value: "Malomsor", Total: 0, Entries: 2},
		{Value: "Óváros", Total: 0, Entries: 3},
	}
	if !reflect.DeepEqual(got.Totals, want) {
		t.Fatalf("totals = %+v, want %+v", got.Totals, want)
	}
}

func TestDimensionTotalsCountsOnlyTheAccountTypeAskedFor(t *testing.T) {
	l := zoldhalomYear(t)

	asset, err := l.repo.DimensionTotals(t.Context(), "neighbourhood", string(accounting.AccountTypeAsset), "", "")
	if err != nil {
		t.Fatalf("DimensionTotals: %v", err)
	}
	want := []accounting.DimensionTotal{
		{Value: "Határrét", Total: 0, Entries: 2},
		{Value: "Kertváros", Total: -200, Entries: 1},
		{Value: "Malomsor", Total: -100, Entries: 2},
		{Value: "Óváros", Total: -1150, Entries: 3},
	}
	if !reflect.DeepEqual(asset.Totals, want) {
		t.Fatalf("asset totals = %+v, want %+v", asset.Totals, want)
	}

	income, err := l.repo.DimensionTotals(t.Context(), "neighbourhood", string(accounting.AccountTypeIncome), "", "")
	if err != nil {
		t.Fatalf("DimensionTotals(income): %v", err)
	}
	if len(income.Totals) != 0 {
		t.Fatalf("income totals = %+v, want none because no entry touches an income account", income.Totals)
	}
}

func TestDimensionTotalsSkipsEntriesThatDoNotCarryTheKey(t *testing.T) {
	l := zoldhalomYear(t)

	got, err := l.repo.DimensionTotals(t.Context(), "department", string(accounting.AccountTypeExpense), "", "")
	if err != nil {
		t.Fatalf("DimensionTotals: %v", err)
	}
	want := []accounting.DimensionTotal{{Value: "roads", Total: 5, Entries: 1}}
	if !reflect.DeepEqual(got.Totals, want) {
		t.Fatalf("totals = %+v, want %+v", got.Totals, want)
	}
}

func TestDimensionTotalsPeriodStartsInclusiveEndsExclusiveAndFallsBackToPostedAt(t *testing.T) {
	l := zoldhalomYear(t)

	got, err := l.repo.DimensionTotals(t.Context(), "neighbourhood", string(accounting.AccountTypeExpense), januaryOpens, februaryOpens)
	if err != nil {
		t.Fatalf("DimensionTotals: %v", err)
	}
	want := []accounting.DimensionTotal{
		{Value: "Határrét", Total: 0, Entries: 2},
		{Value: "Kertváros", Total: 200, Entries: 1},
		{Value: "Malomsor", Total: 30, Entries: 1},
		{Value: "Óváros", Total: 150, Entries: 2},
	}
	if got.PeriodStart != januaryOpens || got.PeriodEnd != februaryOpens {
		t.Fatalf("totals = %+v, want the period echoed", got)
	}
	if !reflect.DeepEqual(got.Totals, want) {
		t.Fatalf("totals = %+v, want %+v", got.Totals, want)
	}
}

func TestDimensionTotalsOverAPeriodWithNoEntriesIsEmptyNotAnError(t *testing.T) {
	l := zoldhalomYear(t)

	got, err := l.repo.DimensionTotals(t.Context(), "neighbourhood", string(accounting.AccountTypeExpense), "2025-01-01T00:00:00Z", "2025-02-01T00:00:00Z")
	if err != nil {
		t.Fatalf("DimensionTotals: %v", err)
	}
	if len(got.Totals) != 0 {
		t.Fatalf("totals = %+v, want none", got.Totals)
	}
}

func TestDimensionTotalsRefusesABadQuery(t *testing.T) {
	l := zoldhalomYear(t)
	expense := string(accounting.AccountTypeExpense)

	for name, call := range map[string]struct{ key, kind, start, end string }{
		"empty key":            {"", expense, "", ""},
		"malformed key":        {"Not A Key", expense, "", ""},
		"unknown account type": {"neighbourhood", "bogus", "", ""},
		"only a period start":  {"neighbourhood", expense, januaryOpens, ""},
		"only a period end":    {"neighbourhood", expense, "", februaryOpens},
		"unparseable period":   {"neighbourhood", expense, "January", februaryOpens},
		"inverted period":      {"neighbourhood", expense, februaryOpens, januaryOpens},
		"empty period":         {"neighbourhood", expense, januaryOpens, januaryOpens},
	} {
		if _, err := l.repo.DimensionTotals(t.Context(), call.key, call.kind, call.start, call.end); !errors.Is(err, accounting.ErrInvalid) {
			t.Fatalf("%s: want ErrInvalid, got %v", name, err)
		}
	}
}
