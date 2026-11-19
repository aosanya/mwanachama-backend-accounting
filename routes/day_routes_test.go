package routes_test

import (
	"context"
	"net/http"
	"testing"

	accounting "github.com/aosanya/mwanachama-backend-accounting"
	"github.com/aosanya/mwanachama-backend-accounting/routes"
)

func TestDayRoutesTakeTheDecidingPersonFromTheMountAndAnswerConflictPastTheDaysBought(t *testing.T) {
	repo := ledgerOnMemory(t)
	submitters, err := routes.BuildFor(repo, routes.Mount{Caller: func(context.Context) string { return "lead-9" }})
	if err != nil {
		t.Fatalf("BuildFor: %v", err)
	}
	approvers, err := routes.BuildFor(repo, routes.Mount{Caller: func(context.Context) string { return "lead-8" }})
	if err != nil {
		t.Fatalf("BuildFor: %v", err)
	}

	rec := serve(submitters, http.MethodPost, "/day-allowances",
		`{"project":"p1","consultant":"Tomas Quill","position":"Trainer","days_bought":100,"rate":500}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("add allowance: got %d, body %s", rec.Code, rec.Body.String())
	}
	var allowance accounting.DayAllowance
	decode(t, rec, &allowance)
	if allowance.SetBy != "lead-9" {
		t.Fatalf("the allowance is set by the mount's caller: %+v", allowance)
	}

	rec = serve(submitters, http.MethodPost, "/day-entries",
		`{"project":"p1","consultant":"Tomas Quill","worked_on":"2026-06-29","days":100,"submitted_by":"forged"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("submit entry: got %d, body %s", rec.Code, rec.Body.String())
	}
	var first accounting.DayEntry
	decode(t, rec, &first)
	if first.SubmittedBy != "lead-9" {
		t.Fatalf("the submitter comes from the mount, not the body: %+v", first)
	}

	rec = serve(submitters, http.MethodPost, "/day-entries/"+first.ID+"/approve", "")
	if rec.Code != http.StatusConflict {
		t.Fatalf("approving one's own entry: got %d, want 409, body %s", rec.Code, rec.Body.String())
	}
	rec = serve(submitters, http.MethodPost, "/day-entries",
		`{"project":"p1","consultant":"tomas quill ","worked_on":"2026-06-29","days":1}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("a second entry on a full date: got %d, want 409, body %s", rec.Code, rec.Body.String())
	}
	rec = serve(submitters, http.MethodPost, "/day-entries",
		`{"project":"p1","consultant":"Tomas Quill","worked_on":"2026-06-30","days":2400}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("more than a day in one entry: got %d, want 400, body %s", rec.Code, rec.Body.String())
	}

	rec = serve(approvers, http.MethodPost, "/day-entries/"+first.ID+"/approve", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("approving the first day: got %d, body %s", rec.Code, rec.Body.String())
	}
	var approved accounting.DayEntry
	decode(t, rec, &approved)
	if approved.Status != accounting.DayEntryApproved || approved.DecidedBy != "lead-8" || approved.Rate != 500 {
		t.Fatalf("approved by the mount's caller at the allowance's rate: %+v", approved)
	}

	rec = serve(submitters, http.MethodPost, "/day-entries",
		`{"project":"p1","consultant":"Tomas Quill","worked_on":"2026-06-30","days":100}`)
	var second accounting.DayEntry
	decode(t, rec, &second)
	rec = serve(approvers, http.MethodPost, "/day-entries/"+second.ID+"/approve", "")
	if rec.Code != http.StatusConflict {
		t.Fatalf("approving past the days bought: got %d, want 409, body %s", rec.Code, rec.Body.String())
	}

	rec = serve(submitters, http.MethodPost, "/day-allowances/"+allowance.ID+"/replace",
		`{"days_bought":50,"rate":500,"reason":"Cut","project":"ignored","consultant":"ignored"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("a replacement below the approved days: got %d, want 409, body %s", rec.Code, rec.Body.String())
	}
	rec = serve(submitters, http.MethodPost, "/day-allowances/"+allowance.ID+"/replace",
		`{"days_bought":200,"rate":900,"reason":"One more day at a new rate","project":"ignored","consultant":"ignored"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("replace allowance: got %d, body %s", rec.Code, rec.Body.String())
	}

	rec = serve(approvers, http.MethodPost, "/day-entries/"+second.ID+"/approve", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("approving after the raise: got %d, body %s", rec.Code, rec.Body.String())
	}

	rec = serve(approvers, http.MethodPost, "/day-entries/"+second.ID+"/reject", "")
	if rec.Code != http.StatusConflict {
		t.Fatalf("deciding twice: got %d, want 409", rec.Code)
	}

	rec = serve(approvers, http.MethodGet, "/projects/p1/days-balance", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("days balance: got %d, body %s", rec.Code, rec.Body.String())
	}
	var balance accounting.DaysBalance
	decode(t, rec, &balance)
	if balance.Total.Bought != 200 || balance.Total.Approved != 200 || balance.Total.Left != 0 || balance.Total.Fee != 1400 {
		t.Fatalf("balance over HTTP, one day at 500 and one at 900: %+v", balance.Total)
	}
}
