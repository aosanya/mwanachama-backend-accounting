package accounting_test

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	accounting "github.com/aosanya/mwanachama-backend-accounting"
	"github.com/aosanya/mwanachama-backend-shared/spec"
)

func openDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	return db
}

func shippedSpec(t *testing.T) *spec.Spec {
	t.Helper()
	s, err := accounting.SpecFor("mwanachama")
	if err != nil {
		t.Fatalf("spec: %v", err)
	}
	return s
}

func newLedgerOn(t *testing.T, db *gorm.DB, s *spec.Spec) accounting.LedgerRepository {
	t.Helper()
	if err := accounting.Provision(db, s); err != nil {
		t.Fatalf("provision: %v", err)
	}
	repo, err := accounting.NewLedgerRepository(db, s)
	if err != nil {
		t.Fatalf("new ledger: %v", err)
	}
	return repo
}

func newLedger(t *testing.T) (accounting.LedgerRepository, context.Context) {
	t.Helper()
	return newLedgerOn(t, openDB(t), shippedSpec(t)), context.Background()
}

func openHolder(t *testing.T, r accounting.LedgerRepository, holderID string) accounting.Account {
	t.Helper()
	a, err := r.OpenAccount(context.Background(), accounting.Account{
		Category: "holder", Type: accounting.AccountTypeIncome, HolderID: holderID,
	})
	if err != nil {
		t.Fatalf("OpenAccount(holder %s): %v", holderID, err)
	}
	return a
}

func openPool(t *testing.T, r accounting.LedgerRepository, poolID string) accounting.Account {
	t.Helper()
	a, err := r.OpenAccount(context.Background(), accounting.Account{
		Category: "pool", Type: accounting.AccountTypeAsset, HolderID: poolID,
	})
	if err != nil {
		t.Fatalf("OpenAccount(pool %s): %v", poolID, err)
	}
	return a
}
