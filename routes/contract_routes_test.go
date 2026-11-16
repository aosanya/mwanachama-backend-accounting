package routes_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	accounting "github.com/aosanya/mwanachama-backend-accounting"
	"github.com/aosanya/mwanachama-backend-accounting/routes"
)

func TestContractRoutesCertifyWithCallerAndRefuseOverCertification(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	s, err := accounting.SpecFor("test")
	if err != nil {
		t.Fatal(err)
	}
	if err := accounting.Provision(db, s); err != nil {
		t.Fatal(err)
	}
	repo, err := accounting.NewLedgerRepository(db, s)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	ward, _ := repo.OpenAccount(ctx, accounting.Account{Category: "ward_spending", HolderID: "w", Type: accounting.AccountTypeExpense})
	payee, _ := repo.OpenAccount(ctx, accounting.Account{Category: "contractor", HolderID: "c", Type: accounting.AccountTypeLiability})
	held, _ := repo.OpenAccount(ctx, accounting.Account{Category: "retention", HolderID: "c", Type: accounting.AccountTypeLiability})
	ms, err := repo.AddMilestone(ctx, accounting.Milestone{AccountID: ward.ID, ContractID: "k", Name: "Roof", Amount: 100})
	if err != nil {
		t.Fatal(err)
	}

	handlers, err := routes.BuildFor(repo, routes.Mount{Caller: func(context.Context) string { return "engineer-9" }})
	if err != nil {
		t.Fatalf("BuildFor: %v", err)
	}
	call := func(method, path, body string) *httptest.ResponseRecorder {
		for _, rt := range handlers {
			if rt.Method == method && strings.HasSuffix(rt.Path, "{milestoneID}/certificates") {
				req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
				req.SetPathValue("milestoneID", ms.ID)
				rec := httptest.NewRecorder()
				rt.Handler(rec, req)
				return rec
			}
		}
		t.Fatalf("no route for %s %s", method, path)
		return nil
	}
	payload := func(amount int) string {
		b, _ := json.Marshal(map[string]any{
			"payee_account_id": payee.ID, "retention_account_id": held.ID,
			"amount": amount, "certified_by": "someone-else",
		})
		return string(b)
	}

	rec := call(http.MethodPost, "/milestones/"+ms.ID+"/certificates", payload(60))
	if rec.Code != http.StatusCreated {
		t.Fatalf("certify = %d %s", rec.Code, rec.Body)
	}
	var got accounting.Certificate
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.CertifiedBy != "engineer-9" || got.MilestoneID != ms.ID {
		t.Fatalf("certificate = %+v, want caller and address to decide", got)
	}
	if rec := call(http.MethodPost, "/milestones/"+ms.ID+"/certificates", payload(41)); rec.Code != http.StatusConflict {
		t.Fatalf("over-certify = %d, want 409", rec.Code)
	}
}
