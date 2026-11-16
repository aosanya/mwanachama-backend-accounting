package models

type MilestoneStatus string

const (
	MilestonePlanned       MilestoneStatus = "planned"
	MilestonePartCertified MilestoneStatus = "part_certified"
	MilestoneCertified     MilestoneStatus = "certified"
)

type Milestone struct {
	ID              string          `json:"id"`
	AccountID       string          `json:"account_id"`
	ContractID      string          `json:"contract_id"`
	Name            string          `json:"name"`
	Amount          int64           `json:"amount"`
	DueAt           string          `json:"due_at,omitempty"`
	Status          MilestoneStatus `json:"status"`
	CertifiedAmount int64           `json:"certified_amount"`
	CreatedAt       string          `json:"created_at"`
}

type Certificate struct {
	ID                 string `json:"id"`
	MilestoneID        string `json:"milestone_id"`
	PayeeAccountID     string `json:"payee_account_id"`
	RetentionAccountID string `json:"retention_account_id"`
	Amount             int64  `json:"amount"`
	RetentionBps       int64  `json:"retention_bps"`
	RetentionAmount    int64  `json:"retention_amount"`
	PaymentEntryID     string `json:"payment_entry_id"`
	RetentionEntryID   string `json:"retention_entry_id,omitempty"`
	CertifiedBy        string `json:"certified_by"`
	CertifiedAt        string `json:"certified_at"`
	ReleaseDueAt       string `json:"release_due_at,omitempty"`
	ReleasedAt         string `json:"released_at,omitempty"`
	ReleaseEntryID     string `json:"release_entry_id,omitempty"`
}
