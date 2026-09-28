package accounting

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/aosanya/mwanachama-backend-accounting/models"
	"github.com/aosanya/mwanachama-backend-shared/spec"
	"github.com/aosanya/mwanachama-backend-shared/specstore"
)

type ledgerStore struct {
	db  *gorm.DB
	st  *store
	now func() string
}

func NewLedgerRepository(db *gorm.DB, s *spec.Spec) (LedgerRepository, error) {
	st, err := newStore(db, s, map[string]any{
		roleAccount: models.Account{},
		roleEntry:   models.Entry{},
	})
	if err != nil {
		return nil, fmt.Errorf("NewLedgerRepository: %w", err)
	}
	return &ledgerStore{db: db, st: st, now: models.NowRFC3339}, nil
}

func (m *ledgerStore) q(ctx context.Context, role string) *gorm.DB {
	return m.db.WithContext(ctx).Table(m.st.Table(role))
}

func (m *ledgerStore) on(ctx context.Context, tx *gorm.DB, role string) *gorm.DB {
	return tx.WithContext(ctx).Table(m.st.Table(role))
}

func (m *ledgerStore) take(q *gorm.DB, role string, out any, notFound error) error {
	return m.st.Take(q, role, out, notFound)
}

func (m *ledgerStore) accountByID(ctx context.Context, tx *gorm.DB, id string) (Account, error) {
	var a Account
	err := m.take(m.on(ctx, tx, roleAccount).Where("id = ?", id), roleAccount, &a, ErrNotFound)
	return a, err
}

func (m *ledgerStore) accountByIdentity(ctx context.Context, tx *gorm.DB, a Account) (Account, error) {
	var out Account
	err := m.take(
		m.on(ctx, tx, roleAccount).Where("category = ? AND holder_id = ?", string(a.Category), a.HolderID),
		roleAccount, &out, ErrNotFound)
	return out, err
}

func (m *ledgerStore) OpenAccount(ctx context.Context, a Account) (Account, error) {
	if err := m.checks(roleAccount, a); err != nil {
		return Account{}, err
	}

	var out Account
	err := m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		existing, err := m.accountByIdentity(ctx, tx, a)
		switch {
		case err == nil:
			out = existing
			return nil
		case !errors.Is(err, ErrNotFound):
			return err
		}

		if a.Code != "" {
			var held int64
			if err := m.on(ctx, tx, roleAccount).Where("code = ?", a.Code).Count(&held).Error; err != nil {
				return err
			}
			if held > 0 {
				return ErrCodeTaken
			}
		}

		a.ID = newID()
		a.OpenedAt = m.now()
		a.ClosedAt = ""

		row, err := encode(m.st.Object(roleAccount), a)
		if err != nil {
			return err
		}
		if err := m.on(ctx, tx, roleAccount).Create(row).Error; err != nil {
			raced, reread := m.accountByIdentity(ctx, tx, a)
			if reread == nil {
				out = raced
				return nil
			}
			return err
		}
		out = a
		return nil
	})
	if err != nil {
		return Account{}, err
	}
	return out, nil
}

func (m *ledgerStore) GetAccount(ctx context.Context, id string) (Account, error) {
	return m.accountByID(ctx, m.db, id)
}

func (m *ledgerStore) ListAccounts(ctx context.Context, limit int) ([]Account, error) {
	if limit <= 0 {
		limit = DefaultPage
	}
	q := m.q(ctx, roleAccount).Order("opened_at").Limit(limit)
	return specstore.List[Account](m.st, q, roleAccount)
}

func (m *ledgerStore) CloseAccount(ctx context.Context, id string) (Account, error) {
	var out Account
	err := m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		a, err := m.accountByID(ctx, tx, id)
		if err != nil {
			return err
		}
		if a.ClosedAt != "" {
			return ErrClosed
		}
		balance, err := m.balanceIn(ctx, tx, id)
		if err != nil {
			return err
		}
		if balance != 0 {
			return ErrNonZeroBalance
		}

		a.ClosedAt = m.now()
		row, err := encode(m.st.Object(roleAccount), a)
		if err != nil {
			return err
		}
		res := m.on(ctx, tx, roleAccount).Where("id = ?", id).
			Updates(mutableColumns(m.st.Object(roleAccount), row))
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrNotFound
		}
		out = a
		return nil
	})
	if err != nil {
		return Account{}, err
	}
	return out, nil
}

func (m *ledgerStore) Post(ctx context.Context, e Entry) (Entry, error) {
	if err := m.checks(roleEntry, e); err != nil {
		return Entry{}, err
	}
	if err := checkEntryBeyondSpec(e); err != nil {
		return Entry{}, err
	}

	var out Entry
	err := m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		debit, err := m.accountByID(ctx, tx, e.DebitAccountID)
		if err != nil {
			return err
		}
		credit, err := m.accountByID(ctx, tx, e.CreditAccountID)
		if err != nil {
			return err
		}
		if debit.ClosedAt != "" || credit.ClosedAt != "" {
			return ErrClosed
		}

		if e.ReversesEntryID != "" {
			var original Entry
			if err := m.take(m.on(ctx, tx, roleEntry).Where("id = ?", e.ReversesEntryID),
				roleEntry, &original, ErrNotFound); err != nil {
				return err
			}
			var reversals int64
			if err := m.on(ctx, tx, roleEntry).
				Where("reverses_entry_id = ?", e.ReversesEntryID).Count(&reversals).Error; err != nil {
				return err
			}
			if reversals > 0 {
				return ErrAlreadyReversed
			}
		}

		e.ID = newID()
		e.PostedAt = m.now()

		row, err := encode(m.st.Object(roleEntry), e)
		if err != nil {
			return err
		}
		if err := m.on(ctx, tx, roleEntry).Create(row).Error; err != nil {
			return err
		}
		out = e
		return nil
	})
	if err != nil {
		return Entry{}, err
	}
	return out, nil
}

func (m *ledgerStore) entriesTouching(ctx context.Context, tx *gorm.DB, accountID string, limit int) ([]Entry, error) {
	q := m.on(ctx, tx, roleEntry).
		Where("debit_account_id = ? OR credit_account_id = ?", accountID, accountID).
		Order("coalesce(nullif(occurred_at, ''), posted_at)")
	if limit > 0 {
		q = q.Limit(limit)
	}
	return specstore.List[Entry](m.st, q, roleEntry)
}

func (m *ledgerStore) balanceIn(ctx context.Context, tx *gorm.DB, accountID string) (int64, error) {
	entries, err := m.entriesTouching(ctx, tx, accountID, 0)
	if err != nil {
		return 0, err
	}
	return balanceOf(accountID, entries), nil
}

func (m *ledgerStore) Balance(ctx context.Context, accountID string) (int64, error) {
	if _, err := m.accountByID(ctx, m.db, accountID); err != nil {
		return 0, err
	}
	return m.balanceIn(ctx, m.db, accountID)
}

func (m *ledgerStore) ListEntries(ctx context.Context, accountID string, limit int) ([]Entry, error) {
	if limit <= 0 {
		limit = DefaultPage
	}
	if _, err := m.accountByID(ctx, m.db, accountID); err != nil {
		return nil, err
	}
	return m.entriesTouching(ctx, m.db, accountID, limit)
}
