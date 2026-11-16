package accounting

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestBalanceOfDebitsAddAndCreditsSubtract(t *testing.T) {
	entries := []Entry{
		{DebitAccountID: "cash", CreditAccountID: "sales", Amount: 500},
		{DebitAccountID: "rent", CreditAccountID: "cash", Amount: 120},
		{DebitAccountID: "rent", CreditAccountID: "bank", Amount: 40},
	}
	if got := balanceOf("cash", entries); got != 380 {
		t.Fatalf("cash %d", got)
	}
	if got := balanceOf("sales", entries); got != -500 {
		t.Fatalf("sales %d", got)
	}
	if got := balanceOf("untouched", entries); got != 0 {
		t.Fatalf("untouched %d", got)
	}
}

func TestOrderKeyPrefersWhenItHappenedOverWhenItWasPosted(t *testing.T) {
	if got := orderKey(Entry{PostedAt: "2026-02-01T00:00:00Z", OccurredAt: "2026-01-15T00:00:00Z"}); got != "2026-01-15T00:00:00Z" {
		t.Fatalf("got %q", got)
	}
	if got := orderKey(Entry{PostedAt: "2026-02-01T00:00:00Z"}); got != "2026-02-01T00:00:00Z" {
		t.Fatalf("got %q", got)
	}
}

func TestOnlyTheFiveAccountTypesAreAccepted(t *testing.T) {
	for _, ok := range []AccountType{AccountTypeAsset, AccountTypeLiability, AccountTypeEquity, AccountTypeIncome, AccountTypeExpense} {
		if !IsAccountType(ok) {
			t.Fatalf("%q refused", ok)
		}
	}
	for _, bad := range []AccountType{"", "revenue", "Asset"} {
		if IsAccountType(bad) {
			t.Fatalf("%q accepted", bad)
		}
	}
}

func TestEntryRefusalsBeyondTheSpec(t *testing.T) {
	good := Entry{DebitAccountID: "a", CreditAccountID: "b", Amount: 10}
	if err := checkEntryBeyondSpec(good); err != nil {
		t.Fatalf("good entry refused: %v", err)
	}
	cases := map[string]Entry{
		"zero amount":     {DebitAccountID: "a", CreditAccountID: "b", Amount: 0},
		"negative amount": {DebitAccountID: "a", CreditAccountID: "b", Amount: -5},
		"same account":    {DebitAccountID: "a", CreditAccountID: " a ", Amount: 10},
		"bad due":         {DebitAccountID: "a", CreditAccountID: "b", Amount: 10, DueAt: "next week"},
		"bad dimension":   {DebitAccountID: "a", CreditAccountID: "b", Amount: 10, Dimensions: map[string]string{"Ward": "3"}},
	}
	for name, e := range cases {
		if err := checkEntryBeyondSpec(e); !errors.Is(err, ErrInvalid) {
			t.Fatalf("%s: err %v", name, err)
		}
	}
	due := good
	due.DueAt = "2026-06-20T00:00:00Z"
	if err := checkEntryBeyondSpec(due); err != nil {
		t.Fatalf("due refused: %v", err)
	}
}

func TestDimensionRules(t *testing.T) {
	many := map[string]string{}
	for i := 0; i <= MaxDimensions; i++ {
		many["k"+strings.Repeat("x", i)] = "v"
	}
	if err := checkDimensions(many); !errors.Is(err, ErrInvalid) {
		t.Fatalf("too many: %v", err)
	}
	if err := checkDimensions(map[string]string{"ward": "3", "project_2": "road"}); err != nil {
		t.Fatalf("valid refused: %v", err)
	}
	for _, key := range []string{"", "Ward", "2ward", "ward-3", strings.Repeat("a", 64)} {
		if err := checkDimensionKey(key); !errors.Is(err, ErrInvalid) {
			t.Fatalf("key %q: %v", key, err)
		}
	}
	if err := checkDimensionValue("ward", "   "); !errors.Is(err, ErrInvalid) {
		t.Fatalf("blank value: %v", err)
	}
	if err := checkDimensionValue("ward", strings.Repeat("é", MaxDimensionValue+1)); !errors.Is(err, ErrInvalid) {
		t.Fatalf("long value: %v", err)
	}
	if err := checkDimensionValue("ward", string([]byte{0xff, 0xfe})); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid utf8: %v", err)
	}
	if err := checkDimensionQuery("ward", "3"); err != nil {
		t.Fatalf("query: %v", err)
	}
	if err := checkDimensionQuery("Ward", "3"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("query key: %v", err)
	}
}

func TestCarryingKeepsOnlyEntriesWithThatDimensionValue(t *testing.T) {
	entries := []Entry{
		{ID: "1", Dimensions: map[string]string{"ward": "3"}},
		{ID: "2", Dimensions: map[string]string{"ward": "4"}},
		{ID: "3"},
	}
	got := carrying(entries, "ward", "3")
	if len(got) != 1 || got[0].ID != "1" {
		t.Fatalf("got %+v", got)
	}
	if none := carrying(entries, "ward", "9"); none == nil || len(none) != 0 {
		t.Fatalf("none %+v", none)
	}
}

func TestPeriodMustBeTwoInstantsInOrder(t *testing.T) {
	from, to, err := parsePeriod(" 2026-01-01T00:00:00Z ", "2026-02-01T00:00:00Z")
	if err != nil || !from.Equal(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)) || !to.Equal(time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("%v %v %v", from, to, err)
	}
	for name, p := range map[string][2]string{
		"bad start":  {"January", "2026-02-01T00:00:00Z"},
		"bad end":    {"2026-01-01T00:00:00Z", "February"},
		"equal":      {"2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z"},
		"end before": {"2026-02-01T00:00:00Z", "2026-01-01T00:00:00Z"},
	} {
		if _, _, err := parsePeriod(p[0], p[1]); !errors.Is(err, ErrInvalid) {
			t.Fatalf("%s: %v", name, err)
		}
	}
}

func TestBalanceWithinCountsTheStartAndStopsBeforeTheEnd(t *testing.T) {
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	entries := []Entry{
		{DebitAccountID: "x", CreditAccountID: "y", Amount: 1, PostedAt: "2025-12-31T23:59:59Z"},
		{DebitAccountID: "x", CreditAccountID: "y", Amount: 10, PostedAt: "2026-01-01T00:00:00Z"},
		{DebitAccountID: "x", CreditAccountID: "y", Amount: 100, PostedAt: "2026-03-01T00:00:00Z", OccurredAt: "2026-01-20T00:00:00Z"},
		{DebitAccountID: "x", CreditAccountID: "y", Amount: 1000, PostedAt: "2026-02-01T00:00:00Z"},
		{DebitAccountID: "x", CreditAccountID: "y", Amount: 10000, PostedAt: "not a time"},
	}
	if got := balanceWithin("x", entries, from, to); got != 110 {
		t.Fatalf("got %d", got)
	}
	if got := balanceWithin("y", entries, from, to); got != -110 {
		t.Fatalf("credit side %d", got)
	}
}

func TestSettlementsSkipReversedPaymentsAndReversalsThemselves(t *testing.T) {
	entries := []Entry{
		{ID: "p1", SettlesEntryID: "c1", Amount: 100},
		{ID: "p2", SettlesEntryID: "c1", Amount: 50},
		{ID: "r2", ReversesEntryID: "p2", Amount: 50},
		{ID: "p3", SettlesEntryID: "c2", Amount: 70},
	}
	reversed := reversedIDs(entries)
	if !reversed["p2"] || len(reversed) != 1 {
		t.Fatalf("reversed %v", reversed)
	}
	paid := paidAgainst(entries, reversed)
	if paid["c1"] != 100 || paid["c2"] != 70 {
		t.Fatalf("paid %v", paid)
	}
}
