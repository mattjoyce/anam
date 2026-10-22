package projection_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/projection"
)

const inlineWeights = `{"types":["health.weight","health.blood-pressure."],"statuses":["admitted","endorsed"],` +
	`"max_classification":"Sensitive","from":"2026-01-01T00:00:00.000000000Z","to":"","shape":"log"}`

// Card 211: an inline template is the knobs of a manifest without a name or a version,
// checked by the loader a registered one is, and known by its content hash.
func TestParseInline_IsKnownByItsContentHash(t *testing.T) {
	t.Parallel()
	a, err := projection.ParseInline([]byte(inlineWeights))
	if err != nil {
		t.Fatal(err)
	}
	if !a.Inline || a.Name != projection.InlineName || a.Version != projection.InlineVersion || !strings.HasPrefix(a.Hash, "sha256:") {
		t.Errorf("template = %+v", a)
	}
	// The same body in another key order and layout is the same template.
	b, err := projection.ParseInline([]byte(`{ "shape":"log", "to":"", "from":"2026-01-01T00:00:00.000000000Z", "max_classification":"Sensitive",
	  "statuses":["admitted","endorsed"], "types":["health.weight","health.blood-pressure."] }`))
	if err != nil || b.Hash != a.Hash {
		t.Fatalf("the same body laid out another way: %v, %s want %s", err, b.Hash, a.Hash)
	}
	// Its Body is what its hash is over, so the store can file it under that address.
	if again, err := projection.ParseInline(a.Body); err != nil || again.Hash != a.Hash {
		t.Errorf("the canonical body parses to %s (%v), want %s", again.Hash, err, a.Hash)
	}
	// Prefixes and window work as they do in a manifest.
	if len(a.Prefixes) != 1 || a.Prefixes[0] != "health.blood-pressure." || len(a.Types) != 1 || a.From == "" {
		t.Errorf("filters = types %v prefixes %v from %q", a.Types, a.Prefixes, a.From)
	}
	// A registered template of the same name is not mistaken for it: the registry only
	// holds what is registered.
	reg, err := projection.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reg.Lookup(projection.InlineName); !errors.Is(err, projection.ErrUnknown) {
		t.Errorf("the registry knows the inline stand-in name: %v", err)
	}
}

// A bad inline template is refused naming the field it got wrong.
func TestParseInline_NamesTheFieldItGotWrong(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct{ body, field string }{
		"no types":               {`{"statuses":[],"max_classification":"Normal"}`, "types"},
		"no statuses":            {`{"types":[],"max_classification":"Normal"}`, "statuses"},
		"no max_classification":  {`{"types":[],"statuses":[]}`, "max_classification"},
		"not a type":             {`{"types":["Not A Type"],"statuses":[],"max_classification":"Normal"}`, "types"},
		"not a status":           {`{"types":[],"statuses":["maybe"],"max_classification":"Normal"}`, "statuses"},
		"not a classification":   {`{"types":[],"statuses":[],"max_classification":"Secret"}`, "max_classification"},
		"not a shape":            {`{"types":[],"statuses":[],"max_classification":"Normal","shape":"pile"}`, "shape"},
		"from is not an instant": {`{"types":[],"statuses":[],"max_classification":"Normal","from":"yesterday"}`, "from"},
		"window is empty":        {`{"types":[],"statuses":[],"max_classification":"Normal","from":"2026-02-01T00:00:00.000000000Z","to":"2026-01-01T00:00:00.000000000Z"}`, "from"},
		"a name":                 {`{"name":"x","types":[],"statuses":[],"max_classification":"Normal"}`, "name"},
		"a version":              {`{"version":1,"types":[],"statuses":[],"max_classification":"Normal"}`, "version"},
		"a description":          {`{"description":"d","types":[],"statuses":[],"max_classification":"Normal"}`, "description"},
		"an unknown key":         {`{"types":[],"statuses":[],"max_classification":"Normal","tombstones":true}`, "tombstones"},
		"types is a string":      {`{"types":"health.weight","statuses":[],"max_classification":"Normal"}`, "types"},
		"not an object":          {`["health.weight"]`, ""},
	} {
		_, err := projection.ParseInline([]byte(tc.body))
		if !errors.Is(err, projection.ErrBadTemplate) {
			t.Errorf("%s: err = %v, want ErrBadTemplate", name, err)
			continue
		}
		if !strings.Contains(err.Error(), tc.field) {
			t.Errorf("%s: %q does not name %q", name, err, tc.field)
		}
	}
}

// An inline template naming a type no Definition publishes is refused as a registered
// one is, so a typo is told to its author rather than served as nothing.
func TestInline_CheckDefined(t *testing.T) {
	t.Parallel()
	tmpl, err := projection.ParseInline([]byte(inlineWeights))
	if err != nil {
		t.Fatal(err)
	}
	if err := tmpl.CheckDefined(nil); err != nil {
		t.Errorf("a nil check refused: %v", err)
	}
	if err := tmpl.CheckDefined(func(facet.Type) bool { return true }); err != nil {
		t.Errorf("every type defined: %v", err)
	}
	err = tmpl.CheckDefined(func(ft facet.Type) bool { return ft != "health.weight" })
	if !errors.Is(err, projection.ErrUndefinedType) || !strings.Contains(err.Error(), "health.weight") {
		t.Errorf("a type without a Definition: %v", err)
	}
}
