package routes_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/aosanya/mwanachama-backend-shared/dispatch"

	accounting "github.com/aosanya/mwanachama-backend-accounting"
	"github.com/aosanya/mwanachama-backend-accounting/routes"
)

const (
	roadRepairJanuary = `"name":"Road repair","department":"Public works","period_start":"2026-01-01T00:00:00Z","period_end":"2026-02-01T00:00:00Z"`
	longAgo           = "2020-01-01T00:00:00Z"
)

func budgetTools(t *testing.T, repo accounting.LedgerRepository) map[string]dispatch.Tool {
	t.Helper()
	spec, err := dispatch.Parse(accounting.Operations())
	if err != nil {
		t.Fatal(err)
	}
	all, err := dispatch.Tools(spec, dispatch.Deps{Manager: repo, Errors: routes.Sentinels()})
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]dispatch.Tool{}
	for _, tool := range all {
		byName[tool.Name] = tool
	}
	return byName
}

func budgetWithALine(t *testing.T) (map[string]dispatch.Tool, accounting.LedgerRepository, string, accounting.Budget) {
	t.Helper()
	repo := ledgerOnMemory(t)
	rent, err := repo.OpenAccount(context.Background(), accounting.Account{Code: "5000", Category: "rent", HolderID: "town", Type: accounting.AccountTypeExpense})
	if err != nil {
		t.Fatal(err)
	}
	tools := budgetTools(t, repo)
	first, err := tools["accounting_budget_set"].Invoke(context.Background(), json.RawMessage(`{"account_id":"`+rent.ID+`",`+roadRepairJanuary+`,"amount":1000}`))
	if err != nil {
		t.Fatalf("first set: %v", err)
	}
	return tools, repo, rent.ID, first.(accounting.Budget)
}

func setBudget(tools map[string]dispatch.Tool, accountID, extra string, amount int) (any, error) {
	body := `{"account_id":"` + accountID + `",` + roadRepairJanuary + `,"amount":` + strconv.Itoa(amount) + extra + `}`
	return tools["accounting_budget_set"].Invoke(context.Background(), json.RawMessage(body))
}

func TestBudgetSetAcceptsTheVersionItWasRead(t *testing.T) {
	tools, _, accountID, first := budgetWithALine(t)

	got, err := setBudget(tools, accountID, `,"version":"`+first.SetAt+`"`, 1200)
	if err != nil {
		t.Fatalf("set on the current version: %v", err)
	}
	if got.(accounting.Budget).Amount != 1200 {
		t.Fatalf("set %+v", got)
	}
}

func TestBudgetSetRefusesAnOldVersionNamingTheChangedField(t *testing.T) {
	tools, repo, accountID, first := budgetWithALine(t)

	_, err := setBudget(tools, accountID, `,"version":"`+longAgo+`"`, 1200)
	if err == nil {
		t.Fatal("a save made on an old version was accepted")
	}
	if err.Error() != "amount changed since you read it" {
		t.Fatalf("refusal %q", err.Error())
	}
	lines, err := repo.ListBudgetLines(context.Background(), accounting.BudgetLineFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 1 || lines[0].Planned != 1000 || lines[0].SetAt != first.SetAt {
		t.Fatalf("a refused save changed the line: %+v", lines)
	}
}

func TestBudgetSetNamesTheFieldTheCallerChangedWhenItIsNotTheAmount(t *testing.T) {
	tools, _, accountID, _ := budgetWithALine(t)

	body := `{"account_id":"` + accountID + `","name":"Road resurfacing","department":"Public works","period_start":"2026-01-01T00:00:00Z","period_end":"2026-02-01T00:00:00Z","amount":1000,"version":"` + longAgo + `"}`
	_, err := tools["accounting_budget_set"].Invoke(context.Background(), json.RawMessage(body))
	if err == nil || err.Error() != "name changed since you read it" {
		t.Fatalf("refusal %v", err)
	}
}

func TestBudgetSetWithNoVersionIsAccepted(t *testing.T) {
	tools, _, accountID, _ := budgetWithALine(t)

	got, err := setBudget(tools, accountID, ``, 1500)
	if err != nil {
		t.Fatalf("set with no version: %v", err)
	}
	if got.(accounting.Budget).Amount != 1500 {
		t.Fatalf("set %+v", got)
	}
}

func TestBudgetSetOfALineThatDoesNotExistYetHasNothingToBeStaleAgainst(t *testing.T) {
	repo := ledgerOnMemory(t)
	rent, err := repo.OpenAccount(context.Background(), accounting.Account{Code: "5000", Category: "rent", HolderID: "town", Type: accounting.AccountTypeExpense})
	if err != nil {
		t.Fatal(err)
	}
	tools := budgetTools(t, repo)

	if _, err := setBudget(tools, rent.ID, `,"version":"`+longAgo+`"`, 1000); err != nil {
		t.Fatalf("first set carrying a version: %v", err)
	}
}

func TestBudgetSetKeepsTheCostSharesAndMilestoneSwitchWhenTheVersionMatches(t *testing.T) {
	tools, repo, accountID, first := budgetWithALine(t)

	extra := `,"version":"` + first.SetAt + `","follow_milestones":true,"cost_shares":{"Northfield":70,"Southfield":30}`
	if _, err := setBudget(tools, accountID, extra, 1000); err != nil {
		t.Fatalf("set with shares: %v", err)
	}
	lines, err := repo.ListBudgetLines(context.Background(), accounting.BudgetLineFilter{})
	if err != nil || len(lines) != 1 || !lines[0].FollowMilestones || lines[0].CostShares["Northfield"] != 70 {
		t.Fatalf("lines %+v %v", lines, err)
	}
}

func TestBudgetSetSchemaDeclaresAnOptionalVersion(t *testing.T) {
	tools, _, _, _ := budgetWithALine(t)

	var schema struct {
		Properties map[string]struct {
			Type string `json:"type"`
		} `json:"properties"`
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(tools["accounting_budget_set"].InputSchema, &schema); err != nil {
		t.Fatal(err)
	}
	if schema.Properties["version"].Type != "string" {
		t.Fatalf("version property %+v", schema.Properties["version"])
	}
	if slices.Contains(schema.Required, "version") {
		t.Fatal("version is required")
	}
}

func TestBudgetSetRefusalOverHTTPIsAConflictCarryingThePhrase(t *testing.T) {
	repo := ledgerOnMemory(t)
	rent, err := repo.OpenAccount(context.Background(), accounting.Account{Code: "5000", Category: "rent", HolderID: "town", Type: accounting.AccountTypeExpense})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.SetBudget(context.Background(), accounting.Budget{AccountID: rent.ID, Name: "Road repair", Department: "Public works", PeriodStart: "2026-01-01T00:00:00Z", PeriodEnd: "2026-02-01T00:00:00Z", Amount: 1000}); err != nil {
		t.Fatal(err)
	}
	list, err := routes.Build(repo)
	if err != nil {
		t.Fatal(err)
	}

	rec := serve(list, http.MethodPut, "/budgets", `{"account_id":"`+rent.ID+`",`+roadRepairJanuary+`,"amount":1200,"version":"`+longAgo+`"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status %d, body %s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), "amount changed since you read it") {
		t.Fatalf("body %s", rec.Body)
	}
}

func TestTheSentinelsAreAnExportedCopyThatNamesEveryDeclaredError(t *testing.T) {
	var declared struct {
		Errors map[string]int `json:"errors"`
	}
	if err := json.Unmarshal(accounting.Operations(), &declared); err != nil {
		t.Fatal(err)
	}
	got := routes.Sentinels()
	for name := range declared.Errors {
		if got[name] == nil {
			t.Errorf("declared error %s has no sentinel", name)
		}
	}
	if got["ErrStale"] == nil || errors.Is(got["ErrStale"], accounting.ErrInvalid) {
		t.Fatalf("ErrStale sentinel %v", got["ErrStale"])
	}
	delete(got, "ErrNotFound")
	if routes.Sentinels()["ErrNotFound"] == nil {
		t.Fatal("changing the returned map changed the routes' own")
	}
}
