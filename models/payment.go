package models

type PaymentKind string

const (
	PaymentOfBill        PaymentKind = "bill"
	PaymentOfCertificate PaymentKind = "certificate"
)

func IsPaymentKind(k PaymentKind) bool {
	return k == PaymentOfBill || k == PaymentOfCertificate
}

type MarkPaidInput struct {
	BillID        string `json:"billID"`
	CashAccountID string `json:"cash_account_id"`
	Amount        int64  `json:"amount,omitempty"`
	PaidOn        string `json:"paid_on,omitempty"`
	Narration     string `json:"narration,omitempty"`
	Method        string `json:"method,omitempty"`
	Reference     string `json:"reference,omitempty"`
	PaidBy        string `json:"paid_by"`
}

type ReversePaymentInput struct {
	PaymentID  string `json:"paymentID"`
	Narration  string `json:"narration,omitempty"`
	ReversedBy string `json:"reversed_by"`
}

type Payment struct {
	ID               string      `json:"id"`
	Kind             PaymentKind `json:"kind"`
	DebitAccountID   string      `json:"debit_account_id"`
	CreditAccountID  string      `json:"credit_account_id"`
	Amount           int64       `json:"amount"`
	PaidAt           string      `json:"paid_at"`
	PaidBy           string      `json:"paid_by"`
	Narration        string      `json:"narration,omitempty"`
	Method           string      `json:"method,omitempty"`
	Reference        string      `json:"reference,omitempty"`
	BillID           string      `json:"bill_id,omitempty"`
	Payer            string      `json:"payer,omitempty"`
	PayerActorID     string      `json:"payer_actor_id,omitempty"`
	PayerRecordRowID string      `json:"payer_record_row_id,omitempty"`
	CertificateID    string      `json:"certificate_id,omitempty"`
	MilestoneID      string      `json:"milestone_id,omitempty"`
	ContractID       string      `json:"contract_id,omitempty"`
}
