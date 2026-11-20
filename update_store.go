package accounting

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aosanya/mwanachama-backend-accounting/models"
)

type (
	BillUpdate   = models.BillUpdate
	TenderUpdate = models.TenderUpdate
	QuoteUpdate  = models.QuoteUpdate
)

func (m *ledgerStore) UpdateBill(ctx context.Context, in BillUpdate) (Bill, error) {
	if in.Amount <= 0 {
		return Bill{}, fmt.Errorf("%w: a bill amount %d is not positive", ErrInvalid, in.Amount)
	}
	if err := checkPayerActorID(in.PayerActorID); err != nil {
		return Bill{}, err
	}
	if err := checkPayerRecordRowID(in.PayerRecordRowID); err != nil {
		return Bill{}, err
	}
	if err := checkBillProject(in.Project); err != nil {
		return Bill{}, err
	}
	if strings.TrimSpace(in.DueAt) == "" {
		return Bill{}, fmt.Errorf("%w: a bill names no due_at", ErrInvalid)
	}
	if err := checkInstant("due_at", in.DueAt); err != nil {
		return Bill{}, err
	}
	bill, err := m.GetBill(ctx, in.ID)
	if err != nil {
		return Bill{}, err
	}
	switch bill.Status {
	case models.BillReversed:
		return Bill{}, fmt.Errorf("%w: bill %s was reversed and can no longer be changed", ErrLocked, bill.ID)
	case models.BillSettled:
		return Bill{}, fmt.Errorf("%w: bill %s is settled and can no longer be changed", ErrLocked, bill.ID)
	}
	if bill.Settled > 0 && in.Amount != bill.Amount {
		return Bill{}, fmt.Errorf("%w: bill %s already has a payment of %d, so its amount stays %d", ErrLocked, bill.ID, bill.Settled, bill.Amount)
	}
	var entry Entry
	if err := m.take(m.q(ctx, roleEntry).Where("id = ?", bill.ID), roleEntry, &entry, ErrNotFound); err != nil {
		return Bill{}, err
	}
	entry.Amount = in.Amount
	entry.DueAt = in.DueAt
	entry.Narration = in.Narration
	entry.Dimensions = billDimensions(in.Payer, in.PayerActorID, in.PayerRecordRowID, in.Project)
	if err := m.rewrite(ctx, roleEntry, bill.ID, entry, "amount", "due_at", "narration", "dimensions"); err != nil {
		return Bill{}, err
	}
	return m.GetBill(ctx, bill.ID)
}

func (m *ledgerStore) tenderClosed(t Tender) bool {
	if t.ClosesAt == "" {
		return false
	}
	closes, err := time.Parse(time.RFC3339Nano, t.ClosesAt)
	if err != nil {
		return false
	}
	now, err := time.Parse(time.RFC3339Nano, m.now())
	if err != nil {
		return false
	}
	return !now.Before(closes)
}

func (m *ledgerStore) UpdateTender(ctx context.Context, in TenderUpdate) (Tender, error) {
	if err := m.keepsProcurement(); err != nil {
		return Tender{}, err
	}
	current, err := m.tenderByID(ctx, in.ID)
	if err != nil {
		return Tender{}, err
	}
	if m.tenderClosed(current) {
		return Tender{}, fmt.Errorf("%w: tender %s closed on %s and can no longer be changed", ErrLocked, current.ID, current.ClosesAt)
	}
	current.Title = strings.TrimSpace(in.Title)
	current.Description = in.Description
	current.ClosesAt = in.ClosesAt
	if err := m.checks(roleTender, current); err != nil {
		return Tender{}, err
	}
	if current.ClosesAt != "" {
		if err := checkInstant("closes_at", current.ClosesAt); err != nil {
			return Tender{}, err
		}
	}
	if err := m.rewrite(ctx, roleTender, current.ID, current, "title", "description", "closes_at"); err != nil {
		return Tender{}, err
	}
	return current, nil
}

func (m *ledgerStore) UpdateQuote(ctx context.Context, in QuoteUpdate) (Quote, error) {
	if err := m.keepsProcurement(); err != nil {
		return Quote{}, err
	}
	var current Quote
	if err := m.take(m.q(ctx, roleQuote).Where("id = ?", in.ID), roleQuote, &current, ErrNotFound); err != nil {
		return Quote{}, err
	}
	tender, err := m.tenderByID(ctx, current.TenderID)
	if err != nil {
		return Quote{}, err
	}
	if m.tenderClosed(tender) {
		return Quote{}, fmt.Errorf("%w: tender %s closed on %s, so its quotes can no longer be changed", ErrLocked, tender.ID, tender.ClosesAt)
	}
	current.Supplier = strings.TrimSpace(in.Supplier)
	current.Amount = in.Amount
	current.Note = in.Note
	current.ValidUntil = in.ValidUntil
	if err := m.checks(roleQuote, current); err != nil {
		return Quote{}, err
	}
	if current.Amount <= 0 {
		return Quote{}, fmt.Errorf("%w: quote amount %d is not positive", ErrInvalid, current.Amount)
	}
	if current.ValidUntil != "" {
		if err := checkInstant("valid_until", current.ValidUntil); err != nil {
			return Quote{}, err
		}
	}
	if err := m.rewrite(ctx, roleQuote, current.ID, current, "supplier", "amount", "note", "valid_until"); err != nil {
		return Quote{}, err
	}
	return current, nil
}

func (m *ledgerStore) rewrite(ctx context.Context, role, id string, record any, columns ...string) error {
	row, err := encode(m.st.Object(role), record)
	if err != nil {
		return err
	}
	changed := make(map[string]any, len(columns))
	for _, column := range columns {
		changed[column] = row[column]
	}
	return m.q(ctx, role).Where("id = ?", id).Updates(changed).Error
}
