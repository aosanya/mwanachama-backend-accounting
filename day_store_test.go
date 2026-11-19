package accounting_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	accounting "github.com/aosanya/mwanachama-backend-accounting"
)

const savingsTraining = "village-savings-training"

type monthlyDays struct {
	consultant string
	worked     [3]int64
}

var workedExampleDays = []monthlyDays{
	{"Selam Brightwater", [3]int64{900, 850, 1000}},
	{"Tomas Quill", [3]int64{1200, 2000, 1800}},
	{"Ilse Marrow", [3]int64{450, 1100, 800}},
}

var workedExampleMonths = [3]string{"2026-03", "2026-04", "2026-05"}

func buyDays(t *testing.T, r accounting.LedgerRepository, consultant, position string, days, rate int64) accounting.DayAllowance {
	t.Helper()
	a, err := r.AddDayAllowance(context.Background(), accounting.DayAllowance{
		Project: savingsTraining, Consultant: consultant, Position: position,
		DaysBought: days, Rate: rate, SetBy: "lead-1",
	})
	if err != nil {
		t.Fatalf("AddDayAllowance(%s): %v", consultant, err)
	}
	return a
}

func submitDays(t *testing.T, r accounting.LedgerRepository, consultant, workedOn string, days int64) accounting.DayEntry {
	t.Helper()
	e, err := r.SubmitDayEntry(context.Background(), accounting.DayEntry{
		Project: savingsTraining, Consultant: consultant, WorkedOn: workedOn,
		Days: days, Description: "Training sessions", SubmittedBy: consultant,
	})
	if err != nil {
		t.Fatalf("SubmitDayEntry(%s %s): %v", consultant, workedOn, err)
	}
	return e
}

func approveDays(t *testing.T, r accounting.LedgerRepository, entryID string) accounting.DayEntry {
	t.Helper()
	e, err := r.ApproveDayEntry(context.Background(), entryID, "lead-1")
	if err != nil {
		t.Fatalf("ApproveDayEntry(%s): %v", entryID, err)
	}
	return e
}

func workedExampleLedger(t *testing.T) accounting.LedgerRepository {
	t.Helper()
	r, _ := newLedger(t)
	buyDays(t, r, "Selam Brightwater", "Team leader", 4000, 900)
	buyDays(t, r, "Tomas Quill", "Trainer", 6000, 500)
	buyDays(t, r, "Ilse Marrow", "Analyst", 3000, 600)
	for _, c := range workedExampleDays {
		for i, days := range c.worked {
			approveEach(t, r, submitDailyEntries(t, r, c.consultant, workedExampleMonths[i], days))
		}
	}
	return r
}

func daysLine(t *testing.T, b accounting.DaysBalance, consultant string) accounting.DaysLine {
	t.Helper()
	for _, l := range b.Lines {
		if l.Consultant == strings.ToLower(consultant) {
			return l
		}
	}
	t.Fatalf("the balance has no line for %s: %+v", consultant, b.Lines)
	return accounting.DaysLine{}
}

func TestDaysBalanceOfTheWorkedExampleReadsBoughtSpentLeftAndFee(t *testing.T) {
	r := workedExampleLedger(t)

	b, err := r.DaysBalance(context.Background(), savingsTraining)
	if err != nil {
		t.Fatal(err)
	}

	want := accounting.DaysLine{
		Bought: 13000, Approved: 10100, Left: 2900,
		FeeCeiling: 84000, Fee: 63850, FeeLeft: 20150,
	}
	if b.Total != want {
		t.Fatalf("total %+v, want %+v", b.Total, want)
	}

	lines := map[string]accounting.DaysLine{
		"Selam Brightwater": {Consultant: "selam brightwater", Position: "Team leader", Rate: 900, Bought: 4000, Approved: 2750, Left: 1250, FeeCeiling: 36000, Fee: 24750, FeeLeft: 11250},
		"Tomas Quill":       {Consultant: "tomas quill", Position: "Trainer", Rate: 500, Bought: 6000, Approved: 5000, Left: 1000, FeeCeiling: 30000, Fee: 25000, FeeLeft: 5000},
		"Ilse Marrow":       {Consultant: "ilse marrow", Position: "Analyst", Rate: 600, Bought: 3000, Approved: 2350, Left: 650, FeeCeiling: 18000, Fee: 14100, FeeLeft: 3900},
	}
	if len(b.Lines) != len(lines) {
		t.Fatalf("got %d lines, want %d: %+v", len(b.Lines), len(lines), b.Lines)
	}
	for name, want := range lines {
		if got := daysLine(t, b, name); got != want {
			t.Errorf("%s: %+v, want %+v", name, got, want)
		}
	}

	leftAtRate := 1250*900/100 + 1000*500/100 + 650*600/100
	if int64(leftAtRate) != b.Total.FeeLeft {
		t.Fatalf("the days left at each rate come to %d, the fee left is %d", leftAtRate, b.Total.FeeLeft)
	}
}

func TestApprovalPastTheDaysBoughtIsRefusedUntilAnAllowanceRowRaisesIt(t *testing.T) {
	r := workedExampleLedger(t)
	ctx := context.Background()

	june := submitDailyEntries(t, r, "Tomas Quill", "2026-06", 1200)

	approveEach(t, r, june[:10])
	for _, over := range june[10:] {
		if _, err := r.ApproveDayEntry(ctx, over.ID, "lead-1"); !errors.Is(err, accounting.ErrOverDaysBought) {
			t.Fatalf("approving a day past the 10.0 left: %v, want ErrOverDaysBought", err)
		}
	}
	entries, err := r.ListDayEntries(ctx, savingsTraining, "Tomas Quill", "submitted", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].DecidedBy != "" || entries[1].DecidedBy != "" {
		t.Fatalf("the two refused entries stay submitted and undecided: %+v", entries)
	}
	b, err := r.DaysBalance(ctx, savingsTraining)
	if err != nil {
		t.Fatal(err)
	}
	if got := daysLine(t, b, "Tomas Quill"); got.Approved != 6000 || got.Submitted != 200 {
		t.Fatalf("a refused approval counts for nothing, a waiting entry shows as submitted: %+v", got)
	}

	current, err := r.ListDayAllowances(ctx, savingsTraining, 0)
	if err != nil {
		t.Fatal(err)
	}
	var tomas accounting.DayAllowance
	for _, a := range current {
		if a.Consultant == "tomas quill" {
			tomas = a
		}
	}
	raised, err := r.ReplaceDayAllowance(ctx, accounting.DayAllowance{
		ReplacesID: tomas.ID, Position: "Trainer", DaysBought: 6500, Rate: 500,
		Reason: "Five more training days agreed with the client", SetBy: "lead-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if raised.Project != savingsTraining || raised.Consultant != "tomas quill" || raised.ReplacesID != tomas.ID {
		t.Fatalf("the new row keeps the project and consultant of the one it replaces: %+v", raised)
	}

	approveEach(t, r, june[10:])

	b, err = r.DaysBalance(ctx, savingsTraining)
	if err != nil {
		t.Fatal(err)
	}
	if got := daysLine(t, b, "Tomas Quill"); got.Bought != 6500 || got.Approved != 6200 || got.Left != 300 || got.Fee != 31000 || got.FeeCeiling != 32500 {
		t.Fatalf("Tomas after the raise: %+v", got)
	}
	if b.Total.Bought != 13500 || b.Total.FeeCeiling != 86500 {
		t.Fatalf("the project is 135 days and a ceiling of 86,500: %+v", b.Total)
	}

	history, err := r.ListDayAllowances(ctx, savingsTraining, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 4 {
		t.Fatalf("the replaced row stays beside the new one: got %d rows, want 4", len(history))
	}
	kept := false
	for _, a := range history {
		kept = kept || (a.ID == tomas.ID && a.DaysBought == 6000)
	}
	if !kept {
		t.Fatalf("the first allowance row is still there at 60 days: %+v", history)
	}
}

func TestAnAllowanceChangeNamesTheCurrentRowAndSaysWhy(t *testing.T) {
	r, ctx := newLedger(t)
	first := buyDays(t, r, "Ilse Marrow", "Analyst", 3000, 600)

	if _, err := r.AddDayAllowance(ctx, accounting.DayAllowance{
		Project: savingsTraining, Consultant: "Ilse Marrow", DaysBought: 500, Rate: 600, SetBy: "lead-1",
	}); !errors.Is(err, accounting.ErrInvalid) {
		t.Fatalf("a second current row for the same consultant: %v, want ErrInvalid", err)
	}
	if _, err := r.ReplaceDayAllowance(ctx, accounting.DayAllowance{
		ReplacesID: first.ID, DaysBought: 3500, Rate: 600, SetBy: "lead-1",
	}); !errors.Is(err, accounting.ErrInvalid) {
		t.Fatalf("a replacement with no reason: %v, want ErrInvalid", err)
	}
	if _, err := r.ReplaceDayAllowance(ctx, accounting.DayAllowance{
		ReplacesID: "no-such-row", DaysBought: 3500, Rate: 600, Reason: "x", SetBy: "lead-1",
	}); !errors.Is(err, accounting.ErrNotFound) {
		t.Fatalf("replacing a row that does not exist: %v, want ErrNotFound", err)
	}

	second, err := r.ReplaceDayAllowance(ctx, accounting.DayAllowance{
		ReplacesID: first.ID, DaysBought: 3500, Rate: 600, Reason: "Scope grew", SetBy: "lead-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.ReplaceDayAllowance(ctx, accounting.DayAllowance{
		ReplacesID: first.ID, DaysBought: 4000, Rate: 600, Reason: "Again", SetBy: "lead-1",
	}); !errors.Is(err, accounting.ErrInvalid) {
		t.Fatalf("replacing a row that was already replaced: %v, want ErrInvalid", err)
	}
	if _, err := r.AddDayAllowance(ctx, accounting.DayAllowance{
		Project: savingsTraining, Consultant: "Ilse Marrow", DaysBought: 100, Rate: 600,
		ReplacesID: second.ID, SetBy: "lead-1",
	}); !errors.Is(err, accounting.ErrInvalid) {
		t.Fatalf("add naming a row to replace: %v, want ErrInvalid", err)
	}
	for _, bad := range []accounting.DayAllowance{
		{Project: savingsTraining, Consultant: "Ilse Marrow 2", DaysBought: 0, Rate: 1, SetBy: "lead-1"},
		{Project: savingsTraining, Consultant: "Ilse Marrow 2", DaysBought: 100, Rate: -1, SetBy: "lead-1"},
		{Project: savingsTraining, Consultant: "Ilse Marrow 2", DaysBought: 100, Rate: 1, PeriodStart: "2026-01-01T00:00:00Z", SetBy: "lead-1"},
		{Project: "", Consultant: "Ilse Marrow 2", DaysBought: 100, Rate: 1, SetBy: "lead-1"},
	} {
		if _, err := r.AddDayAllowance(ctx, bad); !errors.Is(err, accounting.ErrInvalid) {
			t.Errorf("%+v: %v, want ErrInvalid", bad, err)
		}
	}
}

func TestADayEntryIsDecidedOnceAndOnlyApprovedDaysCount(t *testing.T) {
	r, ctx := newLedger(t)
	buyDays(t, r, "Ilse Marrow", "Analyst", 1000, 600)

	kept := submitDays(t, r, "Ilse Marrow", "2026-03-10", 100)
	dropped := submitDays(t, r, "Ilse Marrow", "2026-03-11", 100)
	waiting := submitDays(t, r, "Ilse Marrow", "2026-03-12", 50)

	if kept.Status != accounting.DayEntrySubmitted || kept.ID == "" || kept.SubmittedAt == "" {
		t.Fatalf("an entry starts submitted with an id and a time: %+v", kept)
	}
	approved := approveDays(t, r, kept.ID)
	if approved.Status != accounting.DayEntryApproved || approved.DecidedBy != "lead-1" || approved.DecidedAt == "" {
		t.Fatalf("approval writes who and when: %+v", approved)
	}
	rejected, err := r.RejectDayEntry(ctx, dropped.ID, "lead-1")
	if err != nil {
		t.Fatal(err)
	}
	if rejected.Status != accounting.DayEntryRejected {
		t.Fatalf("rejection: %+v", rejected)
	}

	if _, err := r.ApproveDayEntry(ctx, dropped.ID, "lead-1"); !errors.Is(err, accounting.ErrDayEntryDecided) {
		t.Fatalf("approving a rejected entry: %v, want ErrDayEntryDecided", err)
	}
	if _, err := r.RejectDayEntry(ctx, kept.ID, "lead-1"); !errors.Is(err, accounting.ErrDayEntryDecided) {
		t.Fatalf("rejecting an approved entry: %v, want ErrDayEntryDecided", err)
	}
	if _, err := r.ApproveDayEntry(ctx, waiting.ID, " "); !errors.Is(err, accounting.ErrInvalid) {
		t.Fatalf("a decision with no deciding person: %v, want ErrInvalid", err)
	}
	if _, err := r.ApproveDayEntry(ctx, "no-such-entry", "lead-1"); !errors.Is(err, accounting.ErrNotFound) {
		t.Fatalf("an unknown entry: %v, want ErrNotFound", err)
	}

	b, err := r.DaysBalance(ctx, savingsTraining)
	if err != nil {
		t.Fatal(err)
	}
	if got := daysLine(t, b, "Ilse Marrow"); got.Approved != 100 || got.Submitted != 50 || got.Left != 900 {
		t.Fatalf("only the approved entry is spent, the waiting one is submitted: %+v", got)
	}
}

func TestADayEntryNeedsDaysADateAndAnAllowanceToBeApproved(t *testing.T) {
	r, ctx := newLedger(t)

	for _, bad := range []accounting.DayEntry{
		{Project: savingsTraining, Consultant: "Ilse Marrow", WorkedOn: "2026-03-10", Days: 0, SubmittedBy: "x"},
		{Project: savingsTraining, Consultant: "Ilse Marrow", WorkedOn: "2026-02-30", Days: 100, SubmittedBy: "x"},
		{Project: savingsTraining, Consultant: "Ilse Marrow", WorkedOn: "March", Days: 100, SubmittedBy: "x"},
		{Project: savingsTraining, Consultant: "", WorkedOn: "2026-03-10", Days: 100, SubmittedBy: "x"},
		{Project: savingsTraining, Consultant: "Ilse Marrow", WorkedOn: "2026-03-10", Days: 100, SubmittedBy: ""},
	} {
		if _, err := r.SubmitDayEntry(ctx, bad); !errors.Is(err, accounting.ErrInvalid) {
			t.Errorf("%+v: %v, want ErrInvalid", bad, err)
		}
	}

	unfunded := submitDays(t, r, "Ilse Marrow", "2026-03-10", 100)
	if _, err := r.ApproveDayEntry(ctx, unfunded.ID, "lead-1"); !errors.Is(err, accounting.ErrOverDaysBought) {
		t.Fatalf("approving days on a project the consultant holds none on: %v, want ErrOverDaysBought", err)
	}
	b, err := r.DaysBalance(ctx, savingsTraining)
	if err != nil {
		t.Fatal(err)
	}
	if got := daysLine(t, b, "Ilse Marrow"); got.Bought != 0 || got.Submitted != 100 || got.Left != 0 {
		t.Fatalf("a consultant with days submitted and none bought still shows: %+v", got)
	}
}

func TestADomainThatFillsNoDayRolesRefusesDaysInPlainWords(t *testing.T) {
	s, err := accounting.LoadSpec(filepath.Join(".", "spec", "examples", "school.accounting.json"))
	if err != nil {
		t.Fatal(err)
	}
	r := newLedgerOn(t, openDB(t), s)
	ctx := context.Background()

	if _, err := r.AddDayAllowance(ctx, accounting.DayAllowance{Project: "p", Consultant: "c", DaysBought: 100, SetBy: "x"}); !errors.Is(err, accounting.ErrInvalid) {
		t.Fatalf("AddDayAllowance: %v, want ErrInvalid", err)
	}
	if _, err := r.SubmitDayEntry(ctx, accounting.DayEntry{}); !errors.Is(err, accounting.ErrInvalid) {
		t.Fatalf("SubmitDayEntry: %v, want ErrInvalid", err)
	}
	if _, err := r.DaysBalance(ctx, "p"); !errors.Is(err, accounting.ErrInvalid) {
		t.Fatalf("DaysBalance: %v, want ErrInvalid", err)
	}
}

func TestAProjectsDaysDoNotMixWithAnotherProjects(t *testing.T) {
	r, ctx := newLedger(t)
	buyDays(t, r, "Ilse Marrow", "Analyst", 1000, 600)
	approveDays(t, r, submitDays(t, r, "Ilse Marrow", "2026-03-10", 100).ID)

	other, err := r.DaysBalance(ctx, "another-project")
	if err != nil {
		t.Fatal(err)
	}
	if len(other.Lines) != 0 || other.Total.Bought != 0 || other.Total.Approved != 0 {
		t.Fatalf("another project holds nothing: %+v", other)
	}
}
