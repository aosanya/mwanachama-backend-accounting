package accounting

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	MaxDimensions     = 16
	MaxDimensionValue = 200
)

var DimensionKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)

func checkDimensionKey(key string) error {
	if !DimensionKeyPattern.MatchString(key) {
		return fmt.Errorf("%w: dimension key %q is not lower-case letters, digits and underscores starting with a letter, at most 63 characters", ErrInvalid, key)
	}
	return nil
}

func checkDimensionValue(key, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%w: dimension %q has an empty value", ErrInvalid, key)
	}
	if !utf8.ValidString(value) || utf8.RuneCountInString(value) > MaxDimensionValue {
		return fmt.Errorf("%w: dimension %q is longer than %d characters or not valid text", ErrInvalid, key, MaxDimensionValue)
	}
	if strings.IndexFunc(value, unicode.IsControl) >= 0 {
		return fmt.Errorf("%w: dimension %q carries a control character", ErrInvalid, key)
	}
	return nil
}

func checkDimensions(d map[string]string) error {
	if len(d) > MaxDimensions {
		return fmt.Errorf("%w: an entry carries at most %d dimensions, got %d", ErrInvalid, MaxDimensions, len(d))
	}
	for key, value := range d {
		if err := checkDimensionKey(key); err != nil {
			return err
		}
		if err := checkDimensionValue(key, value); err != nil {
			return err
		}
	}
	return nil
}

func checkDimensionQuery(key, value string) error {
	if err := checkDimensionKey(key); err != nil {
		return err
	}
	return checkDimensionValue(key, value)
}

func carrying(entries []Entry, key, value string) []Entry {
	out := []Entry{}
	for _, e := range entries {
		if v, ok := e.Dimensions[key]; ok && v == value {
			out = append(out, e)
		}
	}
	return out
}

func (m *ledgerStore) entriesCarrying(ctx context.Context, accountID, key, value string) ([]Entry, error) {
	if err := checkDimensionQuery(key, value); err != nil {
		return nil, err
	}
	if _, err := m.accountByID(ctx, m.db, accountID); err != nil {
		return nil, err
	}
	entries, err := m.entriesTouching(ctx, m.db, accountID, 0)
	if err != nil {
		return nil, err
	}
	return carrying(entries, key, value), nil
}

func (m *ledgerStore) ListEntriesByDimension(ctx context.Context, accountID, key, value string, limit int) ([]Entry, error) {
	if limit <= 0 {
		limit = DefaultPage
	}
	entries, err := m.entriesCarrying(ctx, accountID, key, value)
	if err != nil {
		return nil, err
	}
	if len(entries) > limit {
		entries = entries[:limit]
	}
	return entries, nil
}

func (m *ledgerStore) DimensionBalance(ctx context.Context, accountID, key, value string) (int64, error) {
	entries, err := m.entriesCarrying(ctx, accountID, key, value)
	if err != nil {
		return 0, err
	}
	return balanceOf(accountID, entries), nil
}
