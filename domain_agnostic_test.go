package accounting_test

import (
	"context"
	"path/filepath"
	"testing"

	accounting "github.com/aosanya/mwanachama-backend-accounting"
)

// A second domain, filling the same two roles with its own nouns and its
// own table names. The module learns neither word: if anything here needed
// "member" or "basket" to work, it would fail against a school.
func TestASecondDomainRunsTheSameLedger(t *testing.T) {
	s, err := accounting.LoadSpec(filepath.Join(".", "spec", "examples", "school.accounting.json"))
	if err != nil {
		t.Fatalf("load spec: %v", err)
	}

	accounts, ok := s.ByRole("account")
	if !ok {
		t.Fatal("the school spec fills no account")
	}
	if got, want := s.RawNameFor(accounts), "accounting_main_fee_accounts"; got != want {
		t.Fatalf("accounts land in %q, want %q", got, want)
	}
	entries, ok := s.ByRole("entry")
	if !ok {
		t.Fatal("the school spec fills no entry")
	}
	if got, want := s.RawNameFor(entries), "accounting_main_postings"; got != want {
		t.Fatalf("entries land in %q, want %q", got, want)
	}

	r := newLedgerOn(t, openDB(t), s)
	ctx := context.Background()

	pupil, err := r.OpenAccount(ctx, accounting.Account{
		Code: "1200", Name: "Fees receivable — Wanjiru",
		Category: "pupil_fees", Type: accounting.AccountTypeAsset, HolderID: "pupil-88",
	})
	if err != nil {
		t.Fatalf("open pupil account: %v", err)
	}
	tuition, err := r.OpenAccount(ctx, accounting.Account{
		Code: "4000", Name: "Tuition income",
		Category: "tuition_income", Type: accounting.AccountTypeIncome, HolderID: "brookside",
	})
	if err != nil {
		t.Fatalf("open tuition account: %v", err)
	}

	if _, err := r.Post(ctx, accounting.Entry{
		DebitAccountID: pupil.ID, CreditAccountID: tuition.ID, Amount: 45000,
		DocumentKind: "invoice", DocumentID: "INV-2026-114", ActorID: "bursar-2",
		Narration: "Term 1 tuition",
	}); err != nil {
		t.Fatalf("post invoice: %v", err)
	}

	owed, err := r.Balance(ctx, pupil.ID)
	if err != nil {
		t.Fatal(err)
	}
	if owed != 45000 {
		t.Fatalf("pupil receivable = %d, want 45000", owed)
	}
	earned, err := r.Balance(ctx, tuition.ID)
	if err != nil {
		t.Fatal(err)
	}
	if owed+earned != 0 {
		t.Fatalf("ledger does not close: %d + %d != 0", owed, earned)
	}
}

func TestADomainCannotRedeclareAFieldTheModuleOwns(t *testing.T) {
	raw := []byte(`{
	  "module": "accounting",
	  "domain": "rogue",
	  "instance": "rogue",
	  "objects": [
	    {"role": "account", "name": "account", "table": "accounts",
	     "description": "An account whose domain tries to reopen the type vocabulary.",
	     "fields": [{"name": "type", "type": "string", "description": "Anything at all."}]},
	    {"role": "entry", "name": "entry", "table": "entries",
	     "description": "One posting."}
	  ]
	}`)
	if _, err := accounting.ParseSpec(raw); err == nil {
		t.Fatal("want a refusal: a domain may set a default and nothing else on a declared field")
	}
}
