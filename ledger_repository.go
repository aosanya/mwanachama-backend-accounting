package accounting

import "context"

const DefaultPage = 200

type LedgerRepository interface {
	OpenAccount(ctx context.Context, a Account) (Account, error)

	GetAccount(ctx context.Context, id string) (Account, error)

	ListAccounts(ctx context.Context, limit int) ([]Account, error)

	CloseAccount(ctx context.Context, id string) (Account, error)

	Post(ctx context.Context, e Entry) (Entry, error)

	Balance(ctx context.Context, accountID string) (int64, error)

	ListEntries(ctx context.Context, accountID string, limit int) ([]Entry, error)

	SetBudget(ctx context.Context, b Budget) (Budget, error)

	Variance(ctx context.Context, accountID, periodStart, periodEnd string) (Variance, error)

	Overdue(ctx context.Context, accountID, asOf string) (Overdue, error)

	ListEntriesByDimension(ctx context.Context, accountID, key, value string, limit int) ([]Entry, error)

	DimensionBalance(ctx context.Context, accountID, key, value string) (int64, error)
}
