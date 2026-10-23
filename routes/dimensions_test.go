package routes_test

import (
	"encoding/json"
	"net/http"
	"testing"

	accounting "github.com/aosanya/mwanachama-backend-accounting"
)

func openOver(t *testing.T, m *http.ServeMux, a accounting.Account) accounting.Account {
	t.Helper()
	rec := doJSON(t, m, "POST", "/accounts", a)
	if rec.Code != http.StatusCreated {
		t.Fatalf("open: got %d, body %s", rec.Code, rec.Body.String())
	}
	var out accounting.Account
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode account: %v", err)
	}
	return out
}

func TestDimensionsTravelOverTheWireAndNarrowTheReads(t *testing.T) {
	m := mux(newRepo(t))
	spend := openOver(t, m, accounting.Account{Category: "spend", Type: accounting.AccountTypeExpense, HolderID: "org"})
	cash := openOver(t, m, accounting.Account{Category: "cash", Type: accounting.AccountTypeAsset, HolderID: "org"})

	for _, p := range []struct {
		amount int64
		area   string
	}{{100, "north"}, {250, "south"}, {40, "north"}} {
		rec := doJSON(t, m, "POST", "/entries", map[string]any{
			"debit_account_id": spend.ID, "credit_account_id": cash.ID, "amount": p.amount,
			"document_kind": "invoice", "document_id": "doc", "actor_id": "actor-1",
			"dimensions": map[string]string{"area": p.area},
		})
		if rec.Code != http.StatusCreated {
			t.Fatalf("post: got %d, body %s", rec.Code, rec.Body.String())
		}
	}

	bad := doJSON(t, m, "POST", "/entries", map[string]any{
		"debit_account_id": spend.ID, "credit_account_id": cash.ID, "amount": 1,
		"document_kind": "invoice", "document_id": "doc", "actor_id": "actor-1",
		"dimensions": map[string]string{"Not A Key": "x"},
	})
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("malformed dimension: got %d, want 400, body %s", bad.Code, bad.Body.String())
	}

	balRec := doJSON(t, m, "GET", "/accounts/"+spend.ID+"/balance/by-dimension?key=area&value=north", nil)
	if balRec.Code != http.StatusOK {
		t.Fatalf("balance by dimension: got %d, body %s", balRec.Code, balRec.Body.String())
	}
	var bal struct {
		Balance int64 `json:"balance"`
	}
	if err := json.Unmarshal(balRec.Body.Bytes(), &bal); err != nil || bal.Balance != 140 {
		t.Fatalf("balance = %+v, %v, want 140", bal, err)
	}

	listRec := doJSON(t, m, "GET", "/accounts/"+spend.ID+"/entries/by-dimension?key=area&value=south", nil)
	if listRec.Code != http.StatusOK {
		t.Fatalf("entries by dimension: got %d, body %s", listRec.Code, listRec.Body.String())
	}
	var entries []accounting.Entry
	if err := json.Unmarshal(listRec.Body.Bytes(), &entries); err != nil {
		t.Fatalf("decode entries: %v", err)
	}
	if len(entries) != 1 || entries[0].Amount != 250 || entries[0].Dimensions["area"] != "south" {
		t.Fatalf("entries = %+v, want the one south entry", entries)
	}

	if rec := doJSON(t, m, "GET", "/accounts/"+spend.ID+"/balance/by-dimension?value=north", nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("missing key: got %d, want 400", rec.Code)
	}
}
