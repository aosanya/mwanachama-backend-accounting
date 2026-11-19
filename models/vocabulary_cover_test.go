package models

import (
	"encoding/json"
	"sort"
	"testing"
	"time"
)

func TestIsAccountTypeAcceptsOnlyTheFiveBookkeepingTypes(t *testing.T) {
	for _, accepted := range []AccountType{"asset", "liability", "equity", "income", "expense"} {
		if !IsAccountType(accepted) {
			t.Errorf("%q should be an account type", accepted)
		}
	}
	for _, refused := range []AccountType{"", "Asset", "revenue", "assets", " asset", "basket"} {
		if IsAccountType(refused) {
			t.Errorf("%q should not be an account type", refused)
		}
	}
}

func TestAccountTypesListsEveryAcceptedTypeOnce(t *testing.T) {
	listed := AccountTypes()
	if len(listed) != len(accountTypes) {
		t.Fatalf("listed %d types, vocabulary holds %d", len(listed), len(accountTypes))
	}
	seen := map[AccountType]bool{}
	for _, listedType := range listed {
		if seen[listedType] {
			t.Errorf("%q listed twice", listedType)
		}
		seen[listedType] = true
		if !IsAccountType(listedType) {
			t.Errorf("listed %q is not accepted", listedType)
		}
	}
}

func TestAccountTypesReturnsAFreshSliceEachCall(t *testing.T) {
	first := AccountTypes()
	first[0] = "tampered"
	second := AccountTypes()
	names := make([]string, len(second))
	for i, accountType := range second {
		names[i] = string(accountType)
	}
	sort.Strings(names)
	if names[0] == "tampered" || !IsAccountType(second[0]) {
		t.Fatalf("mutating one result leaked into the next: %v", names)
	}
}

func TestNowRFC3339IsUTCNanosecondAndParsesBack(t *testing.T) {
	before := time.Now().UTC().Add(-time.Second)
	stamp := NowRFC3339()
	parsed, err := time.Parse(TimeLayout, stamp)
	if err != nil {
		t.Fatalf("stamp %q does not parse with its own layout: %v", stamp, err)
	}
	if _, offset := parsed.Zone(); offset != 0 {
		t.Errorf("stamp %q is not UTC", stamp)
	}
	if parsed.Before(before) || parsed.After(time.Now().UTC().Add(time.Second)) {
		t.Errorf("stamp %q is not near now", stamp)
	}
	if len(stamp) != len("2006-01-02T15:04:05.000000000Z") {
		t.Errorf("stamp %q is not fixed width to the nanosecond", stamp)
	}
}

func TestNowRFC3339StampsSortAsTextInTimeOrder(t *testing.T) {
	earlier := time.Date(2026, 1, 2, 3, 4, 5, 6, time.UTC).Format(TimeLayout)
	later := time.Date(2026, 1, 2, 3, 4, 5, 700, time.UTC).Format(TimeLayout)
	if !(earlier < later) {
		t.Fatalf("%q should sort before %q", earlier, later)
	}
}

func decodeObject(t *testing.T, value any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	return decoded
}

func TestAccountWireOmitsEmptyOptionalFieldsAndKeepsRequiredOnes(t *testing.T) {
	wire := decodeObject(t, Account{ID: "a1", Category: "cash", Type: AccountTypeAsset, HolderID: "h", OpenedAt: "t"})
	for _, absent := range []string{"code", "name", "closed_at"} {
		if _, present := wire[absent]; present {
			t.Errorf("%s should be omitted when empty", absent)
		}
	}
	for _, required := range []string{"id", "category", "type", "holder_id", "opened_at"} {
		if _, present := wire[required]; !present {
			t.Errorf("%s should always be present", required)
		}
	}
}

func TestEntryWireNamesTheDebitAndCreditSides(t *testing.T) {
	wire := decodeObject(t, Entry{DebitAccountID: "d", CreditAccountID: "c", Amount: 0})
	if wire["debit_account_id"] != "d" || wire["credit_account_id"] != "c" {
		t.Errorf("sides misnamed: %v", wire)
	}
	if _, present := wire["amount"]; !present {
		t.Error("a zero amount must still be on the wire")
	}
	for _, absent := range []string{"narration", "dimensions", "reverses_entry_id", "settles_entry_id", "occurred_at", "due_at"} {
		if _, present := wire[absent]; present {
			t.Errorf("%s should be omitted when empty", absent)
		}
	}
}

func TestOverdueEntryFlattensItsEntryBesideTheAgeing(t *testing.T) {
	wire := decodeObject(t, OverdueEntry{Entry: Entry{ID: "e1", Amount: 50}, Outstanding: 20, DaysOverdue: 3, AgeDays: 9})
	if wire["id"] != "e1" || wire["outstanding"] != float64(20) || wire["days_overdue"] != float64(3) || wire["age_days"] != float64(9) {
		t.Errorf("overdue entry not flattened: %v", wire)
	}
	if _, nested := wire["Entry"]; nested {
		t.Error("entry must not be nested under its type name")
	}
}

func TestAgeBasisValuesAreTheDeclaredWords(t *testing.T) {
	if AgeFromDue != "due" || AgeFromRaised != "raised" {
		t.Fatalf("age basis words changed: %q %q", AgeFromDue, AgeFromRaised)
	}
}

func TestMilestoneAndCertificateWireOmitOptionalDates(t *testing.T) {
	milestone := decodeObject(t, Milestone{ID: "m", Status: MilestonePlanned})
	if _, present := milestone["due_at"]; present {
		t.Error("due_at should be omitted when empty")
	}
	if milestone["status"] != "planned" || milestone["certified_amount"] != float64(0) {
		t.Errorf("milestone wire wrong: %v", milestone)
	}
	certificate := decodeObject(t, Certificate{ID: "c", RetentionBps: 500})
	for _, absent := range []string{"retention_entry_id", "release_due_at", "released_at", "release_entry_id"} {
		if _, present := certificate[absent]; present {
			t.Errorf("%s should be omitted when empty", absent)
		}
	}
	if certificate["retention_bps"] != float64(500) {
		t.Errorf("retention_bps lost: %v", certificate)
	}
}

func TestMilestoneStatusWordsAreTheDeclaredWords(t *testing.T) {
	if MilestonePlanned != "planned" || MilestonePartCertified != "part_certified" || MilestoneCertified != "certified" {
		t.Fatal("milestone status words changed")
	}
}

func TestFigureWireOmitsEmptyNarrationOnly(t *testing.T) {
	wire := decodeObject(t, Figure{ID: "f"})
	if _, present := wire["narration"]; present {
		t.Error("narration should be omitted when empty")
	}
	for _, kept := range []string{"subject_id", "figure", "on_day", "value", "function_code", "function_version", "inputs_fingerprint", "actor_id", "recorded_at"} {
		if _, present := wire[kept]; !present {
			t.Errorf("%s should always be present", kept)
		}
	}
}
