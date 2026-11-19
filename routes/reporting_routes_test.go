package routes_test

import (
	"net/http"
	"testing"

	accounting "github.com/aosanya/mwanachama-backend-accounting"
)

const (
	januaryFirst  = "2026-01-01T00:00:00Z"
	februaryFirst = "2026-02-01T00:00:00Z"
)

func TestABudgetLineIsCreatedOnceReadBackAndFoldedIntoAReportSentUpwardOverHTTP(t *testing.T) {
	handlers := clerkRoutes(t)
	roads := openOverHTTP(t, handlers, "roads", "expense")
	line := `{"account_id":"` + roads.ID + `","period_start":"` + januaryFirst + `","period_end":"` + februaryFirst + `","name":"Road repair","department":"Public works","amount":1000,"create":true}`

	rec := serve(handlers, http.MethodPut, "/budgets", line)
	if rec.Code != http.StatusOK && rec.Code != http.StatusCreated {
		t.Fatalf("set budget line: got %d, body %s", rec.Code, rec.Body.String())
	}
	var set accounting.Budget
	decode(t, rec, &set)

	if rec := serve(handlers, http.MethodPut, "/budgets", line); rec.Code != http.StatusConflict {
		t.Fatalf("creating the same line twice: got %d, want 409, body %s", rec.Code, rec.Body.String())
	}

	rec = serve(handlers, http.MethodGet, "/budgets?department=Public%20works", "")
	var lines []accounting.BudgetLine
	decode(t, rec, &lines)
	if rec.Code != http.StatusOK || len(lines) != 1 || lines[0].Name != "Road repair" || lines[0].Planned != 1000 {
		t.Fatalf("list budget lines: got %d, %+v", rec.Code, lines)
	}
	rec = serve(handlers, http.MethodGet, "/budgets/"+set.ID, "")
	var read accounting.BudgetLine
	decode(t, rec, &read)
	if rec.Code != http.StatusOK || read.ID != set.ID || read.Department != "Public works" {
		t.Fatalf("read budget line: got %d, %+v", rec.Code, read)
	}

	period := "?period_start=" + januaryFirst + "&period_end=" + februaryFirst
	rec = serve(handlers, http.MethodGet, "/reports/period"+period, "")
	var report accounting.FinancialReport
	decode(t, rec, &report)
	if rec.Code != http.StatusOK || report.Figures.Planned != 1000 || len(report.Lines) != 1 {
		t.Fatalf("read report: got %d, %+v", rec.Code, report)
	}
	rec = serve(handlers, http.MethodGet, "/reports", "")
	var reports []accounting.FinancialReport
	decode(t, rec, &reports)
	if rec.Code != http.StatusOK || len(reports) != 1 {
		t.Fatalf("list reports: got %d, %+v", rec.Code, reports)
	}

	rec = serve(handlers, http.MethodPost, "/upward-reports",
		`{"period_start":"`+januaryFirst+`","period_end":"`+februaryFirst+`","kind":"budget_and_actuals","sent_by":"forged"}`)
	var sent accounting.UpwardReport
	decode(t, rec, &sent)
	if rec.Code != http.StatusCreated || sent.SentBy != "clerk-1" || sent.Planned != 1000 || len(sent.Lines) != 1 {
		t.Fatalf("send upward report: got %d, %+v", rec.Code, sent)
	}
	rec = serve(handlers, http.MethodPost, "/upward-reports",
		`{"period_start":"`+januaryFirst+`","period_end":"`+februaryFirst+`","kind":"banana"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown report kind: got %d, want 400, body %s", rec.Code, rec.Body.String())
	}
	rec = serve(handlers, http.MethodGet, "/upward-reports", "")
	var history []accounting.UpwardReport
	decode(t, rec, &history)
	if rec.Code != http.StatusOK || len(history) != 1 {
		t.Fatalf("list upward reports: got %d, %+v", rec.Code, history)
	}
}

func TestATenderDrawsOnABudgetLineTakesQuotesAndBlocksDeletingTheLineOverHTTP(t *testing.T) {
	handlers := clerkRoutes(t)
	roads := openOverHTTP(t, handlers, "roads", "expense")
	rec := serve(handlers, http.MethodPut, "/budgets",
		`{"account_id":"`+roads.ID+`","period_start":"`+januaryFirst+`","period_end":"`+februaryFirst+`","name":"Road repair","department":"Public works","amount":1000}`)
	var line accounting.Budget
	decode(t, rec, &line)

	rec = serve(handlers, http.MethodPost, "/tenders", `{"budget_id":"`+line.ID+`","title":"Resurfacing of Mill Street","created_by":"forged"}`)
	var tender accounting.Tender
	decode(t, rec, &tender)
	if rec.Code != http.StatusCreated || tender.CreatedBy != "clerk-1" {
		t.Fatalf("open tender: got %d, %+v", rec.Code, tender)
	}
	if rec := serve(handlers, http.MethodPost, "/tenders", `{"budget_id":"missing","title":"x"}`); rec.Code != http.StatusNotFound {
		t.Fatalf("tender on a missing line: got %d, want 404", rec.Code)
	}

	for _, offer := range []string{`{"supplier":"Quill Roads","amount":900}`, `{"supplier":"Mason Works","amount":700}`} {
		if rec := serve(handlers, http.MethodPost, "/tenders/"+tender.ID+"/quotes", offer); rec.Code != http.StatusCreated {
			t.Fatalf("record quote: got %d, body %s", rec.Code, rec.Body.String())
		}
	}
	if rec := serve(handlers, http.MethodPost, "/tenders/"+tender.ID+"/quotes", `{"supplier":"Free Roads","amount":0}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("a quote of nothing: got %d, want 400", rec.Code)
	}

	rec = serve(handlers, http.MethodGet, "/tenders/"+tender.ID, "")
	var view accounting.TenderView
	decode(t, rec, &view)
	if rec.Code != http.StatusOK || view.QuoteCount != 2 || view.LowestAmount != 700 {
		t.Fatalf("read tender: got %d, %+v", rec.Code, view)
	}
	rec = serve(handlers, http.MethodGet, "/tenders/"+tender.ID+"/quotes", "")
	var quotes []accounting.Quote
	decode(t, rec, &quotes)
	if rec.Code != http.StatusOK || len(quotes) != 2 || quotes[0].Supplier != "Mason Works" {
		t.Fatalf("quotes lowest first: got %d, %+v", rec.Code, quotes)
	}
	rec = serve(handlers, http.MethodGet, "/tenders?budget_id="+line.ID, "")
	var tenders []accounting.TenderView
	decode(t, rec, &tenders)
	if rec.Code != http.StatusOK || len(tenders) != 1 {
		t.Fatalf("list tenders: got %d, %+v", rec.Code, tenders)
	}

	rec = serve(handlers, http.MethodPatch, "/tenders/"+tender.ID, `{"title":"Resurfacing of Mill and Fo Street"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("correct tender: got %d, body %s", rec.Code, rec.Body.String())
	}
	rec = serve(handlers, http.MethodPatch, "/quotes/"+quotes[0].ID, `{"supplier":"Mason Works","amount":650}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("correct quote: got %d, body %s", rec.Code, rec.Body.String())
	}

	if rec := serve(handlers, http.MethodDelete, "/budgets/"+line.ID, ""); rec.Code != http.StatusConflict {
		t.Fatalf("deleting a line a tender draws on: got %d, want 409, body %s", rec.Code, rec.Body.String())
	}
}

func TestTheEntriesOfOneNeighbourhoodAreListedOverHTTP(t *testing.T) {
	handlers := clerkRoutes(t)
	roads := openOverHTTP(t, handlers, "roads", "expense")
	cash := openOverHTTP(t, handlers, "cash", "asset")
	body := `{"debit_account_id":"` + roads.ID + `","credit_account_id":"` + cash.ID + `","amount":40,"occurred_at":"2026-01-10T09:00:00Z","document_kind":"invoice","document_id":"doc","actor_id":"actor-1","dimensions":{"neighbourhood":"Ovaros"}}`
	if rec := serve(handlers, http.MethodPost, "/entries", body); rec.Code != http.StatusCreated {
		t.Fatalf("post: got %d, body %s", rec.Code, rec.Body.String())
	}
	rec := serve(handlers, http.MethodGet, "/dimension-entries?key=neighbourhood&value=Ovaros&account_type=expense", "")
	var got accounting.DimensionEntries
	decode(t, rec, &got)
	if rec.Code != http.StatusOK || len(got.Entries) != 1 || got.Entries[0].Amount != 40 || got.Entries[0].Account != "roads" {
		t.Fatalf("dimension entries: got %d, %+v", rec.Code, got)
	}
	if rec := serve(handlers, http.MethodGet, "/dimension-entries?key=Neighbourhood&value=Ovaros", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("a key of the wrong shape: got %d, want 400", rec.Code)
	}
}
