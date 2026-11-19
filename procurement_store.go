package accounting

import (
	"context"
	"fmt"

	"github.com/aosanya/mwanachama-backend-accounting/models"
	"github.com/aosanya/mwanachama-backend-shared/specstore"
)

const (
	roleTender = "tender"
	roleQuote  = "quote"
)

type (
	Tender     = models.Tender
	TenderView = models.TenderView
	Quote      = models.Quote
)

type TenderFilter struct {
	BudgetID string `query:"budget_id"`
	Limit    int    `query:"limit"`
}

type ProcurementRepository interface {
	CreateTender(ctx context.Context, t Tender) (Tender, error)

	ListTenders(ctx context.Context, filter TenderFilter) ([]TenderView, error)

	GetTender(ctx context.Context, id string) (TenderView, error)

	CreateQuote(ctx context.Context, q Quote) (Quote, error)

	ListQuotes(ctx context.Context, tenderID string, limit int) ([]Quote, error)

	UpdateTender(ctx context.Context, in TenderUpdate) (Tender, error)

	UpdateQuote(ctx context.Context, in QuoteUpdate) (Quote, error)
}

func (m *ledgerStore) keepsRole(role, what string) error {
	if _, ok := m.st.Spec().ByRole(role); !ok {
		return fmt.Errorf("%w: this domain keeps no %s", ErrInvalid, what)
	}
	return nil
}

func (m *ledgerStore) keepsProcurement() error {
	if err := m.keepsRole(roleTender, "tenders"); err != nil {
		return err
	}
	return m.keepsRole(roleQuote, "quotes")
}

func (m *ledgerStore) tenderByID(ctx context.Context, id string) (Tender, error) {
	var out Tender
	err := m.take(m.q(ctx, roleTender).Where("id = ?", id), roleTender, &out, ErrNotFound)
	return out, err
}

func (m *ledgerStore) CreateTender(ctx context.Context, t Tender) (Tender, error) {
	if err := m.keepsProcurement(); err != nil {
		return Tender{}, err
	}
	if err := m.checks(roleTender, t); err != nil {
		return Tender{}, err
	}
	if t.ClosesAt != "" {
		if err := checkInstant("closes_at", t.ClosesAt); err != nil {
			return Tender{}, err
		}
	}
	var budget Budget
	if err := m.take(m.q(ctx, roleBudget).Where("id = ?", t.BudgetID), roleBudget, &budget, ErrNotFound); err != nil {
		return Tender{}, err
	}
	t.ID = newID()
	t.CreatedAt = m.now()
	row, err := encode(m.st.Object(roleTender), t)
	if err != nil {
		return Tender{}, err
	}
	if err := m.q(ctx, roleTender).Create(row).Error; err != nil {
		return Tender{}, err
	}
	return t, nil
}

func (m *ledgerStore) viewOf(ctx context.Context, t Tender) (TenderView, error) {
	quotes, err := specstore.List[Quote](m.st, m.q(ctx, roleQuote).Where("tender_id = ?", t.ID), roleQuote)
	if err != nil {
		return TenderView{}, err
	}
	view := TenderView{Tender: t, QuoteCount: len(quotes)}
	for i, q := range quotes {
		if i == 0 || q.Amount < view.LowestAmount {
			view.LowestAmount = q.Amount
		}
	}
	return view, nil
}

func (m *ledgerStore) GetTender(ctx context.Context, id string) (TenderView, error) {
	if err := m.keepsProcurement(); err != nil {
		return TenderView{}, err
	}
	t, err := m.tenderByID(ctx, id)
	if err != nil {
		return TenderView{}, err
	}
	return m.viewOf(ctx, t)
}

func (m *ledgerStore) ListTenders(ctx context.Context, filter TenderFilter) ([]TenderView, error) {
	if err := m.keepsProcurement(); err != nil {
		return nil, err
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = DefaultPage
	}
	q := m.q(ctx, roleTender)
	if filter.BudgetID != "" {
		q = q.Where("budget_id = ?", filter.BudgetID)
	}
	tenders, err := specstore.List[Tender](m.st, q.Order("created_at, id").Limit(limit), roleTender)
	if err != nil {
		return nil, err
	}
	views := make([]TenderView, 0, len(tenders))
	for _, t := range tenders {
		view, err := m.viewOf(ctx, t)
		if err != nil {
			return nil, err
		}
		views = append(views, view)
	}
	return views, nil
}

func (m *ledgerStore) CreateQuote(ctx context.Context, q Quote) (Quote, error) {
	if err := m.keepsProcurement(); err != nil {
		return Quote{}, err
	}
	if err := m.checks(roleQuote, q); err != nil {
		return Quote{}, err
	}
	if q.Amount <= 0 {
		return Quote{}, fmt.Errorf("%w: quote amount %d is not positive", ErrInvalid, q.Amount)
	}
	if q.ValidUntil != "" {
		if err := checkInstant("valid_until", q.ValidUntil); err != nil {
			return Quote{}, err
		}
	}
	if _, err := m.tenderByID(ctx, q.TenderID); err != nil {
		return Quote{}, err
	}
	q.ID = newID()
	q.QuotedAt = m.now()
	row, err := encode(m.st.Object(roleQuote), q)
	if err != nil {
		return Quote{}, err
	}
	if err := m.q(ctx, roleQuote).Create(row).Error; err != nil {
		return Quote{}, err
	}
	return q, nil
}

func (m *ledgerStore) ListQuotes(ctx context.Context, tenderID string, limit int) ([]Quote, error) {
	if err := m.keepsProcurement(); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = DefaultPage
	}
	if _, err := m.tenderByID(ctx, tenderID); err != nil {
		return nil, err
	}
	q := m.q(ctx, roleQuote).Where("tender_id = ?", tenderID).Order("amount, quoted_at, id").Limit(limit)
	return specstore.List[Quote](m.st, q, roleQuote)
}
