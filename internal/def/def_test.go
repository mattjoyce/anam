package def_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mattjoyce/anam/internal/facet"

	"github.com/mattjoyce/anam/internal/def"
)

const bpManifest = `{
  "meta": {
    "facet_type": "health.blood-pressure",
    "definition_version": 1,
    "classification": "Normal",
    "validation_mode": "strict",
    "source_schema": "bp-csv/reading",
    "time_semantics": {
      "fact_time": {"pointer": "/measured_at", "layout": "2006-01-02T15:04:05Z07:00", "zone": "UTC"}
    },
    "identity_key": ["/measured_at"]
  },
  "schema": {
    "$schema": "https://json-schema.org/draft/2020-12/schema",
    "type": "object",
    "required": ["measured_at", "systolic", "diastolic"],
    "properties": {
      "measured_at": {"type": "string", "minLength": 1},
      "systolic": {"type": "number"},
      "diastolic": {"type": "number"},
      "pulse": {"type": "number"},
      "position": {"type": "string", "enum": ["sitting", "standing", "lying"]},
      "comment": {"type": "string", "minLength": 1}
    },
    "additionalProperties": false
  }
}`

// verbatimManifest is bpManifest with no schema to hold a payload to.
var verbatimManifest = bpManifest[:strings.Index(bpManifest, `,
  "schema"`)] + "\n}"

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
}

func TestParse_RejectsBadManifests(t *testing.T) {
	t.Parallel()
	breakages := map[string]func(string) string{
		"unknown key":      func(m string) string { return strings.Replace(m, `"meta"`, `"metta"`, 1) },
		"snake type":       func(m string) string { return strings.Replace(m, "health.blood-pressure", "blood_pressure", 1) },
		"no identity":      func(m string) string { return strings.Replace(m, `["/measured_at"]`, `[]`, 1) },
		"bad mode":         func(m string) string { return strings.Replace(m, `"strict"`, `"relaxed"`, 1) },
		"lenient is gone":  func(m string) string { return strings.Replace(m, `"strict"`, `"lenient"`, 1) },
		"pointer no slash": func(m string) string { return strings.Replace(m, `["/measured_at"]`, `["measured_at"]`, 1) },
		"bad pattern": func(m string) string {
			return strings.Replace(m, `"enum": ["sitting", "standing", "lying"]`, `"pattern": "["`, 1)
		},
		"open schema": func(m string) string {
			return strings.Replace(m, `"additionalProperties": false`, `"additionalProperties": true`, 1)
		},
		"closed by nobody": func(m string) string {
			return strings.Replace(m, `,
    "additionalProperties": false`, ``, 1)
		},
		"open nested object": func(m string) string {
			return strings.Replace(m, `"pulse": {"type": "number"}`,
				`"pulse": {"type": "object", "properties": {"bpm": {"type": "number"}}}`, 1)
		},
		"ref leaves the manifest": func(m string) string {
			return strings.Replace(m, `"pulse": {"type": "number"}`, `"pulse": {"$ref": "https://example.com/pulse.json"}`, 1)
		},
		"another draft": func(m string) string {
			return strings.Replace(m, "draft/2020-12", "draft-07", 1)
		},
		"required not declared": func(m string) string {
			return strings.Replace(m, `"required": ["measured_at",`, `"required": ["weight", "measured_at",`, 1)
		},
		"required admits null": func(m string) string {
			return strings.Replace(m, `"systolic": {"type": "number"}`, `"systolic": {"type": ["number", "null"]}`, 1)
		},
		"required admits empty": func(m string) string {
			return strings.Replace(m, `"measured_at": {"type": "string", "minLength": 1}`, `"measured_at": {"type": "string"}`, 1)
		},
		"required names no type": func(m string) string {
			return strings.Replace(m, `"systolic": {"type": "number"}`, `"systolic": {}`, 1)
		},
		"identity not required": func(m string) string {
			return strings.Replace(m, `"required": ["measured_at", `, `"required": [`, 1)
		},
		"verbatim with a schema": func(m string) string { return strings.Replace(m, `"strict"`, `"verbatim"`, 1) },
		"no event layout": func(m string) string {
			return strings.Replace(m, `"layout": "2006-01-02T15:04:05Z07:00", "zone": "UTC"`, `"layout": "", "zone": "UTC"`, 1)
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

// The schema is closed and typed: a valid payload passes, and each way a payload can
// be wrong is refused as not compliant.
func TestCheck_HoldsThePayloadToTheSchema(t *testing.T) {
	t.Parallel()
	d := mustParse(t, bpManifest)

	if err := d.Check(mustDecode(t, bpPayload)); err != nil {
		t.Errorf("valid payload rejected: %v", err)
	}
	refused := map[string]string{
		"missing required": `{"measured_at": "x", "systolic": 118}`,
		"enum violation":   strings.Replace(bpPayload, "sitting", "hovering", 1),
		"undeclared field": strings.Replace(bpPayload, `"comment"`, `"commment"`, 1),
		"wrong type":       strings.Replace(bpPayload, `"systolic": 118`, `"systolic": "118"`, 1),
		"not an object":    `[1, 2]`,
	}
	for name, payload := range refused {
		if err := d.Check(mustDecode(t, payload)); !errors.Is(err, def.ErrNotCompliant) {
			t.Errorf("%s: Check = %v, want ErrNotCompliant", name, err)
		}
	}
}

func TestCheck_VerbatimChecksNothing(t *testing.T) {
	t.Parallel()
	d := mustParse(t, strings.Replace(verbatimManifest, `"strict"`, `"verbatim"`, 1))

	if err := d.Check(mustDecode(t, `{"anything": true}`)); err != nil {
		t.Errorf("verbatim Check: %v", err)
	}
	if _, err := def.Parse([]byte(verbatimManifest)); !errors.Is(err, def.ErrBadManifest) {
		t.Errorf("strict with no schema = %v, want ErrBadManifest", err)
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

// Card 12: a zoneless clock is read in the Definition's zone, on both sides of a
// daylight-saving change, and a clock that carries its own offset ignores the zone.
func TestTimes_ZonelessClockIsReadInTheDefinitionsZone(t *testing.T) {
	t.Parallel()
	sydney := strings.Replace(strings.Replace(bpManifest,
		`"zone": "UTC"`, `"zone": "Australia/Sydney"`, 1),
		`"layout": "2006-01-02T15:04:05Z07:00"`, `"layout": "2006-01-02 15:04"`, 1)
	d := mustParse(t, sydney)

	cases := []struct{ in, want string }{
		{"2024-08-16 07:46", "2024-08-15T21:46:00Z"}, // AEST, +10
		{"2024-10-16 07:46", "2024-10-15T20:46:00Z"}, // AEDT, +11, after the first Sunday of October
		{"2026-04-05 01:30", "2026-04-04T14:30:00Z"}, // the morning the clocks go back
	}
	for _, c := range cases {
		got, err := d.FactTimeOf(mustDecode(t, `{"measured_at": "`+c.in+`", "systolic": 1, "diastolic": 1}`))
		if err != nil {
			t.Fatalf("FactTimeOf(%s): %v", c.in, err)
		}
		if got.Format(time.RFC3339) != c.want || got.Location() != time.UTC {
			t.Errorf("FactTimeOf(%s) = %s, want %s", c.in, got.Format(time.RFC3339), c.want)
		}
	}

	if _, err := def.Parse([]byte(strings.Replace(bpManifest, `"zone": "UTC"`, `"zone": "Mars/Olympus"`, 1))); !errors.Is(err, def.ErrBadManifest) {
		t.Errorf("unknown zone = %v, want ErrBadManifest", err)
	}
	if _, err := def.Parse([]byte(strings.Replace(bpManifest, `, "zone": "UTC"`, ``, 1))); !errors.Is(err, def.ErrBadManifest) {
		t.Errorf("missing zone = %v, want ErrBadManifest", err)
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
	if want := facet.SourceRef("bp-csv", "2026-01-24T06:30:00Z"); ref != want {
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
