package accounting

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
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

func balanceWithin(accountID string, entries []Entry, from, to time.Time) int64 {
	inside := make([]Entry, 0, len(entries))
	for _, e := range entries {
		at, err := time.Parse(time.RFC3339Nano, orderKey(e))
		if err != nil || at.Before(from) || !at.Before(to) {
			continue
		}
		inside = append(inside, e)
	}
	return balanceOf(accountID, inside)
}

func (m *ledgerStore) budgetFor(ctx context.Context, tx *gorm.DB, accountID, start, end string) (Budget, error) {
	var b Budget
	err := m.take(m.on(ctx, tx, roleBudget).
		Where("account_id = ? AND period_start = ? AND period_end = ?", accountID, start, end),
		roleBudget, &b, ErrNotFound)
	return b, err
}

func (m *ledgerStore) SetBudget(ctx context.Context, b Budget) (Budget, error) {
	if err := m.checks(roleBudget, b); err != nil {
		return Budget{}, err
	}
	if _, _, err := parsePeriod(b.PeriodStart, b.PeriodEnd); err != nil {
		return Budget{}, err
	}

	var out Budget
	err := m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := m.accountByID(ctx, tx, b.AccountID); err != nil {
			return err
		}
		existing, err := m.budgetFor(ctx, tx, b.AccountID, b.PeriodStart, b.PeriodEnd)
		switch {
		case err == nil:
			b.ID = existing.ID
		case errors.Is(err, ErrNotFound):
			b.ID = ""
		default:
			return err
		}
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
	actual := balanceWithin(accountID, entries, from, to)
	return Variance{
		AccountID:   accountID,
		PeriodStart: periodStart,
		PeriodEnd:   periodEnd,
		Budgeted:    b.Amount,
		Actual:      actual,
		Difference:  actual - b.Amount,
	}, nil
}
