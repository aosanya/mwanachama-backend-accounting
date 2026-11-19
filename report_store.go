package accounting

import (
	"context"
	"sort"
	"time"

	"github.com/aosanya/mwanachama-backend-accounting/models"
	"github.com/aosanya/mwanachama-backend-shared/specstore"
)

type (
	FinancialReport   = models.FinancialReport
	ReportFigures     = models.ReportFigures
	DepartmentFigures = models.DepartmentFigures
	TypeBalance       = models.TypeBalance
)

type ReportFilter struct {
	Limit int `query:"limit"`
}

type ReportRepository interface {
	ListReports(ctx context.Context, filter ReportFilter) ([]FinancialReport, error)

	GetReport(ctx context.Context, periodStart, periodEnd string) (FinancialReport, error)
}

const reportDateLayout = "2006-01-02"

var reportedTypes = []AccountType{
	AccountTypeAsset, AccountTypeLiability, AccountTypeEquity, AccountTypeIncome, AccountTypeExpense,
}

type reportPeriod struct{ start, end string }

func (m *ledgerStore) allBudgets(ctx context.Context) ([]Budget, error) {
	return specstore.List[Budget](m.st, m.q(ctx, roleBudget).Order("period_start, period_end, set_at, id"), roleBudget)
}

func (m *ledgerStore) closingBalances(ctx context.Context, to time.Time) ([]TypeBalance, error) {
	accounts, err := specstore.List[Account](m.st, m.q(ctx, roleAccount).Order("opened_at"), roleAccount)
	if err != nil {
		return nil, err
	}
	entries, err := specstore.List[Entry](m.st, m.q(ctx, roleEntry), roleEntry)
	if err != nil {
		return nil, err
	}
	before := make([]Entry, 0, len(entries))
	for _, e := range entries {
		at, err := time.Parse(time.RFC3339Nano, orderKey(e))
		if err == nil && at.Before(to) {
			before = append(before, e)
		}
	}
	totals := map[AccountType]int64{}
	for _, a := range accounts {
		totals[a.Type] += balanceOf(a.ID, before)
	}
	out := make([]TypeBalance, 0, len(reportedTypes))
	for _, t := range reportedTypes {
		out = append(out, TypeBalance{Type: t, Balance: totals[t]})
	}
	return out, nil
}

func departmentFigures(lines []BudgetLine) []DepartmentFigures {
	byName := map[string]*DepartmentFigures{}
	for _, l := range lines {
		d := byName[l.Department]
		if d == nil {
			d = &DepartmentFigures{Department: l.Department}
			byName[l.Department] = d
		}
		d.Planned += l.Planned
		d.Spent += l.Spent
		d.Difference = d.Planned - d.Spent
	}
	out := make([]DepartmentFigures, 0, len(byName))
	for _, d := range byName {
		out = append(out, *d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Department < out[j].Department })
	return out
}

func reportName(from, to time.Time) string {
	return "Budget and actuals " + from.UTC().Format(reportDateLayout) + " to " + to.UTC().Format(reportDateLayout)
}

func (m *ledgerStore) reportOf(ctx context.Context, p reportPeriod, budgets []Budget) (FinancialReport, error) {
	from, to, err := parsePeriod(p.start, p.end)
	if err != nil {
		return FinancialReport{}, err
	}
	lines := make([]BudgetLine, 0, len(budgets))
	var figures ReportFigures
	for _, b := range budgets {
		line, err := m.lineOf(ctx, b)
		if err != nil {
			return FinancialReport{}, err
		}
		lines = append(lines, line)
		figures.Planned += line.Planned
		figures.Spent += line.Spent
	}
	figures.Difference = figures.Planned - figures.Spent
	figures.Departments = departmentFigures(lines)
	figures.Balances, err = m.closingBalances(ctx, to)
	if err != nil {
		return FinancialReport{}, err
	}
	return FinancialReport{
		Name:        reportName(from, to),
		PeriodStart: p.start,
		PeriodEnd:   p.end,
		Figures:     figures,
		Lines:       lines,
	}, nil
}

func (m *ledgerStore) ListReports(ctx context.Context, filter ReportFilter) ([]FinancialReport, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = DefaultPage
	}
	budgets, err := m.allBudgets(ctx)
	if err != nil {
		return nil, err
	}
	var order []reportPeriod
	grouped := map[reportPeriod][]Budget{}
	for _, b := range budgets {
		p := reportPeriod{b.PeriodStart, b.PeriodEnd}
		if _, seen := grouped[p]; !seen {
			order = append(order, p)
		}
		grouped[p] = append(grouped[p], b)
	}
	if len(order) > limit {
		order = order[:limit]
	}
	reports := make([]FinancialReport, 0, len(order))
	for _, p := range order {
		report, err := m.reportOf(ctx, p, grouped[p])
		if err != nil {
			return nil, err
		}
		reports = append(reports, report)
	}
	return reports, nil
}

func (m *ledgerStore) GetReport(ctx context.Context, periodStart, periodEnd string) (FinancialReport, error) {
	if _, _, err := parsePeriod(periodStart, periodEnd); err != nil {
		return FinancialReport{}, err
	}
	budgets, err := specstore.List[Budget](m.st,
		m.q(ctx, roleBudget).Where("period_start = ? AND period_end = ?", periodStart, periodEnd).Order("set_at, id"),
		roleBudget)
	if err != nil {
		return FinancialReport{}, err
	}
	if len(budgets) == 0 {
		return FinancialReport{}, ErrNotFound
	}
	return m.reportOf(ctx, reportPeriod{periodStart, periodEnd}, budgets)
}
