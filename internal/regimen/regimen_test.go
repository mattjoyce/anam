package regimen_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/gate"
	"github.com/mattjoyce/anam/internal/regimen"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

func TestExpected(t *testing.T) {
	t.Parallel()
	day := func(s string) time.Time {
		d, err := time.Parse(time.DateOnly, s)
		if err != nil {
			t.Fatalf("parse %s: %v", s, err)
		}
		return d
	}

	s, err := regimen.Expected(day("2026-01-01"), day("2026-01-31"), 1, 0.95)
	if err != nil {
		t.Fatalf("Expected: %v", err)
	}
	if s.Days != 31 || s.Expected != 31 || s.Probable != 31*0.95 {
		t.Errorf("January once a day at 0.95 = %+v", s)
	}
	one, err := regimen.Expected(day("2026-01-01"), day("2026-01-01"), 2, 1)
	if err != nil || one.Expected != 2 {
		t.Errorf("one day twice a day = %+v, %v", one, err)
	}

	for name, c := range map[string]struct {
		from, to string
		perDay   int
		estimate float64
	}{
		"ends before it starts": {"2026-02-01", "2026-01-01", 1, 0.9},
		"no doses":              {"2026-01-01", "2026-01-31", 0, 0.9},
		"estimate zero":         {"2026-01-01", "2026-01-31", 1, 0},
		"estimate over one":     {"2026-01-01", "2026-01-31", 1, 1.5},
	} {
		if _, err := regimen.Expected(day(c.from), day(c.to), c.perDay, c.estimate); !errors.Is(err, regimen.ErrBadSchedule) {
			t.Errorf("%s: %v, want ErrBadSchedule", name, err)
		}
	}
}

// The estimate is a fact and the schedule is not: computing the schedule over a
// ledger that holds the estimate moves nothing. The watermark is the proof.
func TestExpected_LeavesTheLedgerUntouched(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	l, err := ledger.Create(ctx, filepath.Join(t.TempDir(), "ledger.db"), "matt")
	if err != nil {
		t.Fatalf("ledger.Create: %v", err)
	}
	defer func() { _ = l.Close() }()
	defs, err := def.Bundled()
	if err != nil {
		t.Fatalf("def.Bundled: %v", err)
	}
	g := gate.New(l, defs, gate.StaticScopes{"records": {"health.medication-compliance"}}, clock.Real{}, "matt", gate.Limits{MaxPayloadBytes: config.DefaultMaxPayloadBytes, MaxRetries: config.DefaultCommitRetries})

	env := facet.Envelope{
		Type: "health.medication-compliance", Source: "records", Injector: "records",
		SourceRef: facet.SourceRef("records", "self-log:est/ramipril/1/2026-02"),
		Payload: []byte(`{"id": "self-log:est/ramipril/1/2026-02", "date": "2026-02-03",
			"drug": "ramipril", "course": 1, "period_start": "2026-01-01", "period_end": "2026-01-31",
			"estimate": 0.95}`),
	}
	binding, err := g.Resolve(env)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if _, _, err := g.Commit(ctx, env, binding); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	before, err := l.Head(ctx)
	if err != nil {
		t.Fatalf("Head: %v", err)
	}

	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC)
	s, err := regimen.Expected(from, to, 1, 0.95)
	if err != nil {
		t.Fatalf("Expected: %v", err)
	}
	if s.Probable <= 0 {
		t.Errorf("schedule computed nothing: %+v", s)
	}

	after, err := l.Head(ctx)
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	if after != before || before != 1 {
		t.Errorf("watermark moved from %d to %d while a schedule was computed", before, after)
	}
}

// A bad estimate never reaches the ledger: the Definition holds it to a number in
// [0, 1]. A number written as text is refused, as any wrong type is.
func TestComplianceEstimate_IsBoundedAtTheGate(t *testing.T) {
	t.Parallel()
	defs, err := def.Bundled()
	if err != nil {
		t.Fatalf("def.Bundled: %v", err)
	}
	d, ok := defs.Lookup("health.medication-compliance")
	if !ok {
		t.Fatal("no bundled definition for health.medication-compliance")
	}
	for value, want := range map[string]bool{`0.95`: true, `1`: true, `0`: true, `"0.5"`: false, `95`: false, `1.5`: false, `-0.1`: false} {
		doc, err := def.Decode([]byte(`{"id": "e", "date": "2026-02-03", "drug": "x", "course": 1,
			"period_start": "2026-01-01", "period_end": "2026-01-31", "estimate": ` + value + `}`))
		if err != nil {
			t.Fatalf("Decode: %v", err)
		}
		if got := d.Check(doc) == nil; got != want {
			t.Errorf("estimate %s admitted = %t, want %t", value, got, want)
		}
	}
}
