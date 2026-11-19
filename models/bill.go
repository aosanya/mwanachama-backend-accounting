package models

type BillStatus string

const (
	BillOpen     BillStatus = "open"
	BillOverdue  BillStatus = "overdue"
	BillSettled  BillStatus = "settled"
	BillReversed BillStatus = "reversed"
)

func IsBillStatus(s BillStatus) bool {
	switch s {
	case BillOpen, BillOverdue, BillSettled, BillReversed:
		return true
	}
	return false
}

type BillInput struct {
	IncomeAccountID     string `json:"income_account_id"`
	ReceivableAccountID string `json:"receivable_account_id"`
	Payer               string `json:"payer,omitempty"`
	PayerActorID        string `json:"payer_actor_id,omitempty"`
	PayerRecordRowID    string `json:"payer_record_row_id,omitempty"`
	Amount              int64  `json:"amount"`
	DueAt               string `json:"due_at"`
	Narration           string `json:"narration,omitempty"`
	BilledBy            string `json:"billed_by"`
}

type Bill struct {
	ID                  string     `json:"id"`
	IncomeAccountID     string     `json:"income_account_id"`
	ReceivableAccountID string     `json:"receivable_account_id"`
	Payer               string     `json:"payer,omitempty"`
	PayerActorID        string     `json:"payer_actor_id,omitempty"`
	PayerRecordRowID    string     `json:"payer_record_row_id,omitempty"`
	Amount              int64      `json:"amount"`
	Settled             int64      `json:"settled"`
	Outstanding         int64      `json:"outstanding"`
	RaisedAt            string     `json:"raised_at"`
	DueAt               string     `json:"due_at"`
	Status              BillStatus `json:"status"`
	DaysOverdue         int64      `json:"days_overdue"`
	Narration           string     `json:"narration,omitempty"`
	BilledBy            string     `json:"billed_by"`
}
