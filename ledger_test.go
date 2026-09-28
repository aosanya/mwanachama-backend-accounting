package accounting_test

import (
	"errors"
	"testing"

	accounting "github.com/aosanya/mwanachama-backend-accounting"
)

func TestOpenAccountIsLazyAndIdempotent(t *testing.T) {
	r, ctx := newLedger(t)
	a1 := openHolder(t, r, "holder-1")
	a2 := openHolder(t, r, "holder-1")
	if a1.ID != a2.ID {
		t.Fatalf("opening the same (category, holder) twice minted two accounts: %s != %s", a1.ID, a2.ID)
	}

	accounts, err := r.ListAccounts(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(accounts) != 1 {
		t.Fatalf("want 1 account, got %d", len(accounts))
	}
}

func TestOpenAccountKeysOnCategoryAndHolderTogether(t *testing.T) {
	r, _ := newLedger(t)
	holder := openHolder(t, r, "same-id")
	pool := openPool(t, r, "same-id")
	if holder.ID == pool.ID {
		t.Fatalf("two categories sharing a holder id must be distinct accounts, got the same id %s", holder.ID)
	}
}

func TestOpenAccountRejectsEmptyCategoryOrHolder(t *testing.T) {
	r, ctx := newLedger(t)
	if _, err := r.OpenAccount(ctx, accounting.Account{Category: "", Type: accounting.AccountTypeAsset, HolderID: "x"}); !errors.Is(err, accounting.ErrInvalid) {
		t.Fatalf("want ErrInvalid for empty category, got %v", err)
	}
	if _, err := r.OpenAccount(ctx, accounting.Account{Category: "pool", Type: accounting.AccountTypeAsset, HolderID: ""}); !errors.Is(err, accounting.ErrInvalid) {
		t.Fatalf("want ErrInvalid for empty holder, got %v", err)
	}
}

func TestOpenAccountRejectsUnknownType(t *testing.T) {
	r, ctx := newLedger(t)
	if _, err := r.OpenAccount(ctx, accounting.Account{Category: "pool", Type: "bogus", HolderID: "p"}); !errors.Is(err, accounting.ErrInvalid) {
		t.Fatalf("want ErrInvalid for unknown type, got %v", err)
	}
}

func TestOpenAccountRefusesACodeAnotherAccountCarries(t *testing.T) {
	r, ctx := newLedger(t)
	if _, err := r.OpenAccount(ctx, accounting.Account{
		Code: "1000", Name: "Cash at bank", Category: "bank",
		Type: accounting.AccountTypeAsset, HolderID: "bank-1",
	}); err != nil {
		t.Fatal(err)
	}

	_, err := r.OpenAccount(ctx, accounting.Account{
		Code: "1000", Category: "bank", Type: accounting.AccountTypeAsset, HolderID: "bank-2",
	})
	if !errors.Is(err, accounting.ErrCodeTaken) {
		t.Fatalf("want ErrCodeTaken for a code another account carries, got %v", err)
	}
}

func TestOpenAccountAllowsManyAccountsWithNoCode(t *testing.T) {
	r, ctx := newLedger(t)
	openHolder(t, r, "holder-1")
	openHolder(t, r, "holder-2")
	openPool(t, r, "pool-1")

	accounts, err := r.ListAccounts(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(accounts) != 3 {
		t.Fatalf("want 3 codeless accounts, got %d — a partial unique index must not treat the empty code as a value", len(accounts))
	}
}

func TestPostDebitsOneAccountAndCreditsTheOther(t *testing.T) {
	r, ctx := newLedger(t)
	holder := openHolder(t, r, "holder-1")
	pool := openPool(t, r, "pool-1")

	entry, err := r.Post(ctx, accounting.Entry{
		DebitAccountID:  holder.ID,
		CreditAccountID: pool.ID,
		Amount:          10000,
		DocumentKind:    "test-document",
		DocumentID:      "doc-1",
		ActorID:         "actor-1",
		Narration:       "a posting",
	})
	if err != nil {
		t.Fatalf("Post: %v", err)
	}
	if entry.ID == "" || entry.PostedAt == "" {
		t.Fatalf("Post did not fill in ID/PostedAt: %+v", entry)
	}

	debited, err := r.Balance(ctx, holder.ID)
	if err != nil {
		t.Fatal(err)
	}
	if debited != 10000 {
		t.Fatalf("debited account balance = %d, want 10000", debited)
	}

	credited, err := r.Balance(ctx, pool.ID)
	if err != nil {
		t.Fatal(err)
	}
	if credited != -10000 {
		t.Fatalf("credited account balance = %d, want -10000", credited)
	}

	if debited+credited != 0 {
		t.Fatalf("ledger does not close: %d + %d != 0", debited, credited)
	}
}

func TestEntryIsAppendOnlyCorrectedByReversal(t *testing.T) {
	r, ctx := newLedger(t)
	holder := openHolder(t, r, "holder-1")
	pool := openPool(t, r, "pool-1")

	original, err := r.Post(ctx, accounting.Entry{
		DebitAccountID: holder.ID, CreditAccountID: pool.ID, Amount: 5000,
		DocumentKind: "test-document", DocumentID: "doc-2", ActorID: "actor-1",
	})
	if err != nil {
		t.Fatal(err)
	}

	reversal, err := r.Post(ctx, accounting.Entry{
		DebitAccountID: pool.ID, CreditAccountID: holder.ID, Amount: 5000,
		DocumentKind: "correction", DocumentID: "doc-2",
		ActorID: "actor-1", Narration: "wrong holder matched", ReversesEntryID: original.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if reversal.ReversesEntryID != original.ID {
		t.Fatalf("reversal does not point at the original")
	}

	balance, err := r.Balance(ctx, holder.ID)
	if err != nil {
		t.Fatal(err)
	}
	if balance != 0 {
		t.Fatalf("balance after reversal = %d, want 0 — the original must still be in the ledger, uncorrected in place", balance)
	}

	entries, err := r.ListEntries(ctx, holder.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("want 2 entries (original + reversal), got %d", len(entries))
	}
}

func TestPostRefusesOneSidedEntry(t *testing.T) {
	r, ctx := newLedger(t)
	holder := openHolder(t, r, "holder-1")

	_, err := r.Post(ctx, accounting.Entry{
		DebitAccountID: holder.ID, Amount: 100,
		DocumentKind: "test-document", DocumentID: "x", ActorID: "actor-1",
	})
	if !errors.Is(err, accounting.ErrInvalid) {
		t.Fatalf("want ErrInvalid for a one-sided posting, got %v", err)
	}
}

func TestPostRefusesDebitingAndCreditingOneAccount(t *testing.T) {
	r, ctx := newLedger(t)
	holder := openHolder(t, r, "holder-1")

	_, err := r.Post(ctx, accounting.Entry{
		DebitAccountID: holder.ID, CreditAccountID: holder.ID, Amount: 100,
		DocumentKind: "test-document", DocumentID: "x", ActorID: "actor-1",
	})
	if !errors.Is(err, accounting.ErrInvalid) {
		t.Fatalf("want ErrInvalid, got %v", err)
	}
}

func TestPostRefusesNonPositiveAmount(t *testing.T) {
	r, ctx := newLedger(t)
	holder := openHolder(t, r, "holder-1")
	pool := openPool(t, r, "pool-1")

	for _, amount := range []int64{0, -1} {
		_, err := r.Post(ctx, accounting.Entry{
			DebitAccountID: holder.ID, CreditAccountID: pool.ID, Amount: amount,
			DocumentKind: "test-document", DocumentID: "x", ActorID: "actor-1",
		})
		if !errors.Is(err, accounting.ErrInvalid) {
			t.Fatalf("amount %d: want ErrInvalid, got %v", amount, err)
		}
	}
}

func TestPostRefusesClosedAccount(t *testing.T) {
	r, ctx := newLedger(t)
	holder := openHolder(t, r, "holder-1")
	pool := openPool(t, r, "pool-1")

	closed, err := r.CloseAccount(ctx, holder.ID)
	if err != nil {
		t.Fatalf("CloseAccount on a zero-balance account should succeed: %v", err)
	}
	if closed.ClosedAt == "" {
		t.Fatal("CloseAccount did not stamp ClosedAt")
	}

	_, err = r.Post(ctx, accounting.Entry{
		DebitAccountID: holder.ID, CreditAccountID: pool.ID, Amount: 100,
		DocumentKind: "test-document", DocumentID: "x", ActorID: "actor-1",
	})
	if !errors.Is(err, accounting.ErrClosed) {
		t.Fatalf("want ErrClosed posting against a closed account, got %v", err)
	}
}

func TestCloseAccountRefusesNonZeroBalance(t *testing.T) {
	r, ctx := newLedger(t)
	holder := openHolder(t, r, "holder-1")
	pool := openPool(t, r, "pool-1")

	if _, err := r.Post(ctx, accounting.Entry{
		DebitAccountID: holder.ID, CreditAccountID: pool.ID, Amount: 100,
		DocumentKind: "test-document", DocumentID: "x", ActorID: "actor-1",
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := r.CloseAccount(ctx, holder.ID); !errors.Is(err, accounting.ErrNonZeroBalance) {
		t.Fatalf("want ErrNonZeroBalance, got %v", err)
	}
}

func TestCloseAccountRefusesAnAlreadyClosedAccount(t *testing.T) {
	r, ctx := newLedger(t)
	holder := openHolder(t, r, "holder-1")

	if _, err := r.CloseAccount(ctx, holder.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := r.CloseAccount(ctx, holder.ID); !errors.Is(err, accounting.ErrClosed) {
		t.Fatalf("want ErrClosed, got %v", err)
	}
}

func TestPostRefusesUnknownReversesEntryID(t *testing.T) {
	r, ctx := newLedger(t)
	holder := openHolder(t, r, "holder-1")
	pool := openPool(t, r, "pool-1")

	_, err := r.Post(ctx, accounting.Entry{
		DebitAccountID: holder.ID, CreditAccountID: pool.ID, Amount: 100,
		DocumentKind: "correction", DocumentID: "x", ActorID: "actor-1",
		ReversesEntryID: "does-not-exist",
	})
	if !errors.Is(err, accounting.ErrNotFound) {
		t.Fatalf("want ErrNotFound for an unknown ReversesEntryID, got %v", err)
	}
}

func TestPostRefusesASecondReversalOfTheSameEntry(t *testing.T) {
	r, ctx := newLedger(t)
	holder := openHolder(t, r, "holder-1")
	pool := openPool(t, r, "pool-1")

	original, err := r.Post(ctx, accounting.Entry{
		DebitAccountID: holder.ID, CreditAccountID: pool.ID, Amount: 1000,
		DocumentKind: "test-document", DocumentID: "doc-1", ActorID: "actor-1",
	})
	if err != nil {
		t.Fatal(err)
	}

	reversal := accounting.Entry{
		DebitAccountID: pool.ID, CreditAccountID: holder.ID, Amount: 1000,
		DocumentKind: "correction", DocumentID: "doc-1", ActorID: "actor-1",
		ReversesEntryID: original.ID,
	}
	if _, err := r.Post(ctx, reversal); err != nil {
		t.Fatalf("first reversal should succeed: %v", err)
	}

	balance, err := r.Balance(ctx, holder.ID)
	if err != nil {
		t.Fatal(err)
	}
	if balance != 0 {
		t.Fatalf("balance after one reversal = %d, want 0", balance)
	}

	if _, err := r.Post(ctx, reversal); !errors.Is(err, accounting.ErrAlreadyReversed) {
		t.Fatalf("want ErrAlreadyReversed for a second reversal of the same entry, got %v", err)
	}

	balance, err = r.Balance(ctx, holder.ID)
	if err != nil {
		t.Fatal(err)
	}
	if balance != 0 {
		t.Fatalf("balance after the refused second reversal = %d, want 0 — the ledger has drifted off zero-sum", balance)
	}
}

func TestListEntriesOrdersOnOccurredAtFallingBackToPostedAt(t *testing.T) {
	r, ctx := newLedger(t)
	holder := openHolder(t, r, "holder-1")
	pool := openPool(t, r, "pool-1")

	post := func(amount int64, occurred, document string) {
		t.Helper()
		if _, err := r.Post(ctx, accounting.Entry{
			DebitAccountID: holder.ID, CreditAccountID: pool.ID, Amount: amount,
			DocumentKind: "test-document", DocumentID: document, ActorID: "actor-1",
			OccurredAt: occurred,
		}); err != nil {
			t.Fatal(err)
		}
	}
	post(1, "2026-03-01T00:00:00.000000000Z", "third")
	post(2, "2026-01-01T00:00:00.000000000Z", "first")
	post(3, "2026-02-01T00:00:00.000000000Z", "second")

	entries, err := r.ListEntries(ctx, holder.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"first", "second", "third"}
	if len(entries) != len(want) {
		t.Fatalf("want %d entries, got %d", len(want), len(entries))
	}
	for i, document := range want {
		if entries[i].DocumentID != document {
			t.Fatalf("entry %d is %q, want %q", i, entries[i].DocumentID, document)
		}
	}
}

func TestGetAccountAndBalanceRefuseAnUnknownAccount(t *testing.T) {
	r, ctx := newLedger(t)

	if _, err := r.GetAccount(ctx, "nope"); !errors.Is(err, accounting.ErrNotFound) {
		t.Fatalf("GetAccount: want ErrNotFound, got %v", err)
	}
	if _, err := r.Balance(ctx, "nope"); !errors.Is(err, accounting.ErrNotFound) {
		t.Fatalf("Balance: want ErrNotFound, got %v", err)
	}
	if _, err := r.ListEntries(ctx, "nope", 0); !errors.Is(err, accounting.ErrNotFound) {
		t.Fatalf("ListEntries: want ErrNotFound, got %v", err)
	}
}
