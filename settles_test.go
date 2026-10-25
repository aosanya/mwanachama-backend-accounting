package accounting_test

import (
	"errors"
	"testing"

	accounting "github.com/aosanya/mwanachama-backend-accounting"
)

func pay(t *testing.T, r accounting.LedgerRepository, debit, credit accounting.Account, amount int64, settles string) accounting.Entry {
	t.Helper()
	e, err := r.Post(t.Context(), accounting.Entry{
		DebitAccountID: debit.ID, CreditAccountID: credit.ID, Amount: amount, SettlesEntryID: settles,
		DocumentKind: "receipt", DocumentID: "pay", ActorID: "actor-1",
	})
	if err != nil {
		t.Fatalf("pay: %v", err)
	}
	return e
}

func TestAPaidChargeNoLongerReadsAsOverdue(t *testing.T) {
	r, ctx := newLedger(t)
	holder := openHolder(t, r, "h1")
	pool := openPool(t, r, "p1")

	charge := postDue(t, r, holder, pool, 100, "2026-01-01T00:00:00Z")
	p := pay(t, r, pool, holder, 100, charge.ID)
	if p.SettlesEntryID != charge.ID {
		t.Fatalf("payment = %+v, want it to name the charge", p)
	}

	got, err := r.Overdue(ctx, holder.ID, "2026-02-01T00:00:00Z")
	if err != nil {
		t.Fatalf("Overdue: %v", err)
	}
	if len(got.Items) != 0 {
		t.Fatalf("items = %+v, want a paid charge left out", got.Items)
	}
}

func TestAPartPaidChargeReadsWithWhatIsStillOutstanding(t *testing.T) {
	r, ctx := newLedger(t)
	holder := openHolder(t, r, "h1")
	pool := openPool(t, r, "p1")

	charge := postDue(t, r, holder, pool, 100, "2026-01-01T00:00:00Z")
	pay(t, r, pool, holder, 30, charge.ID)
	pay(t, r, pool, holder, 20, charge.ID)

	got, err := r.Overdue(ctx, holder.ID, "2026-02-01T00:00:00Z")
	if err != nil {
		t.Fatalf("Overdue: %v", err)
	}
	if len(got.Items) != 1 || got.Items[0].ID != charge.ID || got.Items[0].Outstanding != 50 || got.Items[0].Amount != 100 {
		t.Fatalf("items = %+v, want the charge outstanding 50 of 100", got.Items)
	}
}

func TestAnUnpaidChargeReadsOutstandingInFull(t *testing.T) {
	r, ctx := newLedger(t)
	holder := openHolder(t, r, "h1")
	pool := openPool(t, r, "p1")

	postDue(t, r, holder, pool, 100, "2026-01-01T00:00:00Z")
	got, err := r.Overdue(ctx, holder.ID, "2026-02-01T00:00:00Z")
	if err != nil {
		t.Fatalf("Overdue: %v", err)
	}
	if len(got.Items) != 1 || got.Items[0].Outstanding != 100 {
		t.Fatalf("items = %+v, want outstanding 100", got.Items)
	}
}

func TestAReversedPaymentNoLongerSettlesItsCharge(t *testing.T) {
	r, ctx := newLedger(t)
	holder := openHolder(t, r, "h1")
	pool := openPool(t, r, "p1")

	charge := postDue(t, r, holder, pool, 100, "2026-01-01T00:00:00Z")
	p := pay(t, r, pool, holder, 100, charge.ID)
	if _, err := r.Post(ctx, accounting.Entry{
		DebitAccountID: holder.ID, CreditAccountID: pool.ID, Amount: 100,
		DocumentKind: "receipt", DocumentID: "bounced", ActorID: "actor-1", ReversesEntryID: p.ID,
	}); err != nil {
		t.Fatalf("reverse payment: %v", err)
	}

	got, err := r.Overdue(ctx, holder.ID, "2026-02-01T00:00:00Z")
	if err != nil {
		t.Fatalf("Overdue: %v", err)
	}
	if len(got.Items) != 1 || got.Items[0].Outstanding != 100 {
		t.Fatalf("items = %+v, want the charge overdue again in full", got.Items)
	}
}

func TestAPaymentSettlingAnEntryThatDoesNotExistIsRefused(t *testing.T) {
	r, ctx := newLedger(t)
	holder := openHolder(t, r, "h1")
	pool := openPool(t, r, "p1")

	_, err := r.Post(ctx, accounting.Entry{
		DebitAccountID: pool.ID, CreditAccountID: holder.ID, Amount: 100, SettlesEntryID: "nope",
		DocumentKind: "receipt", DocumentID: "pay", ActorID: "actor-1",
	})
	if !errors.Is(err, accounting.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	entries, err := r.ListEntries(ctx, holder.ID, 0)
	if err != nil {
		t.Fatalf("ListEntries: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("entries = %+v, want nothing posted", entries)
	}
}
