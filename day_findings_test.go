package accounting_test

import (
	"errors"
	"fmt"
	"testing"

	accounting "github.com/aosanya/mwanachama-backend-accounting"
)

func submitDailyEntries(t *testing.T, r accounting.LedgerRepository, consultant, month string, hundredths int64) []accounting.DayEntry {
	t.Helper()
	var entries []accounting.DayEntry
	for day := 1; hundredths > 0; day++ {
		part := min(hundredths, 100)
		entries = append(entries, submitDays(t, r, consultant, fmt.Sprintf("%s-%02d", month, day), part))
		hundredths -= part
	}
	return entries
}

func approveEach(t *testing.T, r accounting.LedgerRepository, entries []accounting.DayEntry) {
	t.Helper()
	for _, e := range entries {
		approveDays(t, r, e.ID)
	}
}

func TestAReplacingAllowanceWithANewRateLeavesTheFeeOfApprovedDaysAlone(t *testing.T) {
	r, ctx := newLedger(t)
	first := buyDays(t, r, "Ilse Marrow", "Analyst", 2000, 500)
	approveEach(t, r, submitDailyEntries(t, r, "Ilse Marrow", "2026-03", 1000))

	if _, err := r.ReplaceDayAllowance(ctx, accounting.DayAllowance{
		ReplacesID: first.ID, DaysBought: 2000, Rate: 900, Reason: "New rate agreed", SetBy: "lead-1",
	}); err != nil {
		t.Fatal(err)
	}

	b, err := r.DaysBalance(ctx, savingsTraining)
	if err != nil {
		t.Fatal(err)
	}
	if got := daysLine(t, b, "Ilse Marrow"); got.Fee != 5000 || got.Rate != 900 || got.Approved != 1000 {
		t.Fatalf("ten days approved at 500 stay at 5,000 after the rate moves to 900: %+v", got)
	}

	approveEach(t, r, submitDailyEntries(t, r, "Ilse Marrow", "2026-04", 100))
	b, err = r.DaysBalance(ctx, savingsTraining)
	if err != nil {
		t.Fatal(err)
	}
	got := daysLine(t, b, "Ilse Marrow")
	if got.Fee != 5900 || got.FeeLeft != 8100 || got.FeeCeiling != 14000 {
		t.Fatalf("the next day approved is priced at 900 and the days left at 900: %+v", got)
	}
}

func TestAReplacingAllowanceBelowTheApprovedDaysIsRefused(t *testing.T) {
	r, ctx := newLedger(t)
	first := buyDays(t, r, "Ilse Marrow", "Analyst", 500, 600)
	approveEach(t, r, submitDailyEntries(t, r, "Ilse Marrow", "2026-03", 300))

	_, err := r.ReplaceDayAllowance(ctx, accounting.DayAllowance{
		ReplacesID: first.ID, DaysBought: 100, Rate: 600, Reason: "Cut", SetBy: "lead-1",
	})
	if !errors.Is(err, accounting.ErrOverDaysBought) {
		t.Fatalf("a replacement of 1.0 day over 3.0 approved: %v, want ErrOverDaysBought", err)
	}
	if _, err := r.ReplaceDayAllowance(ctx, accounting.DayAllowance{
		ReplacesID: first.ID, DaysBought: 300, Rate: 600, Reason: "Cut to what is spent", SetBy: "lead-1",
	}); err != nil {
		t.Fatalf("a replacement equal to the approved days: %v", err)
	}
	b, err := r.DaysBalance(ctx, savingsTraining)
	if err != nil {
		t.Fatal(err)
	}
	if got := daysLine(t, b, "Ilse Marrow"); got.Bought != 300 || got.Left != 0 {
		t.Fatalf("the refused row left no trace: %+v", got)
	}
}

func TestADayEntryIsNotApprovedByItsSubmitterOrByTheConsultant(t *testing.T) {
	r, ctx := newLedger(t)
	buyDays(t, r, "Ilse Marrow", "Analyst", 1000, 600)

	own := submitDays(t, r, "Ilse Marrow", "2026-03-10", 100)
	if _, err := r.ApproveDayEntry(ctx, own.ID, "Ilse Marrow"); !errors.Is(err, accounting.ErrOwnDayEntry) {
		t.Fatal("the consultant approved their own entry")
	}
	if _, err := r.ApproveDayEntry(ctx, own.ID, "  ilse marrow "); !errors.Is(err, accounting.ErrOwnDayEntry) {
		t.Fatal("the consultant approved their own entry under another spelling of the name")
	}

	forOther, err := r.SubmitDayEntry(ctx, accounting.DayEntry{
		Project: savingsTraining, Consultant: "Ilse Marrow", WorkedOn: "2026-03-11", Days: 100, SubmittedBy: "clerk-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.ApproveDayEntry(ctx, forOther.ID, "clerk-1"); !errors.Is(err, accounting.ErrOwnDayEntry) {
		t.Fatal("the clerk who submitted the entry approved it")
	}
	if _, err := r.RejectDayEntry(ctx, forOther.ID, "clerk-1"); err != nil {
		t.Fatalf("the submitter may withdraw an entry by rejecting it: %v", err)
	}

	if e, err := r.ApproveDayEntry(ctx, own.ID, "lead-1"); err != nil || e.DecidedBy != "lead-1" {
		t.Fatalf("another person approves the entry: %+v, %v", e, err)
	}
}

func TestOneConsultantsDaysOnOneDateNeverPassOneDay(t *testing.T) {
	r, ctx := newLedger(t)
	buyDays(t, r, "Ilse Marrow", "Analyst", 5000, 600)

	submit := func(date string, days int64) (accounting.DayEntry, error) {
		return r.SubmitDayEntry(ctx, accounting.DayEntry{
			Project: savingsTraining, Consultant: "Ilse Marrow", WorkedOn: date, Days: days, SubmittedBy: "Ilse Marrow",
		})
	}

	if _, err := submit("2026-03-10", 2400); !errors.Is(err, accounting.ErrInvalid) {
		t.Fatal("24.00 days on one date were accepted")
	}
	first, err := submit("2026-03-10", 100)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := submit("2026-03-10", 100); !errors.Is(err, accounting.ErrDateOverfull) {
		t.Fatal("a second full day on the same date was accepted, so a double tap doubles the days")
	}
	if _, err := submit("2026-03-10", 1); !errors.Is(err, accounting.ErrDateOverfull) {
		t.Fatal("a hundredth over one day on the same date was accepted")
	}
	if _, err := r.RejectDayEntry(ctx, first.ID, "lead-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := submit("2026-03-10", 50); err != nil {
		t.Fatalf("a rejected entry frees its date: %v", err)
	}
	if _, err := submit("2026-03-10", 50); err != nil {
		t.Fatalf("two half days make one day: %v", err)
	}
	if _, err := submit("2026-03-10", 1); !errors.Is(err, accounting.ErrDateOverfull) {
		t.Fatal("the date is full after two half days")
	}
	if _, err := submit("2026-03-11", 100); err != nil {
		t.Fatalf("another date is free: %v", err)
	}
}

func TestTheDaysBoughtCountEveryApprovedEntryOfTheAllowance(t *testing.T) {
	r, ctx := newLedger(t)
	buyDays(t, r, "Ilse Marrow", "Analyst", 250, 600)
	entries := submitDailyEntries(t, r, "Ilse Marrow", "2026-03", 400)

	approveEach(t, r, entries[:2])
	if _, err := r.ApproveDayEntry(ctx, entries[2].ID, "lead-1"); !errors.Is(err, accounting.ErrOverDaysBought) {
		t.Fatal("a third full day was approved against 2.5 days bought")
	}
	approveDays(t, r, submitDailyEntries(t, r, "Ilse Marrow", "2026-04", 50)[0].ID)

	b, err := r.DaysBalance(ctx, savingsTraining)
	if err != nil {
		t.Fatal(err)
	}
	if got := daysLine(t, b, "Ilse Marrow"); got.Approved != 250 || got.Left != 0 {
		t.Fatalf("all approved entries count against the allowance: %+v", got)
	}
}

func TestAConsultantNameDifferingInCaseOrSpaceIsTheSamePerson(t *testing.T) {
	r, ctx := newLedger(t)
	buyDays(t, r, "Ana Lund", "Analyst", 1000, 600)

	for _, variant := range []string{"ana lund", "ANA LUND", "Ana Lund ", "  Ana Lund"} {
		if _, err := r.AddDayAllowance(ctx, accounting.DayAllowance{
			Project: savingsTraining, Consultant: variant, DaysBought: 1000, Rate: 600, SetBy: "lead-1",
		}); !errors.Is(err, accounting.ErrInvalid) {
			t.Errorf("a second current allowance for %q: %v, want ErrInvalid", variant, err)
		}
	}

	entry, err := r.SubmitDayEntry(ctx, accounting.DayEntry{
		Project: savingsTraining, Consultant: " ANA LUND", WorkedOn: "2026-03-10", Days: 100, SubmittedBy: "clerk-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	approveDays(t, r, entry.ID)

	listed, err := r.ListDayEntries(ctx, savingsTraining, "ana LUND ", "approved", 0)
	if err != nil || len(listed) != 1 {
		t.Fatalf("a list by another spelling finds the entry: %+v, %v", listed, err)
	}
	b, err := r.DaysBalance(ctx, savingsTraining)
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Lines) != 1 || b.Total.Bought != 1000 || b.Total.Approved != 100 {
		t.Fatalf("one person, one line, one allowance: %+v", b)
	}
}

func TestAFeeThatWouldOverflowIsRefused(t *testing.T) {
	r, ctx := newLedger(t)

	if _, err := r.AddDayAllowance(ctx, accounting.DayAllowance{
		Project: savingsTraining, Consultant: "Ilse Marrow", DaysBought: 1 << 62, Rate: 1 << 20, SetBy: "lead-1",
	}); !errors.Is(err, accounting.ErrInvalid) {
		t.Fatal("an allowance whose fee wraps past the largest whole number was accepted")
	}

	first := buyDays(t, r, "Tomas Quill", "Trainer", 6000, 500)
	if _, err := r.ReplaceDayAllowance(ctx, accounting.DayAllowance{
		ReplacesID: first.ID, DaysBought: 1 << 62, Rate: 1 << 20, Reason: "Absurd", SetBy: "lead-1",
	}); !errors.Is(err, accounting.ErrInvalid) {
		t.Fatal("a replacing allowance whose fee overflows was accepted")
	}

	b, err := r.DaysBalance(ctx, savingsTraining)
	if err != nil {
		t.Fatal(err)
	}
	if got := daysLine(t, b, "Tomas Quill"); got.Bought != 6000 || got.FeeCeiling != 30000 {
		t.Fatalf("the refused rows left no trace: %+v", got)
	}
}
