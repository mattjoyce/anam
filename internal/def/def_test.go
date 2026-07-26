package def_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mattjoyce/anam/internal/def"
)

const bpManifest = `{
  "meta": {
    "facet_type": "health.blood-pressure",
    "definition_version": 1,
    "classification": "Normal",
    "validation_mode": "lenient",
    "source_schema": "bp-csv/reading",
    "time_semantics": {
      "fact_time": {"pointer": "/measured_at", "layout": "2006-01-02T15:04:05Z07:00"}
    },
    "identity_key": ["/measured_at"]
  },
  "schema": {
    "required": ["/systolic", "/diastolic"],
    "properties": {
      "/position": {"enum": ["sitting", "standing", "lying"]}
    }
  }
}`

const bpPayload = `{
  "measured_at": "2026-01-24T06:30:00Z",
  "systolic": 118, "diastolic": 76, "pulse": 61,
  "position": "sitting", "comment": "No filodipine"
}`

func mustParse(t *testing.T, manifest string) def.Definition {
	t.Helper()
	d, err := def.Parse([]byte(manifest))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return d
}

func mustDecode(t *testing.T, payload string) any {
	t.Helper()
	doc, err := def.Decode([]byte(payload))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	return doc
}

func TestParse_DefaultsAndValidation(t *testing.T) {
	t.Parallel()
	d := mustParse(t, bpManifest)

	if d.SchemaVersion != 1 || d.MediaType != "application/json" {
		t.Errorf("defaults: schema v%d media %q", d.SchemaVersion, d.MediaType)
	}
	if !d.AdditionalProperties {
		t.Error("lenient mode should allow additional fields")
	}
}

func TestParse_RejectsBadManifests(t *testing.T) {
	t.Parallel()
	breakages := map[string]func(string) string{
		"unknown key":      func(m string) string { return strings.Replace(m, `"meta"`, `"metta"`, 1) },
		"snake type":       func(m string) string { return strings.Replace(m, "health.blood-pressure", "blood_pressure", 1) },
		"no identity":      func(m string) string { return strings.Replace(m, `["/measured_at"]`, `[]`, 1) },
		"bad mode":         func(m string) string { return strings.Replace(m, "lenient", "relaxed", 1) },
		"pointer no slash": func(m string) string { return strings.Replace(m, `"/systolic"`, `"systolic"`, 1) },
		"bad pattern": func(m string) string {
			return strings.Replace(m, `"enum": ["sitting", "standing", "lying"]`, `"pattern": "["`, 1)
		},
		"no event layout": func(m string) string {
			return strings.Replace(m, `"layout": "2006-01-02T15:04:05Z07:00"`, `"layout": ""`, 1)
		},
	}
	for name, corrupt := range breakages {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := def.Parse([]byte(corrupt(bpManifest))); !errors.Is(err, def.ErrBadManifest) {
				t.Errorf("Parse = %v, want ErrBadManifest", err)
			}
		})
	}
}

func TestCheck_LenientAcceptsExtrasAndEnforcesRules(t *testing.T) {
	t.Parallel()
	d := mustParse(t, bpManifest)

	if err := d.Check(mustDecode(t, bpPayload)); err != nil {
		t.Errorf("valid payload rejected: %v", err)
	}
	if err := d.Check(mustDecode(t, `{"measured_at": "x", "systolic": 118}`)); !errors.Is(err, def.ErrNotCompliant) {
		t.Errorf("missing required diastolic = %v, want ErrNotCompliant", err)
	}
	if err := d.Check(mustDecode(t, strings.Replace(bpPayload, "sitting", "hovering", 1))); !errors.Is(err, def.ErrNotCompliant) {
		t.Errorf("enum violation = %v, want ErrNotCompliant", err)
	}
}

func TestCheck_StrictRejectsUnknownTopLevelFields(t *testing.T) {
	t.Parallel()
	strict := strings.Replace(bpManifest, "lenient", "strict", 1)
	d := mustParse(t, strict)

	// pulse and comment are not named by the schema, identity, or time semantics.
	if err := d.Check(mustDecode(t, bpPayload)); !errors.Is(err, def.ErrNotCompliant) {
		t.Errorf("strict Check with extras = %v, want ErrNotCompliant", err)
	}

	trimmed := `{"measured_at": "2026-01-24T06:30:00Z", "systolic": 118, "diastolic": 76, "position": "sitting"}`
	if err := d.Check(mustDecode(t, trimmed)); err != nil {
		t.Errorf("strict Check without extras: %v", err)
	}
}

func TestCheck_VerbatimChecksNothing(t *testing.T) {
	t.Parallel()
	verbatim := strings.Replace(bpManifest, "lenient", "verbatim", 1)
	d := mustParse(t, verbatim)

	if err := d.Check(mustDecode(t, `{"anything": true}`)); err != nil {
		t.Errorf("verbatim Check: %v", err)
	}
}

func TestTimes_ExtractsAndNormalisesToUTC(t *testing.T) {
	t.Parallel()
	d := mustParse(t, bpManifest)

	event, err := d.FactTimeOf(mustDecode(t, strings.Replace(bpPayload,
		"2026-01-24T06:30:00Z", "2026-01-24T16:30:00+10:00", 1)))
	if err != nil {
		t.Fatalf("Times: %v", err)
	}
	want := time.Date(2026, 1, 24, 6, 30, 0, 0, time.UTC)
	if !event.Equal(want) || event.Location() != time.UTC {
		t.Errorf("event = %v, want %v UTC", event, want)
	}
	if _, err := d.FactTimeOf(mustDecode(t, `{"measured_at": "24/01/2026", "systolic": 1, "diastolic": 1}`)); !errors.Is(err, def.ErrNotCompliant) {
		t.Errorf("unparseable time = %v, want ErrNotCompliant", err)
	}
}

// The natural key is identity, never content (§6a): composed from identity_key
// pointers only, so a corrected value yields the SAME source ref.
func TestSourceRef_IsIdentityNotContent(t *testing.T) {
	t.Parallel()
	d := mustParse(t, bpManifest)

	ref, err := d.SourceRef("bp-csv", mustDecode(t, bpPayload))
	if err != nil {
		t.Fatalf("SourceRef: %v", err)
	}
	if want := "bp-csv:2026-01-24T06:30:00Z"; ref != want {
		t.Errorf("SourceRef = %q, want %q", ref, want)
	}

	corrected := strings.Replace(bpPayload, "118", "121", 1)
	ref2, err := d.SourceRef("bp-csv", mustDecode(t, corrected))
	if err != nil {
		t.Fatalf("SourceRef corrected: %v", err)
	}
	if ref2 != ref {
		t.Errorf("a corrected value changed the identity: %q vs %q — supersession would silently become duplication", ref2, ref)
	}

	if _, err := d.SourceRef("bp-csv", mustDecode(t, `{"systolic": 118}`)); !errors.Is(err, def.ErrNotCompliant) {
		t.Errorf("missing identity = %v, want ErrNotCompliant", err)
	}
}

func TestDecode_PreservesNumberLexemes(t *testing.T) {
	t.Parallel()
	doc := mustDecode(t, `{"weight": 82.50}`)
	m, ok := doc.(map[string]any)
	if !ok {
		t.Fatal("not an object")
	}
	if got := m["weight"]; got == nil || got.(interface{ String() string }).String() != "82.50" {
		t.Errorf("number lexeme not preserved: %v", got)
	}
}
