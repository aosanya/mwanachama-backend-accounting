package routes_test

import (
	"context"
	"net/http"
	"testing"

	accounting "github.com/aosanya/mwanachama-backend-accounting"
	"github.com/aosanya/mwanachama-backend-accounting/routes"
)

func clerkRoutes(t *testing.T) []routes.Route {
	t.Helper()
	handlers, err := routes.BuildFor(ledgerOnMemory(t), routes.Mount{Caller: func(context.Context) string { return "clerk-1" }})
	if err != nil {
		t.Fatalf("BuildFor: %v", err)
	}
	return handlers
}

func openOverHTTP(t *testing.T, handlers []routes.Route, category, kind string) accounting.Account {
	t.Helper()
	rec := serve(handlers, http.MethodPost, "/accounts", `{"category":"`+category+`","type":"`+kind+`","holder_id":"town"}`)
	if rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
		t.Fatalf("open %s: got %d, body %s", category, rec.Code, rec.Body.String())
	}
	var account accounting.Account
	decode(t, rec, &account)
	return account
}

func TestABillIsRaisedPaidInPartsAndLockedOnceSettledOverHTTP(t *testing.T) {
	handlers := clerkRoutes(t)
	income := openOverHTTP(t, handlers, "rates", "income")
	receivable := openOverHTTP(t, handlers, "receivable", "asset")
	cash := openOverHTTP(t, handlers, "cash", "asset")

	rec := serve(handlers, http.MethodPost, "/bills",
		`{"income_account_id":"`+income.ID+`","receivable_account_id":"`+receivable.ID+`","amount":5000,"due_at":"2030-01-01T00:00:00Z","payer":"Fo utca 3","billed_by":"forged"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("raise bill: got %d, body %s", rec.Code, rec.Body.String())
	}
	var bill accounting.Bill
	decode(t, rec, &bill)
	if bill.BilledBy != "clerk-1" || bill.Status != accounting.BillStatus("open") || bill.Outstanding != 5000 {
		t.Fatalf("the bill is billed by the mount's caller and starts open: %+v", bill)
	}

	rec = serve(handlers, http.MethodPost, "/bills/"+bill.ID+"/payments", `{"cash_account_id":"`+cash.ID+`","amount":2000,"paid_by":"forged"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("part payment: got %d, body %s", rec.Code, rec.Body.String())
	}
	var payment accounting.Payment
	decode(t, rec, &payment)
	if payment.PaidBy != "clerk-1" || payment.BillID != bill.ID || payment.Amount != 2000 {
		t.Fatalf("payment %+v", payment)
	}

	rec = serve(handlers, http.MethodGet, "/bills/"+bill.ID, "")
	var part accounting.Bill
	decode(t, rec, &part)
	if rec.Code != http.StatusOK || part.Settled != 2000 || part.Outstanding != 3000 {
		t.Fatalf("read bill: got %d, %+v", rec.Code, part)
	}

	rec = serve(handlers, http.MethodPatch, "/bills/"+bill.ID, `{"amount":6000,"due_at":"2030-01-01T00:00:00Z"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("changing the amount of a part paid bill: got %d, want 409, body %s", rec.Code, rec.Body.String())
	}
	rec = serve(handlers, http.MethodPatch, "/bills/"+bill.ID, `{"amount":5000,"due_at":"2030-02-01T00:00:00Z","payer":"Fo utca 5"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("moving the due date: got %d, body %s", rec.Code, rec.Body.String())
	}

	rec = serve(handlers, http.MethodPost, "/bills/"+bill.ID+"/payments", `{"cash_account_id":"`+cash.ID+`"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("paying the rest: got %d, body %s", rec.Code, rec.Body.String())
	}
	rec = serve(handlers, http.MethodPost, "/bills/"+bill.ID+"/payments", `{"cash_account_id":"`+cash.ID+`"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("paying a settled bill: got %d, want 400, body %s", rec.Code, rec.Body.String())
	}
	rec = serve(handlers, http.MethodPatch, "/bills/"+bill.ID, `{"amount":5000,"due_at":"2030-03-01T00:00:00Z"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("changing a settled bill: got %d, want 409", rec.Code)
	}

	rec = serve(handlers, http.MethodGet, "/bills?status=settled", "")
	var settled []accounting.Bill
	decode(t, rec, &settled)
	if rec.Code != http.StatusOK || len(settled) != 1 || settled[0].ID != bill.ID {
		t.Fatalf("settled bills: got %d, %+v", rec.Code, settled)
	}
	rec = serve(handlers, http.MethodGet, "/payments?bill_id="+bill.ID, "")
	var payments []accounting.Payment
	decode(t, rec, &payments)
	if rec.Code != http.StatusOK || len(payments) != 2 {
		t.Fatalf("payments of the bill: got %d, %+v", rec.Code, payments)
	}
	rec = serve(handlers, http.MethodGet, "/bills?status=banana", "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown status: got %d, want 400", rec.Code)
	}
}
