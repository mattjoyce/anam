package app

import (
	"context"
	"fmt"
	"os"
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
	}

	same("before any mint")
	if _, _, err := cli(t, f.cfgPath, "checkpoint"); err != nil {
		t.Fatalf("checkpoint: %v", err)
	}
	same("at the mint")

	// A tail: a correction of one reading and a new one, after the mint.
	more := bpFixture + `  - {type: health.blood-pressure, id: "2026-3-01 07:00:00:1", date_excel: "2026-3-01 07:00:00", reading: "1", systolic: "111.0", diastolic: "70.0", pulse: "60.0"}
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
