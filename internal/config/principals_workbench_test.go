package config_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/facet"
)

// Card 188: the workbench is an importer's entry with the reserved source, no token,
// and the types it may enter by hand.
func TestLoadPrincipalsWorkbench(t *testing.T) {
	t.Parallel()
	valid := `
importers:
  - source: withings
    token_sha256: ` + hashA + `
    types: [health.weight]
  - source: workbench
    types: [health.medication.dose, health.weight]
`
	t.Run("a workbench with no token", func(t *testing.T) {
		t.Parallel()
		got, err := config.LoadPrincipals(writeImporters(t, valid), nil)
		if err != nil {
			t.Fatal(err)
		}
		wb := got[1]
		if wb.Source != config.WorkbenchSource || wb.Name != "workbench" || wb.Role != config.RoleImporter ||
			wb.TokenSHA256 != "" || len(wb.Types) != 2 || wb.Types[0] != facet.Type("health.medication.dose") {
			t.Fatalf("loaded %+v", wb)
		}
	})

	t.Run("it owns none of its types", func(t *testing.T) {
		t.Parallel()
		// health.weight is another importer's and not bundled here: the workbench lists
		// it without taking it from the importer, and without being refused.
		if _, err := config.LoadPrincipals(writeImporters(t, valid), nil); err != nil {
			t.Fatalf("a type the workbench shares with an importer: %v", err)
		}
	})

	cases := []struct{ name, body string }{
		{"a token", strings.Replace(valid, "source: workbench", "source: workbench\n    token_sha256: "+hashB, 1)},
		{"any_type", strings.Replace(valid, "types: [health.medication.dose, health.weight]", "any_type: true", 1)},
		{"no types", strings.Replace(valid, "types: [health.medication.dose, health.weight]", "types: []", 1)},
		{"a type that is not namespaced", strings.Replace(valid, "health.medication.dose", "dose", 1)},
		{"another role", strings.Replace(valid, "importers:", "principals:\n  - name: wb\n    role: reader\n    source: workbench\n    types: [health.weight]\nimporters:", 1)},
		{"the operator's name", strings.Replace(valid, "source: workbench", "source: workbench\n    name: operator", 1)},
		{"the source named twice", valid + "  - source: workbench\n    types: [health.weight]\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			_, err := config.LoadPrincipals(writeImporters(t, c.body), nil)
			if !errors.Is(err, config.ErrInvalidImporters) {
				t.Fatalf("got %v, want ErrInvalidImporters", err)
			}
		})
	}

	t.Run("an importer without a token is still refused", func(t *testing.T) {
		t.Parallel()
		body := strings.Replace(valid, "token_sha256: "+hashA, "", 1)
		if _, err := config.LoadPrincipals(writeImporters(t, body), nil); !errors.Is(err, config.ErrInvalidImporters) {
			t.Fatalf("got %v, want ErrInvalidImporters", err)
		}
	})
}
