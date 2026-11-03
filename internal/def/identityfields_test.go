package def_test

import (
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/catalog/catalogtest"
)

// Card 188 S4: a form needs to know which field the rule composes and which fields
// decide the facet, from the Definition and not from the type's name.
func TestIdentityFieldsOfATemplatedAndAPlainDefinition(t *testing.T) {
	t.Parallel()
	reg := catalogtest.Engine.Definitions(t)
	templated, _ := reg.Lookup("test.templated")
	if got := templated.ComposedField(); got != "/id" {
		t.Fatalf("ComposedField = %q, want /id", got)
	}
	// The offset is the optional segment v2 added (card 198): when a payload names it,
	// it decides the facet too.
	if got := strings.Join(templated.IdentityFields(), ","); got != "/name,/at,/source_time/offset" {
		t.Fatalf("IdentityFields = %q, want /name,/at,/source_time/offset (the id itself is composed, not supplied)", got)
	}
	measure, _ := reg.Lookup("test.measure")
	if got := measure.ComposedField(); got != "" {
		t.Fatalf("ComposedField of a type with no template = %q, want none", got)
	}
	if got, want := strings.Join(measure.IdentityFields(), ","), strings.Join(measure.IdentityKey, ","); got != want || want != "/at,/seq" {
		t.Fatalf("IdentityFields = %q, want the identity_key %q, /at,/seq", got, want)
	}
}
