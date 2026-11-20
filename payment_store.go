package accounting

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/aosanya/mwanachama-backend-accounting/models"
	"github.com/aosanya/mwanachama-backend-shared/specstore"
)

type (
	Payment       = models.Payment
	PaymentKind   = models.PaymentKind
	MarkPaidInput = models.MarkPaidInput

	ReversePaymentInput = models.ReversePaymentInput
)

const DocumentPayment DocumentKind = "payment"

type PaymentFilter struct {
	Kind         string   `query:"kind"`
	BillID       string   `query:"bill_id"`
	Payer        string   `query:"payer"`
	ActorID      string   `query:"actor_id"`
	RecordRowID  string   `query:"record_row_id"`
	RecordRowIDs []string `query:"-"`
	Limit        int      `query:"limit"`
}

type OverpaymentAllowed func(ctx context.Context) (bool, error)

func (m *ledgerStore) UseOverpaymentAllowed(check OverpaymentAllowed) { m.overpayment = check }

func (m *ledgerStore) overpaymentAllowed(ctx context.Context) bool {
	if m.overpayment == nil {
		return false
	}
	allowed, err := m.overpayment(ctx)
	return err == nil && allowed
}

type PaymentRepository interface {
	MarkBillPaid(ctx context.Context, in MarkPaidInput) (Payment, error)

	ReversePayment(ctx context.Context, in ReversePaymentInput) (Payment, error)

	ListPayments(ctx context.Context, filter PaymentFilter) ([]Payment, error)
}

func (m *ledgerStore) MarkBillPaid(ctx context.Context, in MarkPaidInput) (Payment, error) {
	if strings.TrimSpace(in.BillID) == "" {
		return Payment{}, fmt.Errorf("%w: a payment names no billID", ErrInvalid)
	}
	if strings.TrimSpace(in.CashAccountID) == "" {
		return Payment{}, fmt.Errorf("%w: a payment names no cash_account_id", ErrInvalid)
	}
	if strings.TrimSpace(in.PaidBy) == "" {
		return Payment{}, fmt.Errorf("%w: a payment names no paid_by", ErrInvalid)
	}
	if in.Amount < 0 {
		return Payment{}, fmt.Errorf("%w: amount %d is negative", ErrInvalid, in.Amount)
	}
	if in.PaidOn != "" {
		if err := checkInstant("paid_on", in.PaidOn); err != nil {
			return Payment{}, err
		}
		paidOn, _ := time.Parse(time.RFC3339Nano, strings.TrimSpace(in.PaidOn))
		if now, err := time.Parse(time.RFC3339Nano, m.now()); err == nil && paidOn.After(now) {
			return Payment{}, fmt.Errorf("%w: paid_on %q is in the future", ErrInvalid, in.PaidOn)
		}
	}
	bill, err := m.GetBill(ctx, in.BillID)
	if err != nil {
		return Payment{}, err
	}
	switch bill.Status {
	case models.BillReversed:
		return Payment{}, fmt.Errorf("%w: bill %s was reversed and cannot be paid", ErrInvalid, bill.ID)
	case models.BillSettled:
		return Payment{}, fmt.Errorf("%w: bill %s is already settled", ErrInvalid, bill.ID)
	}
	amount := in.Amount
	if amount == 0 {
		amount = bill.Outstanding
	}
	if amount > bill.Outstanding && !m.overpaymentAllowed(ctx) {
		return Payment{}, fmt.Errorf("%w: amount %d is more than the %d still outstanding on bill %s", ErrInvalid, amount, bill.Outstanding, bill.ID)
	}
	if _, err := m.accountOfType(ctx, in.CashAccountID, "cash_account_id", AccountTypeAsset); err != nil {
		return Payment{}, err
	}
	posted, err := m.Post(ctx, Entry{
		DebitAccountID:  in.CashAccountID,
		CreditAccountID: bill.ReceivableAccountID,
		Amount:          amount,
		Currency:        bill.Currency,
		OccurredAt:      in.PaidOn,
		DocumentKind:    DocumentPayment,
		DocumentID:      newID(),
		ActorID:         in.PaidBy,
		Narration:       in.Narration,
		Dimensions:      paymentDimensions(payerDimensions(bill.Payer, bill.PayerActorID, bill.PayerRecordRowID), in.Method, in.Reference),
		SettlesEntryID:  bill.ID,
	})
	if err != nil {
		return Payment{}, err
	}
	return billPaymentOf(posted), nil
}

const (
	dimensionMethod    = "payment_method"
	dimensionReference = "payment_reference"
)

func paymentDimensions(dimensions map[string]string, method, reference string) map[string]string {
	method, reference = strings.TrimSpace(method), strings.TrimSpace(reference)
	if method == "" && reference == "" {
		return dimensions
	}
	if dimensions == nil {
		dimensions = map[string]string{}
	}
	if method != "" {
		dimensions[dimensionMethod] = method
	}
	if reference != "" {
		dimensions[dimensionReference] = reference
	}
	return dimensions
}

func (m *ledgerStore) ReversePayment(ctx context.Context, in ReversePaymentInput) (Payment, error) {
	if strings.TrimSpace(in.PaymentID) == "" {
		return Payment{}, fmt.Errorf("%w: a reversal names no paymentID", ErrInvalid)
	}
	if strings.TrimSpace(in.ReversedBy) == "" {
		return Payment{}, fmt.Errorf("%w: a reversal names no reversed_by", ErrInvalid)
	}
	found, err := specstore.List[Entry](m.st, m.q(ctx, roleEntry).Where("id = ?", in.PaymentID), roleEntry)
	if err != nil {
		return Payment{}, err
	}
	if len(found) == 0 {
		return Payment{}, ErrNotFound
	}
	original := found[0]
	if original.SettlesEntryID == "" || original.ReversesEntryID != "" {
		return Payment{}, fmt.Errorf("%w: entry %s is not a payment of a bill", ErrInvalid, original.ID)
	}
	if _, err := m.Post(ctx, Entry{
		DebitAccountID:  original.CreditAccountID,
		CreditAccountID: original.DebitAccountID,
		Amount:          original.Amount,
		Currency:        original.Currency,
		DocumentKind:    DocumentPayment,
		DocumentID:      newID(),
		ActorID:         in.ReversedBy,
		Narration:       in.Narration,
		ReversesEntryID: original.ID,
	}); err != nil {
		return Payment{}, err
	}
	original.Currency = m.entryCurrency(original)
	return billPaymentOf(original), nil
}

func billPaymentOf(e Entry) Payment {
	return Payment{
		ID:               e.ID,
		Kind:             models.PaymentOfBill,
		DebitAccountID:   e.DebitAccountID,
		CreditAccountID:  e.CreditAccountID,
		Amount:           e.Amount,
		Currency:         e.Currency,
		PaidAt:           e.PostedAt,
		PaidBy:           e.ActorID,
		Narration:        e.Narration,
		Method:           e.Dimensions[dimensionMethod],
		Reference:        e.Dimensions[dimensionReference],
		BillID:           e.SettlesEntryID,
		Payer:            e.Dimensions[dimensionPayer],
		PayerActorID:     e.Dimensions[dimensionPayerActor],
		PayerRecordRowID: e.Dimensions[dimensionPayerRow],
	}
}

func (m *ledgerStore) billPayments(ctx context.Context, filter PaymentFilter) ([]Payment, error) {
	q := m.q(ctx, roleEntry).Where("coalesce(settles_entry_id, '') <> '' AND coalesce(reverses_entry_id, '') = ''")
	if filter.BillID != "" {
		q = q.Where("settles_entry_id = ?", filter.BillID)
	}
	entries, err := specstore.List[Entry](m.st, q.Order("posted_at, id"), roleEntry)
	if err != nil {
		return nil, err
	}
	reversed, err := m.reversedAmong(ctx, entries)
	if err != nil {
		return nil, err
	}
	billIDs, err := m.billIDsAmong(ctx, entries)
	if err != nil {
		return nil, err
	}
	out := make([]Payment, 0, len(entries))
	for _, e := range entries {
		if reversed[e.ID] || !billIDs[e.SettlesEntryID] {
			continue
		}
		e.Currency = m.entryCurrency(e)
		p := billPaymentOf(e)
		if filter.Payer != "" && p.Payer != filter.Payer {
			continue
		}
		if !namesAnyRow(map[string]string{dimensionPayerRow: p.PayerRecordRowID}, filter.RecordRowID, filter.RecordRowIDs) {
			continue
		}
		if filter.ActorID != "" && p.PayerActorID != filter.ActorID {
			continue
		}
		out = append(out, p)
	}
	return out, nil
}

func (m *ledgerStore) billIDsAmong(ctx context.Context, settling []Entry) (map[string]bool, error) {
	ids := make([]string, 0, len(settling))
	for _, e := range settling {
		ids = append(ids, e.SettlesEntryID)
	}
	if len(ids) == 0 {
		return map[string]bool{}, nil
	}
	targets, err := specstore.List[Entry](m.st, m.q(ctx, roleEntry).Where("id IN ?", ids), roleEntry)
	if err != nil {
		return nil, err
	}
	income, err := m.incomeAccountIDs(ctx)
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, t := range targets {
		if t.DueAt != "" && t.ReversesEntryID == "" && income[t.CreditAccountID] {
			out[t.ID] = true
		}
	}
	return out, nil
}

func (m *ledgerStore) certificatePayments(ctx context.Context) ([]Payment, error) {
	certificates, err := specstore.List[Certificate](m.st, m.q(ctx, roleCertificate).Order("certified_at, id"), roleCertificate)
	if err != nil {
		return nil, err
	}
	milestones, err := specstore.List[Milestone](m.st, m.q(ctx, roleMilestone), roleMilestone)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]Milestone, len(milestones))
	for _, ms := range milestones {
		byID[ms.ID] = ms
	}
	out := make([]Payment, 0, len(certificates))
	for _, c := range certificates {
		ms := byID[c.MilestoneID]
		out = append(out, Payment{
			ID:              c.PaymentEntryID,
			Kind:            models.PaymentOfCertificate,
			DebitAccountID:  ms.AccountID,
			CreditAccountID: c.PayeeAccountID,
			Amount:          c.Amount - c.RetentionAmount,
			PaidAt:          c.CertifiedAt,
			PaidBy:          c.CertifiedBy,
			Narration:       ms.Name,
			CertificateID:   c.ID,
			MilestoneID:     c.MilestoneID,
			ContractID:      ms.ContractID,
		})
	}
	return out, nil
}

func (m *ledgerStore) ListPayments(ctx context.Context, filter PaymentFilter) ([]Payment, error) {
	kind := PaymentKind(filter.Kind)
	if filter.Kind != "" && !models.IsPaymentKind(kind) {
		return nil, fmt.Errorf("%w: kind %q is not bill or certificate", ErrInvalid, filter.Kind)
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = DefaultPage
	}
	var all []Payment
	if kind != models.PaymentOfCertificate {
		bills, err := m.billPayments(ctx, filter)
		if err != nil {
			return nil, err
		}
		all = append(all, bills...)
	}
	if kind != models.PaymentOfBill && filter.BillID == "" && filter.Payer == "" && filter.ActorID == "" && filter.RecordRowID == "" && len(filter.RecordRowIDs) == 0 {
		certificates, err := m.certificatePayments(ctx)
		if err != nil {
			return nil, err
		}
		all = append(all, certificates...)
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].PaidAt < all[j].PaidAt })
	if len(all) > limit {
		all = all[:limit]
	}
	return all, nil
}
