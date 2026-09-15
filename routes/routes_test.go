package routes_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aosanya/mwanachama-backend-accounting"
	"github.com/aosanya/mwanachama-backend-accounting/routes"
)

func patterns(rts []routes.Route, prefix string) []string {
	out := make([]string, len(rts))
	for i, rt := range rts {
		out[i] = rt.Pattern(prefix)
	}
	return out
}

func assertPatterns(t *testing.T, got []routes.Route, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d routes, want %d: %v", len(got), len(want), patterns(got, ""))
	}
	for i, p := range patterns(got, "") {
		if p != want[i] {
			t.Fatalf("route %d: got %q, want %q", i, p, want[i])
		}
	}
}

func TestAccountRoutes(t *testing.T) {
	repo := accounting.NewMemoryRepository()
	rts := routes.AccountRoutes(repo)
	assertPatterns(t, rts, []string{
		"POST /accounts",
		"GET /accounts",
		"GET /accounts/{accountID}",
		"POST /accounts/{accountID}/close",
		"GET /accounts/{accountID}/balance",
	})
}

func TestEntryRoutes(t *testing.T) {
	repo := accounting.NewMemoryRepository()
	rts := routes.EntryRoutes(repo)
	assertPatterns(t, rts, []string{
		"POST /entries",
		"GET /accounts/{accountID}/entries",
	})
}

func TestRoutes_ConcatenatesBoth(t *testing.T) {
	repo := accounting.NewMemoryRepository()
	all := routes.Routes(repo)
	want := 5 + 2 // AccountRoutes + EntryRoutes
	if len(all) != want {
		t.Fatalf("got %d routes, want %d: %v", len(all), want, patterns(all, ""))
	}
}

func TestRoutes_NoDuplicatePatterns(t *testing.T) {
	repo := accounting.NewMemoryRepository()
	seen := make(map[string]bool)
	for _, p := range patterns(routes.Routes(repo), "") {
		if seen[p] {
			t.Errorf("duplicate route pattern: %s", p)
		}
		seen[p] = true
	}
}

func TestRoute_PatternWithPrefix(t *testing.T) {
	repo := accounting.NewMemoryRepository()
	rts := routes.AccountRoutes(repo)
	if got := rts[0].Pattern("/v1/accounting"); got != "POST /v1/accounting/accounts" {
		t.Fatalf("got %q", got)
	}
}

// mux builds a real net/http.ServeMux from routes.Routes, the same way a
// mounting process would — proves PathValue extraction and JSON wire shape
// work against an actual HTTP request, not just a direct handler call (see
// mwanachama-backend-comm's CLAUDE.md for why a direct-call-only fixture
// missed real bugs elsewhere in this family).
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

func TestEndToEnd_OpenPostBalanceClose(t *testing.T) {
	repo := accounting.NewMemoryRepository()
	m := mux(repo)

	openRec := doJSON(t, m, "POST", "/accounts", accounting.Account{
		Kind: "cash", Type: accounting.AccountTypeAsset, HolderID: "member-1",
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
		Kind: "revenue", Type: accounting.AccountTypeIncome, HolderID: "org",
	})
	if incomeRec.Code != http.StatusCreated {
		t.Fatalf("open income account: got %d, body %s", incomeRec.Code, incomeRec.Body.String())
	}
	var income accounting.Account
	if err := json.Unmarshal(incomeRec.Body.Bytes(), &income); err != nil {
		t.Fatalf("decode income account: %v", err)
	}

	postRec := doJSON(t, m, "POST", "/entries", accounting.Entry{
		FromAccountID: income.ID, ToAccountID: cash.ID, Amount: 500,
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

	// A non-zero-balance account refuses to close.
	closeRec := doJSON(t, m, "POST", "/accounts/"+cash.ID+"/close", nil)
	if closeRec.Code != http.StatusConflict {
		t.Fatalf("close non-zero-balance account: got %d, want %d, body %s", closeRec.Code, http.StatusConflict, closeRec.Body.String())
	}

	notFoundRec := doJSON(t, m, "GET", "/accounts/does-not-exist", nil)
	if notFoundRec.Code != http.StatusNotFound {
		t.Fatalf("get unknown account: got %d, want %d", notFoundRec.Code, http.StatusNotFound)
	}
}
