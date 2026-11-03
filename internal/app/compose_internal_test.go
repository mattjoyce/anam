package app

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/api"
	"github.com/mattjoyce/anam/internal/catalog/catalogtest"
	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/read"
)

// A timeline composed from checkpoints is the timeline the full fold gives, value for
// value: the entries, the hashes, the counts and each row's version, at head and as of
// an earlier anam-time, through a projection and as a snapshot. It is checked with no
// mint run, after one, and with a tail of new readings after it.
func TestTimeline_ComposedIsTheFullFold(t *testing.T) {
	t.Parallel()
	f := newLedgerFixture(t)
	// A course opened, closed and reopened across three months, so a snapshot
	// read from a prefix of buckets has a series begun in an earlier bucket and a
	// closing event to see (card 64).
	courseRows := []record{
		{"test.course.started", `{"id": "m/1/started", "date": "2025-12-01", "name": "examplazole", "course": 1, "dose": "10 mg daily"}`},
		{"test.course.changed", `{"id": "m/1/dose", "date": "2026-01-05", "name": "examplazole", "course": 1, "dose": "20 mg daily"}`},
		{"test.course.stopped", `{"id": "m/1/stopped", "date": "2026-01-20", "name": "examplazole", "course": 1}`},
		{"test.course.started", `{"id": "m/2/started", "date": "2026-02-05", "name": "examplazole", "course": 2, "dose": "10 mg daily"}`},
	}
	f.admit(t, append(append([]record(nil), measureRows...), courseRows...))
	cfg, err := config.Load(f.cfgPath)
	if err != nil {
		t.Fatal(err)
	}

	same := func(stage string) {
		t.Helper()
		ctx := context.Background()
		src, closeAll, err := readSource(ctx, cfg, catalogtest.Engine.Catalog(t))
		if err != nil {
			t.Fatal(err)
		}
		defer closeAll()
		full := src
		full.Checkpoints = nil
		for _, q := range []api.TimelineQuery{
			{}, {From: "2026-02-01"}, {To: "2026-02-10"}, {At: "2026-02-10"},
			{At: "2025-12-15"}, {At: "2026-01-10"}, {At: "2026-01-25"}, {At: "2026-03-01"},
			{At: "2026-01-10", From: "2026-01-01"}, {At: "2026-02-10", Type: "test.course.started"},
			{At: "2026-01-10", AsOf: "2099-01-01"},
			{Type: "test.measure"}, {Projection: "measures"}, {AsOf: "2000-01-01"},
			{AsOf: "2099-01-01"},
		} {
			q.Zone = "UTC"
			want, err := read.Timeline(ctx, full, q)
			if err != nil {
				t.Fatalf("%s %+v full: %v", stage, q, err)
			}
			got, err := read.Timeline(ctx, src, q)
			if err != nil {
				t.Fatalf("%s %+v composed: %v", stage, q, err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("%s %+v:\ncomposed %+v\nfull     %+v", stage, q, got, want)
			}
		}
		// And the snapshots say what the course was: on the dose change in January,
		// closed after the stop, the second course open in February.
		for at, want := range map[string]string{
			"2026-01-10": "test.course.changed", "2026-01-25": "", "2026-02-10": "test.course.started",
		} {
			got, err := read.Timeline(ctx, src, api.TimelineQuery{At: at, Zone: "UTC"})
			if err != nil {
				t.Fatal(err)
			}
			var courseRows []string
			for _, e := range got.Entries {
				if strings.HasPrefix(string(e.Type), "test.course.") {
					courseRows = append(courseRows, string(e.Type))
				}
			}
			if (want == "" && len(courseRows) != 0) || (want != "" && (len(courseRows) != 1 || courseRows[0] != want)) {
				t.Errorf("%s: snapshot at %s holds %v, want [%s]", stage, at, courseRows, want)
			}
		}
	}

	same("before any mint")
	if _, _, err := cli(t, f.cfgPath, "checkpoint"); err != nil {
		t.Fatalf("checkpoint: %v", err)
	}
	same("at the mint")

	// A tail: a correction of one reading and a new one, after the mint.
	more := append(corrected(measureRows, "114.0", "115.0"), record{"test.measure",
		`{"at": "2026-03-01 07:00:00", "seq": 1, "count": 111.0, "note": "60.0", "source_time": {"value": "2026-03-01 07:00:00", "zone": "Europe/Berlin", "basis": "configuration"}}`})
	f.admit(t, append(more, courseRows...))
	same("with a tail")
}
