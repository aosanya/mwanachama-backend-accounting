package accounting

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/aosanya/mwanachama-backend-accounting/models"
	"github.com/aosanya/mwanachama-backend-shared/specstore"
)

type ConvertedReportRepository interface {
	GetConvertedReport(ctx context.Context, in ConvertedReportInput) (ConvertedReport, error)
}

type leg struct {
	amount int64
	day    string
}

type converter struct {
	reporting models.Currency
	rule      ConversionRule
	book      rateBook
	start     time.Time
	end       time.Time
	used      map[UsedRate]bool
	missing   map[MissingRate]bool
}

func newConverter(reporting models.Currency, rule ConversionRule, book rateBook, start, end time.Time) *converter {
	return &converter{
		reporting: reporting, rule: rule, book: book, start: start, end: end,
		used: map[UsedRate]bool{}, missing: map[MissingRate]bool{},
	}
}

func (c *converter) lacking(from, on string) {
	c.missing[MissingRate{From: from, To: c.reporting.Code, On: on}] = true
}

func (c *converter) apply(amount int64, from models.Currency, rate ExchangeRate, on string) int64 {
	c.used[usedRate(from.Code, c.reporting.Code, on, rate)] = true
	return convertMinor(amount, from, c.reporting, rate)
}

func (c *converter) atDay(amount int64, from models.Currency, day string) (int64, bool) {
	rate, ok := c.book.onDay(from.Code, c.reporting.Code, day)
	if !ok {
		c.lacking(from.Code, day)
		return 0, false
	}
	return c.apply(amount, from, rate, day), true
}

func (c *converter) inMonth(amount int64, from models.Currency, day string) (int64, bool) {
	rate, ok := c.book.inMonth(from.Code, c.reporting.Code, day)
	if !ok {
		c.lacking(from.Code, day[:len(monthLayout)])
		return 0, false
	}
	return c.apply(amount, from, rate, day[:len(monthLayout)]), true
}

func (c *converter) overPeriod(amount int64, from models.Currency) (int64, bool) {
	label := c.start.UTC().Format(dateLayout) + "/" + lastDayOf(c.end)
	rate, ok := c.book.averageOver(from.Code, c.reporting.Code, c.start, c.end)
	if !ok {
		c.lacking(from.Code, label)
		return 0, false
	}
	return c.apply(amount, from, rate, label), true
}

func (c *converter) legs(currency string, legs []leg) (int64, bool) {
	if currency == c.reporting.Code {
		var sum int64
		for _, l := range legs {
			sum += l.amount
		}
		return sum, true
	}
	from, _ := models.CurrencyOf(currency)
	var sum int64
	whole := true
	switch c.rule {
	case RuleRateAtPostingDate, RuleRateOfPostingMonth:
		for _, l := range legs {
			var part int64
			var ok bool
			if c.rule == RuleRateAtPostingDate {
				part, ok = c.atDay(l.amount, from, l.day)
			} else {
				part, ok = c.inMonth(l.amount, from, l.day)
			}
			sum, whole = sum+part, whole && ok
		}
		return sum, whole
	}
	var total int64
	for _, l := range legs {
		total += l.amount
	}
	if c.rule == RuleAverageOverPeriod {
		return c.overPeriod(total, from)
	}
	return c.atDay(total, from, lastDayOf(c.end))
}

func (c *converter) usedRates() []UsedRate {
	out := make([]UsedRate, 0, len(c.used))
	for r := range c.used {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		return a.From+a.On+a.RateID < b.From+b.On+b.RateID
	})
	return out
}

func (c *converter) missingRates() []MissingRate {
	out := make([]MissingRate, 0, len(c.missing))
	for r := range c.missing {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		return a.From+a.On < b.From+b.On
	})
	return out
}

func (m *ledgerStore) legsOf(accountID string, entries []Entry, from, to time.Time) map[string][]leg {
	out := map[string][]leg{}
	for _, e := range entries {
		if !withinPeriod(e, from, to) {
			continue
		}
		out[m.entryCurrency(e)] = append(out[m.entryCurrency(e)], leg{amount: balanceOf(accountID, []Entry{e}), day: dayOf(orderKey(e))})
	}
	return out
}

func originalOf(byCurrency map[string][]leg) map[string]int64 {
	out := map[string]int64{}
	for currency, legs := range byCurrency {
		for _, l := range legs {
			out[currency] += l.amount
		}
	}
	return out
}

func (m *ledgerStore) convertedLine(ctx context.Context, c *converter, b Budget, from, to time.Time) (ConvertedLine, error) {
	entries, err := m.entriesTouching(ctx, m.db, b.AccountID, 0)
	if err != nil {
		return ConvertedLine{}, err
	}
	legsBy := m.legsOf(b.AccountID, entries, from, to)
	outer := c.missing
	c.missing = map[MissingRate]bool{}
	line := ConvertedLine{
		ID: b.ID, Name: b.Name, Department: b.Department, AccountID: b.AccountID,
		Currency: m.currencyOf(b.Currency), Original: sortedAmounts(originalOf(legsBy)),
	}
	line.Planned = b.Amount
	if line.Currency != c.reporting.Code {
		budgeted, _ := models.CurrencyOf(line.Currency)
		line.Planned, _ = c.atDay(b.Amount, budgeted, lastDayOf(to))
	}
	for currency, legs := range legsBy {
		part, _ := c.legs(currency, legs)
		line.Spent += part
	}
	line.Left = line.Planned - line.Spent
	line.Missing = c.missingRates()
	if len(line.Missing) == 0 {
		line.Missing = nil
	}
	for r := range c.missing {
		outer[r] = true
	}
	c.missing = outer
	return line, nil
}

func (m *ledgerStore) GetConvertedReport(ctx context.Context, in ConvertedReportInput) (ConvertedReport, error) {
	reporting, ok := models.CurrencyOf(in.ReportingCurrency)
	if !ok {
		return ConvertedReport{}, fmt.Errorf("%w: reporting_currency %q is not a known ISO 4217 code", ErrInvalid, in.ReportingCurrency)
	}
	if !models.IsConversionRule(in.Rule) {
		return ConvertedReport{}, fmt.Errorf("%w: rule %q is not posting_date, posting_month, period_average or report_date", ErrInvalid, in.Rule)
	}
	from, to, err := parsePeriod(in.PeriodStart, in.PeriodEnd)
	if err != nil {
		return ConvertedReport{}, err
	}
	rows, err := m.allRates(ctx)
	if err != nil {
		return ConvertedReport{}, err
	}
	budgets, err := specstore.List[Budget](m.st,
		m.q(ctx, roleBudget).Where("period_start = ? AND period_end = ?", in.PeriodStart, in.PeriodEnd).Order("set_at, id"),
		roleBudget)
	if err != nil {
		return ConvertedReport{}, err
	}
	if len(budgets) == 0 {
		return ConvertedReport{}, ErrNotFound
	}
	c := newConverter(reporting, in.Rule, rateBook(liveRates(rows)), from, to)
	report := ConvertedReport{
		PeriodStart: in.PeriodStart, PeriodEnd: in.PeriodEnd, ReportingCurrency: reporting.Code, Rule: in.Rule,
		Lines: make([]ConvertedLine, 0, len(budgets)),
	}
	originals := map[string]int64{}
	for _, b := range budgets {
		line, err := m.convertedLine(ctx, c, b, from, to)
		if err != nil {
			return ConvertedReport{}, err
		}
		report.Lines = append(report.Lines, line)
		report.Planned += line.Planned
		report.Spent += line.Spent
		for _, o := range line.Original {
			originals[o.Currency] += o.Amount
		}
	}
	report.Difference = report.Planned - report.Spent
	report.Original = sortedAmounts(originals)
	report.ExchangeDifferences, err = m.exchangeDifferences(ctx, c, from, to)
	if err != nil {
		return ConvertedReport{}, err
	}
	report.Rates = c.usedRates()
	report.Missing = c.missingRates()
	report.Complete = len(report.Missing) == 0
	return report, nil
}

func (m *ledgerStore) exchangeDifferences(ctx context.Context, c *converter, from, to time.Time) ([]ExchangeDifference, error) {
	settling, err := specstore.List[Entry](m.st,
		m.q(ctx, roleEntry).Where("coalesce(settles_entry_id, '') <> '' AND coalesce(reverses_entry_id, '') = ''").Order("posted_at, id"),
		roleEntry)
	if err != nil {
		return nil, err
	}
	reversed, err := m.reversedAmong(ctx, settling)
	if err != nil {
		return nil, err
	}
	out := []ExchangeDifference{}
	for _, p := range settling {
		currency := m.entryCurrency(p)
		if reversed[p.ID] || currency == c.reporting.Code || !withinPeriod(p, from, to) {
			continue
		}
		var bill Entry
		if err := m.take(m.q(ctx, roleEntry).Where("id = ?", p.SettlesEntryID), roleEntry, &bill, ErrNotFound); err != nil {
			return nil, err
		}
		cur, _ := models.CurrencyOf(currency)
		atBilling, billingKnown := c.atDay(p.Amount, cur, dayOf(orderKey(bill)))
		atPayment, paymentKnown := c.atDay(p.Amount, cur, dayOf(orderKey(p)))
		if !billingKnown || !paymentKnown {
			continue
		}
		out = append(out, differenceOf(bill, p, currency, atBilling, atPayment))
	}
	return out, nil
}

func differenceOf(bill, payment Entry, currency string, atBilling, atPayment int64) ExchangeDifference {
	difference := atPayment - atBilling
	outcome := "none"
	switch {
	case difference < 0:
		outcome = "loss"
	case difference > 0:
		outcome = "gain"
	}
	return ExchangeDifference{
		BillID: bill.ID, PaymentID: payment.ID, Currency: currency, Paid: payment.Amount,
		AtBilling: atBilling, AtPayment: atPayment, Difference: difference, Outcome: outcome,
	}
}

func describeMissing(missing []MissingRate) string {
	parts := make([]string, 0, len(missing))
	for _, r := range missing {
		parts = append(parts, fmt.Sprintf("no rate for %s to %s on %s", r.From, r.To, r.On))
	}
	return strings.Join(parts, "; ")
}

type reportToSend struct {
	Figures  ReportFigures
	Lines    []BudgetLine
	Currency string
	Rule     ConversionRule
	Rates    []UsedRate
	Original []CurrencyAmount
}

func checkConversionAsked(currency string, rule ConversionRule) error {
	if currency == "" && rule == "" {
		return nil
	}
	if currency == "" || rule == "" {
		return fmt.Errorf("%w: a converted report names both reporting_currency and rule", ErrInvalid)
	}
	return nil
}

func (m *ledgerStore) reportToSend(ctx context.Context, in UpwardReportInput) (reportToSend, error) {
	if in.ReportingCurrency == "" {
		report, err := m.reportForProject(ctx, in.PeriodStart, in.PeriodEnd, strings.TrimSpace(in.Project))
		return reportToSend{Figures: report.Figures, Lines: report.Lines}, err
	}
	if strings.TrimSpace(in.Project) != "" {
		return reportToSend{}, fmt.Errorf("%w: a report cut to a project is not converted yet", ErrInvalid)
	}
	converted, err := m.GetConvertedReport(ctx, ConvertedReportInput{
		PeriodStart: in.PeriodStart, PeriodEnd: in.PeriodEnd, ReportingCurrency: in.ReportingCurrency, Rule: in.Rule,
	})
	if err != nil {
		return reportToSend{}, err
	}
	if !converted.Complete {
		return reportToSend{}, fmt.Errorf("%w: %s", ErrMissingRate, describeMissing(converted.Missing))
	}
	lines := make([]BudgetLine, 0, len(converted.Lines))
	for _, l := range converted.Lines {
		lines = append(lines, BudgetLine{
			ID: l.ID, Name: l.Name, Department: l.Department, AccountID: l.AccountID,
			PeriodStart: in.PeriodStart, PeriodEnd: in.PeriodEnd, Planned: l.Planned, Spent: l.Spent, Currency: converted.ReportingCurrency,
		})
	}
	figures := ReportFigures{
		Planned: converted.Planned, Spent: converted.Spent, Difference: converted.Difference,
		Departments: departmentFigures(lines),
	}
	return reportToSend{
		Figures: figures, Lines: lines, Currency: converted.ReportingCurrency, Rule: converted.Rule,
		Rates: converted.Rates, Original: converted.Original,
	}, nil
}
