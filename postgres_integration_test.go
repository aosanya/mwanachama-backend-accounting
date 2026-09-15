// postgres_integration_test.go runs the same conformance suite
// TestMemoryRepository runs, against a real Postgres-backed
// entitygraph.DataManager — mwanachama-backend-shared's postgres.Backend —
// instead of MemoryRepository's in-process fake.
//
// Skipped unless POSTGRES_URL is set, mirroring
// mwanachama-backend-taskmanager's own postgres_integration_test.go and
// mwanachama-backend-shared's postgres/backend_test.go: "go test ./..."
// needs no database; set POSTGRES_URL to also run this file (the Makefile's
// test-pg target).
package accounting

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/aosanya/mwanachama-backend-shared/postgres"
)

func applyDDL(ctx context.Context, db *sql.DB, script string) error {
	_, err := db.ExecContext(ctx, script)
	return err
}

// newPostgresRepository opens POSTGRES_URL, creates a scratch set of
// acct_-prefixed tables, seeds+activates DefaultAccountingSchema, and
// returns a ready-to-use LedgerRepository. Skips the calling test if
// POSTGRES_URL is unset. Tables are dropped on cleanup.
func newPostgresRepository(t *testing.T) LedgerRepository {
	t.Helper()
	dsn := os.Getenv("POSTGRES_URL")
	if dsn == "" {
		t.Skip("POSTGRES_URL not set; skipping Postgres integration test (see Makefile's test-pg target)")
	}

	ctx := context.Background()
	db, err := postgres.Open(ctx, postgres.Config{DSN: dsn})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	// The table prefix just needs to be this package's own fixed namespace —
	// same pattern taskmanager's postgres_integration_test.go uses
	// ("workit_"). It must be a valid unquoted SQL identifier fragment: no
	// hyphens.
	tables := postgres.DefaultTableNames("accttest_")
	if err := applyDDL(ctx, db, postgres.DDL(tables)); err != nil {
		t.Fatalf("applying DDL: %v", err)
	}
	t.Cleanup(func() {
		_ = applyDDL(context.Background(), db, postgres.DropDDL(tables))
	})

	backend := postgres.NewBackend(db, tables)

	s := DefaultAccountingSchema()
	if err := backend.SetSchema(ctx, s); err != nil {
		t.Fatalf("SetSchema: %v", err)
	}
	if err := backend.Publish(ctx); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if err := backend.Activate(ctx, 1); err != nil {
		t.Fatalf("Activate: %v", err)
	}

	return NewPostgresRepository(backend)
}

func TestPostgresRepository(t *testing.T) {
	// RunLedgerConformance's subtests run sequentially (none call
	// t.Parallel()), so reusing one table set is safe: each
	// newPostgresRepository call creates fresh tables and t.Cleanup drops
	// them again before the next subtest's call runs.
	RunLedgerConformance(t, func(t *testing.T) LedgerRepository {
		return newPostgresRepository(t)
	})
}
