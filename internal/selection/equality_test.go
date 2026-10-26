package selection_test

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/mattjoyce/anam/internal/consent"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/fold"
	"github.com/mattjoyce/anam/internal/projection"
	"github.com/mattjoyce/anam/internal/query"
	"github.com/mattjoyce/anam/internal/selection"
	"github.com/mattjoyce/anam/internal/serve"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

// The equality check over a real ledger (AGENTS.md: real components): facets admitted,
// one corrected and one retracted, folded, served, and the compiled program run
// through the query evaluator. Its pick/1 is exactly the facets the filter's own
// direct predicate picks, for every filter in the table and over every value served.

const person = "test-person"

type seeded struct {
	ref, typ, source string
	at               time.Time
}

var record = []seeded{
	{"bp-1", "health.blood-pressure", "bp-csv", day(2026, 9, 30, 23)},
	{"bp-2", "health.blood-pressure", "bp-csv", day(2026, 10, 1, 0)},
	{"bp-3", "health.blood-pressure", "withings", day(2026, 10, 7, 23)},
	{"bp-4", "health.blood-pressure", "withings", day(2026, 10, 8, 0)},
	{"hr-1", "health.heart-rate.alert", "garmin", day(2026, 10, 3, 9)},
	{"hr-2", "health.heart-rate.alert", "garmin", day(2026, 10, 3, 10)},
	{"hrd-1", "health.heart-rate.day", "garmin", day(2026, 10, 3, 0)},
	{"hrv-1", "health.heart-rate-variability", "garmin", day(2026, 10, 3, 0)},
	{"hrx-1", "health.heart-rate", "garmin", day(2026, 10, 3, 0)},
	{"run-1", "activity.exercise-session.run", "garmin", day(2026, 10, 4, 7)},
	{"row-1", "activity.exercise-session.row", "concept2", day(2026, 10, 5, 7)},
	{"ses-1", "activity.exercise-sessions", "garmin", day(2026, 10, 5, 7)},
	{"wt-1", "health.weight", "withings", day(2026, 10, 2, 7)},
	{"wt-2", "health.weight", "withings", day(2026, 10, 2, 7)},
	{"gone", "health.weight", "withings", day(2026, 10, 2, 8)},
}

func day(y int, m time.Month, d, h int) time.Time { return time.Date(y, m, d, h, 0, 0, 0, time.UTC) }

func facetOf(s seeded, version int, payload string) facet.Facet {
	canonical, err := facet.Canonicalize([]byte(payload))
	if err != nil {
		panic(err)
	}
	return facet.Facet{
		ID: facet.NewID(s.ref), Type: facet.Type(s.typ), Confidence: 1,
		Classification: facet.ClassificationNormal, ArtifactVersion: version, DefinitionVersion: 1,
		FactTime:   s.at,
		Binding:    facet.Binding{PersonID: person, Assurance: facet.AssuranceHigh},
		Provenance: facet.Provenance{Source: s.source, Injector: s.source, SourceRef: s.ref},
		Descriptor: facet.Descriptor{
			SourceSchema: "test", SchemaVersion: 1, ContentHash: facet.ContentHash(canonical),
			PayloadKind: facet.PayloadVerbatim, MediaType: "application/json",
		},
		Payload: canonical,
	}
}

// seed admits the record, corrects wt-1 and retracts gone, and folds the head.
func seed(t *testing.T) fold.Result {
	t.Helper()
	ctx := context.Background()
	l, err := ledger.Create(ctx, filepath.Join(t.TempDir(), "ledger.db"), person)
	if err != nil {
		t.Fatalf("ledger.Create: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })
	var seq int64
	next := func(kind facet.AnamEventKind, id string, version, prior int) facet.AnamEvent {
		seq++
		return facet.AnamEvent{Kind: kind, FacetID: id, ArtifactVersion: version, PriorArtifactVersion: prior,
			AnamTime: time.Date(2026, 10, 20, 9, 0, 0, int(seq), time.UTC), Seq: seq}
	}
	for i, s := range record {
		f := facetOf(s, 1, fmt.Sprintf(`{"n": %d}`, i))
		w := ledger.Watermark(seq)
		if err := l.Admit(ctx, w, f, next(facet.EventAdmitted, f.ID, 1, 0)); err != nil {
			t.Fatalf("Admit %s: %v", s.ref, err)
		}
	}
	corrected := facetOf(record[13], 2, `{"n": 99}`)
	w := ledger.Watermark(seq)
	if err := l.Admit(ctx, w, corrected, next(facet.EventSuperseded, corrected.ID, 2, 1)); err != nil {
		t.Fatalf("Admit correction: %v", err)
	}
	w = ledger.Watermark(seq)
	if err := l.Mark(ctx, w, next(facet.EventTombstoned, facet.NewID("gone"), 1, 0)); err != nil {
		t.Fatalf("Mark: %v", err)
	}
	res, err := fold.Read(ctx, l)
	if err != nil {
		t.Fatalf("fold.Read: %v", err)
	}
	return res
}

func id(ref string) string { return facet.NewID(ref) }

func TestEquality_ProgramIsThePredicate(t *testing.T) {
	t.Parallel()
	res := seed(t)
	defs, err := def.Bundled()
	if err != nil {
		t.Fatal(err)
	}
	bp, err := projection.Bundled()
	if err != nil {
		t.Fatal(err)
	}
	tmpl, err := bp.Lookup("bp")
	if err != nil {
		t.Fatal(err)
	}
	narrowed, err := serve.Derive(res.Value, tmpl, consent.Open{}, defs, "")
	if err != nil {
		t.Fatal(err)
	}
	values := map[string]fold.Value{"canonical": res.Value, "bp": narrowed}

	cases := map[string]selection.Filter{
		"ticks of facets served and not":  {Ticks: []string{id("hr-1"), id("gone"), "not-a-facet"}},
		"exclusions without criteria":     {Exclusions: []string{id("hr-1")}},
		"a window on its edges":           {From: "2026-10-01T00:00:00.000000000Z", To: "2026-10-08T00:00:00.000000000Z"},
		"a prefix at its boundary":        {Types: []string{"health.heart-rate."}},
		"an exact type is not its prefix": {Types: []string{"health.heart-rate"}},
		"a prefix is not a longer word":   {Types: []string{"activity.exercise-session."}},
		"two types, two sources":          {Types: []string{"health.blood-pressure", "health.weight"}, Sources: []string{"withings", "bp-csv"}},
		"criteria, ticks and exclusions": {
			Types: []string{"health.heart-rate.", "activity.exercise-session."}, From: "2026-10-03T00:00:00.000000000Z",
			Ticks: []string{id("bp-1")}, Exclusions: []string{id("hr-2"), id("bp-1")},
		},
		"superseded over a served value": {Standing: selection.Superseded},
		"a source alone":                 {Sources: []string{"concept2"}},
	}
	for vname, served := range values {
		var rows []selection.Row
		for _, e := range served.Entries() {
			rows = append(rows, selection.RowOf(e))
		}
		for name, f := range cases {
			text, err := f.Program()
			if err != nil {
				t.Fatalf("%s: Program: %v", name, err)
			}
			ans, _, err := query.Run(context.Background(), served, query.Versions(res.Live), defs, text, selection.Relation, 0)
			if err != nil {
				t.Fatalf("%s over %s: Run: %v\n%s", name, vname, err, text)
			}
			var got []string
			for _, tu := range ans.Tuples() {
				got = append(got, tu[0].Text())
			}
			if want := f.Canonical().Picks(rows); !reflect.DeepEqual(got, want) {
				t.Errorf("%s over %s: the program picks %v, the predicate %v\n%s", name, vname, got, want, text)
			}
		}
	}
}

// The table's cases pick what a person would expect of them, so the equality above is
// not two readings agreeing on nothing.
func TestPredicate_PicksWhatTheCriteriaSay(t *testing.T) {
	t.Parallel()
	res := seed(t)
	var rows []selection.Row
	for _, e := range res.Value.Entries() {
		rows = append(rows, selection.RowOf(e))
	}
	ids := func(refs ...string) []string {
		var out []string
		for _, r := range refs {
			out = append(out, id(r))
		}
		return selection.Filter{Ticks: out}.Canonical().Ticks
	}
	for name, c := range map[string]struct {
		f    selection.Filter
		want []string
	}{
		"prefix":  {selection.Filter{Types: []string{"health.heart-rate."}}, ids("hr-1", "hr-2", "hrd-1")},
		"window":  {selection.Filter{Types: []string{"health.blood-pressure"}, From: "2026-10-01T00:00:00.000000000Z", To: "2026-10-08T00:00:00.000000000Z"}, ids("bp-2", "bp-3")},
		"exact":   {selection.Filter{Types: []string{"health.heart-rate"}}, ids("hrx-1")},
		"retired": {selection.Filter{Types: []string{"health.weight"}}, ids("wt-1", "wt-2")},
		"untick":  {selection.Filter{Sources: []string{"concept2", "withings"}, Exclusions: []string{id("bp-3"), id("bp-4")}}, ids("row-1", "wt-1", "wt-2")},
	} {
		if got := c.f.Picks(rows); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: Picks = %v, want %v", name, got, c.want)
		}
	}
}
