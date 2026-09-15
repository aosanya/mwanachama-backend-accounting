package routes

import (
	"errors"
	"net/http"

	"github.com/aosanya/mwanachama-backend-accounting"
)

// ledgerStatusFor maps this package's sentinel errors to a status code —
// the same shape actor's routes package uses for its own errors.
func ledgerStatusFor(err error) int {
	switch {
	case errors.Is(err, accounting.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, accounting.ErrClosed),
		errors.Is(err, accounting.ErrNonZeroBalance):
		return http.StatusConflict
	case errors.Is(err, accounting.ErrInvalid):
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}

func writeLedgerErr(w http.ResponseWriter, err error) {
	code := ledgerStatusFor(err)
	if code == http.StatusInternalServerError {
		writeErr(w, code, "internal error")
		return
	}
	writeErr(w, code, err.Error())
}
