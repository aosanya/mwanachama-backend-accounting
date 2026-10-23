package accounting_test

import (
	"errors"
	"testing"

	accounting "github.com/aosanya/mwanachama-backend-accounting"
)

func postDue(t *testing.T, r accounting.LedgerRepository, debit, credit accounting.Account, amount int64, due string) accounting.Entry {
	t.Helper()
	e, err := r.Post(t.Context(), accounting.Entry{
		DebitAccountID: debit.ID, CreditAccountID: credit.ID, Amount: amount, DueAt: due,
		DocumentKind: "charge", DocumentID: "doc-" + due, ActorID: "actor-1",
	})
	if err != nil {
		t.Fatalf("Post: %v", err)
	}
	return e
}

func TestOverdueAgesFromTheDueDateNotThePostingDate(t *testing.T) {
	r, ctx := newLedger(t)
	holder := openHolder(t, r, "h1")
	pool := openPool(t, r, "p1")

	old := postDue(t, r, holder, pool, 100, "2026-01-01T00:00:00Z")
	recent := postDue(t, r, holder, pool, 200, "2026-03-01T00:00:00Z")
	postDue(t, r, holder, pool, 300, "2026-06-01T00:00:00Z")
	postDue(t, r, holder, pool, 400, "")

	got, err := r.Overdue(ctx, holder.ID, "2026-03-11T12:00:00Z")
	if err != nil {
		t.Fatalf("Overdue: %v", err)
	}
	if len(got.Items) != 2 {
		t.Fatalf("items = %+v, want two", got.Items)
	}
	if got.Items[0].ID != old.ID || got.Items[0].DaysOverdue != 69 {
		t.Fatalf("first = %+v, want %s overdue 69 days", got.Items[0], old.ID)
	}
	if got.Items[1].ID != recent.ID || got.Items[1].DaysOverdue != 10 {
		t.Fatalf("second = %+v, want %s overdue 10 days", got.Items[1], recent.ID)
	}
}

func TestOverdueLeavesOutAReversedEntryAndItsReversal(t *testing.T) {
	r, ctx := newLedger(t)
	holder := openHolder(t, r, "h1")
	pool := openPool(t, r, "p1")

	e := postDue(t, r, holder, pool, 100, "2026-01-01T00:00:00Z")
	if _, err := r.Post(ctx, accounting.Entry{
		DebitAccountID: pool.ID, CreditAccountID: holder.ID, Amount: 100, DueAt: "2026-01-01T00:00:00Z",
		DocumentKind: "charge", DocumentID: "rev", ActorID: "actor-1", ReversesEntryID: e.ID,
	}); err != nil {
		t.Fatalf("reverse: %v", err)
	}
	got, err := r.Overdue(ctx, holder.ID, "2026-02-01T00:00:00Z")
	if err != nil {
		t.Fatalf("Overdue: %v", err)
	}
	if len(got.Items) != 0 {
		t.Fatalf("items = %+v, want none", got.Items)
	}
}

func TestADueDateThatIsNotAnInstantIsRefused(t *testing.T) {
	r, ctx := newLedger(t)
	holder := openHolder(t, r, "h1")
	pool := openPool(t, r, "p1")
	_, err := r.Post(ctx, accounting.Entry{
		DebitAccountID: holder.ID, CreditAccountID: pool.ID, Amount: 1, DueAt: "next tuesday",
		DocumentKind: "charge", DocumentID: "d", ActorID: "actor-1",
	})
	if !errors.Is(err, accounting.ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid", err)
	}
	if _, err := r.Overdue(ctx, holder.ID, "yesterday"); !errors.Is(err, accounting.ErrInvalid) {
		t.Fatalf("as_of err = %v, want ErrInvalid", err)
	}
}

func TestOverdueOnAnUnknownAccountIsNotFound(t *testing.T) {
	r, ctx := newLedger(t)
	if _, err := r.Overdue(ctx, "missing", ""); !errors.Is(err, accounting.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}
