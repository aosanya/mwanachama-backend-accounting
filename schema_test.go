package accounting

import (
	"testing"

	"github.com/aosanya/mwanachama-backend-shared/entitygraph"
)

func TestDefaultAccountingSchemaIsValid(t *testing.T) {
	s := DefaultAccountingSchema()
	if err := entitygraph.ValidateSchema(s); err != nil {
		t.Fatalf("ValidateSchema: %v", err)
	}
}
