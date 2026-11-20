package accounting

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aosanya/mwanachama-backend-accounting/models"
	"github.com/aosanya/mwanachama-backend-shared/specstore"
	"github.com/google/uuid"
)

const roleUpwardReport = "upward_report"

type (
	UpwardReport      = models.UpwardReport
	UpwardReportKind  = models.UpwardReportKind
	UpwardReportInput = models.UpwardReportInput
)

type UpwardReportFilter struct {
	RecipientRecordRowID string `query:"recipient_record_row_id"`
	Project              string `query:"project"`
	Limit                int    `query:"limit"`
}

type UpwardReportRepository interface {
	SendUpwardReport(ctx context.Context, in UpwardReportInput) (UpwardReport, error)

	ListUpwardReports(ctx context.Context, filter UpwardReportFilter) ([]UpwardReport, error)
}

func checkRecipientRecordRowID(recipientRecordRowID string) error {
	recipientRecordRowID = strings.TrimSpace(recipientRecordRowID)
	if recipientRecordRowID == "" {
		return nil
	}
	if _, err := uuid.Parse(recipientRecordRowID); err != nil || len(recipientRecordRowID) != uuidLength {
		return fmt.Errorf("%w: recipient_record_row_id %q is not a record row id", ErrInvalid, recipientRecordRowID)
	}
	return nil
}

func (m *ledgerStore) reportForProject(ctx context.Context, periodStart, periodEnd, project string) (FinancialReport, error) {
	if project == "" {
		return m.GetReport(ctx, periodStart, periodEnd)
	}
	from, to, err := parsePeriod(periodStart, periodEnd)
	if err != nil {
		return FinancialReport{}, err
	}
	budgets, err := specstore.List[Budget](m.st,
		m.q(ctx, roleBudget).Where("period_start = ? AND period_end = ? AND department = ?", periodStart, periodEnd, project).Order("set_at, id"),
		roleBudget)
	if err != nil {
		return FinancialReport{}, err
	}
	if len(budgets) == 0 {
		return FinancialReport{}, ErrNotFound
	}
	lines := make([]BudgetLine, 0, len(budgets))
	var figures ReportFigures
	for _, b := range budgets {
		line, err := m.projectLineOf(ctx, b, project, from, to)
		if err != nil {
			return FinancialReport{}, err
		}
		lines = append(lines, line)
		figures.Planned += line.Planned
		figures.Spent += line.Spent
	}
	figures.Difference = figures.Planned - figures.Spent
	figures.Departments = departmentFigures(lines)
	return FinancialReport{
		Name:        reportName(from, to),
		PeriodStart: periodStart,
		PeriodEnd:   periodEnd,
		Figures:     figures,
		Lines:       lines,
	}, nil
}

func (m *ledgerStore) projectLineOf(ctx context.Context, b Budget, project string, from, to time.Time) (BudgetLine, error) {
	line, err := m.lineOf(ctx, b)
	if err != nil {
		return BudgetLine{}, err
	}
	entries, err := m.entriesTouching(ctx, m.db, b.AccountID, 0)
	if err != nil {
		return BudgetLine{}, err
	}
	line.Spent = balanceOf(b.AccountID, inPeriod(carrying(entries, dimensionProject, project), from, to))
	return line, nil
}

func (m *ledgerStore) SendUpwardReport(ctx context.Context, in UpwardReportInput) (UpwardReport, error) {
	if err := m.keepsRole(roleUpwardReport, "upward reports"); err != nil {
		return UpwardReport{}, err
	}
	if strings.TrimSpace(in.SentBy) == "" {
		return UpwardReport{}, fmt.Errorf("%w: an upward report names no sent_by", ErrInvalid)
	}
	if err := checkRecipientRecordRowID(in.RecipientRecordRowID); err != nil {
		return UpwardReport{}, err
	}
	if err := checkConversionAsked(in.ReportingCurrency, in.Rule); err != nil {
		return UpwardReport{}, err
	}
	sent := UpwardReport{
		Kind: in.Kind, PeriodStart: in.PeriodStart, PeriodEnd: in.PeriodEnd, SentBy: in.SentBy,
		Recipient:            strings.TrimSpace(in.Recipient),
		RecipientRecordRowID: strings.TrimSpace(in.RecipientRecordRowID),
		Reference:            strings.TrimSpace(in.Reference),
		Project:              strings.TrimSpace(in.Project),
	}
	if err := checkBillProject(sent.Project); err != nil {
		return UpwardReport{}, err
	}
	if err := m.checks(roleUpwardReport, sent); err != nil {
		return UpwardReport{}, err
	}
	report, err := m.reportToSend(ctx, in)
	if err != nil {
		return UpwardReport{}, err
	}
	sent.ID = newID()
	sent.Planned = report.Figures.Planned
	sent.Spent = report.Figures.Spent
	sent.Difference = report.Figures.Difference
	sent.Departments = report.Figures.Departments
	sent.ReportingCurrency, sent.Rule = report.Currency, report.Rule
	sent.Rates, sent.Original = report.Rates, report.Original
	if in.Kind == models.UpwardBudgetAndActuals {
		sent.Lines = report.Lines
	}
	sent.SentAt = m.now()
	row, err := encode(m.st.Object(roleUpwardReport), sent)
	if err != nil {
		return UpwardReport{}, err
	}
	if err := m.q(ctx, roleUpwardReport).Create(row).Error; err != nil {
		return UpwardReport{}, err
	}
	return sent, nil
}

func (m *ledgerStore) ListUpwardReports(ctx context.Context, filter UpwardReportFilter) ([]UpwardReport, error) {
	if err := m.keepsRole(roleUpwardReport, "upward reports"); err != nil {
		return nil, err
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = DefaultPage
	}
	q := m.q(ctx, roleUpwardReport).Order("sent_at desc, id desc").Limit(limit)
	if row := strings.TrimSpace(filter.RecipientRecordRowID); row != "" {
		q = q.Where("recipient_record_row_id = ?", row)
	}
	if project := strings.TrimSpace(filter.Project); project != "" {
		q = q.Where("project = ?", project)
	}
	return specstore.List[UpwardReport](m.st, q, roleUpwardReport)
}
