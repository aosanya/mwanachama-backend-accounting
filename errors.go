package accounting

import "errors"

var ErrNotFound = errors.New("accounting: not found")

var ErrInvalid = errors.New("accounting: invalid")

var ErrClosed = errors.New("accounting: the account is closed")

var ErrNonZeroBalance = errors.New("accounting: an account holding a balance cannot be closed")

var ErrAlreadyReversed = errors.New("accounting: that entry has already been reversed")

var ErrCodeTaken = errors.New("accounting: another account already carries that code")
