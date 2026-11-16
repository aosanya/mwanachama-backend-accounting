package accounting

import (
	"errors"
	"testing"
	"time"
)

func chargeRaisedAndDue(id, raised, due string, amount int64) Entry {
	return Entry{ID: id, PostedAt: raised, DueAt: due, Amount: amount}
}

func TestOverdueAgesFromDueByDefault(t *testing.T) {
	at := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)
	entries := []Entry{chargeRaisedAndDue("c1", "2026-01-01T00:00:00Z", "2026-06-20T00:00:00Z", 500)}
	got := overdueAsOf(entries, map[string]bool{}, map[string]int64{}, at, AgeFromDue)
	if len(got) != 1 || got[0].AgeDays != 10 || got[0].DaysOverdue != 10 {
		t.Fatalf("got %+v", got)
	}
}

func TestOverdueAgesFromRaisedWhenAsked(t *testing.T) {
	at := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)
	entries := []Entry{chargeRaisedAndDue("c1", "2026-06-10T00:00:00Z", "2026-06-20T00:00:00Z", 500)}
	got := overdueAsOf(entries, map[string]bool{}, map[string]int64{}, at, AgeFromRaised)
	if len(got) != 1 || got[0].AgeDays != 20 || got[0].DaysOverdue != 10 {
		t.Fatalf("got %+v", got)
	}
}

func TestOverdueRaisedBasisOrdersByRaiseDate(t *testing.T) {
	at := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)
	entries := []Entry{
		chargeRaisedAndDue("late-raised", "2026-06-01T00:00:00Z", "2026-06-05T00:00:00Z", 100),
		chargeRaisedAndDue("early-raised", "2026-01-01T00:00:00Z", "2026-06-25T00:00:00Z", 100),
	}
	byDue := overdueAsOf(entries, map[string]bool{}, map[string]int64{}, at, AgeFromDue)
	byRaised := overdueAsOf(entries, map[string]bool{}, map[string]int64{}, at, AgeFromRaised)
	if byDue[0].ID != "late-raised" || byRaised[0].ID != "early-raised" {
		t.Fatalf("due order %s, raised order %s", byDue[0].ID, byRaised[0].ID)
	}
}

func TestOverdueRaisedBasisStillListsOnlyPastDueUnpaid(t *testing.T) {
	at := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)
	entries := []Entry{
		chargeRaisedAndDue("not-due", "2026-01-01T00:00:00Z", "2026-07-15T00:00:00Z", 100),
		chargeRaisedAndDue("paid", "2026-01-01T00:00:00Z", "2026-06-01T00:00:00Z", 100),
	}
	got := overdueAsOf(entries, map[string]bool{}, map[string]int64{"paid": 100}, at, AgeFromRaised)
	if len(got) != 0 {
		t.Fatalf("got %+v", got)
	}
}

func TestAgeBasisRefusesUnknownValue(t *testing.T) {
	for raw, want := range map[string]AgeBasis{"": AgeFromDue, "due": AgeFromDue, " raised ": AgeFromRaised} {
		got, err := ageBasis(raw)
		if err != nil || got != want {
			t.Fatalf("%q: %v %v", raw, got, err)
		}
	}
	if _, err := ageBasis("posted"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("err %v", err)
	}
}
