package models

type Account struct {
	ID       string          `json:"id"`
	Code     string          `json:"code,omitempty"`
	Name     string          `json:"name,omitempty"`
	Category AccountCategory `json:"category"`
	Type     AccountType     `json:"type"`
	HolderID string          `json:"holder_id"`
	OpenedAt string          `json:"opened_at"`
	ClosedAt string          `json:"closed_at,omitempty"`
}

type Entry struct {
	ID              string       `json:"id"`
	PostedAt        string       `json:"posted_at"`
	OccurredAt      string       `json:"occurred_at,omitempty"`
	DebitAccountID  string       `json:"debit_account_id"`
	CreditAccountID string       `json:"credit_account_id"`
	Amount          int64        `json:"amount"`
	DocumentKind    DocumentKind `json:"document_kind"`
	DocumentID      string       `json:"document_id"`
	ActorID         string       `json:"actor_id"`
	Narration       string       `json:"narration,omitempty"`
	ReversesEntryID string       `json:"reverses_entry_id,omitempty"`
}
