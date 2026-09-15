package routes

import (
	"net/http"

	"github.com/aosanya/mwanachama-backend-accounting"
)

// Route is one HTTP endpoint: a method, a path relative to this package's
// mount point, and the handler. The mounting process wraps Handler with its
// own auth/capability gates and builds the mux itself — see doc.go.
type Route struct {
	Method  string
	Path    string
	Handler http.HandlerFunc
}

// Pattern returns the Go 1.22+ ServeMux pattern for this route under
// prefix, e.g. Pattern("/v1/accounting") on {Method: "GET", Path:
// "/accounts/{accountID}"} yields "GET /v1/accounting/accounts/{accountID}".
func (rt Route) Pattern(prefix string) string {
	return rt.Method + " " + prefix + rt.Path
}

// Routes returns every route this package defines, over repo. Concatenates
// the per-model route lists — see account.go and entry.go.
func Routes(repo accounting.LedgerRepository) []Route {
	var out []Route
	out = append(out, AccountRoutes(repo)...)
	out = append(out, EntryRoutes(repo)...)
	return out
}
