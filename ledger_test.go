package accounting

import (
	"errors"
	"testing"
)

// TestMemoryRepository runs the shared conformance suite against
// MemoryRepository. The same suite runs against Postgres in
// postgres_integration_test.go — both backends must agree.
func TestMemoryRepository(t *testing.T) {
	RunLedgerConformance(t, func(*testing.T) LedgerRepository { return NewMemoryRepository() })
}

func TestAccountValidateRejectsUnknownType(t *testing.T) {
	if err := (Account{Kind: "pool", Type: "bogus", HolderID: "p"}).Validate(); !errors.Is(err, ErrInvalid) {
		t.Fatalf("want ErrInvalid for unknown type, got %v", err)
	}
}

func TestAccountValidateRejectsEmptyKindOrHolder(t *testing.T) {
	if err := (Account{Kind: "", Type: AccountTypeAsset, HolderID: "p"}).Validate(); !errors.Is(err, ErrInvalid) {
		t.Fatalf("want ErrInvalid for empty kind, got %v", err)
	}
	if err := (Account{Kind: "pool", Type: AccountTypeAsset, HolderID: ""}).Validate(); !errors.Is(err, ErrInvalid) {
		t.Fatalf("want ErrInvalid for empty holder, got %v", err)
	}
}
