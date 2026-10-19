package query_test

import (
	"sort"
	"testing"

	"github.com/mattjoyce/anam/internal/fold"
)

// The equality check over the fold package's own snapshot fixture: one regimen
// started, changed, stopped and restarted as a new course, beside a point fact. The
// bundled current(E, At) equals fold.Snapshot at every date the fixture tests and at
// every fact-time it holds.
func TestEquality_CurrentIsTheSnapshot_FoldFixture(t *testing.T) {
	t.Parallel()
	v, versions, dates, text := regimenFixture(t)
	defs := registry(t)
	ans, err := tryRun(v, versions, defs, text, "current")
	if err != nil {
		t.Fatalf("current: %v", err)
	}
	for _, e := range v.Entries() {
		dates = append(dates, e.FactTime)
	}
	for _, at := range dates {
		got, want := currentAt(ans, at), snapshotIDs(t, v, at, defs.Closes)
		if !same(got, want) {
			t.Errorf("current at %s = %v, Snapshot = %v", at[:10], got, want)
		}
	}
	if got := currentAt(ans, day("2026-11-01")); !same(got, []string{"start-2", "tie-b", "weight"}) {
		t.Errorf("current on the tied day = %v, want the later of the pair", got)
	}
}

// displaced/2 is the fold's Displaced fate: the earlier statement of a series, once
// a later one stands at the instant asked about.
func TestEquality_DisplacedIsTheFoldsFate(t *testing.T) {
	t.Parallel()
	v, versions, dates, text := regimenFixture(t)
	defs := registry(t)
	displaced, err := tryRun(v, versions, defs, text, "displaced")
	if err != nil {
		t.Fatalf("displaced: %v", err)
	}
	for _, at := range dates {
		cut, err := fold.Until(v, at)
		if err != nil {
			t.Fatalf("Until: %v", err)
		}
		var want []string
		for i, f := range fold.Fates(cut.Entries(), defs.Closes) {
			if f.Displaced {
				want = append(want, cut.Entries()[i].FacetID)
			}
		}
		sort.Strings(want)
		if got := currentAt(displaced, at); !same(got, want) {
			t.Errorf("displaced at %s = %v, Fates = %v", at[:10], got, want)
		}
	}
}

// regimenFixture is the fold package's snapshot fixture with a tied pair added, the
// dates it asks about, and the query text naming them as instants.
func regimenFixture(t *testing.T) (fold.Value, map[string]int, []string, string) {
	t.Helper()
	const (
		course1 = "med.regimen:ramipril:1"
		course2 = "med.regimen:ramipril:2"
	)
	v, versions := value(t,
		entry("start-1", "health.medication-started", day("2026-03-01"), course1, `{}`),
		entry("dose-1", "health.medication-dose-changed", day("2026-05-10"), course1, `{}`),
		entry("stop-1", "health.medication-stopped", day("2026-08-01"), course1, `{}`),
		entry("start-2", "health.medication-started", day("2026-10-01"), course2, `{}`),
		entry("weight", "health.weight", day("2026-06-01"), "", `{}`),
		// Two statements at one instant: the value's order (fact_time, facet_id)
		// breaks the tie, in the fold and in the rules alike.
		entry("tie-a", "health.medication-started", day("2026-11-01"), "med.regimen:atorvastatin:1", `{}`),
		entry("tie-b", "health.medication-dose-changed", day("2026-11-01"), "med.regimen:atorvastatin:1", `{}`),
		entry("tie-c", "health.medication-stopped", day("2026-12-01"), "med.regimen:atorvastatin:1", `{}`),
	)
	dates := []string{day("2026-02-01"), day("2026-03-01"), day("2026-04-01"), day("2026-06-15"), day("2026-09-01"), day("2026-12-01"), day("2026-11-15"), day("2027-01-01")}
	text := "instant(At) :- entry(_, _, At, _, _, _, _).\n"
	for _, at := range dates {
		text += "instant(\"" + at + "\").\n"
	}
	return v, versions, dates, text
}
