package accounting

import (
	"math/big"
	"time"

	"github.com/aosanya/mwanachama-backend-accounting/models"
)

type (
	ConversionRule       = models.ConversionRule
	MissingRate          = models.MissingRate
	UsedRate             = models.UsedRate
	ConvertedLine        = models.ConvertedLine
	ConvertedReport      = models.ConvertedReport
	ConvertedReportInput = models.ConvertedReportInput
	ExchangeDifference   = models.ExchangeDifference
)

const (
	RuleRateAtPostingDate  = models.RuleRateAtPostingDate
	RuleRateOfPostingMonth = models.RuleRateOfPostingMonth
	RuleAverageOverPeriod  = models.RuleAverageOverPeriod
	RuleRateAtReportDate   = models.RuleRateAtReportDate
)

const monthLayout = "2006-01"

func liveRates(rows []ExchangeRate) []ExchangeRate {
	replaced := map[string]bool{}
	for _, r := range rows {
		if r.ReplacesID != "" {
			replaced[r.ReplacesID] = true
		}
	}
	out := make([]ExchangeRate, 0, len(rows))
	for _, r := range rows {
		if !replaced[r.ID] {
			out = append(out, r)
		}
	}
	return out
}

type rateBook []ExchangeRate

func (b rateBook) joins(r ExchangeRate, from, to string) bool {
	return r.Base == from && r.Counter == to || r.Base == to && r.Counter == from
}

func (b rateBook) onDay(from, to, day string) (ExchangeRate, bool) {
	return b.latest(func(r ExchangeRate) bool {
		return b.joins(r, from, to) && r.Kind != models.RateMonthly && r.AppliesOn == day
	})
}

func (b rateBook) inMonth(from, to, day string) (ExchangeRate, bool) {
	month := day[:len(monthLayout)]
	return b.latest(func(r ExchangeRate) bool {
		return b.joins(r, from, to) && r.Kind == models.RateMonthly && r.AppliesOn[:len(monthLayout)] == month
	})
}

func (b rateBook) latest(match func(ExchangeRate) bool) (ExchangeRate, bool) {
	var found ExchangeRate
	ok := false
	for _, r := range b {
		if match(r) {
			found, ok = r, true
		}
	}
	return found, ok
}

func (b rateBook) averageOver(from, to string, start, end time.Time) (ExchangeRate, bool) {
	sum := new(big.Int)
	var count int64
	var sample ExchangeRate
	for _, r := range b {
		if !b.joins(r, from, to) || r.Kind != models.RateDaily {
			continue
		}
		at, err := time.Parse(dateLayout, r.AppliesOn)
		if err != nil || at.Before(start) || !at.Before(end) {
			continue
		}
		sum.Add(sum, big.NewInt(r.Rate))
		count++
		sample = r
	}
	if count == 0 {
		return ExchangeRate{}, false
	}
	mean := roundedQuotient(sum, big.NewInt(count))
	return ExchangeRate{Base: sample.Base, Counter: sample.Counter, Rate: mean.Int64(), Kind: models.AverageRateKind}, true
}

func roundedQuotient(num, den *big.Int) *big.Int {
	negative := num.Sign() < 0
	abs := new(big.Int).Abs(num)
	twice := new(big.Int).Mul(abs, big.NewInt(2))
	twice.Add(twice, den)
	quotient := twice.Div(twice, new(big.Int).Mul(den, big.NewInt(2)))
	if negative {
		quotient.Neg(quotient)
	}
	return quotient
}

func pow10(n int) *big.Int {
	return new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(n)), nil)
}

func convertMinor(amount int64, from, to models.Currency, rate ExchangeRate) int64 {
	num := new(big.Int).Mul(big.NewInt(amount), pow10(to.Decimals))
	den := pow10(from.Decimals)
	if rate.Base == to.Code {
		num.Mul(num, big.NewInt(models.RateScale))
		den.Mul(den, big.NewInt(rate.Rate))
	} else {
		num.Mul(num, big.NewInt(rate.Rate))
		den.Mul(den, big.NewInt(models.RateScale))
	}
	return roundedQuotient(num, den).Int64()
}

func dayOf(instant string) string {
	at, err := time.Parse(time.RFC3339Nano, instant)
	if err != nil {
		return ""
	}
	return at.UTC().Format(dateLayout)
}

func lastDayOf(end time.Time) string {
	return end.Add(-time.Nanosecond).UTC().Format(dateLayout)
}

func usedRate(from, to, on string, r ExchangeRate) UsedRate {
	return UsedRate{From: from, To: to, On: on, Rate: r.Rate, Kind: r.Kind, Source: r.Source, RateID: r.ID}
}
