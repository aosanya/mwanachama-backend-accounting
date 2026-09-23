package accounting

import (
	"context"
	"errors"
	"testing"
)

// RunLedgerConformance exercises the rules that matter against any
// LedgerRepository implementation — memory or Postgres. Both backends must
// agree, per this platform's "test both stores" rule (nothing forces a
// memory backend and a Postgres backend to behave alike unless the same
// suite runs against both).
//
// Deliberately domain-agnostic: every Kind/DocumentKind value here is
// arbitrary test data, not a vocabulary this package defines — a domain
// layered on top (e.g. the contribution subpackage) runs its own
// conformance-style suite in its own vocabulary against the same
// LedgerRepository.
//
// newRepo returns a fresh, empty repository for each subtest, given that
// subtest's own *testing.T — required so a backend that needs to skip (e.g.
// Postgres when POSTGRES_URL is unset) calls Skip/Fatal on the subtest
// itself rather than on RunLedgerConformance's caller, which panics ("may
// have called FailNow on a parent test").
func RunLedgerConformance(t *testing.T, newRepo func(t *testing.T) LedgerRepository) {
	t.Helper()

	openHolder := func(t *testing.T, r LedgerRepository, holderID string) Account {
		t.Helper()
		a, err := r.OpenAccount(context.Background(), Account{
			Kind: "holder", Type: AccountTypeIncome, HolderID: holderID,
		})
		if err != nil {
			t.Fatalf("OpenAccount(holder %s): %v", holderID, err)
		}
		return a
	}
	openPool := func(t *testing.T, r LedgerRepository, poolID string) Account {
		t.Helper()
		a, err := r.OpenAccount(context.Background(), Account{
			Kind: "pool", Type: AccountTypeAsset, HolderID: poolID,
		})
		if err != nil {
			t.Fatalf("OpenAccount(pool %s): %v", poolID, err)
		}
		return a
	}

	t.Run("OpenAccountIsLazyAndIdempotent", func(t *testing.T) {
		r := newRepo(t)
		a1 := openHolder(t, r, "holder-1")
		a2 := openHolder(t, r, "holder-1")
		if a1.ID != a2.ID {
			t.Fatalf("opening the same (kind, holder) twice minted two accounts: %s != %s", a1.ID, a2.ID)
		}

		accounts, err := r.ListAccounts(context.Background(), 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(accounts) != 1 {
			t.Fatalf("want 1 account, got %d", len(accounts))
		}
	})

	t.Run("OpenAccountKeysOnKindAndHolderTogether", func(t *testing.T) {
		r := newRepo(t)
		holder := openHolder(t, r, "same-id")
		pool := openPool(t, r, "same-id")
		if holder.ID == pool.ID {
			t.Fatalf("two different kinds sharing a holder id must be distinct accounts, got the same id %s", holder.ID)
		}
	})

	t.Run("OpenAccountRejectsEmptyKindOrHolder", func(t *testing.T) {
		r := newRepo(t)
		if _, err := r.OpenAccount(context.Background(), Account{Kind: "", Type: AccountTypeAsset, HolderID: "x"}); !errors.Is(err, ErrInvalid) {
			t.Fatalf("want ErrInvalid for empty kind, got %v", err)
		}
		if _, err := r.OpenAccount(context.Background(), Account{Kind: "pool", Type: AccountTypeAsset, HolderID: ""}); !errors.Is(err, ErrInvalid) {
			t.Fatalf("want ErrInvalid for empty holder, got %v", err)
		}
	})

	t.Run("PostEntryCreditsOneAccountDebitsTheOther", func(t *testing.T) {
		r := newRepo(t)
		ctx := context.Background()
		holder := openHolder(t, r, "holder-1")
		pool := openPool(t, r, "pool-1")

		entry, err := r.Post(ctx, Entry{
			FromAccountID: pool.ID,
			ToAccountID:   holder.ID,
			Amount:        10000,
			DocumentKind:  "test-document",
			DocumentID:    "doc-1",
			ActorID:       "actor-1",
			Detail:        "a posting",
		})
		if err != nil {
			t.Fatalf("Post: %v", err)
		}
		if entry.ID == "" || entry.PostedAt.IsZero() {
			t.Fatalf("Post did not fill in ID/PostedAt: %+v", entry)
		}

		holderBalance, err := r.Balance(ctx, holder.ID)
		if err != nil {
			t.Fatal(err)
		}
		if holderBalance != 10000 {
			t.Fatalf("holder balance = %d, want 10000", holderBalance)
		}

		poolBalance, err := r.Balance(ctx, pool.ID)
		if err != nil {
			t.Fatal(err)
		}
		if poolBalance != -10000 {
			t.Fatalf("pool balance = %d, want -10000", poolBalance)
		}

		// The ledger is zero-sum by construction: every entry's two sides
		// always net to zero across the whole set of accounts.
		if holderBalance+poolBalance != 0 {
			t.Fatalf("ledger does not close: holder %d + pool %d != 0", holderBalance, poolBalance)
		}
	})

	t.Run("EntryIsAppendOnlyCorrectedByReversal", func(t *testing.T) {
		r := newRepo(t)
		ctx := context.Background()
		holder := openHolder(t, r, "holder-1")
		pool := openPool(t, r, "pool-1")

		original, err := r.Post(ctx, Entry{
			FromAccountID: pool.ID, ToAccountID: holder.ID, Amount: 5000,
			DocumentKind: "test-document", DocumentID: "doc-2", ActorID: "actor-1",
		})
		if err != nil {
			t.Fatal(err)
		}

		reversal, err := r.Post(ctx, Entry{
			FromAccountID: holder.ID, ToAccountID: pool.ID, Amount: 5000,
			DocumentKind: "correction", DocumentID: "doc-2",
			ActorID: "actor-1", Detail: "wrong holder matched", ReversesEntryID: original.ID,
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
			t.Fatalf("holder balance after reversal = %d, want 0 — the original must still be in the ledger, uncorrected in place", balance)
		}

		entries, err := r.ListEntries(ctx, holder.ID, 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 2 {
			t.Fatalf("want 2 entries (original + reversal), got %d", len(entries))
		}
	})

	t.Run("PostRefusesOneSidedEntry", func(t *testing.T) {
		r := newRepo(t)
		holder := openHolder(t, r, "holder-1")

		_, err := r.Post(context.Background(), Entry{
			ToAccountID: holder.ID, Amount: 100,
			DocumentKind: "test-document", DocumentID: "x", ActorID: "actor-1",
		})
		if !errors.Is(err, ErrInvalid) {
			t.Fatalf("want ErrInvalid for a one-sided posting, got %v", err)
		}
	})

	t.Run("PostRefusesClosedAccount", func(t *testing.T) {
		r := newRepo(t)
		ctx := context.Background()
		holder := openHolder(t, r, "holder-1")
		pool := openPool(t, r, "pool-1")

		closed, err := r.CloseAccount(ctx, holder.ID)
		if err != nil {
			t.Fatalf("CloseAccount on a zero-balance account should succeed: %v", err)
		}
		if closed.ClosedAt == nil {
			t.Fatal("CloseAccount did not stamp ClosedAt")
		}

		_, err = r.Post(ctx, Entry{
			FromAccountID: pool.ID, ToAccountID: holder.ID, Amount: 100,
			DocumentKind: "test-document", DocumentID: "x", ActorID: "actor-1",
		})
		if !errors.Is(err, ErrClosed) {
			t.Fatalf("want ErrClosed posting against a closed account, got %v", err)
		}
	})

	t.Run("CloseAccountRefusesNonZeroBalance", func(t *testing.T) {
		r := newRepo(t)
		ctx := context.Background()
		holder := openHolder(t, r, "holder-1")
		pool := openPool(t, r, "pool-1")

		if _, err := r.Post(ctx, Entry{
			FromAccountID: pool.ID, ToAccountID: holder.ID, Amount: 100,
			DocumentKind: "test-document", DocumentID: "x", ActorID: "actor-1",
		}); err != nil {
			t.Fatal(err)
		}

		_, err := r.CloseAccount(ctx, holder.ID)
		if !errors.Is(err, ErrNonZeroBalance) {
			t.Fatalf("want ErrNonZeroBalance, got %v", err)
		}
	})

	t.Run("PostRefusesUnknownReversesEntryID", func(t *testing.T) {
		r := newRepo(t)
		ctx := context.Background()
		holder := openHolder(t, r, "holder-1")
		pool := openPool(t, r, "pool-1")

		_, err := r.Post(ctx, Entry{
			FromAccountID: pool.ID, ToAccountID: holder.ID, Amount: 100,
			DocumentKind: "correction", DocumentID: "x", ActorID: "actor-1",
			ReversesEntryID: "does-not-exist",
		})
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("want ErrNotFound for an unknown ReversesEntryID, got %v", err)
		}
	})

	// W15_PostDoesNotRefuseADoubleReversal pins a known defect (board row
	// W15): Post only checks that ReversesEntryID names an existing entry —
	// it never checks that entry has not already been reversed. A second
	// Post naming the same ReversesEntryID succeeds exactly like the first,
	// so the "original" entry silently gets corrected twice and the
	// holder's balance drifts away from zero. This test must go RED once
	// W15's fix lands (Post must refuse a ReversesEntryID that some
	// existing entry already reverses) — flip the two Fatalf calls below to
	// assert the second Post is rejected and the balance stays at 0.
	t.Run("W15_PostDoesNotRefuseADoubleReversal", func(t *testing.T) {
		r := newRepo(t)
		ctx := context.Background()
		holder := openHolder(t, r, "holder-1")
		pool := openPool(t, r, "pool-1")

		original, err := r.Post(ctx, Entry{
			FromAccountID: pool.ID, ToAccountID: holder.ID, Amount: 1000,
			DocumentKind: "test-document", DocumentID: "doc-1", ActorID: "actor-1",
		})
		if err != nil {
			t.Fatal(err)
		}

		if _, err := r.Post(ctx, Entry{
			FromAccountID: holder.ID, ToAccountID: pool.ID, Amount: 1000,
			DocumentKind: "correction", DocumentID: "doc-1", ActorID: "actor-1",
			ReversesEntryID: original.ID,
		}); err != nil {
			t.Fatalf("first reversal should succeed: %v", err)
		}

		balanceAfterOneReversal, err := r.Balance(ctx, holder.ID)
		if err != nil {
			t.Fatal(err)
		}
		if balanceAfterOneReversal != 0 {
			t.Fatalf("balance after one reversal = %d, want 0", balanceAfterOneReversal)
		}

		// KNOWN DEFECT (W15): a second Post reversing the same original
		// entry is accepted with no error at all.
		if _, err := r.Post(ctx, Entry{
			FromAccountID: holder.ID, ToAccountID: pool.ID, Amount: 1000,
			DocumentKind: "correction", DocumentID: "doc-1", ActorID: "actor-1",
			ReversesEntryID: original.ID,
		}); err != nil {
			t.Fatalf("pin: current (buggy) behaviour accepts a second reversal of the same entry; got an error instead, meaning W15 may already be fixed: %v", err)
		}

		// KNOWN DEFECT (W15): the double reversal drags the holder's
		// balance to -1000 instead of leaving it at 0.
		balanceAfterDoubleReversal, err := r.Balance(ctx, holder.ID)
		if err != nil {
			t.Fatal(err)
		}
		if balanceAfterDoubleReversal != -1000 {
			t.Fatalf("pin: current (buggy) behaviour drives the balance to -1000 after a double reversal; got %d instead, meaning W15 may already be fixed", balanceAfterDoubleReversal)
		}
	})
}
