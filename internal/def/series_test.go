package def_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/facet"

	"github.com/mattjoyce/anam/internal/def"
)

const medStartedManifest = `{
  "meta": {
    "facet_type": "health.medication-started",
    "definition_version": 1,
    "classification": "Sensitive",
    "validation_mode": "strict",
    "source_schema": "records/health.medication-started",
    "time_semantics": {"fact_time": {"pointer": "/date", "layout": "2006-01-02", "zone": "UTC"}},
    "identity_key": ["/id"],
    "series": {"name": "med.regimen", "key": ["/drug", "/course"]}
  },
  "schema": {"type": "object", "required": ["id", "date", "drug", "course"],
    "properties": {"id": {"type": "string", "minLength": 1}, "date": {"type": "string", "minLength": 1},
      "drug": {"type": "string", "minLength": 1}, "course": {"type": "number"}},
    "additionalProperties": false}
}`

const medStoppedManifest = `{
  "meta": {
    "facet_type": "health.medication-stopped",
    "definition_version": 1,
    "classification": "Sensitive",
    "validation_mode": "strict",
    "source_schema": "records/health.medication-stopped",
    "time_semantics": {"fact_time": {"pointer": "/date", "layout": "2006-01-02", "zone": "UTC"}},
    "identity_key": ["/id"],
    "series": {"name": "med.regimen", "key": ["/drug", "/course"], "closes": true}
  },
  "schema": {"type": "object", "required": ["id", "date", "drug", "course"],
    "properties": {"id": {"type": "string", "minLength": 1}, "date": {"type": "string", "minLength": 1},
      "drug": {"type": "string", "minLength": 1}, "course": {"type": "number"}},
    "additionalProperties": false}
}`

func TestSeries_ParsesAndComposesTheStamp(t *testing.T) {
	t.Parallel()
	started := mustParse(t, medStartedManifest)
	stopped := mustParse(t, medStoppedManifest)

	if started.Series == nil || started.Series.Closes {
		t.Fatalf("health.medication-started series = %+v, want an open series", started.Series)
	}
	if stopped.Series == nil || !stopped.Series.Closes {
		t.Fatalf("health.medication-stopped series = %+v, want a closing series", stopped.Series)
	}

	doc := mustDecode(t, `{"id": "r1", "date": "2026-03-01", "drug": "ramipril", "course": 1}`)
	got, err := started.SeriesOf(doc)
	if err != nil {
		t.Fatalf("SeriesOf: %v", err)
	}
	if want := facet.Stamp("med.regimen", "ramipril", "1"); got != want {
		t.Errorf("SeriesOf = %q, want %q", got, want)
	}

	// The same thing named by a different type composes the same stamp: that is
	// what lets a stop close a start.
	fromStop, err := stopped.SeriesOf(doc)
	if err != nil {
		t.Fatalf("SeriesOf(stopped): %v", err)
	}
	if fromStop != got {
		t.Errorf("stop composes %q, start composes %q", fromStop, got)
	}

	// A type without a series stamps nothing.
	bp := mustParse(t, bpManifest)
	none, err := bp.SeriesOf(mustDecode(t, bpPayload))
	if err != nil || none != "" {
		t.Errorf("SeriesOf on a type without a series = %q, %v", none, err)
	}
}

func TestSeries_UnresolvedKeyIsNotCompliant(t *testing.T) {
	t.Parallel()
	started := mustParse(t, medStartedManifest)

	cases := map[string]string{
		"course missing": `{"id": "r1", "date": "2026-03-01", "drug": "ramipril"}`,
		"drug empty":     `{"id": "r1", "date": "2026-03-01", "drug": "", "course": 1}`,
		"course is null": `{"id": "r1", "date": "2026-03-01", "drug": "ramipril", "course": null}`,
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := started.SeriesOf(mustDecode(t, payload))
			if !errors.Is(err, def.ErrNotCompliant) {
				t.Errorf("SeriesOf = %v, want ErrNotCompliant", err)
			}
		})
	}
}

func TestSeries_RejectsBadDeclarations(t *testing.T) {
	t.Parallel()
	breakages := map[string]string{
		"name not namespaced": `"series": {"name": "regimen", "key": ["/drug"]}`,
		"empty key":           `"series": {"name": "med.regimen", "key": []}`,
		"key not a pointer":   `"series": {"name": "med.regimen", "key": ["drug"]}`,
		"unknown field":       `"series": {"name": "med.regimen", "key": ["/drug"], "group": true}`,
	}
	for name, series := range breakages {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			m := strings.Replace(medStartedManifest,
				`"series": {"name": "med.regimen", "key": ["/drug", "/course"]}`, series, 1)
			if m == medStartedManifest {
				t.Fatal("replacement did not apply")
			}
			if _, err := def.Parse([]byte(m)); !errors.Is(err, def.ErrBadManifest) {
				t.Errorf("Parse = %v, want ErrBadManifest", err)
			}
		})
	}
}

// A series key keys the facet as much as its identity does, so the schema must
// require it: a schema that let course be absent would admit a payload the gate
// cannot place in a series.
func TestSeries_TheKeyFieldsMustBeRequired(t *testing.T) {
	t.Parallel()
	d := mustParse(t, medStartedManifest)
	doc := mustDecode(t, `{"id": "r1", "date": "2026-03-01", "drug": "ramipril", "course": 1}`)
	if err := d.Check(doc); err != nil {
		t.Errorf("Check refused the series key fields: %v", err)
	}
	m := strings.Replace(medStartedManifest, `"required": ["id", "date", "drug", "course"]`, `"required": ["id", "date", "drug"]`, 1)
	if _, err := def.Parse([]byte(m)); !errors.Is(err, def.ErrBadManifest) {
		t.Errorf("course left optional = %v, want ErrBadManifest", err)
	}
}

func TestRegistry_RefusesAFamilyKeyedTwoWays(t *testing.T) {
	t.Parallel()
	started := mustParse(t, medStartedManifest)
	stopped := mustParse(t, strings.Replace(medStoppedManifest, `["/drug", "/course"]`, `["/drug"]`, 1))

	if _, err := def.NewRegistry(started, stopped); !errors.Is(err, def.ErrBadManifest) {
		t.Fatalf("NewRegistry over two keys for one family = %v, want ErrBadManifest", err)
	}

	reg, err := def.NewRegistry(started, mustParse(t, medStoppedManifest), mustParse(t, bpManifest))
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	if !reg.Closes("health.medication-stopped") || reg.Closes("health.medication-started") || reg.Closes("health.blood-pressure") || reg.Closes("no.such") {
		t.Error("Closes: only health.medication-stopped closes")
	}
}

func TestParse_PayloadKind(t *testing.T) {
	t.Parallel()
	ref := strings.Replace(medStartedManifest, `"validation_mode": "strict",`, `"validation_mode": "strict", "payload_kind": "reference",`, 1)
	if d := mustParse(t, ref); d.PayloadKind != "reference" {
		t.Errorf("payload_kind = %q", d.PayloadKind)
	}
	bad := strings.Replace(medStartedManifest, `"validation_mode": "strict",`, `"validation_mode": "strict", "payload_kind": "pointer",`, 1)
	if _, err := def.Parse([]byte(bad)); !errors.Is(err, def.ErrBadManifest) {
		t.Errorf("payload_kind pointer = %v, want ErrBadManifest", err)
	}
}

// Required means filled: a required field present but null or "" is refused, and a
// zero is a value the source wrote, not a gap. The schema says so (Parse refuses one
// that does not), so this is the validator honouring it.
func TestCheck_RequiredMeansFilled(t *testing.T) {
	t.Parallel()
	d := mustParse(t, bpManifest)
	cases := map[string]bool{
		`{"measured_at": "2026-01-24T06:30:00Z", "systolic": 118, "diastolic": 76}`:  true,
		`{"measured_at": "2026-01-24T06:30:00Z", "systolic": null, "diastolic": 76}`: false,
		`{"measured_at": "2026-01-24T06:30:00Z", "systolic": "", "diastolic": 76}`:   false,
		`{"measured_at": "2026-01-24T06:30:00Z", "systolic": 0, "diastolic": 76}`:    true,
		`{"measured_at": "", "systolic": 118, "diastolic": 76}`:                      false,
	}
	for payload, want := range cases {
		err := d.Check(mustDecode(t, payload))
		if got := err == nil; got != want {
			t.Errorf("Check(%s) = %v, want admitted=%t", payload, err, want)
		}
		if err != nil && !errors.Is(err, def.ErrNotCompliant) {
			t.Errorf("%v is not ErrNotCompliant", err)
		}
	}
}
