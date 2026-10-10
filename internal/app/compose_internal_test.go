package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/api"
	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/read"
)

// A timeline composed from checkpoints is the timeline the full fold gives, value for
// value: the entries, the hashes, the counts and each row's version, at head and as of
// an earlier anam-time, through a projection and as a snapshot. It is checked with no
// mint run, after one, and with a tail of new readings after it.
func TestTimeline_ComposedIsTheFullFold(t *testing.T) {
	t.Parallel()
	f := newIngestFixture(t)
	imports := fmt.Sprintf("imports:\n  - source: records\n    path: %s\n", f.recordsDir)
	// A medicine course opened, closed and reopened across three months, so a snapshot
	// read from a prefix of buckets has a series begun in an earlier bucket and a
	// closing event to see (card 64).
	meds := `document: {name: meds.md, sha256: "0000000000000000000000000000000000000000000000000000000000000000"}
extracted: {method: by hand, version: 1}
records:
  - {type: med.started, id: "m/1/started", date: "2025-12-01", drug: examplazole, course: 1, dose: "10 mg daily"}
  - {type: med.dose-changed, id: "m/1/dose", date: "2026-01-05", drug: examplazole, course: 1, dose: "20 mg daily"}
  - {type: med.stopped, id: "m/1/stopped", date: "2026-01-20", drug: examplazole, course: 1}
  - {type: med.started, id: "m/2/started", date: "2026-02-05", drug: examplazole, course: 2, dose: "10 mg daily"}
`
	if err := os.WriteFile(filepath.Join(f.recordsDir, "meds.yaml"), []byte(meds), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.ingest(t, imports); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(f.cfgPath)
	if err != nil {
		t.Fatal(err)
	}

	same := func(stage string) {
		t.Helper()
		ctx := context.Background()
		src, closeAll, err := readSource(ctx, cfg)
		if err != nil {
			t.Fatal(err)
		}
		defer closeAll()
		full := src
		full.Checkpoints = nil
		for _, q := range []api.TimelineQuery{
			{}, {From: "2026-02-01"}, {To: "2026-02-10"}, {At: "2026-02-10"},
			{At: "2025-12-15"}, {At: "2026-01-10"}, {At: "2026-01-25"}, {At: "2026-03-01"},
			{At: "2026-01-10", From: "2026-01-01"}, {At: "2026-02-10", Type: "med.started"},
			{At: "2026-01-10", AsOf: "2099-01-01"},
			{Type: "health.blood-pressure"}, {Projection: "bp"}, {AsOf: "2000-01-01"},
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
			"2026-01-10": "med.dose-changed", "2026-01-25": "", "2026-02-10": "med.started",
		} {
			got, err := read.Timeline(ctx, src, api.TimelineQuery{At: at, Zone: "UTC"})
			if err != nil {
				t.Fatal(err)
			}
			var meds []string
			for _, e := range got.Entries {
				if strings.HasPrefix(string(e.Type), "med.") {
					meds = append(meds, string(e.Type))
				}
			}
			if (want == "" && len(meds) != 0) || (want != "" && (len(meds) != 1 || meds[0] != want)) {
				t.Errorf("%s: snapshot at %s holds %v, want [%s]", stage, at, meds, want)
			}
		}
	}

	same("before any mint")
	if _, _, err := cli(t, f.cfgPath, "checkpoint"); err != nil {
		t.Fatalf("checkpoint: %v", err)
	}
	same("at the mint")

	// A tail: a correction of one reading and a new one, after the mint.
	more := bpFixture + `  - {type: health.blood-pressure, id: "2026-3-01 07:00:00:1", date_excel: "2026-3-01 07:00:00", reading: "1", systolic_blood_pressure: {value: 111.0, unit: mmHg}, diastolic_blood_pressure: {value: 70.0, unit: mmHg}, effective_time_frame: {date_time: "2026-03-01T07:00:00+11:00"}, pulse: "60.0"}
`
	more = strings.Replace(more, "114.0", "115.0", 1)
	if err := os.WriteFile(f.bpPath, []byte(more), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.ingest(t, imports); err != nil {
		t.Fatal(err)
	}
	same("with a tail")
}
