package routes_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
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

func TestTheBudgetDeleteToolRemovesALineAndReturnsIt(t *testing.T) {
	tools, repo, _, line := budgetWithALine(t)
	tool, ok := tools["accounting_budget_delete"]
	if !ok {
		t.Fatal("no tool accounting_budget_delete")
	}
	got, err := tool.Invoke(context.Background(), json.RawMessage(`{"budgetID":"`+line.ID+`","version":"`+line.SetAt+`"}`))
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if got.(accounting.BudgetLine).ID != line.ID {
		t.Fatalf("returned %+v", got)
	}
	if _, err := repo.GetBudgetLine(context.Background(), line.ID); !errors.Is(err, accounting.ErrNotFound) {
		t.Fatalf("the line is still there: %v", err)
	}
}

func TestTheBudgetDeleteToolSchemaNamesTheLineAndAnOptionalVersion(t *testing.T) {
	tools, _, _, _ := budgetWithALine(t)
	tool, ok := tools["accounting_budget_delete"]
	if !ok {
		t.Fatal("no tool accounting_budget_delete")
	}
	var schema struct {
		Properties map[string]struct {
			Type string `json:"type"`
		} `json:"properties"`
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(tool.InputSchema, &schema); err != nil {
		t.Fatal(err)
	}
	if schema.Properties["budgetID"].Type != "string" || schema.Properties["version"].Type != "string" {
		t.Fatalf("properties %+v", schema.Properties)
	}
	if slices.Contains(schema.Required, "version") || !slices.Contains(schema.Required, "budgetID") {
		t.Fatalf("required %v", schema.Required)
	}
	if tool.Action != "accounting.budget.delete" {
		t.Fatalf("action %s", tool.Action)
	}
}

func TestADeleteOfALineWithSpendingIsRefusedThroughTheTool(t *testing.T) {
	repo := ledgerOnMemory(t)
	ctx := context.Background()
	rent, err := repo.OpenAccount(ctx, accounting.Account{Code: "5000", Category: "rent", HolderID: "town", Type: accounting.AccountTypeExpense})
	if err != nil {
		t.Fatal(err)
	}
	cash, err := repo.OpenAccount(ctx, accounting.Account{Code: "1000", Category: "cash", HolderID: "town", Type: accounting.AccountTypeAsset})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Post(ctx, accounting.Entry{
		DebitAccountID: rent.ID, CreditAccountID: cash.ID, Amount: 250, DocumentKind: "invoice", DocumentID: "inv-1",
		ActorID: "clerk", OccurredAt: "2026-01-12T00:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}
	line, err := repo.SetBudget(ctx, accounting.Budget{
		AccountID: rent.ID, Name: "Road repair", Department: "Public works",
		PeriodStart: "2026-01-01T00:00:00Z", PeriodEnd: "2026-02-01T00:00:00Z", Amount: 1000,
	})
	if err != nil {
		t.Fatal(err)
	}
	tool := budgetTools(t, repo)["accounting_budget_delete"]
	if _, err := tool.Invoke(ctx, json.RawMessage(`{"budgetID":"`+line.ID+`"}`)); !errors.Is(err, accounting.ErrBudgetLineInUse) {
		t.Fatalf("delete of a line with spending: %v", err)
	}
}

func TestADeleteOnAnOldVersionOverHTTPIsAConflictAndKeepsTheLine(t *testing.T) {
	repo := ledgerOnMemory(t)
	rent, err := repo.OpenAccount(context.Background(), accounting.Account{Code: "5000", Category: "rent", HolderID: "town", Type: accounting.AccountTypeExpense})
	if err != nil {
		t.Fatal(err)
	}
	line, err := repo.SetBudget(context.Background(), accounting.Budget{AccountID: rent.ID, Name: "Road repair", Department: "Public works", PeriodStart: "2026-01-01T00:00:00Z", PeriodEnd: "2026-02-01T00:00:00Z", Amount: 1000})
	if err != nil {
		t.Fatal(err)
	}
	list, err := routes.Build(repo)
	if err != nil {
		t.Fatal(err)
	}

	rec := serve(list, http.MethodDelete, "/budgets/"+line.ID+"?version="+longAgo, "")
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "record changed since you read it") {
		t.Fatalf("status %d, body %s", rec.Code, rec.Body)
	}
	if _, err := repo.GetBudgetLine(context.Background(), line.ID); err != nil {
		t.Fatalf("the refused line is gone: %v", err)
	}

	rec = serve(list, http.MethodDelete, "/budgets/"+line.ID+"?version="+line.SetAt, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, body %s", rec.Code, rec.Body)
	}
	var removed accounting.BudgetLine
	decode(t, rec, &removed)
	if removed.ID != line.ID || removed.Name != "Road repair" || removed.Planned != 1000 {
		t.Fatalf("removed %+v", removed)
	}
}
