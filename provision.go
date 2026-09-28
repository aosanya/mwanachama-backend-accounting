package accounting

import (
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/aosanya/mwanachama-backend-accounting/models"
	"github.com/aosanya/mwanachama-backend-shared/spec"
)

const LegacyEntityTable = "acct_entities"

const (
	legacyTypeAccount = "account"
	legacyTypeEntry   = "entry"
)

func Provision(db *gorm.DB, s *spec.Spec) error {
	if err := spec.Migrate(db, s); err != nil {
		return err
	}
	if err := uniqueWhereGiven(db, s); err != nil {
		return err
	}
	return AdoptEntityGraph(db, s, LegacyEntityTable)
}

type partialUnique struct {
	role   string
	column string
	suffix string
}

var partialUniques = []partialUnique{
	{roleAccount, "code", "code_once"},
	{roleEntry, "reverses_entry_id", "reverses_once"},
}

func uniqueWhereGiven(db *gorm.DB, s *spec.Spec) error {
	for _, u := range partialUniques {
		o, err := objectFor(s, u.role)
		if err != nil {
			return err
		}
		table := s.TableFor(o)
		name := table + "_" + u.suffix + "_idx"
		if len(name) > spec.MaxIdentifier {
			return fmt.Errorf("accounting: index %q needs %d bytes and the database truncates at %d",
				name, len(name), spec.MaxIdentifier)
		}
		stmt := fmt.Sprintf("create unique index if not exists %s on %s (%s) where %s <> ''",
			name, table, u.column, u.column)
		if err := db.Exec(stmt).Error; err != nil {
			return fmt.Errorf("accounting: %s: %w", name, err)
		}
	}
	return nil
}

func objectFor(s *spec.Spec, role string) (spec.Object, error) {
	o, ok := s.ByRole(role)
	if !ok {
		return spec.Object{}, fmt.Errorf("accounting: the domain %q fills no object for the role %q", s.Domain, role)
	}
	return o, nil
}

func AdoptEntityGraph(db *gorm.DB, s *spec.Spec, entities string) error {
	if !db.Migrator().HasTable(entities) {
		return nil
	}
	accounts, err := objectFor(s, roleAccount)
	if err != nil {
		return err
	}
	entryObject, err := objectFor(s, roleEntry)
	if err != nil {
		return err
	}

	return db.Transaction(func(tx *gorm.DB) error {
		if err := adoptAccounts(tx, s, accounts, entities); err != nil {
			return err
		}
		return adoptEntries(tx, s, entryObject, entities)
	})
}

func adoptAccounts(tx *gorm.DB, s *spec.Spec, o spec.Object, entities string) error {
	rows, err := legacyRows(tx, s, o, entities, legacyTypeAccount)
	if err != nil {
		return err
	}
	for _, r := range rows {
		a := models.Account{
			ID:       r.id,
			Category: models.AccountCategory(r.text("account_kind")),
			Type:     models.AccountType(r.text("account_type")),
			HolderID: r.text("holder_id"),
			OpenedAt: r.createdAt,
			ClosedAt: stamp(r.text("closed_at")),
		}
		if err := insertAdopted(tx, s, o, a); err != nil {
			return err
		}
	}
	return nil
}

func adoptEntries(tx *gorm.DB, s *spec.Spec, o spec.Object, entities string) error {
	rows, err := legacyRows(tx, s, o, entities, legacyTypeEntry)
	if err != nil {
		return err
	}
	for _, r := range rows {
		e := models.Entry{
			ID:              r.id,
			PostedAt:        r.createdAt,
			OccurredAt:      stamp(r.text("occurred_at")),
			DebitAccountID:  r.text("to_account_id"),
			CreditAccountID: r.text("from_account_id"),
			Amount:          r.number("amount"),
			DocumentKind:    models.DocumentKind(r.text("document_kind")),
			DocumentID:      r.text("document_id"),
			ActorID:         r.text("actor_id"),
			Narration:       r.text("detail"),
			ReversesEntryID: r.text("reverses_entry_id"),
		}
		if err := insertAdopted(tx, s, o, e); err != nil {
			return err
		}
	}
	return nil
}

func insertAdopted(tx *gorm.DB, s *spec.Spec, o spec.Object, v any) error {
	row, err := encode(o, v)
	if err != nil {
		return fmt.Errorf("accounting: adopt %s: %w", o.Name, err)
	}
	if err := tx.Table(s.TableFor(o)).Create(row).Error; err != nil {
		return fmt.Errorf("accounting: adopt %s: %w", o.Name, err)
	}
	return nil
}

type legacyRow struct {
	id         string
	createdAt  string
	properties map[string]any
}

func (r legacyRow) text(property string) string {
	switch v := r.properties[property].(type) {
	case string:
		return v
	case []byte:
		return string(v)
	case nil:
		return ""
	default:
		return fmt.Sprint(v)
	}
}

func (r legacyRow) number(property string) int64 {
	switch v := r.properties[property].(type) {
	case float64:
		return int64(v)
	case int64:
		return v
	case int:
		return int64(v)
	default:
		return 0
	}
}

func legacyRows(tx *gorm.DB, s *spec.Spec, o spec.Object, entities, typeID string) ([]legacyRow, error) {
	var held int64
	if err := tx.Table(s.TableFor(o)).Count(&held).Error; err != nil {
		return nil, fmt.Errorf("accounting: count %s: %w", s.TableFor(o), err)
	}
	if held > 0 {
		return nil, nil
	}

	var raw []map[string]any
	if err := tx.Table(entities).
		Where("type_id = ? AND deleted = ?", typeID, false).
		Order("created_at").Find(&raw).Error; err != nil {
		return nil, fmt.Errorf("accounting: read %s: %w", entities, err)
	}

	out := make([]legacyRow, 0, len(raw))
	for _, r := range raw {
		id, _ := r["id"].(string)
		props, err := properties(r["properties"])
		if err != nil {
			return nil, fmt.Errorf("accounting: read %s row %s: %w", entities, id, err)
		}
		out = append(out, legacyRow{id: id, createdAt: stampOf(r["created_at"]), properties: props})
	}
	return out, nil
}

func properties(raw any) (map[string]any, error) {
	var text []byte
	switch v := raw.(type) {
	case []byte:
		text = v
	case string:
		text = []byte(v)
	case map[string]any:
		return v, nil
	case nil:
		return map[string]any{}, nil
	default:
		return nil, fmt.Errorf("cannot read %T as a property document", raw)
	}
	if len(text) == 0 {
		return map[string]any{}, nil
	}
	out := map[string]any{}
	if err := json.Unmarshal(text, &out); err != nil {
		return nil, err
	}
	return out, nil
}

var legacyStampLayouts = []string{
	models.TimeLayout, time.RFC3339Nano, time.RFC3339,
	"2006-01-02 15:04:05.999999999-07:00", "2006-01-02 15:04:05",
}

func stampOf(raw any) string {
	switch v := raw.(type) {
	case time.Time:
		return v.UTC().Format(models.TimeLayout)
	case []byte:
		return stamp(string(v))
	case string:
		return stamp(v)
	default:
		return ""
	}
}

func stamp(text string) string {
	if text == "" {
		return ""
	}
	for _, layout := range legacyStampLayouts {
		if t, err := time.Parse(layout, text); err == nil {
			return t.UTC().Format(models.TimeLayout)
		}
	}
	return text
}
