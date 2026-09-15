// account.go — HTTP routes over accounting.LedgerRepository's Account
// operations: OpenAccount, GetAccount, ListAccounts, CloseAccount, and the
// derived Balance read. See doc.go.
package routes

import (
	"net/http"
	"strconv"

	"github.com/aosanya/mwanachama-backend-accounting"
)

// AccountRoutes returns the plain Account CRUD plus the balance read.
func AccountRoutes(repo accounting.LedgerRepository) []Route {
	return []Route{
		{Method: "POST", Path: "/accounts", Handler: OpenAccount(repo)},
		{Method: "GET", Path: "/accounts", Handler: ListAccounts(repo)},
		{Method: "GET", Path: "/accounts/{accountID}", Handler: GetAccount(repo)},
		{Method: "POST", Path: "/accounts/{accountID}/close", Handler: CloseAccount(repo)},
		{Method: "GET", Path: "/accounts/{accountID}/balance", Handler: GetAccountBalance(repo)},
	}
}

// OpenAccount handles POST /accounts — decode, open (lazy-create on
// (Kind, HolderID)), encode.
func OpenAccount(repo accounting.LedgerRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var a accounting.Account
		if err := readJSON(r, &a); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid request body")
			return
		}
		out, err := repo.OpenAccount(r.Context(), a)
		if err != nil {
			writeLedgerErr(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, out)
	}
}

// GetAccount handles GET /accounts/{accountID}.
func GetAccount(repo accounting.LedgerRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		out, err := repo.GetAccount(r.Context(), r.PathValue("accountID"))
		if err != nil {
			writeLedgerErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	}
}

// ListAccounts handles GET /accounts?limit=.
func ListAccounts(repo accounting.LedgerRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit := 0
		if l := r.URL.Query().Get("limit"); l != "" {
			if n, err := strconv.Atoi(l); err == nil {
				limit = n
			}
		}
		out, err := repo.ListAccounts(r.Context(), limit)
		if err != nil {
			writeLedgerErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	}
}

// CloseAccount handles POST /accounts/{accountID}/close.
func CloseAccount(repo accounting.LedgerRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		out, err := repo.CloseAccount(r.Context(), r.PathValue("accountID"))
		if err != nil {
			writeLedgerErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	}
}

// GetAccountBalance handles GET /accounts/{accountID}/balance — there is no
// stored balance anywhere in the root package; this folds one on every
// call, matching LedgerRepository.Balance's own contract.
func GetAccountBalance(repo accounting.LedgerRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		bal, err := repo.Balance(r.Context(), r.PathValue("accountID"))
		if err != nil {
			writeLedgerErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]int64{"balance": bal})
	}
}
