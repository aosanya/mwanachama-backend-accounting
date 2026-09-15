package accounting

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
)

// MemoryRepository is a reference LedgerRepository, in-process and
// unpersisted — a hand-rolled fake in the same spirit as taskmanager's
// fakeDataManager, standing in for the Postgres/entity-graph backend (W4 on
// the task board) so the ledger's business rules can be built and tested
// before storage wiring lands. Safe for concurrent use.
type MemoryRepository struct {
	mu       sync.Mutex
	accounts map[string]Account
	entries  map[string]Entry
}

// NewMemoryRepository returns an empty repository.
func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		accounts: make(map[string]Account),
		entries:  make(map[string]Entry),
	}
}

func (r *MemoryRepository) findByIdentity(a Account) (Account, bool) {
	for _, existing := range r.accounts {
		if existing.Kind == a.Kind && existing.HolderID == a.HolderID {
			return existing, true
		}
	}
	return Account{}, false
}

// OpenAccount implements LedgerRepository.
func (r *MemoryRepository) OpenAccount(_ context.Context, a Account) (Account, error) {
	if err := a.Validate(); err != nil {
		return Account{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	if existing, ok := r.findByIdentity(a); ok {
		return existing, nil
	}
	a.ID = uuid.NewString()
	a.OpenedAt = time.Now().UTC()
	a.ClosedAt = nil
	r.accounts[a.ID] = a
	return a, nil
}

// GetAccount implements LedgerRepository.
func (r *MemoryRepository) GetAccount(_ context.Context, id string) (Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.accounts[id]
	if !ok {
		return Account{}, ErrNotFound
	}
	return a, nil
}

// ListAccounts implements LedgerRepository.
func (r *MemoryRepository) ListAccounts(_ context.Context, limit int) ([]Account, error) {
	if limit <= 0 {
		limit = DefaultPage
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	out := make([]Account, 0, len(r.accounts))
	for _, a := range r.accounts {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].OpenedAt.Before(out[j].OpenedAt) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// balanceLocked folds one account's entries. Caller must hold r.mu.
func (r *MemoryRepository) balanceLocked(accountID string) int64 {
	var total int64
	for _, e := range r.entries {
		switch accountID {
		case e.ToAccountID:
			total += e.Amount
		case e.FromAccountID:
			total -= e.Amount
		}
	}
	return total
}

// CloseAccount implements LedgerRepository.
func (r *MemoryRepository) CloseAccount(_ context.Context, id string) (Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	a, ok := r.accounts[id]
	if !ok {
		return Account{}, ErrNotFound
	}
	if a.ClosedAt != nil {
		return Account{}, ErrClosed
	}
	if r.balanceLocked(id) != 0 {
		return Account{}, ErrNonZeroBalance
	}
	now := time.Now().UTC()
	a.ClosedAt = &now
	r.accounts[id] = a
	return a, nil
}

// Post implements LedgerRepository.
func (r *MemoryRepository) Post(_ context.Context, e Entry) (Entry, error) {
	if err := e.Validate(); err != nil {
		return Entry{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	from, ok := r.accounts[e.FromAccountID]
	if !ok {
		return Entry{}, ErrNotFound
	}
	to, ok := r.accounts[e.ToAccountID]
	if !ok {
		return Entry{}, ErrNotFound
	}
	if from.ClosedAt != nil || to.ClosedAt != nil {
		return Entry{}, ErrClosed
	}
	if e.ReversesEntryID != "" {
		if _, ok := r.entries[e.ReversesEntryID]; !ok {
			return Entry{}, ErrNotFound
		}
	}

	e.ID = uuid.NewString()
	e.PostedAt = time.Now().UTC()
	r.entries[e.ID] = e
	return e, nil
}

// Balance implements LedgerRepository.
func (r *MemoryRepository) Balance(_ context.Context, accountID string) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.accounts[accountID]; !ok {
		return 0, ErrNotFound
	}
	return r.balanceLocked(accountID), nil
}

// entryOrderKey is OccurredAt, coalescing to PostedAt (G271's rule, carried
// over from the merchandise ledger).
func entryOrderKey(e Entry) time.Time {
	if e.OccurredAt != nil {
		return *e.OccurredAt
	}
	return e.PostedAt
}

// ListEntries implements LedgerRepository.
func (r *MemoryRepository) ListEntries(_ context.Context, accountID string, limit int) ([]Entry, error) {
	if limit <= 0 {
		limit = DefaultPage
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.accounts[accountID]; !ok {
		return nil, ErrNotFound
	}
	out := make([]Entry, 0)
	for _, e := range r.entries {
		if e.FromAccountID == accountID || e.ToAccountID == accountID {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return entryOrderKey(out[i]).Before(entryOrderKey(out[j])) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
