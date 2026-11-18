package accounting_test

import (
	"errors"
	"strings"
	"testing"

	accounting "github.com/aosanya/mwanachama-backend-accounting"
	"github.com/aosanya/mwanachama-backend-accounting/models"
	"github.com/aosanya/mwanachama-backend-shared/spec"
)

func validAccount() models.Account {
	return models.Account{
		Category: "member",
		Type:     models.AccountTypeAsset,
		HolderID: "holder-1",
	}
}

func TestCheckAcceptsACompleteAccount(t *testing.T) {
	if err := accounting.Check(shippedSpec(t), accounting.RoleAccount, validAccount()); err != nil {
		t.Fatalf("complete account refused: %v", err)
	}
}

func TestCheckAcceptsAPointerToAnAccount(t *testing.T) {
	a := validAccount()
	if err := accounting.Check(shippedSpec(t), accounting.RoleAccount, &a); err != nil {
		t.Fatalf("pointer to a complete account refused: %v", err)
	}
}

func TestCheckRefusesAnAccountWithNoHolder(t *testing.T) {
	a := validAccount()
	a.HolderID = "   "
	err := accounting.Check(shippedSpec(t), accounting.RoleAccount, a)
	if !errors.Is(err, accounting.ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid", err)
	}
	if !strings.Contains(err.Error(), "holder_id") {
		t.Fatalf("refusal %q does not name holder_id", err)
	}
}

func TestCheckRefusesAnAccountWithNoCategory(t *testing.T) {
	a := validAccount()
	a.Category = ""
	err := accounting.Check(shippedSpec(t), accounting.RoleAccount, a)
	if !errors.Is(err, accounting.ErrInvalid) || !strings.Contains(err.Error(), "category") {
		t.Fatalf("err = %v, want ErrInvalid naming category", err)
	}
}

func TestCheckRefusesAnUnknownAccountTypeAndListsTheDeclaredOnes(t *testing.T) {
	a := validAccount()
	a.Type = "revenue"
	err := accounting.Check(shippedSpec(t), accounting.RoleAccount, a)
	if !errors.Is(err, accounting.ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid", err)
	}
	for _, want := range []string{`"revenue"`, "asset", "liability", "equity", "income", "expense"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("refusal %q does not mention %s", err, want)
		}
	}
}

func TestCheckAcceptsEveryDeclaredAccountType(t *testing.T) {
	for _, ty := range models.AccountTypes() {
		a := validAccount()
		a.Type = ty
		if err := accounting.Check(shippedSpec(t), accounting.RoleAccount, a); err != nil {
			t.Errorf("type %q refused: %v", ty, err)
		}
	}
}

func TestCheckRefusesAnEntryMissingARequiredField(t *testing.T) {
	full := models.Entry{
		DebitAccountID:  "a",
		CreditAccountID: "b",
		Amount:          10,
		DocumentKind:    "invoice",
		DocumentID:      "doc-1",
		ActorID:         "actor-1",
	}
	if err := accounting.Check(shippedSpec(t), accounting.RoleEntry, full); err != nil {
		t.Fatalf("complete entry refused: %v", err)
	}
	cases := map[string]func(e *models.Entry){
		"debit_account_id":  func(e *models.Entry) { e.DebitAccountID = "" },
		"credit_account_id": func(e *models.Entry) { e.CreditAccountID = "" },
		"document_kind":     func(e *models.Entry) { e.DocumentKind = "" },
		"document_id":       func(e *models.Entry) { e.DocumentID = "" },
		"actor_id":          func(e *models.Entry) { e.ActorID = "" },
	}
	for field, blank := range cases {
		e := full
		blank(&e)
		err := accounting.Check(shippedSpec(t), accounting.RoleEntry, e)
		if !errors.Is(err, accounting.ErrInvalid) || !strings.Contains(err.Error(), field) {
			t.Errorf("blank %s: err = %v, want ErrInvalid naming it", field, err)
		}
	}
}

func TestCheckRefusesABudgetWithNoPeriod(t *testing.T) {
	b := models.Budget{AccountID: "a", PeriodStart: "2026-01-01T00:00:00Z"}
	err := accounting.Check(shippedSpec(t), accounting.RoleBudget, b)
	if !errors.Is(err, accounting.ErrInvalid) || !strings.Contains(err.Error(), "period_end") {
		t.Fatalf("err = %v, want ErrInvalid naming period_end", err)
	}
}

func TestCheckRefusesARoleTheDomainDoesNotFill(t *testing.T) {
	err := accounting.Check(shippedSpec(t), "ghost", validAccount())
	if err == nil || !strings.Contains(err.Error(), `"ghost"`) {
		t.Fatalf("err = %v, want a refusal naming the role", err)
	}
}

func TestCheckRefusesAValueThatIsNotAStruct(t *testing.T) {
	for _, v := range []any{"account", 42, map[string]string{"category": "x"}} {
		if err := accounting.Check(shippedSpec(t), accounting.RoleAccount, v); err == nil {
			t.Errorf("%T accepted, want a refusal", v)
		}
	}
}

func TestCheckOnAnEmptySpecNamesTheMissingRole(t *testing.T) {
	err := accounting.Check(&spec.Spec{}, accounting.RoleAccount, validAccount())
	if err == nil {
		t.Fatal("empty spec accepted an account")
	}
}
