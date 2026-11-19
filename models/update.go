package models

type BillUpdate struct {
	ID               string `json:"id"`
	Payer            string `json:"payer,omitempty"`
	PayerActorID     string `json:"payer_actor_id,omitempty"`
	PayerRecordRowID string `json:"payer_record_row_id,omitempty"`
	Amount           int64  `json:"amount"`
	DueAt            string `json:"due_at"`
	Narration        string `json:"narration,omitempty"`
}

type TenderUpdate struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	ClosesAt    string `json:"closes_at,omitempty"`
}

type QuoteUpdate struct {
	ID         string `json:"id"`
	Supplier   string `json:"supplier"`
	Amount     int64  `json:"amount"`
	Note       string `json:"note,omitempty"`
	ValidUntil string `json:"valid_until,omitempty"`
}
