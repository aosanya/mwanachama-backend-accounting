package routes_test

import (
	"encoding/json"
	"net/http"
	"net/url"
	"reflect"
	"testing"

	accounting "github.com/aosanya/mwanachama-backend-accounting"
)

type totalRow struct {
	Value   string `json:"value"`
	Total   int64  `json:"total"`
	Entries int    `json:"entries"`
}

type totalsBody struct {
	Key         string     `json:"key"`
	AccountType string     `json:"account_type"`
	PeriodStart string     `json:"period_start"`
	PeriodEnd   string     `json:"period_end"`
	Totals      []totalRow `json:"totals"`
}

type townOverTheWire struct {
	m      *http.ServeMux
	wages  accounting.Account
	repair accounting.Account
	cash   accounting.Account
}

func (w townOverTheWire) spend(t *testing.T, from accounting.Account, amount int64, occurredAt, area string) {
	t.Helper()
	body := map[string]any{
		"debit_account_id": from.ID, "credit_account_id": w.cash.ID, "amount": amount, "occurred_at": occurredAt,
		"document_kind": "invoice", "document_id": "doc", "actor_id": "actor-1",
	}
	if area != "" {
		body["dimensions"] = map[string]string{"neighbourhood": area}
	}
	if rec := doJSON(t, w.m, "POST", "/entries", body); rec.Code != http.StatusCreated {
		t.Fatalf("post: got %d, body %s", rec.Code, rec.Body.String())
	}
}

func zoldhalomOverTheWire(t *testing.T) townOverTheWire {
	t.Helper()
	m := mux(newRepo(t))
	w := townOverTheWire{
		m:      m,
		wages:  openOver(t, m, accounting.Account{Category: "wages", Type: accounting.AccountTypeExpense, HolderID: "zoldhalom"}),
		repair: openOver(t, m, accounting.Account{Category: "repairs", Type: accounting.AccountTypeExpense, HolderID: "zoldhalom"}),
		cash:   openOver(t, m, accounting.Account{Category: "cash", Type: accounting.AccountTypeAsset, HolderID: "zoldhalom"}),
	}
	w.spend(t, w.wages, 100, "2026-01-10T09:00:00Z", "Óváros")
	w.spend(t, w.repair, 50, "2026-01-20T09:00:00Z", "Óváros")
	w.spend(t, w.wages, 200, "2026-01-01T00:00:00Z", "Kertváros")
	w.spend(t, w.repair, 70, "2026-02-01T00:00:00Z", "Malomsor")
	w.spend(t, w.wages, 999, "2026-01-12T09:00:00Z", "")
	return w
}

func totalsOver(t *testing.T, m *http.ServeMux, query url.Values) totalsBody {
	t.Helper()
	rec := doJSON(t, m, "GET", "/dimension-totals?"+query.Encode(), nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("dimension totals: got %d, want 200, body %s", rec.Code, rec.Body.String())
	}
	var out totalsBody
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode totals: %v", err)
	}
	return out
}

func TestDimensionTotalsOverTheWireGivesTheSpendPerNeighbourhood(t *testing.T) {
	w := zoldhalomOverTheWire(t)

	got := totalsOver(t, w.m, url.Values{"key": {"neighbourhood"}, "account_type": {"expense"}})

	want := []totalRow{
		{Value: "Kertváros", Total: 200, Entries: 1},
		{Value: "Malomsor", Total: 70, Entries: 1},
		{Value: "Óváros", Total: 150, Entries: 2},
	}
	if got.Key != "neighbourhood" || got.AccountType != "expense" {
		t.Fatalf("totals = %+v, want the key and account type echoed", got)
	}
	if !reflect.DeepEqual(got.Totals, want) {
		t.Fatalf("totals = %+v, want %+v", got.Totals, want)
	}
}

func TestDimensionTotalsOverTheWireNarrowsToThePeriod(t *testing.T) {
	w := zoldhalomOverTheWire(t)

	got := totalsOver(t, w.m, url.Values{
		"key": {"neighbourhood"}, "account_type": {"expense"},
		"period_start": {"2026-01-01T00:00:00Z"}, "period_end": {"2026-02-01T00:00:00Z"},
	})

	want := []totalRow{
		{Value: "Kertváros", Total: 200, Entries: 1},
		{Value: "Óváros", Total: 150, Entries: 2},
	}
	if got.PeriodStart != "2026-01-01T00:00:00Z" || got.PeriodEnd != "2026-02-01T00:00:00Z" {
		t.Fatalf("totals = %+v, want the period echoed", got)
	}
	if !reflect.DeepEqual(got.Totals, want) {
		t.Fatalf("totals = %+v, want %+v", got.Totals, want)
	}
}

func TestDimensionTotalsOverTheWireRefusesABadQueryWith400(t *testing.T) {
	w := zoldhalomOverTheWire(t)

	for name, query := range map[string]url.Values{
		"missing key":          {"account_type": {"expense"}},
		"unknown account type": {"key": {"neighbourhood"}, "account_type": {"bogus"}},
		"only a period start":  {"key": {"neighbourhood"}, "period_start": {"2026-01-01T00:00:00Z"}},
		"inverted period":      {"key": {"neighbourhood"}, "period_start": {"2026-02-01T00:00:00Z"}, "period_end": {"2026-01-01T00:00:00Z"}},
	} {
		rec := doJSON(t, w.m, "GET", "/dimension-totals?"+query.Encode(), nil)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s: got %d, want 400, body %s", name, rec.Code, rec.Body.String())
		}
	}
}
