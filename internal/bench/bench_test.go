package bench_test

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mattjoyce/anam/internal/bench"
)

// A tiny shape built and run end to end: the bench's own correctness, so a timing it
// reports is a timing of the reads it names.
func TestGenerateThenRun(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	sh := bench.Shape{
		Name: "tiny", Months: 3, PerMonth: 40, CorrectEvery: 10, Retract: 2, TailNew: 5, TailCorrect: 3,
		Start: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	if err := bench.Generate(context.Background(), dir, sh, nil); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	rep, err := bench.Run(context.Background(), bench.Options{
		Name: "tiny", Person: bench.Person, Paths: bench.PathsIn(dir), Reps: 3, Timeout: time.Minute,
	}, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// 120 readings, 12 corrected (no new facets), 2 retracted, 5 new in the tail.
	if d := rep.Dataset; d.Facets != 125 || d.Live != 123 || d.Checkpoints == 0 {
		t.Errorf("dataset = %+v", d)
	}
	rows := map[string]int{}
	steps := map[string]int{}
	for _, s := range rep.Scenarios {
		rows[s.Name] = s.Rows
		steps[s.Name] = s.Steps
		if s.MinMS > s.MedianMS || s.MedianMS > s.MaxMS {
			t.Errorf("%s: min %f > med %f > max %f", s.Name, s.MinMS, s.MedianMS, s.MaxMS)
		}
	}
	if rows["timeline, whole"] != 123 || rows["timeline, one month"] == 0 || rows["changes, last day"] == 0 {
		t.Errorf("scenario rows = %v", rows)
	}
	for _, name := range []string{"query, every entry", "query, every attr", "query, current at every fact-time", "query, one type joined to its attrs"} {
		if steps[name] <= 0 {
			t.Errorf("%s steps = %d", name, steps[name])
		}
	}
	if rows["query, every entry"] != rep.Dataset.Live {
		t.Errorf("query, every entry rows = %d live = %d", rows["query, every entry"], rep.Dataset.Live)
	}
	if steps["replay all events"] != 0 {
		t.Errorf("non-query steps = %d", steps["replay all events"])
	}
	var out bytes.Buffer
	if err := bench.WriteComparison(&out, rep, rep); err != nil || !strings.Contains(out.String(), "full fold") {
		t.Errorf("comparison: %v\n%s", err, out.String())
	}
}
