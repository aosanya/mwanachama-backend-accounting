package accounting

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/aosanya/mwanachama-backend-shared/entitygraph"
)

// PostgresRepository implements LedgerRepository on top of
// mwanachama-backend-shared's entitygraph.DataManager — the real storage
// this package is meant to ship with (W4 on the task board), as opposed to
// MemoryRepository's in-process fake.
//
// entitygraph.DataManager is single-tenant (the real deployment has exactly
// one ORG_SLUG, hardcoded), so there is nothing left to scope per instance.
type PostgresRepository struct {
	dm entitygraph.DataManager
}

// NewPostgresRepository returns a LedgerRepository backed by dm. The caller
// must have already published and activated DefaultAccountingSchema
// (SetSchema → Publish → Activate) — this constructor does not do it, the
// same division of responsibility taskmanager's NewTaskManager leaves to its
// own callers/tests.
func NewPostgresRepository(dm entitygraph.DataManager) *PostgresRepository {
	return &PostgresRepository{dm: dm}
}

func accountFromEntity(e entitygraph.Entity) Account {
	a := Account{
		ID:       e.ID,
		Kind:     AccountKind(entitygraph.StringProp(e.Properties, propAccountKind)),
		Type:     AccountType(entitygraph.StringProp(e.Properties, propAccountType)),
		HolderID: entitygraph.StringProp(e.Properties, propHolderID),
		OpenedAt: e.CreatedAt,
	}
	if raw, ok := e.Properties[propClosedAt]; ok {
		if s, ok := raw.(string); ok && s != "" {
			if t, err := time.Parse(time.RFC3339, s); err == nil {
				a.ClosedAt = &t
			}
		}
	}
	return a
}

func entryFromEntity(e entitygraph.Entity) Entry {
	entry := Entry{
		ID:              e.ID,
		PostedAt:        e.CreatedAt,
		FromAccountID:   entitygraph.StringProp(e.Properties, propFromAccountID),
		ToAccountID:     entitygraph.StringProp(e.Properties, propToAccountID),
		Amount:          entitygraph.Int64Prop(e.Properties, propAmount),
		DocumentKind:    DocumentKind(entitygraph.StringProp(e.Properties, propDocumentKind)),
		DocumentID:      entitygraph.StringProp(e.Properties, propDocumentID),
		ActorID:         entitygraph.StringProp(e.Properties, propActorID),
		Detail:          entitygraph.StringProp(e.Properties, propDetail),
		ReversesEntryID: entitygraph.StringProp(e.Properties, propReversesEntryID),
	}
	if s := entitygraph.StringProp(e.Properties, propOccurredAt); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			entry.OccurredAt = &t
		}
	}
	return entry
}

func (r *PostgresRepository) getAccountEntity(ctx context.Context, id string) (entitygraph.Entity, error) {
	e, err := r.dm.GetEntity(ctx, id)
	if err != nil {
		if errors.Is(err, entitygraph.ErrEntityNotFound) {
			return entitygraph.Entity{}, ErrNotFound
		}
		return entitygraph.Entity{}, err
	}
	if e.TypeID != TypeAccount {
		return entitygraph.Entity{}, ErrNotFound
	}
	return e, nil
}

// OpenAccount implements LedgerRepository.
func (r *PostgresRepository) OpenAccount(ctx context.Context, a Account) (Account, error) {
	if err := a.Validate(); err != nil {
		return Account{}, err
	}

	e, err := r.dm.UpsertEntity(ctx, entitygraph.CreateEntityRequest{
		TypeID: TypeAccount,
		Properties: map[string]any{
			propAccountKind: string(a.Kind),
			propHolderID:    a.HolderID,
			propAccountType: string(a.Type),
		},
	})
	if err != nil {
		return Account{}, err
	}
	return accountFromEntity(e), nil
}

// GetAccount implements LedgerRepository.
func (r *PostgresRepository) GetAccount(ctx context.Context, id string) (Account, error) {
	e, err := r.getAccountEntity(ctx, id)
	if err != nil {
		return Account{}, err
	}
	return accountFromEntity(e), nil
}

// ListAccounts implements LedgerRepository. entitygraph.EntityFilter carries
// no limit/offset, so every account is fetched in full and paged in Go —
// the same self-limiting MemoryRepository already does.
func (r *PostgresRepository) ListAccounts(ctx context.Context, limit int) ([]Account, error) {
	if limit <= 0 {
		limit = DefaultPage
	}

	entities, err := r.dm.ListEntities(ctx, entitygraph.EntityFilter{TypeID: TypeAccount})
	if err != nil {
		return nil, err
	}
	out := make([]Account, 0, len(entities))
	for _, e := range entities {
		out = append(out, accountFromEntity(e))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].OpenedAt.Before(out[j].OpenedAt) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// CloseAccount implements LedgerRepository.
func (r *PostgresRepository) CloseAccount(ctx context.Context, id string) (Account, error) {
	e, err := r.getAccountEntity(ctx, id)
	if err != nil {
		return Account{}, err
	}
	current := accountFromEntity(e)
	if current.ClosedAt != nil {
		return Account{}, ErrClosed
	}
	balance, err := r.Balance(ctx, id)
	if err != nil {
		return Account{}, err
	}
	if balance != 0 {
		return Account{}, ErrNonZeroBalance
	}

	now := time.Now().UTC()
	updated, err := r.dm.UpdateEntity(ctx, id, entitygraph.UpdateEntityRequest{
		Properties: map[string]any{propClosedAt: now.Format(time.RFC3339)},
	})
	if err != nil {
		if errors.Is(err, entitygraph.ErrEntityNotFound) {
			return Account{}, ErrNotFound
		}
		return Account{}, err
	}
	return accountFromEntity(updated), nil
}

// Post implements LedgerRepository.
func (r *PostgresRepository) Post(ctx context.Context, e Entry) (Entry, error) {
	if err := e.Validate(); err != nil {
		return Entry{}, err
	}

	from, err := r.getAccountEntity(ctx, e.FromAccountID)
	if err != nil {
		return Entry{}, err
	}
	to, err := r.getAccountEntity(ctx, e.ToAccountID)
	if err != nil {
		return Entry{}, err
	}
	if accountFromEntity(from).ClosedAt != nil || accountFromEntity(to).ClosedAt != nil {
		return Entry{}, ErrClosed
	}
	if e.ReversesEntryID != "" {
		reversed, err := r.dm.GetEntity(ctx, e.ReversesEntryID)
		if err != nil || reversed.TypeID != TypeEntry {
			return Entry{}, ErrNotFound
		}
	}

	props := map[string]any{
		propFromAccountID:   e.FromAccountID,
		propToAccountID:     e.ToAccountID,
		propAmount:          e.Amount,
		propDocumentKind:    string(e.DocumentKind),
		propDocumentID:      e.DocumentID,
		propActorID:         e.ActorID,
		propDetail:          e.Detail,
		propReversesEntryID: e.ReversesEntryID,
	}
	if e.OccurredAt != nil {
		props[propOccurredAt] = e.OccurredAt.Format(time.RFC3339)
	}

	created, err := r.dm.CreateEntity(ctx, entitygraph.CreateEntityRequest{
		TypeID:     TypeEntry,
		Properties: props,
	})
	if err != nil {
		return Entry{}, err
	}
	return entryFromEntity(created), nil
}

func (r *PostgresRepository) entriesTouching(ctx context.Context, accountID string) ([]Entry, error) {
	var out []Entry
	seen := make(map[string]bool)
	for _, prop := range []string{propFromAccountID, propToAccountID} {
		entities, err := r.dm.ListEntities(ctx, entitygraph.EntityFilter{
			TypeID:     TypeEntry,
			Properties: map[string]any{prop: accountID},
		})
		if err != nil {
			return nil, err
		}
		for _, e := range entities {
			if seen[e.ID] {
				continue
			}
			seen[e.ID] = true
			out = append(out, entryFromEntity(e))
		}
	}
	return out, nil
}

// Balance implements LedgerRepository.
func (r *PostgresRepository) Balance(ctx context.Context, accountID string) (int64, error) {
	if _, err := r.getAccountEntity(ctx, accountID); err != nil {
		return 0, err
	}
	entries, err := r.entriesTouching(ctx, accountID)
	if err != nil {
		return 0, err
	}
	var total int64
	for _, e := range entries {
		switch accountID {
		case e.ToAccountID:
			total += e.Amount
		case e.FromAccountID:
			total -= e.Amount
		}
	}
	return total, nil
}

// ListEntries implements LedgerRepository.
func (r *PostgresRepository) ListEntries(ctx context.Context, accountID string, limit int) ([]Entry, error) {
	if limit <= 0 {
		limit = DefaultPage
	}
	if _, err := r.getAccountEntity(ctx, accountID); err != nil {
		return nil, err
	}
	entries, err := r.entriesTouching(ctx, accountID)
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool { return entryOrderKey(entries[i]).Before(entryOrderKey(entries[j])) })
	if len(entries) > limit {
		entries = entries[:limit]
	}
	return entries, nil
}
