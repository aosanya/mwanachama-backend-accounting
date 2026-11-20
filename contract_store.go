package accounting

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/aosanya/mwanachama-backend-accounting/models"
	"github.com/aosanya/mwanachama-backend-shared/specstore"
)

const (
	MaxRetentionBps = 5000

	basisPoints = 10000

	DocumentPaymentCertificate DocumentKind = "payment_certificate"
	DocumentRetentionRelease   DocumentKind = "retention_release"

	dimensionContract  = "contract"
	dimensionMilestone = "milestone"
)

type ContractRepository interface {
	AddMilestone(ctx context.Context, m Milestone) (Milestone, error)

	ListMilestones(ctx context.Context, accountID string, limit int) ([]Milestone, error)

	CertifyPayment(ctx context.Context, c Certificate) (Certificate, error)

	ListCertificates(ctx context.Context, milestoneID string, limit int) ([]Certificate, error)

	ReleaseRetention(ctx context.Context, certificateID, releasedBy string) (Certificate, error)
}

func (m *ledgerStore) milestoneByID(ctx context.Context, tx *gorm.DB, id string) (Milestone, error) {
	var out Milestone
	err := m.take(m.on(ctx, tx, roleMilestone).Where("id = ?", id), roleMilestone, &out, ErrNotFound)
	return out, err
}

func (m *ledgerStore) certificateByID(ctx context.Context, tx *gorm.DB, id string) (Certificate, error) {
	var out Certificate
	err := m.take(m.on(ctx, tx, roleCertificate).Where("id = ?", id), roleCertificate, &out, ErrNotFound)
	return out, err
}

func checkInstant(name, raw string) error {
	if _, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(raw)); err != nil {
		return fmt.Errorf("%w: %s %q is not an RFC 3339 instant", ErrInvalid, name, raw)
	}
	return nil
}

func (m *ledgerStore) AddMilestone(ctx context.Context, ms Milestone) (Milestone, error) {
	ms.Status = models.MilestonePlanned
	if err := m.checks(roleMilestone, ms); err != nil {
		return Milestone{}, err
	}
	if ms.Amount <= 0 {
		return Milestone{}, fmt.Errorf("%w: milestone amount %d is not positive", ErrInvalid, ms.Amount)
	}
	if ms.DueAt != "" {
		if err := checkInstant("due_at", ms.DueAt); err != nil {
			return Milestone{}, err
		}
	}
	if err := checkCurrency(ms.Currency); err != nil {
		return Milestone{}, err
	}
	ms.Currency = m.currencyOf(ms.Currency)
	if _, err := m.accountByID(ctx, m.db, ms.AccountID); err != nil {
		return Milestone{}, err
	}
	ms.ID = newID()
	ms.CertifiedAmount = 0
	ms.CreatedAt = m.now()
	row, err := encode(m.st.Object(roleMilestone), ms)
	if err != nil {
		return Milestone{}, err
	}
	if err := m.q(ctx, roleMilestone).Create(row).Error; err != nil {
		return Milestone{}, err
	}
	return ms, nil
}

func (m *ledgerStore) ListMilestones(ctx context.Context, accountID string, limit int) ([]Milestone, error) {
	if limit <= 0 {
		limit = DefaultPage
	}
	if _, err := m.accountByID(ctx, m.db, accountID); err != nil {
		return nil, err
	}
	q := m.q(ctx, roleMilestone).Where("account_id = ?", accountID).Order("created_at").Limit(limit)
	return specstore.List[Milestone](m.st, q, roleMilestone)
}

func (m *ledgerStore) ListCertificates(ctx context.Context, milestoneID string, limit int) ([]Certificate, error) {
	if limit <= 0 {
		limit = DefaultPage
	}
	if _, err := m.milestoneByID(ctx, m.db, milestoneID); err != nil {
		return nil, err
	}
	q := m.q(ctx, roleCertificate).Where("milestone_id = ?", milestoneID).Order("certified_at").Limit(limit)
	return specstore.List[Certificate](m.st, q, roleCertificate)
}

func checkCertificate(c Certificate) error {
	if c.Amount <= 0 {
		return fmt.Errorf("%w: certified amount %d is not positive", ErrInvalid, c.Amount)
	}
	if c.RetentionBps < 0 || c.RetentionBps > MaxRetentionBps {
		return fmt.Errorf("%w: retention_bps %d is outside 0 to %d", ErrInvalid, c.RetentionBps, MaxRetentionBps)
	}
	if c.ReleaseDueAt != "" {
		return checkInstant("release_due_at", c.ReleaseDueAt)
	}
	if c.RetentionBps > 0 {
		return fmt.Errorf("%w: a retention needs a release_due_at", ErrInvalid)
	}
	return nil
}

func milestoneStatus(certified, amount int64) models.MilestoneStatus {
	switch {
	case certified >= amount:
		return models.MilestoneCertified
	case certified > 0:
		return models.MilestonePartCertified
	}
	return models.MilestonePlanned
}

func (m *ledgerStore) CertifyPayment(ctx context.Context, c Certificate) (Certificate, error) {
	if err := m.checks(roleCertificate, c); err != nil {
		return Certificate{}, err
	}
	if err := checkCertificate(c); err != nil {
		return Certificate{}, err
	}
	var out Certificate
	err := m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		ms, err := m.milestoneByID(ctx, tx, c.MilestoneID)
		if err != nil {
			return err
		}
		if c.Amount > ms.Amount-ms.CertifiedAmount {
			return ErrOverCertified
		}
		c.ID = newID()
		c.Currency = m.currencyOf(ms.Currency)
		c.RetentionAmount = c.Amount * c.RetentionBps / basisPoints
		dimensions := map[string]string{dimensionContract: ms.ContractID, dimensionMilestone: ms.ID}
		payment, err := m.postIn(ctx, tx, Entry{
			DebitAccountID:  ms.AccountID,
			CreditAccountID: c.PayeeAccountID,
			Amount:          c.Amount - c.RetentionAmount,
			Currency:        c.Currency,
			DocumentKind:    DocumentPaymentCertificate,
			DocumentID:      c.ID,
			ActorID:         c.CertifiedBy,
			Narration:       ms.Name,
			Dimensions:      dimensions,
		})
		if err != nil {
			return err
		}
		c.PaymentEntryID = payment.ID
		if c.RetentionAmount > 0 {
			held, err := m.postIn(ctx, tx, Entry{
				DebitAccountID:  ms.AccountID,
				CreditAccountID: c.RetentionAccountID,
				Amount:          c.RetentionAmount,
				Currency:        c.Currency,
				DocumentKind:    DocumentPaymentCertificate,
				DocumentID:      c.ID,
				ActorID:         c.CertifiedBy,
				Narration:       ms.Name,
				Dimensions:      dimensions,
			})
			if err != nil {
				return err
			}
			c.RetentionEntryID = held.ID
		} else {
			c.ReleaseDueAt = ""
		}
		c.CertifiedAt = m.now()
		row, err := encode(m.st.Object(roleCertificate), c)
		if err != nil {
			return err
		}
		if err := m.on(ctx, tx, roleCertificate).Create(row).Error; err != nil {
			return err
		}
		ms.CertifiedAmount += c.Amount
		ms.Status = milestoneStatus(ms.CertifiedAmount, ms.Amount)
		mrow, err := encode(m.st.Object(roleMilestone), ms)
		if err != nil {
			return err
		}
		if err := m.on(ctx, tx, roleMilestone).Where("id = ?", ms.ID).
			Updates(mutableColumns(m.st.Object(roleMilestone), mrow)).Error; err != nil {
			return err
		}
		out = c
		return nil
	})
	if err != nil {
		return Certificate{}, err
	}
	return out, nil
}

func (m *ledgerStore) ReleaseRetention(ctx context.Context, certificateID, releasedBy string) (Certificate, error) {
	if strings.TrimSpace(releasedBy) == "" {
		return Certificate{}, fmt.Errorf("%w: a release names no releasing person", ErrInvalid)
	}
	var out Certificate
	err := m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		c, err := m.certificateByID(ctx, tx, certificateID)
		if err != nil {
			return err
		}
		if c.RetentionAmount <= 0 {
			return fmt.Errorf("%w: the certificate withheld nothing", ErrInvalid)
		}
		if c.ReleasedAt != "" {
			return ErrRetentionReleased
		}
		now := m.now()
		at, err := time.Parse(time.RFC3339Nano, now)
		if err != nil {
			return err
		}
		due, err := time.Parse(time.RFC3339Nano, c.ReleaseDueAt)
		if err != nil {
			return err
		}
		if at.Before(due) {
			return ErrRetentionHeld
		}
		ms, err := m.milestoneByID(ctx, tx, c.MilestoneID)
		if err != nil {
			return err
		}
		paid, err := m.postIn(ctx, tx, Entry{
			DebitAccountID:  c.RetentionAccountID,
			CreditAccountID: c.PayeeAccountID,
			Amount:          c.RetentionAmount,
			Currency:        m.currencyOf(c.Currency),
			DocumentKind:    DocumentRetentionRelease,
			DocumentID:      c.ID,
			ActorID:         releasedBy,
			Narration:       ms.Name,
			Dimensions:      map[string]string{dimensionContract: ms.ContractID, dimensionMilestone: ms.ID},
		})
		if err != nil {
			return err
		}
		c.ReleasedAt = now
		c.ReleaseEntryID = paid.ID
		row, err := encode(m.st.Object(roleCertificate), c)
		if err != nil {
			return err
		}
		if err := m.on(ctx, tx, roleCertificate).Where("id = ?", c.ID).
			Updates(mutableColumns(m.st.Object(roleCertificate), row)).Error; err != nil {
			return err
		}
		out = c
		return nil
	})
	if err != nil {
		return Certificate{}, err
	}
	return out, nil
}
