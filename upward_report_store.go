package accounting

import (
	"context"
	"fmt"
	"strings"

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
	sent := UpwardReport{
		Kind: in.Kind, PeriodStart: in.PeriodStart, PeriodEnd: in.PeriodEnd, SentBy: in.SentBy,
		Recipient:            strings.TrimSpace(in.Recipient),
		RecipientRecordRowID: strings.TrimSpace(in.RecipientRecordRowID),
		Reference:            strings.TrimSpace(in.Reference),
	}
	if err := m.checks(roleUpwardReport, sent); err != nil {
		return UpwardReport{}, err
	}
	report, err := m.GetReport(ctx, in.PeriodStart, in.PeriodEnd)
	if err != nil {
		return UpwardReport{}, err
	}
	sent.ID = newID()
	sent.Planned = report.Figures.Planned
	sent.Spent = report.Figures.Spent
	sent.Difference = report.Figures.Difference
	sent.Departments = report.Figures.Departments
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
	return specstore.List[UpwardReport](m.st, q, roleUpwardReport)
}
