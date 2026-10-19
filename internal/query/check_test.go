package query_test

import (
	"context"
	"errors"
	"testing"

	"github.com/mattjoyce/anam/internal/query"
)

// Check compares one instant at a time, samples every Nth fact-time, and reports what
// it compared; the fixtures agree.
func TestCheck_AgreesAndSamples(t *testing.T) {
	t.Parallel()
	v, versions, _, _ := regimenFixture(t)
	defs := registry(t)
	cases := []struct {
		every, factTimes, instants int
	}{
		{1, 7, 7}, // eight entries, two at one instant
		{3, 7, 3},
		{10, 7, 1},
	}
	for _, c := range cases {
		r, err := query.Check(context.Background(), v, versions, defs, c.every)
		if err != nil {
			t.Fatalf("Check(every=%d): %v", c.every, err)
		}
		if !r.Agrees() {
			t.Errorf("every=%d: disagreement at %s: current %v, snapshot %v", c.every, r.Disagreement.At, r.Disagreement.Current, r.Disagreement.Snapshot)
		}
		if r.FactTimes != c.factTimes || r.Instants != c.instants {
			t.Errorf("every=%d: compared %d of %d fact-times, want %d of %d", c.every, r.Instants, r.FactTimes, c.instants, c.factTimes)
		}
		if r.SlowestAt == "" || r.Elapsed < r.Slowest {
			t.Errorf("every=%d: timing not reported: %+v", c.every, r)
		}
	}
	if _, err := query.Check(context.Background(), v, versions, defs, 0); !errors.Is(err, query.ErrInvalid) {
		t.Errorf("Check(every=0) = %v, want ErrInvalid", err)
	}
}
