package routes_test

import (
	"net/http"
	"strconv"
	"testing"

	accounting "github.com/aosanya/mwanachama-backend-accounting"
)

func TestARateIsEnteredAndAReportConvertedOverHTTPAndAMixedBalanceIsAConflict(t *testing.T) {
	handlers := clerkRoutes(t)
	workshops := openOverHTTP(t, handlers, "workshops", "expense")
	cash := openOverHTTP(t, handlers, "cash", "asset")
	post := func(amount int64, currency, day, document string) {
		t.Helper()
		body := `{"debit_account_id":"` + workshops.ID + `","credit_account_id":"` + cash.ID + `","amount":` + itoa(amount) + `,"currency":"` + currency + `","occurred_at":"` + day + `","document_kind":"expense","document_id":"` + document + `","actor_id":"a"}`
		if rec := serve(handlers, http.MethodPost, "/entries", body); rec.Code != http.StatusCreated {
			t.Fatalf("post: got %d, body %s", rec.Code, rec.Body.String())
		}
	}
	post(140_000, "KES", "2027-02-10T09:00:00Z", "venue")
	post(500, "EUR", "2027-02-11T09:00:00Z", "expert")

	if rec := serve(handlers, http.MethodGet, "/accounts/"+workshops.ID+"/balance", ""); rec.Code != http.StatusConflict {
		t.Fatalf("mixed balance: got %d, want 409, body %s", rec.Code, rec.Body.String())
	}
	var held []accounting.CurrencyAmount
	rec := serve(handlers, http.MethodGet, "/accounts/"+workshops.ID+"/balance/by-currency", "")
	decode(t, rec, &held)
	if rec.Code != http.StatusOK || len(held) != 2 {
		t.Fatalf("by currency: got %d, %+v", rec.Code, held)
	}

	rate := `{"base":"EUR","counter":"KES","rate":14000000000,"applies_on":"2027-02-10","kind":"daily","source":"test table","recorded_by":"forged"}`
	rec = serve(handlers, http.MethodPost, "/rates", rate)
	var entered accounting.ExchangeRate
	decode(t, rec, &entered)
	if rec.Code != http.StatusCreated || entered.RecordedBy != "clerk-1" {
		t.Fatalf("enter rate: got %d, %+v", rec.Code, entered)
	}
	line := `{"account_id":"` + workshops.ID + `","period_start":"2027-02-01T00:00:00Z","period_end":"2027-03-01T00:00:00Z","amount":2500000,"currency":"EUR"}`
	if rec := serve(handlers, http.MethodPut, "/budgets", line); rec.Code != http.StatusOK && rec.Code != http.StatusCreated {
		t.Fatalf("budget: got %d, body %s", rec.Code, rec.Body.String())
	}

	query := "?period_start=2027-02-01T00:00:00Z&period_end=2027-03-01T00:00:00Z&reporting_currency=EUR&rule=posting_date"
	rec = serve(handlers, http.MethodGet, "/reports/converted"+query, "")
	var report accounting.ConvertedReport
	decode(t, rec, &report)
	if rec.Code != http.StatusOK || report.Spent != 1_500 || !report.Complete || report.Rule != accounting.RuleRateAtPostingDate {
		t.Fatalf("converted report: got %d, %+v", rec.Code, report)
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
