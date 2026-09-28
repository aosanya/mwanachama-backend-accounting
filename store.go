package accounting

import (
	"gorm.io/gorm"

	"github.com/aosanya/mwanachama-backend-shared/spec"
	"github.com/aosanya/mwanachama-backend-shared/specstore"
)

const (
	roleAccount = "account"
	roleEntry   = "entry"
)

type store = specstore.Store

func newStore(db *gorm.DB, s *spec.Spec, carriers map[string]any) (*store, error) {
	return specstore.New(db, s, carriers)
}

func newID() string { return specstore.NewID() }

func columnName(field string) string { return specstore.ColumnName(field) }

func encode(o spec.Object, v any) (map[string]any, error) { return specstore.Encode(o, v) }

func decode(o spec.Object, row map[string]any, out any) error {
	return specstore.Decode(o, row, out)
}

func mutableColumns(o spec.Object, row map[string]any) map[string]any {
	out := make(map[string]any, len(row))
	for _, f := range o.Fields {
		if f.Primary || f.Immutable {
			continue
		}
		if v, ok := row[f.Name]; ok {
			out[f.Name] = v
		}
	}
	return out
}
