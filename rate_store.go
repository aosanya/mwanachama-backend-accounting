package accounting

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aosanya/mwanachama-backend-accounting/models"
	"github.com/aosanya/mwanachama-backend-shared/spec"
	"github.com/aosanya/mwanachama-backend-shared/specstore"
)

const roleRate = "rate"

const dateLayout = "2006-01-02"

type (
	ExchangeRate = models.ExchangeRate
	RateInput    = models.RateInput
)

type RateFilter struct {
	Base    string `query:"base"`
	Counter string `query:"counter"`
	Kind    string `query:"kind"`
	Limit   int    `query:"limit"`
}

type RateRepository interface {
	CreateRate(ctx context.Context, in RateInput) (ExchangeRate, error)

	ListRates(ctx context.Context, filter RateFilter) ([]ExchangeRate, error)
}

func addRateCarriers(s *spec.Spec, carriers map[string]any) {
	if _, ok := s.ByRole(roleRate); ok {
		carriers[roleRate] = models.ExchangeRate{}
	}
}

func isRateKind(kind string) bool {
	return kind == models.RateDaily || kind == models.RateMonthly || kind == models.RateAgreed
}

func checkRateInput(in RateInput) error {
	if !models.IsCurrency(in.Base) || !models.IsCurrency(in.Counter) {
		return fmt.Errorf("%w: base %q and counter %q must both be known ISO 4217 codes", ErrInvalid, in.Base, in.Counter)
	}
	if in.Base == in.Counter {
		return fmt.Errorf("%w: a rate needs two different currencies", ErrInvalid)
	}
	if in.Rate <= 0 {
		return fmt.Errorf("%w: rate %d is not positive; it counts hundred-millionths of the counter currency per unit of the base", ErrInvalid, in.Rate)
	}
	if _, err := time.Parse(dateLayout, in.AppliesOn); err != nil {
		return fmt.Errorf("%w: applies_on %q is not a date as 2006-01-02", ErrInvalid, in.AppliesOn)
	}
	if !isRateKind(in.Kind) {
		return fmt.Errorf("%w: kind %q is not daily, monthly or agreed", ErrInvalid, in.Kind)
	}
	if strings.TrimSpace(in.Source) == "" {
		return fmt.Errorf("%w: a rate names its source", ErrInvalid)
	}
	if strings.TrimSpace(in.RecordedBy) == "" {
		return fmt.Errorf("%w: a rate names no recorded_by", ErrInvalid)
	}
	return nil
}

func (m *ledgerStore) keepsRates() error { return m.keepsRole(roleRate, "exchange rates") }

func (m *ledgerStore) CreateRate(ctx context.Context, in RateInput) (ExchangeRate, error) {
	if err := m.keepsRates(); err != nil {
		return ExchangeRate{}, err
	}
	in.Source = strings.TrimSpace(in.Source)
	if err := checkRateInput(in); err != nil {
		return ExchangeRate{}, err
	}
	rows, err := m.allRates(ctx)
	if err != nil {
		return ExchangeRate{}, err
	}
	for _, held := range rows {
		if held.Base == in.Counter && held.Counter == in.Base {
			return ExchangeRate{}, fmt.Errorf("%w: the pair is held as %s per %s, not %s per %s", ErrInvalid, held.Counter, held.Base, in.Base, in.Counter)
		}
	}
	if in.ReplacesID != "" {
		if err := replaceableRate(rows, in); err != nil {
			return ExchangeRate{}, err
		}
	}
	rate := ExchangeRate{
		ID: newID(), Base: in.Base, Counter: in.Counter, Rate: in.Rate, AppliesOn: in.AppliesOn,
		Kind: in.Kind, Source: in.Source, ReplacesID: in.ReplacesID, RecordedBy: in.RecordedBy, RecordedAt: m.now(),
	}
	if err := m.checks(roleRate, rate); err != nil {
		return ExchangeRate{}, err
	}
	row, err := encode(m.st.Object(roleRate), rate)
	if err != nil {
		return ExchangeRate{}, err
	}
	if err := m.q(ctx, roleRate).Create(row).Error; err != nil {
		return ExchangeRate{}, err
	}
	return rate, nil
}

func replaceableRate(rows []ExchangeRate, in RateInput) error {
	for _, held := range rows {
		if held.ID != in.ReplacesID {
			continue
		}
		if held.Base != in.Base || held.Counter != in.Counter || held.Kind != in.Kind || held.AppliesOn != in.AppliesOn {
			return fmt.Errorf("%w: a correction keeps the pair, kind and date of the rate it replaces", ErrInvalid)
		}
		for _, other := range rows {
			if other.ReplacesID == held.ID {
				return fmt.Errorf("%w: rate %s has already been replaced", ErrInvalid, held.ID)
			}
		}
		return nil
	}
	return ErrNotFound
}

func (m *ledgerStore) allRates(ctx context.Context) ([]ExchangeRate, error) {
	if err := m.keepsRates(); err != nil {
		return nil, err
	}
	return specstore.List[ExchangeRate](m.st, m.q(ctx, roleRate).Order("recorded_at, id"), roleRate)
}

func (m *ledgerStore) ListRates(ctx context.Context, filter RateFilter) ([]ExchangeRate, error) {
	rows, err := m.allRates(ctx)
	if err != nil {
		return nil, err
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = DefaultPage
	}
	out := make([]ExchangeRate, 0, len(rows))
	for _, r := range liveRates(rows) {
		if filter.Base != "" && r.Base != filter.Base || filter.Counter != "" && r.Counter != filter.Counter || filter.Kind != "" && r.Kind != filter.Kind {
			continue
		}
		if len(out) == limit {
			break
		}
		out = append(out, r)
	}
	return out, nil
}
