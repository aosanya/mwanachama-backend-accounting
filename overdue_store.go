package accounting

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/aosanya/mwanachama-backend-shared/specstore"
)

const day = 24 * time.Hour

func (m *ledgerStore) asOf(raw string) (time.Time, string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = m.now()
	}
	at, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}, "", fmt.Errorf("%w: as_of %q is not an RFC 3339 instant", ErrInvalid, raw)
	}
	return at, raw, nil
}

func reversedIDs(entries []Entry) map[string]bool {
	out := map[string]bool{}
	for _, e := range entries {
		if e.ReversesEntryID != "" {
			out[e.ReversesEntryID] = true
		}
	}
	return out
}

func paidAgainst(settling []Entry, reversed map[string]bool) map[string]int64 {
	out := map[string]int64{}
	for _, p := range settling {
		if p.ReversesEntryID != "" || reversed[p.ID] {
			continue
		}
		out[p.SettlesEntryID] += p.Amount
	}
	return out
}

func ageBasis(raw string) (AgeBasis, error) {
	switch AgeBasis(strings.TrimSpace(raw)) {
	case "", AgeFromDue:
		return AgeFromDue, nil
	case AgeFromRaised:
		return AgeFromRaised, nil
	}
	return "", fmt.Errorf("%w: age_from %q is not %q or %q", ErrInvalid, raw, AgeFromDue, AgeFromRaised)
}

func ageStart(e Entry, due time.Time, basis AgeBasis) (time.Time, bool) {
	if basis != AgeFromRaised {
		return due, true
	}
	raised, err := time.Parse(time.RFC3339Nano, e.PostedAt)
	return raised, err == nil
}

func overdueAsOf(entries []Entry, reversed map[string]bool, paid map[string]int64, at time.Time, basis AgeBasis) []OverdueEntry {
	out := []OverdueEntry{}
	for _, e := range entries {
		if e.DueAt == "" || e.ReversesEntryID != "" || reversed[e.ID] {
			continue
		}
		due, err := time.Parse(time.RFC3339Nano, e.DueAt)
		if err != nil || !due.Before(at) {
			continue
		}
		outstanding := e.Amount - paid[e.ID]
		if outstanding <= 0 {
			continue
		}
		start, ok := ageStart(e, due, basis)
		if !ok {
			continue
		}
		out = append(out, OverdueEntry{
			Entry:       e,
			Outstanding: outstanding,
			DaysOverdue: int64(at.Sub(due) / day),
			AgeDays:     int64(at.Sub(start) / day),
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].AgeDays > out[j].AgeDays
	})
	return out
}

func (m *ledgerStore) Overdue(ctx context.Context, accountID, asOf, ageFrom string) (Overdue, error) {
	basis, err := ageBasis(ageFrom)
	if err != nil {
		return Overdue{}, err
	}
	at, raw, err := m.asOf(asOf)
	if err != nil {
		return Overdue{}, err
	}
	if _, err := m.accountByID(ctx, m.db, accountID); err != nil {
		return Overdue{}, err
	}
	entries, err := m.entriesTouching(ctx, m.db, accountID, 0)
	if err != nil {
		return Overdue{}, err
	}
	paid, err := m.settledAmounts(ctx, entries)
	if err != nil {
		return Overdue{}, err
	}
	return Overdue{
		AccountID: accountID,
		AsOf:      raw,
		AgeFrom:   basis,
		Items:     overdueAsOf(entries, reversedIDs(entries), paid, at, basis),
	}, nil
}

func (m *ledgerStore) settledAmounts(ctx context.Context, charges []Entry) (map[string]int64, error) {
	ids := make([]string, 0, len(charges))
	for _, e := range charges {
		if e.DueAt != "" {
			ids = append(ids, e.ID)
		}
	}
	if len(ids) == 0 {
		return map[string]int64{}, nil
	}
	settling, err := specstore.List[Entry](m.st, m.q(ctx, roleEntry).Where("settles_entry_id IN ?", ids), roleEntry)
	if err != nil {
		return nil, err
	}
	reversed, err := m.reversedAmong(ctx, settling)
	if err != nil {
		return nil, err
	}
	return paidAgainst(settling, reversed), nil
}

func (m *ledgerStore) reversedAmong(ctx context.Context, entries []Entry) (map[string]bool, error) {
	if len(entries) == 0 {
		return map[string]bool{}, nil
	}
	ids := make([]string, 0, len(entries))
	for _, e := range entries {
		ids = append(ids, e.ID)
	}
	reversals, err := specstore.List[Entry](m.st, m.q(ctx, roleEntry).Where("reverses_entry_id IN ?", ids), roleEntry)
	if err != nil {
		return nil, err
	}
	return reversedIDs(reversals), nil
}
