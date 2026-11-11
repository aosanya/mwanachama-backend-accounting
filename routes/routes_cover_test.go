package routes_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	accounting "github.com/aosanya/mwanachama-backend-accounting"
	"github.com/aosanya/mwanachama-backend-accounting/routes"
)

func muxFor(t *testing.T, rts []routes.Route) *http.ServeMux {
	t.Helper()
	m := http.NewServeMux()
	for _, rt := range rts {
		m.HandleFunc(rt.Pattern(""), rt.Handler)
	}
	return m
}

func TestShapeListsEveryDeclaredAddressWithNoHandler(t *testing.T) {
	shape := routes.Shape()
	if len(shape) != 12 {
		t.Fatalf("got %d shaped routes, want 12", len(shape))
	}
	for _, rt := range shape {
		if rt.Handler != nil {
			t.Errorf("%s carries a handler, but a shape is the table alone", rt.Pattern(""))
		}
		if rt.Action == "" || !strings.HasPrefix(rt.Action, "accounting.") {
			t.Errorf("%s carries action %q", rt.Pattern(""), rt.Action)
		}
	}
}

func TestShapeIsOrderedByOperationNameAndRepeatable(t *testing.T) {
	first, second := routes.Shape(), routes.Shape()
	for i := range first {
		if first[i].Pattern("") != second[i].Pattern("") {
			t.Fatalf("shape order changed between calls at %d", i)
		}
	}
	if first[0].Action != "accounting.account.balance" {
		t.Fatalf("first shaped route is %q, want the operation sorting first by name", first[0].Action)
	}
}

func TestShapeAddressesMatchTheBuiltRoutes(t *testing.T) {
	built := patterns(routes.Routes(newRepo(t)), "")
	shaped := patterns(routes.Shape(), "")
	if len(built) != len(shaped) {
		t.Fatalf("built %d routes, shaped %d", len(built), len(shaped))
	}
	for i := range built {
		if built[i] != shaped[i] {
			t.Fatalf("address %d: built %q, shaped %q", i, built[i], shaped[i])
		}
	}
}

func TestBuildAnswersEveryDeclaredActionExactlyOnce(t *testing.T) {
	rts, err := routes.Build(newRepo(t))
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	actions := make([]string, 0, len(rts))
	for _, rt := range rts {
		actions = append(actions, rt.Action)
	}
	sort.Strings(actions)
	for i := 1; i < len(actions); i++ {
		if actions[i] == actions[i-1] {
			t.Fatalf("action %q is declared twice", actions[i])
		}
	}
	if len(actions) != len(routes.Shape()) {
		t.Fatalf("built %d routes, shape declares %d", len(actions), len(routes.Shape()))
	}
}

func TestAnAuthorizerThatRefusesStopsTheRequestBeforeTheLedger(t *testing.T) {
	refuse := func(ctx context.Context, action string) error { return errors.New("no") }
	rts, err := routes.BuildWith(newRepo(t), refuse)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	m := muxFor(t, rts)
	rec := doJSON(t, m, "POST", "/accounts", accounting.Account{
		Category: "cash", Type: accounting.AccountTypeAsset, HolderID: "h1",
	})
	if rec.Code != http.StatusForbidden && rec.Code != http.StatusUnauthorized {
		t.Fatalf("refused open: got %d, body %s", rec.Code, rec.Body.String())
	}
	list := doJSON(t, muxFor(t, routes.RoutesWith(newRepo(t), nil)), "GET", "/accounts", nil)
	if list.Code != http.StatusOK || strings.Contains(list.Body.String(), "h1") {
		t.Fatalf("refused account must not exist: %d %s", list.Code, list.Body.String())
	}
}

func TestAnAuthorizerIsAskedForTheDeclaredActionOfTheAddress(t *testing.T) {
	var asked []string
	record := func(ctx context.Context, action string) error {
		asked = append(asked, action)
		return nil
	}
	m := muxFor(t, routes.RoutesWith(newRepo(t), record))
	doJSON(t, m, "GET", "/accounts", nil)
	if len(asked) != 1 || asked[0] != "accounting.account.list" {
		t.Fatalf("authorizer was asked %v, want [accounting.account.list]", asked)
	}
}

func TestRoutesForPassesTheMountsAuthorizerThrough(t *testing.T) {
	denied := 0
	m := muxFor(t, routes.RoutesFor(newRepo(t), routes.Mount{
		Authorize: func(ctx context.Context, action string) error { denied++; return errors.New("no") },
	}))
	rec := doJSON(t, m, "GET", "/accounts", nil)
	if denied != 1 || rec.Code == http.StatusOK {
		t.Fatalf("denied %d times, status %d", denied, rec.Code)
	}
}

func TestOpeningAnAccountWithAnUnknownTypeIsRefusedAsBadRequest(t *testing.T) {
	m := mux(newRepo(t))
	rec := doJSON(t, m, "POST", "/accounts", map[string]any{
		"category": "cash", "type": "banana", "holder_id": "h1",
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("got %d, want 400, body %s", rec.Code, rec.Body.String())
	}
}

func TestOpeningTwoAccountsWithTheSameCodeAnswersConflict(t *testing.T) {
	m := mux(newRepo(t))
	open := func(holder string) int {
		return doJSON(t, m, "POST", "/accounts", accounting.Account{
			Category: "cash", Type: accounting.AccountTypeAsset, HolderID: holder, Code: "1000",
		}).Code
	}
	if code := open("h1"); code != http.StatusCreated {
		t.Fatalf("first open: %d", code)
	}
	if code := open("h2"); code != http.StatusConflict {
		t.Fatalf("second open with the same code: got %d, want 409", code)
	}
}

func TestPostingAnEntryBetweenUnknownAccountsIsRefused(t *testing.T) {
	m := mux(newRepo(t))
	rec := doJSON(t, m, "POST", "/entries", accounting.Entry{
		DebitAccountID: "nope", CreditAccountID: "nada", Amount: 5,
		DocumentKind: "d", DocumentID: "1", ActorID: "a",
	})
	if rec.Code != http.StatusNotFound && rec.Code != http.StatusBadRequest {
		t.Fatalf("got %d, body %s", rec.Code, rec.Body.String())
	}
}

func TestPostingToAClosedAccountAnswersConflict(t *testing.T) {
	m := mux(newRepo(t))
	open := func(cat string, ty accounting.AccountType, h string) accounting.Account {
		var a accounting.Account
		rec := doJSON(t, m, "POST", "/accounts", accounting.Account{Category: accounting.AccountCategory(cat), Type: ty, HolderID: h})
		if rec.Code != http.StatusCreated {
			t.Fatalf("open: %d %s", rec.Code, rec.Body.String())
		}
		decode(t, rec, &a)
		return a
	}
	cash := open("cash", accounting.AccountTypeAsset, "h1")
	income := open("revenue", accounting.AccountTypeIncome, "org")
	if rec := doJSON(t, m, "POST", "/accounts/"+cash.ID+"/close", nil); rec.Code != http.StatusOK && rec.Code != http.StatusNoContent {
		t.Fatalf("close empty account: %d %s", rec.Code, rec.Body.String())
	}
	rec := doJSON(t, m, "POST", "/entries", accounting.Entry{
		DebitAccountID: cash.ID, CreditAccountID: income.ID, Amount: 5,
		DocumentKind: "d", DocumentID: "1", ActorID: "a",
	})
	if rec.Code != http.StatusConflict {
		t.Fatalf("posting to a closed account: got %d, want 409, body %s", rec.Code, rec.Body.String())
	}
}

func decode(t *testing.T, rec *httptest.ResponseRecorder, v any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), v); err != nil {
		t.Fatalf("decode %s: %v", rec.Body.String(), err)
	}
}
