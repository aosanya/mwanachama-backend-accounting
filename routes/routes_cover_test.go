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

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

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
	if len(shape) != 17 {
		t.Fatalf("got %d shaped routes, want 17", len(shape))
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

func ledgerOnMemory(t *testing.T) accounting.LedgerRepository {
	t.Helper()
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
	return repo
}

func serve(list []routes.Route, method, path, body string) *httptest.ResponseRecorder {
	mux := http.NewServeMux()
	for _, r := range list {
		mux.HandleFunc(r.Pattern(""), r.Handler)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)
	return rec
}

func patternsOf(list []routes.Route) map[string]bool {
	out := map[string]bool{}
	for _, r := range list {
		out[r.Pattern("")] = true
	}
	return out
}

func TestShapeNamesEveryBuiltRouteAndCarriesNoHandler(t *testing.T) {
	repo := ledgerOnMemory(t)
	built, err := routes.Build(repo)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	shape := routes.Shape()
	if len(shape) == 0 || len(shape) != len(built) {
		t.Fatalf("shape %d, built %d", len(shape), len(built))
	}
	have := patternsOf(built)
	for _, r := range shape {
		if r.Method == "" || r.Path == "" || r.Action == "" || r.Handler != nil {
			t.Fatalf("shape route %+v", r)
		}
		if !have[r.Pattern("")] {
			t.Fatalf("shape names %q but nothing is built for it", r.Pattern(""))
		}
	}
	for i := 1; i < len(shape); i++ {
		if shape[i-1].Action > shape[i].Action && shape[i-1].Path == shape[i].Path {
			t.Fatalf("shape is not in a stable order at %d", i)
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

func TestEveryConstructorBuildsTheSameRoutes(t *testing.T) {
	repo := ledgerOnMemory(t)
	allow := func(context.Context, string) error { return nil }
	want := len(routes.Routes(repo))
	if want == 0 {
		t.Fatal("no routes")
	}
	if got := len(routes.RoutesWith(repo, allow)); got != want {
		t.Fatalf("RoutesWith %d, want %d", got, want)
	}
	if got := len(routes.RoutesFor(repo, routes.Mount{Authorize: allow})); got != want {
		t.Fatalf("RoutesFor %d, want %d", got, want)
	}
	with, err := routes.BuildWith(repo, allow)
	if err != nil || len(with) != want {
		t.Fatalf("BuildWith %d %v", len(with), err)
	}
}

func TestAnAuthorizerThatRefusesStopsTheRouteBeforeTheLedger(t *testing.T) {
	repo := ledgerOnMemory(t)
	var asked []string
	refuse := func(_ context.Context, action string) error {
		asked = append(asked, action)
		return errors.New("no")
	}
	rec := serve(routes.RoutesWith(repo, refuse), http.MethodGet, "/accounts", "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("refused list answered %d %s", rec.Code, rec.Body.String())
	}
	if len(asked) != 1 || asked[0] != "accounting.account.list" {
		t.Fatalf("asked %v", asked)
	}
	accounts, _ := repo.ListAccounts(context.Background(), 0)
	if len(accounts) != 0 {
		t.Fatalf("accounts %+v", accounts)
	}
}

func TestAnAccountIsOpenedAndReadBackOverHTTP(t *testing.T) {
	repo := ledgerOnMemory(t)
	list := routes.Routes(repo)
	opened := serve(list, http.MethodPost, "/accounts", `{"category":"cash","holder_id":"hub-1","type":"asset","code":"1000"}`)
	if opened.Code != http.StatusCreated {
		t.Fatalf("open answered %d %s", opened.Code, opened.Body.String())
	}
	var account accounting.Account
	if err := json.Unmarshal(opened.Body.Bytes(), &account); err != nil || account.ID == "" || account.Code != "1000" {
		t.Fatalf("account %+v %v", account, err)
	}
	read := serve(list, http.MethodGet, "/accounts/"+account.ID, "")
	if read.Code != http.StatusOK || !strings.Contains(read.Body.String(), account.ID) {
		t.Fatalf("read answered %d %s", read.Code, read.Body.String())
	}
	balance := serve(list, http.MethodGet, "/accounts/"+account.ID+"/balance", "")
	if balance.Code != http.StatusOK {
		t.Fatalf("balance answered %d %s", balance.Code, balance.Body.String())
	}
}

func TestRefusalsMapToTheirStatusCodes(t *testing.T) {
	repo := ledgerOnMemory(t)
	list := routes.Routes(repo)
	if rec := serve(list, http.MethodGet, "/accounts/missing", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("missing account answered %d %s", rec.Code, rec.Body.String())
	}
	if rec := serve(list, http.MethodGet, "/accounts/missing/balance", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("missing balance answered %d %s", rec.Code, rec.Body.String())
	}
	if rec := serve(list, http.MethodPost, "/entries", `{"debit_account_id":"a","credit_account_id":"a","amount":0,"document_kind":"x","document_id":"d","actor_id":"a"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad entry answered %d %s", rec.Code, rec.Body.String())
	}
}

func TestAnEntryPostedOverHTTPShowsInTheAccountsList(t *testing.T) {
	repo := ledgerOnMemory(t)
	ctx := context.Background()
	cash, _ := repo.OpenAccount(ctx, accounting.Account{Category: "cash", HolderID: "h", Type: accounting.AccountTypeAsset})
	sales, _ := repo.OpenAccount(ctx, accounting.Account{Category: "sales", HolderID: "h", Type: accounting.AccountTypeIncome})
	list := routes.Routes(repo)
	body := `{"debit_account_id":"` + cash.ID + `","credit_account_id":"` + sales.ID + `","amount":250,"document_kind":"invoice","document_id":"inv-1","actor_id":"a"}`
	posted := serve(list, http.MethodPost, "/entries", body)
	if posted.Code != http.StatusCreated {
		t.Fatalf("post answered %d %s", posted.Code, posted.Body.String())
	}
	entries := serve(list, http.MethodGet, "/accounts/"+cash.ID+"/entries", "")
	if entries.Code != http.StatusOK || !strings.Contains(entries.Body.String(), "inv-1") {
		t.Fatalf("entries answered %d %s", entries.Code, entries.Body.String())
	}
	if got, _ := repo.Balance(ctx, cash.ID); got != 250 {
		t.Fatalf("balance %d", got)
	}
}
