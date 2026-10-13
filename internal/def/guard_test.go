package def_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
)

// Card 14: a Definition whose identity_key or fact_time differs from the newest
// stored version of its type is refused, and the refusal says why that matters to
// the ledger. Everything else about a Definition may change.
func TestGuard_RefusesAChangedKeyingAndNothingElse(t *testing.T) {
	t.Parallel()

	stored := []def.Stored{{Type: "health.blood-pressure", Version: 1, Body: []byte(bpManifest)}}
	bundle := func(t *testing.T, manifest string) *def.Registry {
		t.Helper()
		d, err := def.Parse([]byte(manifest))
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		r, err := def.NewRegistry(d)
		if err != nil {
			t.Fatalf("NewRegistry: %v", err)
		}
		return r
	}
	v2 := strings.Replace(bpManifest, `"definition_version": 1`, `"definition_version": 2`, 1)

	cases := []struct {
		name     string
		manifest string
		refused  bool
	}{
		{"unchanged", bpManifest, false},
		{"new version, same keying", v2, false},
		{"schema tightened", strings.Replace(v2, `"required": [`, `"required": ["comment", `, 1), false},
		{"classification raised", strings.Replace(v2, `"classification": "Sensitive"`, `"classification": "Confidential"`, 1), false},
		{"identity key changed", strings.Replace(v2, `"identity_key": ["/measured_at"]`, `"identity_key": ["/measured_at", "/systolic"]`, 1), true},
		{"fact-time zone changed", strings.Replace(v2, `"zone": "UTC"`, `"zone": "Australia/Sydney"`, 1), true},
		{"fact-time pointer changed", strings.Replace(v2, `"pointer": "/measured_at"`, `"pointer": "/systolic"`, 1), true},
		{"same version, identity changed", strings.Replace(bpManifest, `"identity_key": ["/measured_at"]`, `"identity_key": ["/systolic"]`, 1), true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if c.manifest == bpManifest && c.name != "unchanged" {
				t.Fatalf("test case did not change the manifest")
			}
			err := def.Guard(stored, bundle(t, c.manifest))
			if !c.refused {
				if err != nil {
					t.Fatalf("Guard refused: %v", err)
				}
				return
			}
			if !errors.Is(err, def.ErrKeyed) {
				t.Fatalf("Guard = %v, want ErrKeyed", err)
			}
			for _, want := range []string{"health.blood-pressure", "new ledger"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("refusal %q does not say %q", err, want)
				}
			}
		})
	}

	// A type the store has never seen is new, and passes whatever it declares.
	fresh := strings.Replace(bpManifest, `"facet_type": "health.blood-pressure"`, `"facet_type": "health.pulse"`, 1)
	if err := def.Guard(stored, bundle(t, fresh)); err != nil {
		t.Errorf("Guard refused a new type: %v", err)
	}
	// And the bundled set is what it compares against itself: a store holding exactly
	// the bundled manifests passes.
	manifests, err := def.Manifests()
	if err != nil {
		t.Fatalf("Manifests: %v", err)
	}
	bundled, err := def.Bundled()
	if err != nil {
		t.Fatalf("Bundled: %v", err)
	}
	if err := def.Guard(manifests, bundled); err != nil {
		t.Errorf("Guard refused the bundled set against itself: %v", err)
	}
	// Manifests carries the current body of every bundled type plus the history
	// (card 111); the current ones match Bundled one for one.
	current := 0
	for _, m := range manifests {
		if m.Type == facet.Type("") {
			t.Errorf("a manifest with no type: %+v", m)
		}
		if d, ok := bundled.Lookup(m.Type); ok && d.Version == m.Version {
			current++
		}
	}
	if current != len(bundled.All()) {
		t.Errorf("Manifests() lists %d current bodies, Bundled() holds %d", current, len(bundled.All()))
	}
}

// The store keeps each body in the format current when it was stored, and the schema
// half's format changed at card 52. The guard reads only the keying, so a body from
// before still compares: same keying passes, a changed one is refused.
func TestGuard_ComparesAStoredBodyOfAnEarlierFormat(t *testing.T) {
	t.Parallel()
	before := `{
  "meta": {
    "facet_type": "health.blood-pressure", "definition_version": 1, "classification": "Normal",
    "validation_mode": "lenient", "source_schema": "bp-csv/reading",
    "time_semantics": {"fact_time": {"pointer": "/measured_at", "layout": "2006-01-02T15:04:05Z07:00", "zone": "UTC"}},
    "identity_key": ["/measured_at"]
  },
  "schema": {"required": ["/systolic", "/diastolic"], "properties": {"/position": {"enum": ["sitting"]}}}
}`
	stored := []def.Stored{{Type: "health.blood-pressure", Version: 1, Body: []byte(before)}}

	same, err := def.NewRegistry(mustParse(t, strings.Replace(bpManifest, `"definition_version": 1`, `"definition_version": 2`, 1)))
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	if err := def.Guard(stored, same); err != nil {
		t.Errorf("Guard refused the same keying across formats: %v", err)
	}
	rekeyed, err := def.NewRegistry(mustParse(t, strings.Replace(bpManifest, `"identity_key": ["/measured_at"]`, `"identity_key": ["/measured_at", "/systolic"]`, 1)))
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	if err := def.Guard(stored, rekeyed); !errors.Is(err, def.ErrKeyed) {
		t.Errorf("Guard = %v across formats, want ErrKeyed", err)
	}
}
