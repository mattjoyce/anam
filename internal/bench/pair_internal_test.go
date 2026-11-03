package bench

import (
	"testing"
	"time"

	"github.com/mattjoyce/anam/internal/catalog/catalogtest"
)

// test.pair reads its wall clock in Berlin, which repeats an hour on the last Sunday
// of October. A month of readings every nine minutes, as dense as shape L, has some in
// that hour: each is moved past it, so every reading's clock reads as one instant,
// none is lost and none shares a clock with another.
func TestReadings_StepPastAnHourTheZoneRepeats(t *testing.T) {
	t.Parallel()
	pr, err := PairOf(catalogtest.Engine.Catalog(t), "test.pair")
	if err != nil {
		t.Fatal(err)
	}
	sh := Shape{Name: "october", Months: 1, PerMonth: 31 * 24 * 60 / 9, Start: time.Date(2020, 10, 1, 0, 0, 0, 0, time.UTC)}
	rs, err := pr.readings(sh)
	if err != nil {
		t.Fatalf("readings: %v", err)
	}
	if len(rs) != sh.PerMonth {
		t.Fatalf("%d readings, want %d", len(rs), sh.PerMonth)
	}
	moved := 0
	for i, r := range rs {
		got, err := pr.def.FactTimeOf(map[string]any{pr.at: pr.clock(r.at)})
		if err != nil {
			t.Fatalf("reading %d at %s: %v", i, pr.clock(r.at), err)
		}
		if !got.Equal(r.at) {
			t.Errorf("reading %d: clock %s reads as %s, not %s", i, pr.clock(r.at), got, r.at)
		}
		if i > 0 && !r.at.After(rs[i-1].at) {
			t.Errorf("reading %d at %s is not after %s", i, r.at, rs[i-1].at)
		}
		first := time.Date(2020, 10, 1, 0, 0, 0, 0, pr.loc)
		step := first.AddDate(0, 1, 0).Sub(first) / time.Duration(sh.PerMonth)
		if !r.at.Equal(first.Add(time.Duration(i) * step).Truncate(time.Second)) {
			moved++
		}
	}
	// Two hours of instants share the repeated hour's clocks: about 13 readings.
	if moved < 10 {
		t.Errorf("%d readings moved; the repeated hour was not met", moved)
	}
}
