package routes_test

import (
	"encoding/json"
	"net/http"
	"testing"

	accounting "github.com/aosanya/mwanachama-backend-accounting"
	"github.com/aosanya/mwanachama-backend-accounting/routes"
)

func TestTheShapeNamesTheBudgetDeleteUnderItsAction(t *testing.T) {
	for _, rt := range routes.Shape() {
		if rt.Method == http.MethodDelete && rt.Path == "/budgets/{budgetID}" {
			if rt.Action != "accounting.budget.delete" {
				t.Fatalf("action %q", rt.Action)
			}
			return
		}
	}
	t.Fatal("no DELETE /budgets/{budgetID} in the shape")
}

func TestABudgetInUseAnswers409AndAMissingOneAnswers404(t *testing.T) {
	var declared struct {
		Errors map[string]int `json:"errors"`
	}
	if err := json.Unmarshal(accounting.Operations(), &declared); err != nil {
		t.Fatal(err)
	}
	if declared.Errors["ErrBudgetLineInUse"] != http.StatusConflict || declared.Errors["ErrNotFound"] != http.StatusNotFound {
		t.Fatalf("errors %+v", declared.Errors)
	}
}
