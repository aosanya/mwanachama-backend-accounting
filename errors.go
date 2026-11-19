package accounting

import "errors"

var ErrNotFound = errors.New("accounting: not found")

var ErrInvalid = errors.New("accounting: invalid")

var ErrClosed = errors.New("accounting: the account is closed")

var ErrNonZeroBalance = errors.New("accounting: an account holding a balance cannot be closed")

var ErrAlreadyReversed = errors.New("accounting: that entry has already been reversed")

var ErrCodeTaken = errors.New("accounting: another account already carries that code")

var ErrOverCertified = errors.New("accounting: the certificates would exceed the milestone's amount")

var ErrRetentionHeld = errors.New("accounting: the retention cannot be released before its release date")

var ErrRetentionReleased = errors.New("accounting: the retention has already been released")

var ErrLocked = errors.New("accounting: the record can no longer be changed")

var ErrBudgetLineInUse = errors.New("accounting: a budget line with spending in its period, or a tender drawing on it, cannot be deleted")

type BudgetLineExistsError struct {
	AccountID   string
	Category    string
	PeriodStart string
	PeriodEnd   string
}

func (e *BudgetLineExistsError) Error() string {
	account := e.AccountID
	if e.Category != "" {
		account += " (" + e.Category + ")"
	}
	return "account " + account + " already has a budget line for " + dateOf(e.PeriodStart) + " to " + dateOf(e.PeriodEnd)
}

func dateOf(instant string) string {
	if len(instant) >= len("2006-01-02") {
		return instant[:len("2006-01-02")]
	}
	return instant
}
