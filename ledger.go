package accounting

import (
	"fmt"
	"strings"
	"time"

	"github.com/aosanya/mwanachama-backend-accounting/models"
)

type (
	Account          = models.Account
	Entry            = models.Entry
	AccountType      = models.AccountType
	AccountCategory  = models.AccountCategory
	DocumentKind     = models.DocumentKind
	Budget           = models.Budget
	BudgetLine       = models.BudgetLine
	Variance         = models.Variance
	DimensionTotal   = models.DimensionTotal
	DimensionTotals  = models.DimensionTotals
	DimensionEntry   = models.DimensionEntry
	DimensionEntries = models.DimensionEntries
	OverdueEntry     = models.OverdueEntry
	Overdue          = models.Overdue
	Milestone        = models.Milestone
	Certificate      = models.Certificate
	AgeBasis         = models.AgeBasis
)

type BudgetLineFilter struct {
	AccountID  string `query:"account_id"`
	Department string `query:"department"`
	Limit      int    `query:"limit"`
}

const (
	AgeFromDue    = models.AgeFromDue
	AgeFromRaised = models.AgeFromRaised
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
	if e.DueAt != "" {
		if _, err := time.Parse(time.RFC3339Nano, e.DueAt); err != nil {
			return fmt.Errorf("%w: due_at %q is not an RFC 3339 instant", ErrInvalid, e.DueAt)
		}
	}
	return checkDimensions(e.Dimensions)
}
