package def_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/def"
)

const medStartedManifest = `{
  "meta": {
    "facet_type": "med.started",
    "definition_version": 1,
    "classification": "Sensitive",
    "validation_mode": "lenient",
    "source_schema": "records/med.started",
    "time_semantics": {"fact_time": {"pointer": "/date", "layout": "2006-01-02"}},
    "identity_key": ["/id"],
    "series": {"name": "med.regimen", "key": ["/drug", "/course"]}
  },
  "schema": {"required": ["/id", "/date", "/drug"]}
}`

const medStoppedManifest = `{
  "meta": {
    "facet_type": "med.stopped",
    "definition_version": 1,
    "classification": "Sensitive",
    "validation_mode": "lenient",
    "source_schema": "records/med.stopped",
    "time_semantics": {"fact_time": {"pointer": "/date", "layout": "2006-01-02"}},
    "identity_key": ["/id"],
    "series": {"name": "med.regimen", "key": ["/drug", "/course"], "closes": true}
  },
  "schema": {"required": ["/id", "/date", "/drug"]}
}`

func TestSeries_ParsesAndComposesTheStamp(t *testing.T) {
	t.Parallel()
	started := mustParse(t, medStartedManifest)
	stopped := mustParse(t, medStoppedManifest)

	if started.Series == nil || started.Series.Closes {
		t.Fatalf("med.started series = %+v, want an open series", started.Series)
	}
	if stopped.Series == nil || !stopped.Series.Closes {
		t.Fatalf("med.stopped series = %+v, want a closing series", stopped.Series)
	}

	doc := mustDecode(t, `{"id": "r1", "date": "2026-03-01", "drug": "ramipril", "course": 1}`)
	got, err := started.SeriesOf(doc)
	if err != nil {
		t.Fatalf("SeriesOf: %v", err)
	}
	if want := "med.regimen:ramipril:1"; got != want {
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

func TestSeries_StrictModeAdmitsTheKeyFields(t *testing.T) {
	t.Parallel()
	m := strings.Replace(medStartedManifest, `"validation_mode": "lenient"`, `"validation_mode": "strict"`, 1)
	d := mustParse(t, m)
	doc := mustDecode(t, `{"id": "r1", "date": "2026-03-01", "drug": "ramipril", "course": 1}`)
	if err := d.Check(doc); err != nil {
		t.Errorf("strict Check refused the series key fields: %v", err)
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
	if !reg.Closes("med.stopped") || reg.Closes("med.started") || reg.Closes("health.blood-pressure") || reg.Closes("no.such") {
		t.Error("Closes: only med.stopped closes")
	}
}
