package accounting_test

import (
	"reflect"
	"sort"
	"strings"
	"testing"

	accounting "github.com/aosanya/mwanachama-backend-accounting"
	"github.com/aosanya/mwanachama-backend-accounting/models"
	"github.com/aosanya/mwanachama-backend-shared/spec"
	"github.com/aosanya/mwanachama-backend-shared/specstore"
)

func blueprint(t *testing.T) *spec.Blueprint {
	t.Helper()
	b, err := accounting.Blueprint()
	if err != nil {
		t.Fatalf("blueprint: %v", err)
	}
	return b
}

func TestBlueprintDeclaresBothRoles(t *testing.T) {
	got := blueprint(t).Roles()
	want := []string{"account", "entry"}
	if len(got) != len(want) {
		t.Fatalf("roles = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("roles = %v, want %v", got, want)
		}
	}
}

// specstore.New refuses a carrier that disagrees with its object, so this
// would already fail at construction. Asserting it here names the field
// instead of failing a whole manager, which is what a spec edit needs.
func TestEveryDeclaredFieldIsCarried(t *testing.T) {
	carriers := map[string]any{
		"account": models.Account{},
		"entry":   models.Entry{},
	}
	b := blueprint(t)
	for role, carrier := range carriers {
		o, ok := b.Object(role)
		if !ok {
			t.Fatalf("the blueprint declares no %q", role)
		}
		carried := specstore.ColumnsOf(reflect.TypeOf(carrier))
		for _, f := range o.Fields {
			if !carried[f.Name] {
				t.Errorf("%s declares %q, which %T does not carry", role, f.Name, carrier)
			}
		}
		declared := map[string]bool{}
		for _, f := range o.Fields {
			declared[f.Name] = true
		}
		for column := range carried {
			if !declared[column] {
				t.Errorf("%T carries %q, which the role %q does not declare", carrier, column, role)
			}
		}
	}
}

// A stored value outlives a rename, which makes a drifted constant a data
// bug rather than a compile error — so the two are held to each other in
// both directions.
func TestAccountTypeValuesAndGoConstantsAgree(t *testing.T) {
	o, ok := blueprint(t).Object("account")
	if !ok {
		t.Fatal("the blueprint declares no account")
	}
	var declared []string
	for _, f := range o.Fields {
		if f.Name == "type" {
			declared = append(declared, f.Values...)
		}
	}
	if len(declared) == 0 {
		t.Fatal("the account's type field declares no values")
	}

	var constants []string
	for _, v := range models.AccountTypes() {
		constants = append(constants, string(v))
	}

	sort.Strings(declared)
	sort.Strings(constants)
	if strings.Join(declared, ",") != strings.Join(constants, ",") {
		t.Fatalf("the blueprint declares [%s] and Go holds [%s]",
			strings.Join(declared, ", "), strings.Join(constants, ", "))
	}

	for _, v := range models.AccountTypes() {
		if !models.IsAccountType(v) {
			t.Errorf("IsAccountType(%q) is false for a declared value", v)
		}
	}
	if models.IsAccountType("bogus") {
		t.Error("IsAccountType accepts a value the blueprint does not declare")
	}
}

func TestShippedSpecNamesTablesUnderInstanceAndModule(t *testing.T) {
	s := shippedSpec(t)
	want := map[string]string{
		"account": "mwanachama_accounting_accounts",
		"entry":   "mwanachama_accounting_entries",
	}
	for role, table := range want {
		o, ok := s.ByRole(role)
		if !ok {
			t.Fatalf("the shipped spec fills no %q", role)
		}
		if got := s.TableFor(o); got != table {
			t.Errorf("%s lands in %q, want %q", role, got, table)
		}
	}
}

func TestSpecForRefusesAnUnusableInstance(t *testing.T) {
	if _, err := accounting.SpecFor("Not A Segment"); err == nil {
		t.Fatal("want an error for an instance that is not a usable name segment")
	}
}
