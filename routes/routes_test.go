package routes_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	accounting "github.com/aosanya/mwanachama-backend-accounting"
	"github.com/aosanya/mwanachama-backend-accounting/routes"
)

func newRepo(t *testing.T) accounting.LedgerRepository {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	s, err := accounting.SpecFor("mwanachama")
	if err != nil {
		t.Fatalf("spec: %v", err)
	}
	if err := accounting.Provision(db, s); err != nil {
		t.Fatalf("provision: %v", err)
	}
	repo, err := accounting.NewLedgerRepository(db, s)
	if err != nil {
		t.Fatalf("new ledger: %v", err)
	}
	return repo
}

func patterns(rts []routes.Route, prefix string) []string {
	out := make([]string, len(rts))
	for i, rt := range rts {
		out[i] = rt.Pattern(prefix)
	}
	sort.Strings(out)
	return out
}

// The address set is what the conversion must not move: the gateway's
// Postman gate reads addresses, so a route that changed shape would show up
// there as a coverage change rather than as a test failure here.
func TestRoutesAnswerTheSameAddressesAsBeforeTheConversion(t *testing.T) {
	want := []string{
		"GET /accounts",
		"GET /accounts/{accountID}",
		"GET /accounts/{accountID}/balance",
		"GET /accounts/{accountID}/entries",
		"POST /accounts",
		"POST /accounts/{accountID}/close",
		"POST /entries",
	}
	got := patterns(routes.Routes(newRepo(t)), "")
	if len(got) != len(want) {
		t.Fatalf("got %d routes, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("route %d: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestEveryRouteArrivesGated(t *testing.T) {
	for _, rt := range routes.Routes(newRepo(t)) {
		if rt.Action == "" {
			t.Errorf("%s carries no action, so it would mount ungated", rt.Pattern(""))
		}
	}
}

func TestRoutesCarryNoDuplicateAddress(t *testing.T) {
	seen := map[string]bool{}
	for _, p := range patterns(routes.Routes(newRepo(t)), "") {
		if seen[p] {
			t.Errorf("duplicate route pattern: %s", p)
		}
		seen[p] = true
	}
}

func TestRoutePatternTakesTheMountPrefix(t *testing.T) {
	for _, rt := range routes.Routes(newRepo(t)) {
		if rt.Method == http.MethodPost && rt.Path == "/accounts" {
			if got := rt.Pattern("/v1/accounting"); got != "POST /v1/accounting/accounts" {
				t.Fatalf("got %q", got)
			}
			return
		}
	}
	t.Fatal("POST /accounts is not in the route table")
}

func mux(repo accounting.LedgerRepository) *http.ServeMux {
	m := http.NewServeMux()
	for _, rt := range routes.Routes(repo) {
		m.HandleFunc(rt.Pattern(""), rt.Handler)
	}
	return m
}

func doJSON(t *testing.T, m *http.ServeMux, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode request body: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	rec := httptest.NewRecorder()
	m.ServeHTTP(rec, req)
	return rec
}

func TestEndToEndOpenPostBalanceClose(t *testing.T) {
	m := mux(newRepo(t))

	openRec := doJSON(t, m, "POST", "/accounts", accounting.Account{
		Category: "cash", Type: accounting.AccountTypeAsset, HolderID: "member-1",
	})
	if openRec.Code != http.StatusCreated {
		t.Fatalf("open cash account: got %d, body %s", openRec.Code, openRec.Body.String())
	}
	var cash accounting.Account
	if err := json.Unmarshal(openRec.Body.Bytes(), &cash); err != nil {
		t.Fatalf("decode cash account: %v", err)
	}
	if cash.ID == "" || cash.HolderID != "member-1" {
		t.Fatalf("unexpected cash account: %+v", cash)
	}

	incomeRec := doJSON(t, m, "POST", "/accounts", accounting.Account{
		Category: "revenue", Type: accounting.AccountTypeIncome, HolderID: "org",
	})
	if incomeRec.Code != http.StatusCreated {
		t.Fatalf("open income account: got %d, body %s", incomeRec.Code, incomeRec.Body.String())
	}
	var income accounting.Account
	if err := json.Unmarshal(incomeRec.Body.Bytes(), &income); err != nil {
		t.Fatalf("decode income account: %v", err)
	}

	postRec := doJSON(t, m, "POST", "/entries", accounting.Entry{
		DebitAccountID: cash.ID, CreditAccountID: income.ID, Amount: 500,
		DocumentKind: "test-doc", DocumentID: "doc-1", ActorID: "actor-1",
	})
	if postRec.Code != http.StatusCreated {
		t.Fatalf("post entry: got %d, body %s", postRec.Code, postRec.Body.String())
	}

	balRec := doJSON(t, m, "GET", "/accounts/"+cash.ID+"/balance", nil)
	if balRec.Code != http.StatusOK {
		t.Fatalf("get balance: got %d, body %s", balRec.Code, balRec.Body.String())
	}
	var balOut struct {
		Balance int64 `json:"balance"`
	}
	if err := json.Unmarshal(balRec.Body.Bytes(), &balOut); err != nil {
		t.Fatalf("decode balance: %v", err)
	}
	if balOut.Balance != 500 {
		t.Fatalf("got balance %d, want 500", balOut.Balance)
	}

	entriesRec := doJSON(t, m, "GET", "/accounts/"+cash.ID+"/entries", nil)
	if entriesRec.Code != http.StatusOK {
		t.Fatalf("list entries: got %d, body %s", entriesRec.Code, entriesRec.Body.String())
	}
	var entries []accounting.Entry
	if err := json.Unmarshal(entriesRec.Body.Bytes(), &entries); err != nil {
		t.Fatalf("decode entries: %v", err)
	}
	if len(entries) != 1 || entries[0].Amount != 500 {
		t.Fatalf("got entries %+v, want one entry of amount 500", entries)
	}

	closeRec := doJSON(t, m, "POST", "/accounts/"+cash.ID+"/close", nil)
	if closeRec.Code != http.StatusConflict {
		t.Fatalf("close non-zero-balance account: got %d, want %d, body %s",
			closeRec.Code, http.StatusConflict, closeRec.Body.String())
	}

	notFoundRec := doJSON(t, m, "GET", "/accounts/does-not-exist", nil)
	if notFoundRec.Code != http.StatusNotFound {
		t.Fatalf("get unknown account: got %d, want %d", notFoundRec.Code, http.StatusNotFound)
	}
}

func TestDoubleReversalOverHTTPAnswersConflict(t *testing.T) {
	m := mux(newRepo(t))

	open := func(category string, accountType accounting.AccountType, holder string) accounting.Account {
		t.Helper()
		rec := doJSON(t, m, "POST", "/accounts", accounting.Account{
			Category: accounting.AccountCategory(category), Type: accountType, HolderID: holder,
		})
		if rec.Code != http.StatusCreated {
			t.Fatalf("open %s: got %d, body %s", category, rec.Code, rec.Body.String())
		}
		var a accounting.Account
		if err := json.Unmarshal(rec.Body.Bytes(), &a); err != nil {
			t.Fatalf("decode %s: %v", category, err)
		}
		return a
	}

	cash := open("cash", accounting.AccountTypeAsset, "member-1")
	income := open("revenue", accounting.AccountTypeIncome, "org")

	postRec := doJSON(t, m, "POST", "/entries", accounting.Entry{
		DebitAccountID: cash.ID, CreditAccountID: income.ID, Amount: 500,
		DocumentKind: "test-doc", DocumentID: "doc-1", ActorID: "actor-1",
	})
	if postRec.Code != http.StatusCreated {
		t.Fatalf("post entry: got %d, body %s", postRec.Code, postRec.Body.String())
	}
	var original accounting.Entry
	if err := json.Unmarshal(postRec.Body.Bytes(), &original); err != nil {
		t.Fatalf("decode entry: %v", err)
	}

	reversal := accounting.Entry{
		DebitAccountID: income.ID, CreditAccountID: cash.ID, Amount: 500,
		DocumentKind: "correction", DocumentID: "doc-1", ActorID: "actor-1",
		ReversesEntryID: original.ID,
	}
	if rec := doJSON(t, m, "POST", "/entries", reversal); rec.Code != http.StatusCreated {
		t.Fatalf("first reversal: got %d, body %s", rec.Code, rec.Body.String())
	}
	if rec := doJSON(t, m, "POST", "/entries", reversal); rec.Code != http.StatusConflict {
		t.Fatalf("second reversal of the same entry: got %d, want %d, body %s",
			rec.Code, http.StatusConflict, rec.Body.String())
	}
}
