package models

type UpwardReportKind string

const (
	UpwardBudgetAndActuals     UpwardReportKind = "budget_and_actuals"
	UpwardSpendingByDepartment UpwardReportKind = "spending_by_department"
)

type UpwardReportInput struct {
	PeriodStart string           `json:"period_start"`
	PeriodEnd   string           `json:"period_end"`
	Kind        UpwardReportKind `json:"kind"`
	SentBy      string           `json:"sent_by"`

	Recipient            string `json:"recipient,omitempty"`
	RecipientRecordRowID string `json:"recipient_record_row_id,omitempty"`
	Reference            string `json:"reference,omitempty"`
}

type UpwardReport struct {
	ID          string              `json:"id"`
	Kind        UpwardReportKind    `json:"kind"`
	PeriodStart string              `json:"period_start"`
	PeriodEnd   string              `json:"period_end"`
	Planned     int64               `json:"planned"`
	Spent       int64               `json:"spent"`
	Difference  int64               `json:"difference"`
	Departments []DepartmentFigures `json:"departments"`
	Lines       []BudgetLine        `json:"lines,omitempty"`
	SentBy      string              `json:"sent_by"`
	SentAt      string              `json:"sent_at"`

	Recipient            string `json:"recipient,omitempty"`
	RecipientRecordRowID string `json:"recipient_record_row_id,omitempty"`
	Reference            string `json:"reference,omitempty"`
}
