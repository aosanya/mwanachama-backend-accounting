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
	handlers, err := routes.BuildFor(repo, routes.Mount{Caller: func(context.Context) string { return "lead-9" }})
	if err != nil {
		t.Fatalf("BuildFor: %v", err)
	}

	rec := serve(handlers, http.MethodPost, "/day-allowances",
		`{"project":"p1","consultant":"Tomas Quill","position":"Trainer","days_bought":6000,"rate":500}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("add allowance: got %d, body %s", rec.Code, rec.Body.String())
	}
	var allowance accounting.DayAllowance
	decode(t, rec, &allowance)
	if allowance.SetBy != "lead-9" {
		t.Fatalf("the allowance is set by the mount's caller: %+v", allowance)
	}

	rec = serve(handlers, http.MethodPost, "/day-entries",
		`{"project":"p1","consultant":"Tomas Quill","worked_on":"2026-06-30","days":6200,"submitted_by":"forged"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("submit entry: got %d, body %s", rec.Code, rec.Body.String())
	}
	var entry accounting.DayEntry
	decode(t, rec, &entry)
	if entry.SubmittedBy != "lead-9" {
		t.Fatalf("the submitter comes from the mount, not the body: %+v", entry)
	}

	rec = serve(handlers, http.MethodPost, "/day-entries/"+entry.ID+"/approve", "")
	if rec.Code != http.StatusConflict {
		t.Fatalf("approving past the days bought: got %d, want 409, body %s", rec.Code, rec.Body.String())
	}

	rec = serve(handlers, http.MethodPost, "/day-allowances/"+allowance.ID+"/replace",
		`{"days_bought":6500,"rate":500,"reason":"Five more days","project":"ignored","consultant":"ignored"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("replace allowance: got %d, body %s", rec.Code, rec.Body.String())
	}

	rec = serve(handlers, http.MethodPost, "/day-entries/"+entry.ID+"/approve", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("approving after the raise: got %d, body %s", rec.Code, rec.Body.String())
	}
	var approved accounting.DayEntry
	decode(t, rec, &approved)
	if approved.Status != accounting.DayEntryApproved || approved.DecidedBy != "lead-9" {
		t.Fatalf("approved by the mount's caller: %+v", approved)
	}

	rec = serve(handlers, http.MethodPost, "/day-entries/"+entry.ID+"/reject", "")
	if rec.Code != http.StatusConflict {
		t.Fatalf("deciding twice: got %d, want 409", rec.Code)
	}

	rec = serve(handlers, http.MethodGet, "/projects/p1/days-balance", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("days balance: got %d, body %s", rec.Code, rec.Body.String())
	}
	var balance accounting.DaysBalance
	decode(t, rec, &balance)
	if balance.Total.Bought != 6500 || balance.Total.Approved != 6200 || balance.Total.Left != 300 {
		t.Fatalf("balance over HTTP: %+v", balance.Total)
	}
}
