package accounting

import "errors"

// ErrNotFound is returned when an account or entry id names nothing.
var ErrNotFound = errors.New("accounting: not found")

// ErrInvalid is returned by Validate and by any write that would violate it.
var ErrInvalid = errors.New("accounting: invalid")

// ErrClosed is returned when a posting names an account that has been closed.
var ErrClosed = errors.New("accounting: the account is closed")

// ErrNonZeroBalance is returned by CloseAccount for an account that still
// carries a balance — the money equivalent of merchandise-account.md's "a
// closed account must have a zero balance".
var ErrNonZeroBalance = errors.New("accounting: an account holding a balance cannot be closed")
