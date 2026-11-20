package accounting

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/aosanya/mwanachama-backend-accounting/models"
)

type (
	Currency       = models.Currency
	CurrencyAmount = models.CurrencyAmount
)

var ErrMixedCurrency = errors.New("accounting: the amounts are in more than one currency")

var ErrMissingRate = errors.New("accounting: no exchange rate for the conversion")

type Option func(*ledgerStore)

func WithDefaultCurrency(code string) Option {
	return func(m *ledgerStore) { m.defaultCurrency = code }
}

func (m *ledgerStore) currencyOf(code string) string {
	if code == "" {
		return m.defaultCurrency
	}
	return code
}

func (m *ledgerStore) entryCurrency(e Entry) string { return m.currencyOf(e.Currency) }

func checkCurrency(code string) error {
	if code == "" || models.IsCurrency(code) {
		return nil
	}
	return fmt.Errorf("%w: currency %q is not a known ISO 4217 code", ErrInvalid, code)
}

func checkDefaultCurrency(code string) error {
	if !models.IsCurrency(code) {
		return fmt.Errorf("default currency %q is not a known ISO 4217 code", code)
	}
	return nil
}

func (m *ledgerStore) balancesByCurrency(accountID string, entries []Entry) map[string]int64 {
	out := map[string]int64{}
	for _, e := range entries {
		out[m.entryCurrency(e)] += balanceOf(accountID, []Entry{e})
	}
	return out
}

func (m *ledgerStore) balanceInCurrency(accountID string, entries []Entry, currency string) int64 {
	var total int64
	for _, e := range entries {
		if m.entryCurrency(e) == currency {
			total += balanceOf(accountID, []Entry{e})
		}
	}
	return total
}

func sortedAmounts(byCurrency map[string]int64) []CurrencyAmount {
	out := make([]CurrencyAmount, 0, len(byCurrency))
	for currency, amount := range byCurrency {
		out = append(out, CurrencyAmount{Currency: currency, Amount: amount})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Currency < out[j].Currency })
	return out
}

func otherAmounts(byCurrency map[string]int64, kept string) []CurrencyAmount {
	rest := map[string]int64{}
	for currency, amount := range byCurrency {
		if currency != kept {
			rest[currency] = amount
		}
	}
	if len(rest) == 0 {
		return nil
	}
	return sortedAmounts(rest)
}

func (m *ledgerStore) mixedCurrencyError(accountID string, byCurrency map[string]int64) error {
	codes := make([]string, 0, len(byCurrency))
	for _, a := range sortedAmounts(byCurrency) {
		codes = append(codes, a.Currency)
	}
	return fmt.Errorf("%w: account %s holds %s", ErrMixedCurrency, accountID, strings.Join(codes, ", "))
}

func (m *ledgerStore) singleBalance(accountID string, entries []Entry) (int64, error) {
	balances := m.balancesByCurrency(accountID, entries)
	if len(balances) > 1 {
		return 0, m.mixedCurrencyError(accountID, balances)
	}
	return balanceOf(accountID, entries), nil
}

func (m *ledgerStore) labelledCurrency(code string) string {
	if code == m.defaultCurrency {
		return ""
	}
	return code
}
