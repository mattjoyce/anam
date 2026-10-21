package def_test

import (
	"errors"
	"os"
	"strconv"
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
		{"handling added", strings.Replace(v2, `"schema"`, `"handling": {"guidance": "sitting"}, "schema"`, 1), false},
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

// Card 198: the two widenings the guard accepts, and that each is the only one. A
// fact time may gain a zone_pointer with pointer, layout and zone unchanged; an id template may gain optional placeholders at its end. Each holds
// because no stored version let a payload carry the field it reads.
func TestGuard_AcceptsTheTwoWideningsAndNothingElse(t *testing.T) {
	t.Parallel()
	shape, err := os.ReadFile("sourcetime/v2.json")
	if err != nil {
		t.Fatal(err)
	}
	const (
		plainTime = `{"pointer": "/at", "layout": "2006-01-02 15:04", "zone": "Australia/Sydney"}`
		zonedTime = `{"pointer": "/at", "layout": "2006-01-02 15:04", "zone": "Australia/Sydney", "zone_pointer": "/source_time/offset"}`
		plainID   = `{source}:r/{/at|2006-01-02T15:04}`
	)
	st := `, "source_time": ` + string(shape)
	manifest := func(version int, fact, tpl, props, mode string) string {
		template := ""
		if tpl != "" {
			template = `, "identity_template": "` + tpl + `"`
		}
		schema := `, "schema": {"type": "object", "required": ["id", "at"], "additionalProperties": false,
		    "properties": {"id": {"type": "string", "minLength": 1}, "at": {"type": "string", "minLength": 1}` + props + `}}`
		if mode == "verbatim" {
			schema = ""
		}
		return `{"meta": {"facet_type": "health.reading", "definition_version": ` + strconv.Itoa(version) + `, "classification": "Normal",
		  "validation_mode": "` + mode + `", "source_schema": "test/reading",
		  "time_semantics": {"fact_time": ` + fact + `}, "identity_key": ["/id"]` + template + `}` + schema + `}`
	}
	stored := func(bodies ...string) []def.Stored {
		var out []def.Stored
		for i, b := range bodies {
			out = append(out, def.Stored{Type: "health.reading", Version: i + 1, Body: []byte(b)})
		}
		return out
	}
	v1 := manifest(1, plainTime, plainID, "", "strict")
	cases := []struct {
		name    string
		stored  []def.Stored
		now     string
		refused string // "" when accepted
	}{
		{"a zone_pointer added", stored(v1), manifest(2, zonedTime, plainID, st, "strict"), ""},
		{"an optional id segment appended", stored(v1), manifest(2, plainTime, plainID+`{/source_time/offset?}`, st, "strict"), ""},
		{"both at once", stored(v1), manifest(2, zonedTime, plainID+`{/source_time/offset?}`, st, "strict"), ""},
		{"after a version in the format before card 52", stored(lenientReading, v1), manifest(3, zonedTime, plainID+`{/source_time/offset?}`, st, "strict"), ""},
		{"a zone_pointer with the zone changed", stored(v1), manifest(2, strings.Replace(zonedTime, "Australia/Sydney", "UTC", 1), plainID, st, "strict"), "fact_time was"},
		{"a zone_pointer with the layout changed", stored(v1), manifest(2, strings.Replace(zonedTime, "2006-01-02 15:04", "2006-01-02T15:04", 1), plainID, st, "strict"), "fact_time was"},
		{"a zone_pointer dropped", stored(v1, manifest(2, zonedTime, plainID, st, "strict")), manifest(3, plainTime, plainID, st, "strict"), "fact_time was"},
		{"a zone_pointer moved", stored(v1, manifest(2, zonedTime, plainID, st, "strict")), manifest(3, strings.Replace(zonedTime, "/source_time/offset", "/source_time/zone", 1), plainID, st, "strict"), "fact_time was"},
		{"after a version that declared source_time", stored(manifest(1, plainTime, plainID, st, "strict")), manifest(2, zonedTime, plainID, st, "strict"), "already declares source_time"},
		{"after a verbatim version", stored(manifest(1, plainTime, "", "", "verbatim")), manifest(2, zonedTime, "", st, "strict"), "is verbatim"},
		{"an optional segment after one that declared it", stored(manifest(1, plainTime, plainID, st, "strict")), manifest(2, plainTime, plainID+`{/source_time/offset?}`, st, "strict"), "already declares source_time"},
		{"an id segment edited", stored(v1), manifest(2, plainTime, `{source}:r/{/at|2006-01-02}`, st, "strict"), "identity_template was"},
		{"literal text appended", stored(v1), manifest(2, plainTime, plainID+`/z`, st, "strict"), "identity_template was"},
		{"an optional segment and text appended", stored(v1), manifest(2, plainTime, plainID+`{/source_time/offset?}/z`, st, "strict"), "identity_template was"},
		{"a required segment appended", stored(v1), manifest(2, plainTime, plainID+`{/at}`, st, "strict"), "identity_template was"},
		{"an optional segment put first", stored(v1), manifest(2, plainTime, `{/source_time/offset?}`+plainID, st, "strict"), "identity_template was"},
		{"a template where there was none", stored(manifest(1, plainTime, "", "", "strict")), manifest(2, plainTime, plainID, "", "strict"), "identity_template was"},
		{"a template dropped", stored(v1), manifest(2, plainTime, "", "", "strict"), "identity_template was"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			d, err := def.Parse([]byte(c.now))
			if err != nil {
				t.Fatalf("the new version does not parse: %v", err)
			}
			reg, err := def.NewRegistry(d)
			if err != nil {
				t.Fatal(err)
			}
			err = def.Guard(c.stored, reg)
			if c.refused == "" {
				if err != nil {
					t.Fatalf("Guard refused the widening: %v", err)
				}
			} else if !errors.Is(err, def.ErrKeyed) || !strings.Contains(err.Error(), c.refused) {
				t.Fatalf("Guard = %v, want ErrKeyed naming %q", err, c.refused)
			}
			// Register, for a type an importer owns, holds the same rule.
			_, err = def.Register([]byte(c.now), nil, c.stored, emptyRegistry(t))
			if c.refused == "" && err != nil {
				t.Errorf("Register refused the widening: %v", err)
			}
			if c.refused != "" && !errors.Is(err, def.ErrKeyed) {
				t.Errorf("Register = %v, want ErrKeyed", err)
			}
		})
	}
}

// lenientReading is health.reading in the format before card 52, which did not close
// its fields.
const lenientReading = `{"meta": {"facet_type": "health.reading", "definition_version": 1, "classification": "Normal",
  "validation_mode": "lenient", "source_schema": "test/reading",
  "time_semantics": {"fact_time": {"pointer": "/at", "layout": "2006-01-02 15:04", "zone": "Australia/Sydney"}},
  "identity_key": ["/id"], "identity_template": "{source}:r/{/at|2006-01-02T15:04}"},
  "schema": {"required": ["/at"], "properties": {"/at": {}}}}`

func emptyRegistry(t *testing.T) *def.Registry {
	t.Helper()
	r, err := def.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	return r
}
