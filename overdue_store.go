package accounting

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
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

func overdueAsOf(entries []Entry, reversed map[string]bool, at time.Time) []OverdueEntry {
	out := []OverdueEntry{}
	for _, e := range entries {
		if e.DueAt == "" || e.ReversesEntryID != "" || reversed[e.ID] {
			continue
		}
		due, err := time.Parse(time.RFC3339Nano, e.DueAt)
		if err != nil || !due.Before(at) {
			continue
		}
		out = append(out, OverdueEntry{Entry: e, DaysOverdue: int64(at.Sub(due) / day)})
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].DaysOverdue > out[j].DaysOverdue
	})
	return out
}

func (m *ledgerStore) Overdue(ctx context.Context, accountID, asOf string) (Overdue, error) {
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
	return Overdue{
		AccountID: accountID,
		AsOf:      raw,
		Items:     overdueAsOf(entries, reversedIDs(entries), at),
	}, nil
}
