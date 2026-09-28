package accounting_test

import (
	"context"
	"testing"
	"time"

	"gorm.io/gorm"

	accounting "github.com/aosanya/mwanachama-backend-accounting"
	"github.com/aosanya/mwanachama-backend-accounting/models"
)

const legacyEntities = `create table acct_entities (
	id text primary key,
	type_id text not null,
	properties text not null default '{}',
	unique_key text,
	created_at text not null,
	updated_at text not null,
	deleted boolean not null default false,
	deleted_at text
)`

func seedLegacy(t *testing.T, db *gorm.DB, id, typeID, createdAt, properties string, deleted bool) {
	t.Helper()
	err := db.Exec(`insert into acct_entities
		(id, type_id, properties, created_at, updated_at, deleted)
		values (?, ?, ?, ?, ?, ?)`,
		id, typeID, properties, createdAt, createdAt, deleted).Error
	if err != nil {
		t.Fatalf("seed %s: %v", id, err)
	}
}

func TestProvisionAdoptsTheEntityGraphLedger(t *testing.T) {
	db := openDB(t)
	if err := db.Exec(legacyEntities).Error; err != nil {
		t.Fatalf("create legacy table: %v", err)
	}

	seedLegacy(t, db, "acct-pool", "account", "2026-01-01T00:00:00.000000000Z",
		`{"account_kind":"pool","account_type":"asset","holder_id":"pool-1"}`, false)
	seedLegacy(t, db, "acct-holder", "account", "2026-01-02T00:00:00.000000000Z",
		`{"account_kind":"holder","account_type":"income","holder_id":"holder-1","closed_at":"2026-05-01T00:00:00Z"}`, false)
	seedLegacy(t, db, "acct-gone", "account", "2026-01-03T00:00:00.000000000Z",
		`{"account_kind":"stale","account_type":"asset","holder_id":"holder-9"}`, true)
	seedLegacy(t, db, "entry-1", "entry", "2026-02-01T00:00:00.000000000Z",
		`{"from_account_id":"acct-pool","to_account_id":"acct-holder","amount":1000,`+
			`"document_kind":"contribution","document_id":"doc-1","actor_id":"actor-1",`+
			`"detail":"a posting","occurred_at":"2026-01-31T00:00:00Z"}`, false)

	s := shippedSpec(t)
	r := newLedgerOn(t, db, s)
	ctx := context.Background()

	accounts, err := r.ListAccounts(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(accounts) != 2 {
		t.Fatalf("adopted %d accounts, want 2 — a soft-deleted entity must not come across", len(accounts))
	}

	pool, err := r.GetAccount(ctx, "acct-pool")
	if err != nil {
		t.Fatalf("the adopted account kept its id: %v", err)
	}
	if pool.Category != "pool" {
		t.Errorf("account_kind became category %q, want \"pool\"", pool.Category)
	}
	if pool.Type != accounting.AccountTypeAsset {
		t.Errorf("type = %q, want asset", pool.Type)
	}
	if pool.OpenedAt == "" {
		t.Error("opened_at did not come across from created_at")
	}

	holder, err := r.GetAccount(ctx, "acct-holder")
	if err != nil {
		t.Fatal(err)
	}
	if holder.ClosedAt == "" {
		t.Error("closed_at did not come across")
	}
	if _, err := time.Parse(models.TimeLayout, holder.ClosedAt); err != nil {
		t.Errorf("adopted closed_at %q is not in this module's layout: %v", holder.ClosedAt, err)
	}

	entries, err := r.ListEntries(ctx, "acct-holder", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("adopted %d entries, want 1", len(entries))
	}
	e := entries[0]
	if e.DebitAccountID != "acct-holder" {
		t.Errorf("debit_account_id = %q, want the old to_account_id (acct-holder)", e.DebitAccountID)
	}
	if e.CreditAccountID != "acct-pool" {
		t.Errorf("credit_account_id = %q, want the old from_account_id (acct-pool)", e.CreditAccountID)
	}
	if e.Amount != 1000 {
		t.Errorf("amount = %d, want 1000", e.Amount)
	}
	if e.Narration != "a posting" {
		t.Errorf("narration = %q, want the old detail", e.Narration)
	}

	// The balances the entitygraph ledger read must survive the move: the
	// account that gained is still the one that gained.
	balance, err := r.Balance(ctx, "acct-holder")
	if err != nil {
		t.Fatal(err)
	}
	if balance != 1000 {
		t.Fatalf("adopted balance = %d, want 1000 — the debit/credit sides have been swapped", balance)
	}
}

func TestProvisionIsIdempotentAndDoesNotReadoptOverLiveRows(t *testing.T) {
	db := openDB(t)
	if err := db.Exec(legacyEntities).Error; err != nil {
		t.Fatalf("create legacy table: %v", err)
	}
	seedLegacy(t, db, "acct-pool", "account", "2026-01-01T00:00:00.000000000Z",
		`{"account_kind":"pool","account_type":"asset","holder_id":"pool-1"}`, false)

	s := shippedSpec(t)
	r := newLedgerOn(t, db, s)

	if err := accounting.Provision(db, s); err != nil {
		t.Fatalf("second provision: %v", err)
	}

	accounts, err := r.ListAccounts(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(accounts) != 1 {
		t.Fatalf("after a second provision there are %d accounts, want 1", len(accounts))
	}
}

func TestProvisionWithNoLegacyTableIsFine(t *testing.T) {
	r, ctx := newLedger(t)
	accounts, err := r.ListAccounts(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(accounts) != 0 {
		t.Fatalf("a fresh ledger holds %d accounts, want 0", len(accounts))
	}
}
