package accounting

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/aosanya/mwanachama-backend-accounting/models"
	"github.com/aosanya/mwanachama-backend-shared/specstore"
	"github.com/google/uuid"
)

type (
	Bill       = models.Bill
	BillInput  = models.BillInput
	BillStatus = models.BillStatus
)

const (
	DocumentBill DocumentKind = "bill"

	uuidLength = 36

	dimensionPayer      = "payer"
	dimensionPayerActor = "payer_actor_id"
	dimensionPayerRow   = "payer_record_row_id"
	dimensionProject    = "project"
)

type BillFilter struct {
	IncomeAccountID string   `query:"income_account_id"`
	Status          string   `query:"status"`
	Payer           string   `query:"payer"`
	Project         string   `query:"project"`
	ActorID         string   `query:"actor_id"`
	RecordRowID     string   `query:"record_row_id"`
	RecordRowIDs    []string `query:"-"`
	AsOf            string   `query:"as_of"`
	Limit           int      `query:"limit"`
}

type BillRepository interface {
	CreateBill(ctx context.Context, in BillInput) (Bill, error)

	ListBills(ctx context.Context, filter BillFilter) ([]Bill, error)

	GetBill(ctx context.Context, id string) (Bill, error)

	UpdateBill(ctx context.Context, in BillUpdate) (Bill, error)
}

func (m *ledgerStore) accountOfType(ctx context.Context, id, field string, want AccountType) (Account, error) {
	a, err := m.accountByID(ctx, m.db, id)
	if err != nil {
		return Account{}, err
	}
	if a.Type != want {
		return Account{}, fmt.Errorf("%w: %s is a %s account, a bill needs a %s account there", ErrInvalid, field, a.Type, want)
	}
	return a, nil
}

func payerDimensions(payer, payerActorID, payerRecordRowID string) map[string]string {
	dimensions := map[string]string{}
	if payer = strings.TrimSpace(payer); payer != "" {
		dimensions[dimensionPayer] = payer
	}
	if payerActorID = strings.TrimSpace(payerActorID); payerActorID != "" {
		dimensions[dimensionPayerActor] = payerActorID
	}
	if payerRecordRowID = strings.TrimSpace(payerRecordRowID); payerRecordRowID != "" {
		dimensions[dimensionPayerRow] = payerRecordRowID
	}
	if len(dimensions) == 0 {
		return nil
	}
	return dimensions
}

func billDimensions(payer, payerActorID, payerRecordRowID, project string) map[string]string {
	dimensions := payerDimensions(payer, payerActorID, payerRecordRowID)
	if project = strings.TrimSpace(project); project != "" {
		if dimensions == nil {
			dimensions = map[string]string{}
		}
		dimensions[dimensionProject] = project
	}
	return dimensions
}

func checkBillProject(project string) error {
	project = strings.TrimSpace(project)
	if project == "" {
		return nil
	}
	return checkDimensionValue("project", project)
}

func checkPayerActorID(payerActorID string) error {
	payerActorID = strings.TrimSpace(payerActorID)
	if payerActorID == "" {
		return nil
	}
	if _, err := uuid.Parse(payerActorID); err != nil || len(payerActorID) != uuidLength {
		return fmt.Errorf("%w: payer_actor_id %q is not an actor id", ErrInvalid, payerActorID)
	}
	return nil
}

func checkPayerRecordRowID(payerRecordRowID string) error {
	payerRecordRowID = strings.TrimSpace(payerRecordRowID)
	if payerRecordRowID == "" {
		return nil
	}
	if _, err := uuid.Parse(payerRecordRowID); err != nil || len(payerRecordRowID) != uuidLength {
		return fmt.Errorf("%w: payer_record_row_id %q is not a record row id", ErrInvalid, payerRecordRowID)
	}
	return nil
}

func checkBillInput(in BillInput) error {
	if err := checkPayerActorID(in.PayerActorID); err != nil {
		return err
	}
	if err := checkPayerRecordRowID(in.PayerRecordRowID); err != nil {
		return err
	}
	if err := checkBillProject(in.Project); err != nil {
		return err
	}
	if strings.TrimSpace(in.IncomeAccountID) == "" {
		return fmt.Errorf("%w: a bill names no income_account_id", ErrInvalid)
	}
	if strings.TrimSpace(in.ReceivableAccountID) == "" {
		return fmt.Errorf("%w: a bill names no receivable_account_id", ErrInvalid)
	}
	if strings.TrimSpace(in.DueAt) == "" {
		return fmt.Errorf("%w: a bill names no due_at", ErrInvalid)
	}
	if strings.TrimSpace(in.BilledBy) == "" {
		return fmt.Errorf("%w: a bill names no billed_by", ErrInvalid)
	}
	return checkInstant("due_at", in.DueAt)
}

func (m *ledgerStore) CreateBill(ctx context.Context, in BillInput) (Bill, error) {
	if err := checkBillInput(in); err != nil {
		return Bill{}, err
	}
	if _, err := m.accountOfType(ctx, in.IncomeAccountID, "income_account_id", AccountTypeIncome); err != nil {
		return Bill{}, err
	}
	if _, err := m.accountOfType(ctx, in.ReceivableAccountID, "receivable_account_id", AccountTypeAsset); err != nil {
		return Bill{}, err
	}
	posted, err := m.Post(ctx, Entry{
		DebitAccountID:  in.ReceivableAccountID,
		CreditAccountID: in.IncomeAccountID,
		Amount:          in.Amount,
		DueAt:           in.DueAt,
		DocumentKind:    DocumentBill,
		DocumentID:      newID(),
		ActorID:         in.BilledBy,
		Narration:       in.Narration,
		Dimensions:      billDimensions(in.Payer, in.PayerActorID, in.PayerRecordRowID, in.Project),
	})
	if err != nil {
		return Bill{}, err
	}
	return m.GetBill(ctx, posted.ID)
}

func (m *ledgerStore) incomeAccountIDs(ctx context.Context) (map[string]bool, error) {
	accounts, err := specstore.List[Account](m.st, m.q(ctx, roleAccount), roleAccount)
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, a := range accounts {
		if a.Type == AccountTypeIncome {
			out[a.ID] = true
		}
	}
	return out, nil
}

func (m *ledgerStore) billEntries(ctx context.Context, filter BillFilter) ([]Entry, error) {
	q := m.q(ctx, roleEntry).
		Where("coalesce(due_at, '') <> '' AND coalesce(reverses_entry_id, '') = '' AND coalesce(settles_entry_id, '') = ''").
		Order("due_at, posted_at, id")
	if filter.IncomeAccountID != "" {
		q = q.Where("credit_account_id = ?", filter.IncomeAccountID)
	}
	entries, err := specstore.List[Entry](m.st, q, roleEntry)
	if err != nil {
		return nil, err
	}
	income, err := m.incomeAccountIDs(ctx)
	if err != nil {
		return nil, err
	}
	bills := make([]Entry, 0, len(entries))
	for _, e := range entries {
		if !income[e.CreditAccountID] {
			continue
		}
		if filter.Payer != "" && e.Dimensions[dimensionPayer] != filter.Payer {
			continue
		}
		if filter.ActorID != "" && e.Dimensions[dimensionPayerActor] != filter.ActorID {
			continue
		}
		if filter.Project != "" && e.Dimensions[dimensionProject] != filter.Project {
			continue
		}
		if !namesAnyRow(e.Dimensions, filter.RecordRowID, filter.RecordRowIDs) {
			continue
		}
		bills = append(bills, e)
	}
	return bills, nil
}

func billStatus(e Entry, reversed bool, outstanding int64, at time.Time) (BillStatus, int64) {
	switch {
	case reversed:
		return models.BillReversed, 0
	case outstanding <= 0:
		return models.BillSettled, 0
	}
	due, err := time.Parse(time.RFC3339Nano, e.DueAt)
	if err == nil && due.Before(at) {
		return models.BillOverdue, int64(at.Sub(due) / day)
	}
	return models.BillOpen, 0
}

func billOf(e Entry, reversed bool, paid int64, at time.Time) Bill {
	outstanding := e.Amount - paid
	if reversed {
		outstanding = 0
	}
	status, daysOverdue := billStatus(e, reversed, outstanding, at)
	return Bill{
		ID:                  e.ID,
		IncomeAccountID:     e.CreditAccountID,
		ReceivableAccountID: e.DebitAccountID,
		Payer:               e.Dimensions[dimensionPayer],
		PayerActorID:        e.Dimensions[dimensionPayerActor],
		PayerRecordRowID:    e.Dimensions[dimensionPayerRow],
		Project:             e.Dimensions[dimensionProject],
		Amount:              e.Amount,
		Settled:             paid,
		Outstanding:         outstanding,
		RaisedAt:            e.PostedAt,
		DueAt:               e.DueAt,
		Status:              status,
		DaysOverdue:         daysOverdue,
		Narration:           e.Narration,
		BilledBy:            e.ActorID,
	}
}

func (m *ledgerStore) billsOf(ctx context.Context, entries []Entry, at time.Time) ([]Bill, error) {
	reversals, err := m.reversedAmong(ctx, entries)
	if err != nil {
		return nil, err
	}
	paid, err := m.settledAmounts(ctx, entries)
	if err != nil {
		return nil, err
	}
	out := make([]Bill, 0, len(entries))
	for _, e := range entries {
		out = append(out, billOf(e, reversals[e.ID], paid[e.ID], at))
	}
	return out, nil
}

func (m *ledgerStore) ListBills(ctx context.Context, filter BillFilter) ([]Bill, error) {
	if filter.Status != "" && !models.IsBillStatus(BillStatus(filter.Status)) {
		return nil, fmt.Errorf("%w: status %q is not open, overdue, settled or reversed", ErrInvalid, filter.Status)
	}
	at, _, err := m.asOf(filter.AsOf)
	if err != nil {
		return nil, err
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = DefaultPage
	}
	entries, err := m.billEntries(ctx, filter)
	if err != nil {
		return nil, err
	}
	all, err := m.billsOf(ctx, entries, at)
	if err != nil {
		return nil, err
	}
	out := make([]Bill, 0, len(all))
	for _, b := range all {
		if filter.Status != "" && string(b.Status) != filter.Status {
			continue
		}
		if len(out) == limit {
			break
		}
		out = append(out, b)
	}
	return out, nil
}

func (m *ledgerStore) GetBill(ctx context.Context, id string) (Bill, error) {
	var e Entry
	if err := m.take(m.q(ctx, roleEntry).Where("id = ?", id), roleEntry, &e, ErrNotFound); err != nil {
		return Bill{}, err
	}
	income, err := m.incomeAccountIDs(ctx)
	if err != nil {
		return Bill{}, err
	}
	if e.DueAt == "" || e.ReversesEntryID != "" || e.SettlesEntryID != "" || !income[e.CreditAccountID] {
		return Bill{}, ErrNotFound
	}
	at, _, err := m.asOf("")
	if err != nil {
		return Bill{}, err
	}
	bills, err := m.billsOf(ctx, []Entry{e}, at)
	if err != nil {
		return Bill{}, err
	}
	return bills[0], nil
}

func namesAnyRow(dimensions map[string]string, recordRowID string, recordRowIDs []string) bool {
	if recordRowID == "" && len(recordRowIDs) == 0 {
		return true
	}
	got := dimensions[dimensionPayerRow]
	if got == "" {
		return false
	}
	if len(recordRowIDs) > 0 {
		return slices.Contains(recordRowIDs, got)
	}
	return got == recordRowID
}
