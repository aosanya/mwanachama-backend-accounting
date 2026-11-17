package accounting_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"strings"
	"testing"

	accounting "github.com/aosanya/mwanachama-backend-accounting"
)

func ratioFigure(subject, day, value string) accounting.Figure {
	return accounting.Figure{
		SubjectID:         subject,
		Figure:            "repayment_ratio",
		OnDay:             day,
		Value:             value,
		FunctionCode:      "ratio.repayment",
		FunctionVersion:   "2",
		InputsFingerprint: "fp-3f9a1c",
		ActorID:           "actor-1",
		Narration:         "Repayment ratio for the quarter.",
	}
}

func TestFigureIsAppendedAndReadBackWithItsProvenance(t *testing.T) {
	r, ctx := newLedger(t)

	got, err := r.AppendFigure(ctx, ratioFigure("member-1", "2026-03-31", "0.7500"))
	if err != nil {
		t.Fatal(err)
	}
	if got.ID == "" || got.RecordedAt == "" {
		t.Fatalf("the store writes the id and the time: %+v", got)
	}

	history, err := r.ListFigureHistory(ctx, "member-1", "repayment_ratio", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 {
		t.Fatalf("want 1 entry, got %d", len(history))
	}
	read := history[0]
	want := ratioFigure("member-1", "2026-03-31", "0.7500")
	want.ID, want.RecordedAt = got.ID, got.RecordedAt
	if read != want {
		t.Fatalf("read back %+v, want %+v", read, want)
	}
}

func TestFigureValueKeepsItsDigitsAsWritten(t *testing.T) {
	r, ctx := newLedger(t)

	for i, value := range []string{"45.20", "5.125", "-0.00", "12", "0.1000000000"} {
		day := "2026-04-0" + string(rune('1'+i))
		if _, err := r.AppendFigure(ctx, ratioFigure("member-2", day, value)); err != nil {
			t.Fatalf("%s: %v", value, err)
		}
	}
	history, err := r.ListFigureHistory(ctx, "member-2", "repayment_ratio", 0)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, f := range history {
		got = append(got, f.Value)
	}
	sort.Strings(got)
	want := []string{"-0.00", "0.1000000000", "12", "45.20", "5.125"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("values = %v, want %v", got, want)
	}
}

func TestFigureHistoryComesBackNewestFirst(t *testing.T) {
	r, ctx := newLedger(t)

	for _, day := range []string{"2026-01-31", "2026-03-31", "2026-02-28"} {
		if _, err := r.AppendFigure(ctx, ratioFigure("member-3", day, "0.5000")); err != nil {
			t.Fatal(err)
		}
	}
	history, err := r.ListFigureHistory(ctx, "member-3", "repayment_ratio", 0)
	if err != nil {
		t.Fatal(err)
	}
	var days []string
	for _, f := range history {
		days = append(days, f.OnDay)
	}
	want := []string{"2026-03-31", "2026-02-28", "2026-01-31"}
	if !reflect.DeepEqual(days, want) {
		t.Fatalf("days = %v, want %v", days, want)
	}
}

func TestFigureHistoryHonoursALimit(t *testing.T) {
	r, ctx := newLedger(t)

	for _, day := range []string{"2026-01-31", "2026-02-28", "2026-03-31"} {
		if _, err := r.AppendFigure(ctx, ratioFigure("member-4", day, "0.5000")); err != nil {
			t.Fatal(err)
		}
	}
	history, err := r.ListFigureHistory(ctx, "member-4", "repayment_ratio", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 2 || history[0].OnDay != "2026-03-31" || history[1].OnDay != "2026-02-28" {
		t.Fatalf("want the two newest, got %+v", history)
	}
}

func TestFigureHistoryKeepsSubjectsAndFiguresApart(t *testing.T) {
	r, ctx := newLedger(t)

	other := ratioFigure("member-5", "2026-03-31", "0.2500")
	other.Figure = "savings_score"
	for _, f := range []accounting.Figure{
		ratioFigure("member-5", "2026-03-31", "0.7500"),
		ratioFigure("member-6", "2026-03-31", "0.9000"),
		other,
	} {
		if _, err := r.AppendFigure(ctx, f); err != nil {
			t.Fatal(err)
		}
	}
	history, err := r.ListFigureHistory(ctx, "member-5", "repayment_ratio", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 || history[0].Value != "0.7500" {
		t.Fatalf("want only member-5's repayment_ratio, got %+v", history)
	}
}

func TestFigureHistoryOfAFigureNeverRecordedIsEmpty(t *testing.T) {
	r, ctx := newLedger(t)

	history, err := r.ListFigureHistory(ctx, "member-7", "repayment_ratio", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 0 {
		t.Fatalf("want no entries, got %+v", history)
	}
}

func TestFigureSecondEntryForTheSameSubjectFigureAndDayIsRefused(t *testing.T) {
	r, ctx := newLedger(t)

	first, err := r.AppendFigure(ctx, ratioFigure("member-8", "2026-03-31", "0.7500"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = r.AppendFigure(ctx, ratioFigure("member-8", "2026-03-31", "0.9900"))
	if !errors.Is(err, accounting.ErrFigureTaken) {
		t.Fatalf("second entry for the same day: err = %v, want ErrFigureTaken", err)
	}
	if !strings.Contains(err.Error(), "already has an entry for this figure on this day") {
		t.Fatalf("the refusal must read as a plain sentence, got %q", err.Error())
	}

	history, err := r.ListFigureHistory(ctx, "member-8", "repayment_ratio", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 || history[0].ID != first.ID || history[0].Value != "0.7500" {
		t.Fatalf("the first entry must stand untouched, got %+v", history)
	}
}

func TestFigureAnotherDayOrAnotherSubjectIsNotARepeat(t *testing.T) {
	r, ctx := newLedger(t)

	for _, f := range []accounting.Figure{
		ratioFigure("member-9", "2026-03-31", "0.7500"),
		ratioFigure("member-9", "2026-04-30", "0.7500"),
		ratioFigure("member-10", "2026-03-31", "0.7500"),
	} {
		if _, err := r.AppendFigure(ctx, f); err != nil {
			t.Fatalf("%+v: %v", f, err)
		}
	}
}

func TestFigureRefusesAnEntryThatIsMissingOrMalformed(t *testing.T) {
	r, ctx := newLedger(t)

	cases := []struct {
		name   string
		change func(*accounting.Figure)
	}{
		{"no subject", func(f *accounting.Figure) { f.SubjectID = "" }},
		{"no figure name", func(f *accounting.Figure) { f.Figure = "" }},
		{"a figure name with a capital", func(f *accounting.Figure) { f.Figure = "Repayment" }},
		{"a figure name with a space", func(f *accounting.Figure) { f.Figure = "repayment ratio" }},
		{"no value", func(f *accounting.Figure) { f.Value = "" }},
		{"a value with a thousands comma", func(f *accounting.Figure) { f.Value = "1,200.50" }},
		{"a value that is a percentage", func(f *accounting.Figure) { f.Value = "95%" }},
		{"a value in exponent form", func(f *accounting.Figure) { f.Value = "1e3" }},
		{"a value that is not a number", func(f *accounting.Figure) { f.Value = "abc" }},
		{"no day", func(f *accounting.Figure) { f.OnDay = "" }},
		{"a day that does not exist", func(f *accounting.Figure) { f.OnDay = "2026-02-30" }},
		{"a day written the other way round", func(f *accounting.Figure) { f.OnDay = "31/03/2026" }},
		{"no function code", func(f *accounting.Figure) { f.FunctionCode = "" }},
		{"no function version", func(f *accounting.Figure) { f.FunctionVersion = "" }},
		{"no inputs fingerprint", func(f *accounting.Figure) { f.InputsFingerprint = "" }},
		{"no actor", func(f *accounting.Figure) { f.ActorID = "" }},
	}
	for _, c := range cases {
		f := ratioFigure("member-11", "2026-03-31", "0.7500")
		c.change(&f)
		if _, err := r.AppendFigure(ctx, f); !errors.Is(err, accounting.ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", c.name, err)
		}
	}
}

func TestFigureAppendIsRefusedWhereTheDomainKeepsNoRecordOfFigures(t *testing.T) {
	s, err := accounting.LoadSpec("spec/examples/school.accounting.json")
	if err != nil {
		t.Fatal(err)
	}
	r := newLedgerOn(t, openDB(t), s)

	_, err = r.AppendFigure(context.Background(), ratioFigure("pupil-1", "2026-03-31", "0.7500"))
	if !errors.Is(err, accounting.ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid", err)
	}
	if !strings.Contains(err.Error(), "keeps no record of figures") {
		t.Fatalf("the refusal must say so in plain words, got %q", err.Error())
	}
}

func TestFigureEveryFieldIsPrimaryOrImmutable(t *testing.T) {
	o, ok := blueprint(t).Object("figure")
	if !ok {
		t.Fatal("the blueprint declares no figure")
	}
	for _, f := range o.Fields {
		if !f.Primary && !f.Immutable {
			t.Errorf("figure field %q can change after it is written", f.Name)
		}
	}
}

func TestFigureIsUniquePerSubjectFigureAndDay(t *testing.T) {
	o, ok := blueprint(t).Object("figure")
	if !ok {
		t.Fatal("the blueprint declares no figure")
	}
	for _, ix := range o.Indexes {
		if ix.Unique && reflect.DeepEqual(ix.Fields, []string{"subject_id", "figure", "on_day"}) {
			return
		}
	}
	t.Fatal("the figure declares no unique index on subject_id, figure and on_day")
}

func TestFigureDeclaresNoUpdateOrDelete(t *testing.T) {
	var doc struct {
		Operations map[string]struct {
			Method string `json:"method"`
			Call   string `json:"call"`
			Action string `json:"action"`
		} `json:"operations"`
	}
	if err := json.Unmarshal(accounting.Operations(), &doc); err != nil {
		t.Fatal(err)
	}

	var figureCalls []string
	for name, op := range doc.Operations {
		if !strings.Contains(op.Action, ".figure.") && !strings.Contains(op.Call, "Figure") {
			continue
		}
		figureCalls = append(figureCalls, op.Call)
		if op.Method != "POST" && op.Method != "GET" {
			t.Errorf("%s: a figure is only appended and read, not %s", name, op.Method)
		}
		for _, word := range []string{"update", "delete", "remove", "edit", "correct"} {
			if strings.Contains(strings.ToLower(name+op.Call+op.Action), word) {
				t.Errorf("%s: a figure declares no %s", name, word)
			}
		}
	}
	sort.Strings(figureCalls)
	if want := []string{"AppendFigure", "ListFigureHistory"}; !reflect.DeepEqual(figureCalls, want) {
		t.Fatalf("figure operations = %v, want %v", figureCalls, want)
	}

	repo := reflect.TypeOf((*accounting.LedgerRepository)(nil)).Elem()
	var methods []string
	for i := 0; i < repo.NumMethod(); i++ {
		if strings.Contains(repo.Method(i).Name, "Figure") {
			methods = append(methods, repo.Method(i).Name)
		}
	}
	sort.Strings(methods)
	if want := []string{"AppendFigure", "ListFigureHistory"}; !reflect.DeepEqual(methods, want) {
		t.Fatalf("repository figure methods = %v, want %v", methods, want)
	}
}
