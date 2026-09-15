package accounting

import "context"

// DefaultPage is the page size a store falls back to when a caller passes
// limit <= 0.
const DefaultPage = 200

// LedgerRepository is the persistence boundary for accounts and entries.
//
// **There is no Update and no Delete, on either type, at any scope.** A
// mistake is corrected by a new entry with ReversesEntryID set; the original
// is never touched — the same discipline merchandise.LedgerRepository
// documents for its own tables, for the same reason: an auditable ledger is
// the thing a reader can be shown.
//
// The one write an account has is CloseAccount, and it sets a timestamp
// rather than changing anything a balance is folded from.
//
// No method takes a caller id. Authorization is not designed yet (open
// question 2, documentation/2. design/README.md) — when it is, the same
// discipline applies: a rule expressed as an argument is one wrong call site
// away from being passed the wrong value.
type LedgerRepository interface {
	// OpenAccount returns the account matching (a.Kind, a.HolderID), opening
	// it if it does not exist yet — lazy creation, the same rule the
	// merchandise ledger uses (G235). An account exists because something
	// was posted to it, never because its holder merely registered.
	//
	// ErrInvalid from Validate, without writing.
	OpenAccount(ctx context.Context, a Account) (Account, error)

	// GetAccount returns one account by id. ErrNotFound if unknown.
	GetAccount(ctx context.Context, id string) (Account, error)

	// ListAccounts returns every account, oldest-opened first. Limit <= 0
	// means DefaultPage.
	ListAccounts(ctx context.Context, limit int) ([]Account, error)

	// CloseAccount stamps ClosedAt. ErrNonZeroBalance if the account still
	// carries a balance; ErrClosed if it is already closed.
	CloseAccount(ctx context.Context, id string) (Account, error)

	// Post appends one entry and returns it with ID and PostedAt filled in.
	//
	// ErrInvalid from Validate, ErrNotFound for an account that is not
	// there, and ErrClosed for a posting against a closed account.
	Post(ctx context.Context, e Entry) (Entry, error)

	// Balance folds one account: amount in minus amount out, over every
	// entry naming it. There is no stored balance anywhere in this package.
	Balance(ctx context.Context, accountID string) (int64, error)

	// ListEntries returns one account's ledger, both sides, ordered on
	// OccurredAt (coalescing to PostedAt), oldest first so a running balance
	// can be accumulated down the page. Limit <= 0 means DefaultPage.
	ListEntries(ctx context.Context, accountID string, limit int) ([]Entry, error)
}
