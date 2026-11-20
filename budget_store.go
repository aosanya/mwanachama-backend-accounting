package accounting

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/aosanya/mwanachama-backend-accounting/internal/versioncheck"
	"github.com/aosanya/mwanachama-backend-shared/specstore"
)

func parsePeriod(start, end string) (time.Time, time.Time, error) {
	from, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(start))
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("%w: period_start %q is not an RFC 3339 instant", ErrInvalid, start)
	}
	to, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(end))
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("%w: period_end %q is not an RFC 3339 instant", ErrInvalid, end)
	}
	if !to.After(from) {
		return time.Time{}, time.Time{}, fmt.Errorf("%w: period_end must fall after period_start", ErrInvalid)
	}
	return from, to, nil
}

func withinPeriod(e Entry, from, to time.Time) bool {
	at, err := time.Parse(time.RFC3339Nano, orderKey(e))
	return err == nil && !at.Before(from) && at.Before(to)
}

func inPeriod(entries []Entry, from, to time.Time) []Entry {
	inside := make([]Entry, 0, len(entries))
	for _, e := range entries {
		if withinPeriod(e, from, to) {
			inside = append(inside, e)
		}
	}
	return inside
}

func (m *ledgerStore) balanceWithinCurrency(accountID string, entries []Entry, from, to time.Time, currency string) int64 {
	return m.balanceInCurrency(accountID, inPeriod(entries, from, to), currency)
}

func (m *ledgerStore) budgetFor(ctx context.Context, tx *gorm.DB, accountID, start, end string) (Budget, error) {
	var b Budget
	err := m.take(m.on(ctx, tx, roleBudget).
		Where("account_id = ? AND period_start = ? AND period_end = ?", accountID, start, end),
		roleBudget, &b, ErrNotFound)
	return b, err
}

func staleBudget(sent, stored Budget) error {
	if sent.Version == "" {
		return nil
	}
	setAt, err := time.Parse(time.RFC3339Nano, stored.SetAt)
	if err != nil {
		return nil
	}
	return versioncheck.Stale(sent.Version, setAt, []versioncheck.Field{
		{Name: "name", Sent: sent.Name, Stored: stored.Name},
		{Name: "department", Sent: sent.Department, Stored: stored.Department},
		{Name: "amount", Sent: strconv.FormatInt(sent.Amount, 10), Stored: strconv.FormatInt(stored.Amount, 10)},
	})
}

func staleBudgetLine(version string, stored Budget) error {
	if version == "" {
		return nil
	}
	setAt, err := time.Parse(time.RFC3339Nano, stored.SetAt)
	if err != nil {
		return nil
	}
	return versioncheck.Stale(version, setAt, nil)
}

func (m *ledgerStore) SetBudget(ctx context.Context, b Budget) (Budget, error) {
	if err := m.checks(roleBudget, b); err != nil {
		return Budget{}, err
	}
	if _, _, err := parsePeriod(b.PeriodStart, b.PeriodEnd); err != nil {
		return Budget{}, err
	}
	if err := checkCostShares(b.CostShares); err != nil {
		return Budget{}, err
	}
	if err := checkCurrency(b.Currency); err != nil {
		return Budget{}, err
	}
	b.Currency = m.currencyOf(b.Currency)

	var out Budget
	err := m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		account, err := m.accountByID(ctx, tx, b.AccountID)
		if err != nil {
			return err
		}
		existing, err := m.budgetFor(ctx, tx, b.AccountID, b.PeriodStart, b.PeriodEnd)
		switch {
		case err == nil:
			if b.Create {
				return &BudgetLineExistsError{AccountID: b.AccountID, Category: string(account.Category), PeriodStart: b.PeriodStart, PeriodEnd: b.PeriodEnd}
			}
			if err := staleBudget(b, existing); err != nil {
				return err
			}
			b.ID = existing.ID
		case errors.Is(err, ErrNotFound):
			b.ID = ""
		default:
			return err
		}
		b.Version = ""
		b.Create = false
		b.SetAt = m.now()

		o := m.st.Object(roleBudget)
		if b.ID != "" {
			row, err := encode(o, b)
			if err != nil {
				return err
			}
			if err := m.on(ctx, tx, roleBudget).Where("id = ?", b.ID).
				Updates(mutableColumns(o, row)).Error; err != nil {
				return err
			}
			out = b
			return nil
		}

		b.ID = newID()
		row, err := encode(o, b)
		if err != nil {
			return err
		}
		if err := m.on(ctx, tx, roleBudget).Create(row).Error; err != nil {
			return err
		}
		out = b
		return nil
	})
	if err != nil {
		return Budget{}, err
	}
	return out, nil
}

func (m *ledgerStore) DeleteBudgetLine(ctx context.Context, id, version string) (BudgetLine, error) {
	var removed BudgetLine
	err := m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var b Budget
		if err := m.take(m.on(ctx, tx, roleBudget).Where("id = ?", id), roleBudget, &b, ErrNotFound); err != nil {
			return err
		}
		if err := staleBudgetLine(version, b); err != nil {
			return err
		}
		from, to, err := parsePeriod(b.PeriodStart, b.PeriodEnd)
		if err != nil {
			return err
		}
		entries, err := m.entriesTouching(ctx, tx, b.AccountID, 0)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if withinPeriod(e, from, to) {
				return ErrBudgetLineInUse
			}
		}
		if err := m.refuseWhileATenderDraws(ctx, tx, id); err != nil {
			return err
		}
		removed = BudgetLine{
			ID: b.ID, Name: b.Name, Department: b.Department, AccountID: b.AccountID,
			PeriodStart: b.PeriodStart, PeriodEnd: b.PeriodEnd, Planned: b.Amount, Currency: m.currencyOf(b.Currency), SetAt: b.SetAt,
			FollowMilestones: b.FollowMilestones, CostShares: b.CostShares,
		}
		return m.on(ctx, tx, roleBudget).Where("id = ?", id).Delete(nil).Error
	})
	if err != nil {
		return BudgetLine{}, err
	}
	return removed, nil
}

func (m *ledgerStore) Variance(ctx context.Context, accountID, periodStart, periodEnd string) (Variance, error) {
	from, to, err := parsePeriod(periodStart, periodEnd)
	if err != nil {
		return Variance{}, err
	}
	if _, err := m.accountByID(ctx, m.db, accountID); err != nil {
		return Variance{}, err
	}
	b, err := m.budgetFor(ctx, m.db, accountID, periodStart, periodEnd)
	if err != nil {
		return Variance{}, err
	}
	entries, err := m.entriesTouching(ctx, m.db, accountID, 0)
	if err != nil {
		return Variance{}, err
	}
	currency := m.currencyOf(b.Currency)
	actual := m.balanceWithinCurrency(accountID, entries, from, to, currency)
	return Variance{
		AccountID:   accountID,
		PeriodStart: periodStart,
		PeriodEnd:   periodEnd,
		Currency:    currency,
		Budgeted:    b.Amount,
		Actual:      actual,
		Difference:  actual - b.Amount,
	}, nil
}

func (m *ledgerStore) refuseWhileATenderDraws(ctx context.Context, tx *gorm.DB, budgetID string) error {
	if _, ok := m.st.Spec().ByRole(roleTender); !ok {
		return nil
	}
	var drawing int64
	if err := m.on(ctx, tx, roleTender).Where("budget_id = ?", budgetID).Count(&drawing).Error; err != nil {
		return err
	}
	if drawing > 0 {
		return ErrBudgetLineInUse
	}
	return nil
}

func (m *ledgerStore) lineOf(ctx context.Context, b Budget) (BudgetLine, error) {
	from, to, err := parsePeriod(b.PeriodStart, b.PeriodEnd)
	if err != nil {
		return BudgetLine{}, err
	}
	entries, err := m.entriesTouching(ctx, m.db, b.AccountID, 0)
	if err != nil {
		return BudgetLine{}, err
	}
	currency := m.currencyOf(b.Currency)
	spent := m.balancesByCurrency(b.AccountID, inPeriod(entries, from, to))
	return BudgetLine{ID: b.ID, Name: b.Name, Department: b.Department, AccountID: b.AccountID, PeriodStart: b.PeriodStart, PeriodEnd: b.PeriodEnd, Planned: b.Amount, Spent: spent[currency], Currency: currency, OtherSpent: otherAmounts(spent, currency), SetAt: b.SetAt, FollowMilestones: b.FollowMilestones, CostShares: b.CostShares}, nil
}

func (m *ledgerStore) GetBudgetLine(ctx context.Context, id string) (BudgetLine, error) {
	var b Budget
	if err := m.take(m.q(ctx, roleBudget).Where("id = ?", id), roleBudget, &b, ErrNotFound); err != nil {
		return BudgetLine{}, err
	}
	return m.lineOf(ctx, b)
}

func (m *ledgerStore) ListBudgetLines(ctx context.Context, filter BudgetLineFilter) ([]BudgetLine, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = DefaultPage
	}
	q := m.q(ctx, roleBudget)
	if filter.AccountID != "" {
		q = q.Where("account_id = ?", filter.AccountID)
	}
	if filter.Department != "" {
		q = q.Where("department = ?", filter.Department)
	}
	budgets, err := specstore.List[Budget](m.st, q.Order("period_start, set_at, id").Limit(limit), roleBudget)
	if err != nil {
		return nil, err
	}
	lines := make([]BudgetLine, 0, len(budgets))
	for _, b := range budgets {
		line, err := m.lineOf(ctx, b)
		if err != nil {
			return nil, err
		}
		lines = append(lines, line)
	}
	return lines, nil
}
