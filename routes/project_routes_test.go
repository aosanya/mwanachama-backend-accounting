package routes_test

import (
	"net/http"
	"testing"

	accounting "github.com/aosanya/mwanachama-backend-accounting"
)

func TestBillsOfOneProjectAreRaisedCorrectedAndListedOverHTTP(t *testing.T) {
	handlers := clerkRoutes(t)
	income := openOverHTTP(t, handlers, "fees", "income")
	receivable := openOverHTTP(t, handlers, "receivable", "asset")
	raise := func(project string) accounting.Bill {
		body := `{"income_account_id":"` + income.ID + `","receivable_account_id":"` + receivable.ID + `","amount":4000,"due_at":"2030-01-01T00:00:00Z","payer":"Harbour Savings Union"`
		if project != "" {
			body += `,"project":"` + project + `"`
		}
		rec := serve(handlers, http.MethodPost, "/bills", body+`}`)
		if rec.Code != http.StatusCreated {
			t.Fatalf("raise bill on %q: got %d, body %s", project, rec.Code, rec.Body.String())
		}
		var bill accounting.Bill
		decode(t, rec, &bill)
		return bill
	}
	reef := raise("reef_survey")
	raise("kelp_study")
	raise("")
	if reef.Project != "reef_survey" {
		t.Fatalf("the bill answers with its project: %+v", reef)
	}

	rec := serve(handlers, http.MethodGet, "/bills?project=reef_survey", "")
	var listed []accounting.Bill
	decode(t, rec, &listed)
	if rec.Code != http.StatusOK || len(listed) != 1 || listed[0].ID != reef.ID {
		t.Fatalf("list by project: got %d, %+v", rec.Code, listed)
	}
	rec = serve(handlers, http.MethodGet, "/bills", "")
	var all []accounting.Bill
	decode(t, rec, &all)
	if rec.Code != http.StatusOK || len(all) != 3 {
		t.Fatalf("list without a project: got %d, %d bills", rec.Code, len(all))
	}

	rec = serve(handlers, http.MethodPatch, "/bills/"+reef.ID, `{"amount":4000,"due_at":"2030-01-01T00:00:00Z","payer":"Harbour Savings Union","project":"coral_atlas"}`)
	var moved accounting.Bill
	decode(t, rec, &moved)
	if rec.Code != http.StatusOK || moved.Project != "coral_atlas" {
		t.Fatalf("correct the project: got %d, %+v", rec.Code, moved)
	}
}

func TestAReportCutForOneProjectIsSentAndListedByProjectOverHTTP(t *testing.T) {
	const donorRow = "5c0e8a17-3d92-4b6f-a1e4-9f7b2c6d8e03"
	handlers := clerkRoutes(t)
	reefCosts := openOverHTTP(t, handlers, "reef_costs", "expense")
	kelpCosts := openOverHTTP(t, handlers, "kelp_costs", "expense")
	for _, line := range []struct {
		account    accounting.Account
		department string
	}{{reefCosts, "reef_survey"}, {kelpCosts, "kelp_study"}} {
		rec := serve(handlers, http.MethodPut, "/budgets",
			`{"account_id":"`+line.account.ID+`","period_start":"`+januaryFirst+`","period_end":"`+februaryFirst+`","name":"Costs","department":"`+line.department+`","amount":1000,"create":true}`)
		if rec.Code != http.StatusOK && rec.Code != http.StatusCreated {
			t.Fatalf("set budget line: got %d, body %s", rec.Code, rec.Body.String())
		}
	}
	period := `"period_start":"` + januaryFirst + `","period_end":"` + februaryFirst + `","kind":"budget_and_actuals"`

	rec := serve(handlers, http.MethodPost, "/upward-reports",
		`{`+period+`,"project":"reef_survey","recipient_record_row_id":"`+donorRow+`","reference":"RF-11"}`)
	var sent accounting.UpwardReport
	decode(t, rec, &sent)
	if rec.Code != http.StatusCreated || sent.Project != "reef_survey" || sent.Planned != 1000 || len(sent.Lines) != 1 {
		t.Fatalf("send for a project: got %d, %+v", rec.Code, sent)
	}
	if rec := serve(handlers, http.MethodPost, "/upward-reports", `{`+period+`,"project":"coral_atlas"}`); rec.Code != http.StatusNotFound {
		t.Fatalf("a project with no budget line: got %d, want 404, body %s", rec.Code, rec.Body.String())
	}
	if rec := serve(handlers, http.MethodPost, "/upward-reports", `{`+period+`}`); rec.Code != http.StatusCreated {
		t.Fatalf("send for the whole period: got %d, body %s", rec.Code, rec.Body.String())
	}

	rec = serve(handlers, http.MethodGet, "/upward-reports?project=reef_survey", "")
	var listed []accounting.UpwardReport
	decode(t, rec, &listed)
	if rec.Code != http.StatusOK || len(listed) != 1 || listed[0].ID != sent.ID || listed[0].RecipientRecordRowID != donorRow {
		t.Fatalf("list by project: got %d, %+v", rec.Code, listed)
	}
}
