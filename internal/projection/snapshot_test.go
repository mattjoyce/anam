package projection_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/consent"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/fold"
	"github.com/mattjoyce/anam/internal/projection"
)

const medsTemplate = `{
  "name": "meds", "version": 1, "description": "the current regimen",
  "types": ["health.medication-started", "health.medication-dose-changed", "health.medication-stopped"],
  "statuses": ["admitted", "endorsed"],
  "max_classification": "Sensitive",
  "from": "", "to": "",
  "shape": "snapshot"
}`

// closer is the Definitions' word, stubbed: health.medication-stopped closes.
type closer struct{}

func (closer) Closes(t facet.Type) bool { return t == "health.medication-stopped" }

func seriesEntry(id string, typ facet.Type, factTime, series string) fold.Entry {
	e := entry(id, typ, factTime, facet.StatusAdmitted, facet.ClassificationSensitive)
	e.Series = series
	return e
}

// A regimen as a log, then as a snapshot. The log serves every statement; the
// snapshot serves the latest per course, drops the closed course whole, and says why
// for each one it demoted — by the same procedure Apply used.
func TestSnapshotShape_LatestPerSeriesWithReasons(t *testing.T) {
	t.Parallel()
	v, err := fold.NewValue([]fold.Entry{
		seriesEntry("start-1", "health.medication-started", "2026-03-01T00:00:00.000000000Z", "med.regimen:ramipril:1"),
		seriesEntry("dose-1", "health.medication-dose-changed", "2026-05-10T00:00:00.000000000Z", "med.regimen:ramipril:1"),
		seriesEntry("stop-1", "health.medication-stopped", "2026-08-01T00:00:00.000000000Z", "med.regimen:ramipril:1"),
		seriesEntry("start-2", "health.medication-started", "2026-10-01T00:00:00.000000000Z", "med.regimen:ramipril:2"),
		seriesEntry("start-x", "health.medication-started", "2026-04-01T00:00:00.000000000Z", "med.regimen:atorvastatin:1"),
	})
	if err != nil {
		t.Fatalf("NewValue: %v", err)
	}
	snapshot := mustParse(t, medsTemplate)
	log := mustParse(t, strings.Replace(medsTemplate, `"shape": "snapshot"`, `"shape": "log"`, 1))

	served, err := projection.Apply(v, snapshot, consent.Open{}, closer{})
	if err != nil {
		t.Fatalf("Apply(snapshot): %v", err)
	}
	if got, want := ids(served), []string{"start-x", "start-2"}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("snapshot served %v, want %v", got, want)
	}

	asLog, err := projection.Apply(v, log, consent.Open{}, nil)
	if err != nil {
		t.Fatalf("Apply(log): %v", err)
	}
	if asLog.Len() != 5 {
		t.Errorf("log served %d, want all 5", asLog.Len())
	}

	verdicts, err := projection.Explain(v, snapshot, consent.Open{}, closer{})
	if err != nil {
		t.Fatalf("Explain: %v", err)
	}
	want := map[string]struct {
		reason projection.Reason
		detail string
	}{
		"start-1": {projection.ReasonDisplaced, "by dose-1"},
		"dose-1":  {projection.ReasonDisplaced, "by stop-1"},
		"stop-1":  {projection.ReasonClosed, "by stop-1"},
		"start-2": {projection.ReasonServed, ""},
		"start-x": {projection.ReasonServed, ""},
	}
	for _, x := range verdicts {
		w := want[x.Entry.FacetID]
		if x.Reason != w.reason || x.Detail != w.detail {
			t.Errorf("%s: %q %q, want %q %q", x.Entry.FacetID, x.Reason, x.Detail, w.reason, w.detail)
		}
	}
}

// The window's To is the snapshot's "at": with To set to September, the stop is
// outside the window and the dose change is what was in force.
func TestSnapshotShape_WindowToIsTheAt(t *testing.T) {
	t.Parallel()
	v, err := fold.NewValue([]fold.Entry{
		seriesEntry("start-1", "health.medication-started", "2026-03-01T00:00:00.000000000Z", "med.regimen:ramipril:1"),
		seriesEntry("dose-1", "health.medication-dose-changed", "2026-05-10T00:00:00.000000000Z", "med.regimen:ramipril:1"),
		seriesEntry("stop-1", "health.medication-stopped", "2026-08-01T00:00:00.000000000Z", "med.regimen:ramipril:1"),
	})
	if err != nil {
		t.Fatalf("NewValue: %v", err)
	}
	tmpl := mustParse(t, strings.Replace(medsTemplate, `"to": ""`, `"to": "2026-07-01T00:00:00.000000000Z"`, 1))
	served, err := projection.Apply(v, tmpl, consent.Open{}, closer{})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if got := ids(served); strings.Join(got, ",") != "dose-1" {
		t.Errorf("snapshot at July served %v, want [dose-1]", got)
	}
}

// A snapshot shape without the Definitions' series rules is refused, as a missing
// consent gate is: a serve path cannot be written that forgets it.
func TestSnapshotShape_NeedsTheSeriesRules(t *testing.T) {
	t.Parallel()
	if _, err := projection.Apply(canonical(t), mustParse(t, medsTemplate), consent.Open{}, nil); err == nil {
		t.Fatal("Apply(snapshot, nil series) succeeded")
	}
}

// The shape admits two words, and a template that says nothing is a log: exactly
// what it served before the knob existed.
func TestParse_ShapeDefaultsToLog(t *testing.T) {
	t.Parallel()
	unknown := strings.Replace(medsTemplate, `"snapshot"`, `"latest"`, 1)
	if _, err := projection.Parse([]byte(unknown)); !errors.Is(err, projection.ErrBadTemplate) {
		t.Errorf("Parse(shape latest) = %v, want ErrBadTemplate", err)
	}
	absent := strings.Replace(medsTemplate, "\"to\": \"\",\n  \"shape\": \"snapshot\"", `"to": ""`, 1)
	if absent == medsTemplate {
		t.Fatal("replacement did not apply")
	}
	tmpl, err := projection.Parse([]byte(absent))
	if err != nil || tmpl.Shape != projection.ShapeLog {
		t.Errorf("Parse(no shape) = %q, %v; want log", tmpl.Shape, err)
	}
}

// The bundled meds template is snapshot-shaped over the three regimen types.
func TestBundled_MedsIsASnapshot(t *testing.T) {
	t.Parallel()
	reg, err := projection.Bundled()
	if err != nil {
		t.Fatalf("Bundled: %v", err)
	}
	meds, err := reg.Lookup("meds")
	if err != nil {
		t.Fatalf("Lookup(meds): %v", err)
	}
	if meds.Shape != projection.ShapeSnapshot || len(meds.Types) != 3 {
		t.Errorf("meds = shape %q over %v", meds.Shape, meds.Types)
	}
	for _, name := range []string{"bp", "sleep"} {
		tmpl, err := reg.Lookup(name)
		if err != nil || tmpl.Shape != projection.ShapeLog {
			t.Errorf("%s = shape %q, %v; want log", name, tmpl.Shape, err)
		}
	}
}
