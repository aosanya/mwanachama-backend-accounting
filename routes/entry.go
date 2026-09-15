// entry.go — HTTP routes over accounting.LedgerRepository's Entry
// operations: PostEntry and ListEntries. See doc.go.
package routes

import (
	"net/http"
	"strconv"

	"github.com/aosanya/mwanachama-backend-accounting"
)

// EntryRoutes returns the append-only Entry posting and read routes. There
// is no update or delete route — LedgerRepository has neither, on purpose
// (see ledger_repository.go's doc comment): a mistake is corrected by a new
// entry with ReversesEntryID set, never by touching the original.
func EntryRoutes(repo accounting.LedgerRepository) []Route {
	return []Route{
		{Method: "POST", Path: "/entries", Handler: PostEntry(repo)},
		{Method: "GET", Path: "/accounts/{accountID}/entries", Handler: ListEntries(repo)},
	}
}

// PostEntry handles POST /entries — decode, post, encode. A correction sets
// reverses_entry_id in the request body; the original entry is never
// touched.
func PostEntry(repo accounting.LedgerRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var e accounting.Entry
		if err := readJSON(r, &e); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid request body")
			return
		}
		out, err := repo.Post(r.Context(), e)
		if err != nil {
			writeLedgerErr(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, out)
	}
}

// ListEntries handles GET /accounts/{accountID}/entries?limit= — one
// account's ledger, both sides, oldest first.
func ListEntries(repo accounting.LedgerRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit := 0
		if l := r.URL.Query().Get("limit"); l != "" {
			if n, err := strconv.Atoi(l); err == nil {
				limit = n
			}
		}
		out, err := repo.ListEntries(r.Context(), r.PathValue("accountID"), limit)
		if err != nil {
			writeLedgerErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	}
}
