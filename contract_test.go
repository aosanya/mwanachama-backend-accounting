package accounting_test

import (
	"context"
	"errors"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	accounting "github.com/aosanya/mwanachama-backend-accounting"
)

const (
	pastInstant   = "2000-01-01T00:00:00Z"
	futureInstant = "2999-01-01T00:00:00Z"
)

type contractFixture struct {
	repo      accounting.LedgerRepository
	ward      accounting.Account
	contactor accounting.Account
	retention accounting.Account
}

func newContractFixture(t *testing.T) contractFixture {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open: %v", err)
	}
	s, err := accounting.SpecFor("test")
	if err != nil {
		t.Fatalf("SpecFor: %v", err)
	}
	if err := accounting.Provision(db, s); err != nil {
		t.Fatalf("Provision: %v", err)
	}
	repo, err := accounting.NewLedgerRepository(db, s)
	if err != nil {
		t.Fatalf("NewLedgerRepository: %v", err)
	}
	open := func(category, holder string, kind accounting.AccountType) accounting.Account {
		a, err := repo.OpenAccount(context.Background(), accounting.Account{
			Category: accounting.AccountCategory(category), HolderID: holder, Type: kind,
		})
		if err != nil {
			t.Fatalf("OpenAccount %s: %v", category, err)
		}
		return a
	}
	return contractFixture{
		repo:      repo,
		ward:      open("ward_spending", "ward-3", accounting.AccountTypeExpense),
		contactor: open("contractor", "contractor-1", accounting.AccountTypeLiability),
		retention: open("retention", "contractor-1", accounting.AccountTypeLiability),
	}
}

func (f contractFixture) milestone(t *testing.T, amount int64) accounting.Milestone {
	t.Helper()
	ms, err := f.repo.AddMilestone(context.Background(), accounting.Milestone{
		AccountID: f.ward.ID, ContractID: "contract-7", Name: "Foundations", Amount: amount, DueAt: futureInstant,
	})
	if err != nil {
		t.Fatalf("AddMilestone: %v", err)
	}
	return ms
}

func (f contractFixture) certificate(milestoneID string, amount, bps int64, release string) accounting.Certificate {
	return accounting.Certificate{
		MilestoneID: milestoneID, PayeeAccountID: f.contactor.ID, RetentionAccountID: f.retention.ID,
		Amount: amount, RetentionBps: bps, CertifiedBy: "engineer-1", ReleaseDueAt: release,
	}
}

func balance(t *testing.T, f contractFixture, id string) int64 {
	t.Helper()
	b, err := f.repo.Balance(context.Background(), id)
	if err != nil {
		t.Fatalf("Balance: %v", err)
	}
	return b
}

func TestAddMilestoneStartsPlannedAndRefusesBadInput(t *testing.T) {
	f := newContractFixture(t)
	ms := f.milestone(t, 1000)
	if ms.Status != "planned" || ms.CertifiedAmount != 0 || ms.ID == "" {
		t.Fatalf("milestone = %+v", ms)
	}
	bad := []accounting.Milestone{
		{AccountID: f.ward.ID, ContractID: "c", Name: "n", Amount: 0},
		{AccountID: f.ward.ID, ContractID: "c", Name: "n", Amount: 10, DueAt: "soon"},
		{AccountID: f.ward.ID, ContractID: "", Name: "n", Amount: 10},
	}
	for _, b := range bad {
		if _, err := f.repo.AddMilestone(context.Background(), b); !errors.Is(err, accounting.ErrInvalid) {
			t.Fatalf("AddMilestone(%+v) = %v, want ErrInvalid", b, err)
		}
	}
	if _, err := f.repo.AddMilestone(context.Background(), accounting.Milestone{
		AccountID: "missing", ContractID: "c", Name: "n", Amount: 10,
	}); !errors.Is(err, accounting.ErrNotFound) {
		t.Fatalf("unknown account = %v, want ErrNotFound", err)
	}
	listed, err := f.repo.ListMilestones(context.Background(), f.ward.ID, 0)
	if err != nil || len(listed) != 1 || listed[0].ID != ms.ID {
		t.Fatalf("ListMilestones = %+v, %v", listed, err)
	}
}

func TestCertifyPaymentPostsNetAndRetentionAndTracksStatus(t *testing.T) {
	f := newContractFixture(t)
	ctx := context.Background()
	ms := f.milestone(t, 1000)

	c, err := f.repo.CertifyPayment(ctx, f.certificate(ms.ID, 400, 1000, futureInstant))
	if err != nil {
		t.Fatalf("CertifyPayment: %v", err)
	}
	if c.RetentionAmount != 40 || c.PaymentEntryID == "" || c.RetentionEntryID == "" || c.CertifiedAt == "" {
		t.Fatalf("certificate = %+v", c)
	}
	if got := balance(t, f, f.contactor.ID); got != -360 {
		t.Fatalf("payee balance = %d, want -360", got)
	}
	if got := balance(t, f, f.retention.ID); got != -40 {
		t.Fatalf("retention balance = %d, want -40", got)
	}
	if got := balance(t, f, f.ward.ID); got != 400 {
		t.Fatalf("ward balance = %d, want 400", got)
	}

	part, _ := f.repo.ListMilestones(ctx, f.ward.ID, 0)
	if part[0].Status != "part_certified" || part[0].CertifiedAmount != 400 {
		t.Fatalf("after part = %+v", part[0])
	}

	if _, err := f.repo.CertifyPayment(ctx, f.certificate(ms.ID, 600, 0, "")); err != nil {
		t.Fatalf("second CertifyPayment: %v", err)
	}
	done, _ := f.repo.ListMilestones(ctx, f.ward.ID, 0)
	if done[0].Status != "certified" || done[0].CertifiedAmount != 1000 {
		t.Fatalf("after all = %+v", done[0])
	}
	certs, err := f.repo.ListCertificates(ctx, ms.ID, 0)
	if err != nil || len(certs) != 2 {
		t.Fatalf("ListCertificates = %+v, %v", certs, err)
	}
	byContract, err := f.repo.DimensionBalance(ctx, f.ward.ID, "contract", "contract-7")
	if err != nil || byContract != 1000 {
		t.Fatalf("contract balance = %d, %v", byContract, err)
	}
}

func TestCertifyPaymentRefusesMoreThanTheMilestoneIsWorth(t *testing.T) {
	f := newContractFixture(t)
	ctx := context.Background()
	ms := f.milestone(t, 500)
	if _, err := f.repo.CertifyPayment(ctx, f.certificate(ms.ID, 300, 0, "")); err != nil {
		t.Fatalf("first: %v", err)
	}
	if _, err := f.repo.CertifyPayment(ctx, f.certificate(ms.ID, 201, 0, "")); !errors.Is(err, accounting.ErrOverCertified) {
		t.Fatalf("over = %v, want ErrOverCertified", err)
	}
	if got := balance(t, f, f.contactor.ID); got != -300 {
		t.Fatalf("refused certificate posted money: payee balance %d", got)
	}
}

func TestCertifyPaymentRefusesBadInput(t *testing.T) {
	f := newContractFixture(t)
	ms := f.milestone(t, 500)
	cases := map[string]accounting.Certificate{
		"zero amount":          f.certificate(ms.ID, 0, 0, ""),
		"retention too high":   f.certificate(ms.ID, 100, 5001, futureInstant),
		"negative retention":   f.certificate(ms.ID, 100, -1, ""),
		"retention no release": f.certificate(ms.ID, 100, 500, ""),
		"bad release":          f.certificate(ms.ID, 100, 500, "later"),
		"no certifier":         {MilestoneID: ms.ID, PayeeAccountID: f.contactor.ID, RetentionAccountID: f.retention.ID, Amount: 10},
	}
	for name, c := range cases {
		if _, err := f.repo.CertifyPayment(context.Background(), c); !errors.Is(err, accounting.ErrInvalid) {
			t.Fatalf("%s = %v, want ErrInvalid", name, err)
		}
	}
	if _, err := f.repo.CertifyPayment(context.Background(), f.certificate("missing", 10, 0, "")); !errors.Is(err, accounting.ErrNotFound) {
		t.Fatalf("unknown milestone = %v, want ErrNotFound", err)
	}
}

func TestCertifyPaymentIsAtomicWhenAnAccountIsMissing(t *testing.T) {
	f := newContractFixture(t)
	ctx := context.Background()
	ms := f.milestone(t, 500)
	c := f.certificate(ms.ID, 100, 1000, futureInstant)
	c.RetentionAccountID = "missing"
	if _, err := f.repo.CertifyPayment(ctx, c); !errors.Is(err, accounting.ErrNotFound) {
		t.Fatalf("CertifyPayment = %v, want ErrNotFound", err)
	}
	if got := balance(t, f, f.contactor.ID); got != 0 {
		t.Fatalf("payee balance = %d after a refused certificate, want 0", got)
	}
	listed, _ := f.repo.ListMilestones(ctx, f.ward.ID, 0)
	if listed[0].CertifiedAmount != 0 || listed[0].Status != "planned" {
		t.Fatalf("milestone moved by a refused certificate: %+v", listed[0])
	}
}

func TestReleaseRetentionWaitsForTheDateAndPaysOnce(t *testing.T) {
	f := newContractFixture(t)
	ctx := context.Background()
	ms := f.milestone(t, 1000)

	held, err := f.repo.CertifyPayment(ctx, f.certificate(ms.ID, 1000, 5000, futureInstant))
	if err != nil {
		t.Fatalf("CertifyPayment: %v", err)
	}
	if _, err := f.repo.ReleaseRetention(ctx, held.ID, "treasurer-1"); !errors.Is(err, accounting.ErrRetentionHeld) {
		t.Fatalf("early release = %v, want ErrRetentionHeld", err)
	}
	if got := balance(t, f, f.retention.ID); got != -500 {
		t.Fatalf("retention balance moved by a refused release: %d", got)
	}

	ms2 := f.milestone(t, 1000)
	due, err := f.repo.CertifyPayment(ctx, f.certificate(ms2.ID, 1000, 5000, pastInstant))
	if err != nil {
		t.Fatalf("CertifyPayment due: %v", err)
	}
	released, err := f.repo.ReleaseRetention(ctx, due.ID, "treasurer-1")
	if err != nil {
		t.Fatalf("ReleaseRetention: %v", err)
	}
	if released.ReleasedAt == "" || released.ReleaseEntryID == "" {
		t.Fatalf("released = %+v", released)
	}
	if got := balance(t, f, f.retention.ID); got != -500 {
		t.Fatalf("retention balance = %d, want -500 (one held, one paid out)", got)
	}
	if got := balance(t, f, f.contactor.ID); got != -1500 {
		t.Fatalf("payee balance = %d, want -1500", got)
	}
	if _, err := f.repo.ReleaseRetention(ctx, due.ID, "treasurer-1"); !errors.Is(err, accounting.ErrRetentionReleased) {
		t.Fatalf("second release = %v, want ErrRetentionReleased", err)
	}
}

func TestReleaseRetentionRefusesNothingWithheldAndUnknown(t *testing.T) {
	f := newContractFixture(t)
	ctx := context.Background()
	ms := f.milestone(t, 100)
	c, err := f.repo.CertifyPayment(ctx, f.certificate(ms.ID, 100, 0, ""))
	if err != nil {
		t.Fatalf("CertifyPayment: %v", err)
	}
	if _, err := f.repo.ReleaseRetention(ctx, c.ID, "treasurer-1"); !errors.Is(err, accounting.ErrInvalid) {
		t.Fatalf("nothing withheld = %v, want ErrInvalid", err)
	}
	if _, err := f.repo.ReleaseRetention(ctx, "missing", "treasurer-1"); !errors.Is(err, accounting.ErrNotFound) {
		t.Fatalf("unknown = %v, want ErrNotFound", err)
	}
	if _, err := f.repo.ReleaseRetention(ctx, c.ID, ""); !errors.Is(err, accounting.ErrInvalid) {
		t.Fatalf("no releaser = %v, want ErrInvalid", err)
	}
}
