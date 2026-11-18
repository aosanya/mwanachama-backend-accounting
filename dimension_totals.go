package accounting

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/aosanya/mwanachama-backend-shared/specstore"
)

type accountSelection func(accountID string) bool

func everyAccount(string) bool { return true }

func (m *ledgerStore) accountsOfType(ctx context.Context, accountType string) (accountSelection, error) {
	if accountType == "" {
		return everyAccount, nil
	}
	if !IsAccountType(AccountType(accountType)) {
		return nil, fmt.Errorf("%w: account type %q is not one of asset, liability, equity, income, expense", ErrInvalid, accountType)
	}
	accounts, err := specstore.List[Account](m.st, m.q(ctx, roleAccount).Where("type = ?", accountType), roleAccount)
	if err != nil {
		return nil, err
	}
	ids := make(map[string]bool, len(accounts))
	for _, a := range accounts {
		ids[a.ID] = true
	}
	return func(id string) bool { return ids[id] }, nil
}

type dimensionWindow struct {
	from, to time.Time
	bounded  bool
}

func parseDimensionWindow(start, end string) (dimensionWindow, error) {
	if strings.TrimSpace(start) == "" && strings.TrimSpace(end) == "" {
		return dimensionWindow{}, nil
	}
	from, to, err := parsePeriod(start, end)
	if err != nil {
		return dimensionWindow{}, err
	}
	return dimensionWindow{from: from, to: to, bounded: true}, nil
}

func (w dimensionWindow) admits(e Entry) bool {
	return !w.bounded || withinPeriod(e, w.from, w.to)
}

func sideTotal(e Entry, selected accountSelection) (int64, bool) {
	var total int64
	touched := false
	if selected(e.DebitAccountID) {
		total += e.Amount
		touched = true
	}
	if selected(e.CreditAccountID) {
		total -= e.Amount
		touched = true
	}
	return total, touched
}

func portionsOf(e Entry, key string, total int64, shared sharedLines, selected accountSelection) []portion {
	if key == NeighbourhoodKey {
		if shares, ok := shared.sharesOf(e, selected); ok {
			return apportion(total, shares)
		}
	}
	if value, carries := e.Dimensions[key]; carries {
		return []portion{{value: value, amount: total}}
	}
	return nil
}

func foldByValue(entries []Entry, key string, selected accountSelection, w dimensionWindow, shared sharedLines) []DimensionTotal {
	byValue := map[string]*DimensionTotal{}
	for _, e := range entries {
		if !w.admits(e) {
			continue
		}
		total, touched := sideTotal(e, selected)
		if !touched {
			continue
		}
		for _, part := range portionsOf(e, key, total, shared, selected) {
			row, seen := byValue[part.value]
			if !seen {
				row = &DimensionTotal{Value: part.value}
				byValue[part.value] = row
			}
			row.Total += part.amount
			row.Entries++
		}
	}
	return sortedTotals(byValue)
}

func sortedTotals(byValue map[string]*DimensionTotal) []DimensionTotal {
	out := make([]DimensionTotal, 0, len(byValue))
	for _, row := range byValue {
		out = append(out, *row)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Value < out[j].Value })
	return out
}

func (m *ledgerStore) DimensionTotals(ctx context.Context, key, accountType, periodStart, periodEnd string) (DimensionTotals, error) {
	if err := checkDimensionKey(key); err != nil {
		return DimensionTotals{}, err
	}
	window, err := parseDimensionWindow(periodStart, periodEnd)
	if err != nil {
		return DimensionTotals{}, err
	}
	selected, err := m.accountsOfType(ctx, accountType)
	if err != nil {
		return DimensionTotals{}, err
	}
	entries, err := specstore.List[Entry](m.st, m.q(ctx, roleEntry), roleEntry)
	if err != nil {
		return DimensionTotals{}, err
	}
	budgets, err := specstore.List[Budget](m.st, m.q(ctx, roleBudget), roleBudget)
	if err != nil {
		return DimensionTotals{}, err
	}
	return DimensionTotals{
		Key:         key,
		AccountType: accountType,
		PeriodStart: periodStart,
		PeriodEnd:   periodEnd,
		Totals:      foldByValue(entries, key, selected, window, sharedLinesOf(budgets)),
	}, nil
}
