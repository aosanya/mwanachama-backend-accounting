package accounting

import (
	"fmt"
	"strings"
	"time"
)

// The general ledger — Account and Entry, a domain-agnostic double-entry
// core: entries, not balances; an account opened lazily by its first
// posting, keyed on (Kind, HolderID); a mistake corrected by a new entry,
// never an edit. What Kind values exist, what a HolderID names, and which
// AccountType a given Kind should carry are all decisions for whatever
// domain layers on top (see vocabulary.go) — this file enforces only the
// invariants true of double-entry bookkeeping itself.
//
// **The zero-sum invariant is structural, not a rule enforced separately.**
// Every entry moves one Amount from exactly one account to exactly one
// other, so a posting cannot exist that changes the sum of every balance in
// the ledger. A domain that needs a real second side for a not-yet-resolved
// posting (an unmatched statement row, a shipment in transit) adds its own
// suspense-shaped AccountKind for it — the same role the merchandise
// ledger's `in_transit` account plays — rather than this package special
// casing a one-sided entry.

// Account is the place money can be. Kind and HolderID together are an
// account's identity — OpenAccount opens the same (Kind, HolderID) pair at
// most once, lazily, on first posting (G235: an account exists because
// something was posted to it, never because its holder merely registered).
// Type is the account's place in the accounting equation; this package does
// not pair a Type to a Kind — a domain layer that wants that discipline
// enforces it in its own account-opening helper, the same way it owns Kind's
// vocabulary.
type Account struct {
	ID   string      `json:"id"`
	Kind AccountKind `json:"kind"`
	Type AccountType `json:"type"`

	// HolderID is an opaque id the caller supplies and interprets — a
	// member, a paybill, a student, a wallet address. This package never
	// reads it for meaning, only for identity: two OpenAccount calls with
	// the same (Kind, HolderID) return the same account.
	HolderID string `json:"holder_id"`

	OpenedAt time.Time `json:"opened_at"`
	// ClosedAt mirrors the merchandise ledger: refused while the balance is
	// non-zero (ErrNonZeroBalance).
	ClosedAt *time.Time `json:"closed_at,omitempty"`
}

// Entry is one posting. Append-only: there is no update path and no delete
// path anywhere in this package. A mistake is corrected by a new entry with
// ReversesEntryID set; the original is never touched.
type Entry struct {
	ID string `json:"id"`

	// PostedAt is when it hit the books.
	PostedAt time.Time `json:"posted_at"`
	// OccurredAt is when the underlying event happened. Reads that order a
	// ledger should order on this, coalescing to PostedAt where absent — the
	// same rule G271 states for the merchandise ledger.
	OccurredAt *time.Time `json:"occurred_at,omitempty"`

	FromAccountID string `json:"from_account_id"`
	ToAccountID   string `json:"to_account_id"`

	// Amount is in the deployment's smallest denominated unit (cents, a
	// token's smallest indivisible unit) and is always positive; direction
	// is the two accounts, never a sign. Multi-currency is out of scope —
	// there is no currency field; a deployment needing more than one runs
	// one ledger per currency.
	Amount int64 `json:"amount"`

	DocumentKind DocumentKind `json:"document_kind"`
	DocumentID   string       `json:"document_id"`

	// ActorID is who/what caused the posting, never only a role — the
	// ledger is an accountability surface, the same reason
	// merchandise_entry.actor_id is required.
	ActorID string `json:"actor_id"`

	// Detail is the sentence the ledger reads back; on a correction it is
	// the reason for the reversal.
	Detail string `json:"detail,omitempty"`

	// ReversesEntryID points at what this corrects. The original is never
	// touched.
	ReversesEntryID string `json:"reverses_entry_id,omitempty"`
}

// Validate refuses a malformed account.
func (a Account) Validate() error {
	if strings.TrimSpace(string(a.Kind)) == "" {
		return fmt.Errorf("%w: an account names no kind", ErrInvalid)
	}
	if strings.TrimSpace(a.HolderID) == "" {
		return fmt.Errorf("%w: an account names no holder", ErrInvalid)
	}
	if !IsAccountType(a.Type) {
		return fmt.Errorf("%w: account type %q is not one of the five standard types", ErrInvalid, a.Type)
	}
	return nil
}

// Validate refuses a malformed posting.
func (e Entry) Validate() error {
	if strings.TrimSpace(string(e.DocumentKind)) == "" {
		return fmt.Errorf("%w: every entry names what document caused it and this one names none", ErrInvalid)
	}
	if strings.TrimSpace(e.DocumentID) == "" {
		return fmt.Errorf("%w: every entry is posted by a document and this one names none", ErrInvalid)
	}
	if strings.TrimSpace(e.ActorID) == "" {
		return fmt.Errorf("%w: the ledger is an accountability surface and this entry names no actor", ErrInvalid)
	}
	if e.Amount <= 0 {
		return fmt.Errorf("%w: amount %d is not positive; direction is the two accounts, never a sign", ErrInvalid, e.Amount)
	}
	from, to := strings.TrimSpace(e.FromAccountID), strings.TrimSpace(e.ToAccountID)
	if from == "" || to == "" {
		return fmt.Errorf("%w: every entry names both accounts; there is no one-sided posting in this ledger", ErrInvalid)
	}
	if from == to {
		return fmt.Errorf("%w: an entry cannot post from an account to itself", ErrInvalid)
	}
	return nil
}
