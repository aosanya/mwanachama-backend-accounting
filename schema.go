package accounting

import "github.com/aosanya/mwanachama-backend-shared/schema"

// Entity type names. One type for every account regardless of Kind — Kind
// and HolderID are both plain properties now that HolderID is the one
// identity field every account carries, so entitygraph.TypeDefinition's
// composite UniqueKey (an ordered list of properties) can enforce "at most
// one account per (Kind, HolderID)" directly, the way three separate types
// keyed on three different single fields used to before HolderID unified
// them.
const (
	TypeAccount = "account"
	TypeEntry   = "entry"
)

// Account property names.
const (
	propAccountKind = "account_kind"
	propHolderID    = "holder_id"
	propAccountType = "account_type"
	propClosedAt    = "closed_at"
)

// Entry property names.
const (
	propOccurredAt      = "occurred_at"
	propFromAccountID   = "from_account_id"
	propToAccountID     = "to_account_id"
	propAmount          = "amount"
	propDocumentKind    = "document_kind"
	propDocumentID      = "document_id"
	propActorID         = "actor_id"
	propDetail          = "detail"
	propReversesEntryID = "reverses_entry_id"
)

// DefaultAccountingSchema is the entity-graph schema this package writes
// against — two types, no relationships. FromAccountID/ToAccountID are
// plain string properties on Entry rather than entitygraph relationships:
// an entry can point at any account regardless of Kind, and a typed edge
// would need one RelationshipDefinition per Kind for a property this
// package never needs to traverse the graph for.
func DefaultAccountingSchema() schema.Schema {
	return schema.Schema{
		Tag: "v1",
		Types: []schema.TypeDefinition{
			{
				Name:        TypeAccount,
				DisplayName: "Account",
				Properties: []schema.PropertyDefinition{
					{Name: propAccountKind, Type: schema.PropertyTypeString, Required: true},
					{Name: propHolderID, Type: schema.PropertyTypeString, Required: true},
					{Name: propAccountType, Type: schema.PropertyTypeString, Required: true},
					{Name: propClosedAt, Type: schema.PropertyTypeDatetime},
				},
				UniqueKey: []string{propAccountKind, propHolderID},
			},
			{
				// Immutable: there is no update path and no delete path on
				// an entry anywhere in this package — a correction is a new
				// entry with ReversesEntryID set. Marking the type itself
				// Immutable makes entitygraph.DataManager refuse
				// UpdateEntity with ErrImmutableType as a second line of
				// defense, not only a rule PostgresRepository happens to
				// follow.
				Name:        TypeEntry,
				DisplayName: "Ledger Entry",
				Immutable:   true,
				Properties: []schema.PropertyDefinition{
					{Name: propOccurredAt, Type: schema.PropertyTypeDatetime},
					{Name: propFromAccountID, Type: schema.PropertyTypeString, Required: true},
					{Name: propToAccountID, Type: schema.PropertyTypeString, Required: true},
					{Name: propAmount, Type: schema.PropertyTypeInteger, Required: true},
					{Name: propDocumentKind, Type: schema.PropertyTypeString, Required: true},
					{Name: propDocumentID, Type: schema.PropertyTypeString, Required: true},
					{Name: propActorID, Type: schema.PropertyTypeString, Required: true},
					{Name: propDetail, Type: schema.PropertyTypeString},
					{Name: propReversesEntryID, Type: schema.PropertyTypeString},
				},
			},
		},
	}
}
