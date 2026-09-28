package accounting

import (
	"fmt"
	"strings"

	"github.com/aosanya/mwanachama-backend-accounting/models"
)

type (
	Account         = models.Account
	Entry           = models.Entry
	AccountType     = models.AccountType
	AccountCategory = models.AccountCategory
	DocumentKind    = models.DocumentKind
)

const (
	AccountTypeAsset     = models.AccountTypeAsset
	AccountTypeLiability = models.AccountTypeLiability
	AccountTypeEquity    = models.AccountTypeEquity
	AccountTypeIncome    = models.AccountTypeIncome
	AccountTypeExpense   = models.AccountTypeExpense
)

func IsAccountType(t AccountType) bool { return models.IsAccountType(t) }

func balanceOf(accountID string, entries []Entry) int64 {
	var total int64
	for _, e := range entries {
		switch accountID {
		case e.DebitAccountID:
			total += e.Amount
		case e.CreditAccountID:
			total -= e.Amount
		}
	}
	return total
}

func orderKey(e Entry) string {
	if e.OccurredAt != "" {
		return e.OccurredAt
	}
	return e.PostedAt
}

func checkEntryBeyondSpec(e Entry) error {
	if e.Amount <= 0 {
		return fmt.Errorf("%w: amount %d is not positive; which side is which is the two accounts, never a sign", ErrInvalid, e.Amount)
	}
	if strings.TrimSpace(e.DebitAccountID) == strings.TrimSpace(e.CreditAccountID) {
		return fmt.Errorf("%w: an entry cannot debit and credit the same account", ErrInvalid)
	}
	return nil
}
