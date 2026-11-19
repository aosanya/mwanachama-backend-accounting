package models

type DepartmentFigures struct {
	Department string `json:"department"`
	Planned    int64  `json:"planned"`
	Spent      int64  `json:"spent"`
	Difference int64  `json:"difference"`
}

type TypeBalance struct {
	Type    AccountType `json:"type"`
	Balance int64       `json:"balance"`
}

type ReportFigures struct {
	Planned     int64               `json:"planned"`
	Spent       int64               `json:"spent"`
	Difference  int64               `json:"difference"`
	Departments []DepartmentFigures `json:"departments"`
	Balances    []TypeBalance       `json:"balances"`
}

type FinancialReport struct {
	Name        string        `json:"name"`
	PeriodStart string        `json:"period_start"`
	PeriodEnd   string        `json:"period_end"`
	Figures     ReportFigures `json:"figures"`
	Lines       []BudgetLine  `json:"lines"`
}
