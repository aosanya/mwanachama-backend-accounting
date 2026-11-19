package models

type Tender struct {
	ID          string `json:"id"`
	BudgetID    string `json:"budget_id"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	ClosesAt    string `json:"closes_at,omitempty"`
	CreatedBy   string `json:"created_by"`
	CreatedAt   string `json:"created_at"`
}

type TenderView struct {
	Tender
	QuoteCount   int   `json:"quote_count"`
	LowestAmount int64 `json:"lowest_amount"`
}

type Quote struct {
	ID         string `json:"id"`
	TenderID   string `json:"tender_id"`
	Supplier   string `json:"supplier"`
	Amount     int64  `json:"amount"`
	Note       string `json:"note,omitempty"`
	ValidUntil string `json:"valid_until,omitempty"`
	QuotedBy   string `json:"quoted_by"`
	QuotedAt   string `json:"quoted_at"`
}
