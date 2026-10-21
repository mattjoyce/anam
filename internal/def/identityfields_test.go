package def_test

import (
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/def"
)

// Card 188 S4: a form needs to know which field the rule composes and which fields
// decide the facet, from the Definition and not from the type's name.
func TestIdentityFieldsOfATemplatedAndAPlainDefinition(t *testing.T) {
	reg, err := def.Bundled()
	if err != nil {
		t.Fatal(err)
	}
	dose, _ := reg.Lookup("health.medication.dose")
	if got := dose.ComposedField(); got != "/id" {
		t.Fatalf("ComposedField = %q, want /id", got)
	}
	// The offset is the optional segment card 198 added: when a dose names it, it
	// decides the facet too.
	if got := strings.Join(dose.IdentityFields(), ","); got != "/drug,/taken_at,/source_time/offset" {
		t.Fatalf("IdentityFields = %q, want /drug,/taken_at,/source_time/offset (the id itself is composed, not supplied)", got)
	}
	weight, _ := reg.Lookup("health.weight")
	if got := weight.ComposedField(); got != "" {
		t.Fatalf("ComposedField of a type with no template = %q, want none", got)
	}
	if got, want := strings.Join(weight.IdentityFields(), ","), strings.Join(weight.IdentityKey, ","); got != want {
		t.Fatalf("IdentityFields = %q, want the identity_key %q", got, want)
	}
}
