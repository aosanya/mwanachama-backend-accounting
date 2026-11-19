package models

type DayEntryStatus string

const (
	DayEntrySubmitted DayEntryStatus = "submitted"
	DayEntryApproved  DayEntryStatus = "approved"
	DayEntryRejected  DayEntryStatus = "rejected"
)

type DayAllowance struct {
	ID          string `json:"id"`
	Project     string `json:"project"`
	Consultant  string `json:"consultant"`
	Position    string `json:"position,omitempty"`
	DaysBought  int64  `json:"days_bought"`
	Rate        int64  `json:"rate"`
	PeriodStart string `json:"period_start,omitempty"`
	PeriodEnd   string `json:"period_end,omitempty"`
	ReplacesID  string `json:"replaces_id,omitempty"`
	Reason      string `json:"reason,omitempty"`
	SetBy       string `json:"set_by"`
	SetAt       string `json:"set_at"`
}

type DayEntry struct {
	ID          string         `json:"id"`
	Project     string         `json:"project"`
	Consultant  string         `json:"consultant"`
	WorkedOn    string         `json:"worked_on"`
	Days        int64          `json:"days"`
	Description string         `json:"description,omitempty"`
	Status      DayEntryStatus `json:"status"`
	SubmittedBy string         `json:"submitted_by"`
	SubmittedAt string         `json:"submitted_at"`
	DecidedBy   string         `json:"decided_by,omitempty"`
	DecidedAt   string         `json:"decided_at,omitempty"`
}

type DaysLine struct {
	Consultant string `json:"consultant"`
	Position   string `json:"position,omitempty"`
	Rate       int64  `json:"rate"`
	Bought     int64  `json:"bought"`
	Submitted  int64  `json:"submitted"`
	Approved   int64  `json:"approved"`
	Left       int64  `json:"left"`
	FeeCeiling int64  `json:"fee_ceiling"`
	Fee        int64  `json:"fee"`
	FeeLeft    int64  `json:"fee_left"`
}

type DaysBalance struct {
	Project string     `json:"project"`
	Lines   []DaysLine `json:"lines"`
	Total   DaysLine   `json:"total"`
}
