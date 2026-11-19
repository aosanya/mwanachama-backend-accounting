package routes_test

import (
	"net/http"
	"testing"

	accounting "github.com/aosanya/mwanachama-backend-accounting"
)

func TestAPaymentPostedOverHTTPKeepsItsReferenceAndMethodAndListsThemBack(t *testing.T) {
	handlers := clerkRoutes(t)
	income := openOverHTTP(t, handlers, "fees", "income")
	receivable := openOverHTTP(t, handlers, "receivable", "asset")
	cash := openOverHTTP(t, handlers, "cash", "asset")

	rec := serve(handlers, http.MethodPost, "/bills",
		`{"income_account_id":"`+income.ID+`","receivable_account_id":"`+receivable.ID+`","amount":3000,"due_at":"2030-01-01T00:00:00Z","payer":"Harbour Studio"}`)
	var bill accounting.Bill
	decode(t, rec, &bill)

	rec = serve(handlers, http.MethodPost, "/bills/"+bill.ID+"/payments",
		`{"cash_account_id":"`+cash.ID+`","amount":3000,"paid_on":"2026-11-01T00:00:00Z","method":"bank transfer","reference":"TR-2026-77","narration":"Invoice 12"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("payment with a reference: got %d, body %s", rec.Code, rec.Body.String())
	}
	var payment accounting.Payment
	decode(t, rec, &payment)
	if payment.Reference != "TR-2026-77" || payment.Method != "bank transfer" {
		t.Fatalf("payment %+v", payment)
	}

	rec = serve(handlers, http.MethodGet, "/payments?bill_id="+bill.ID, "")
	var listed []accounting.Payment
	decode(t, rec, &listed)
	if rec.Code != http.StatusOK || len(listed) != 1 || listed[0].Reference != "TR-2026-77" || listed[0].Method != "bank transfer" {
		t.Fatalf("listed payments: got %d, %+v", rec.Code, listed)
	}
}

func TestAPaymentWithAnUnknownFieldIsStillRefusedByName(t *testing.T) {
	handlers := clerkRoutes(t)
	rec := serve(handlers, http.MethodPost, "/bills/any/payments", `{"cash_account_id":"x","receipt_no":"1"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("got %d, body %s", rec.Code, rec.Body.String())
	}
}
