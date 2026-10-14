package app

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/fold"
	"github.com/mattjoyce/anam/internal/gate"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

var threeWeights = []record{
	{"health.weight", `{"id": "w1", "date": "2026-01-01 07:00", "kg": 80.1}`},
	{"health.weight", `{"id": "w2", "date": "2026-01-02 07:00", "kg": 80.2}`},
	{"health.weight", `{"id": "w3", "date": "2026-01-03 07:00", "kg": 80.3}`},
}

// heldOf is the facet id of each records id the source holds now.
func heldOf(ids ...string) map[string]bool {
	held := map[string]bool{}
	for _, id := range ids {
		held[facet.NewID(facet.SourceRef("records", id))] = true
	}
	return held
}

// A row the source no longer holds is reported, and only tombstoned on purpose; a
// re-run after the tombstone has nothing left to do (card 17).
func TestRetract_PlansOnlyWhatTheSourceDropped(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	l, err := ledger.Create(ctx, filepath.Join(t.TempDir(), "ledger.db"), "matt")
	if err != nil {
		t.Fatalf("ledger.Create: %v", err)
	}
	defer func() { _ = l.Close() }()
	if _, err := admitRecords(ctx, l, threeWeights); err != nil {
		t.Fatalf("admit: %v", err)
	}

	// Nothing dropped: nothing planned.
	res, err := fold.Read(ctx, l)
	if err != nil {
		t.Fatalf("fold.Read: %v", err)
	}
	absent, total := planRetraction(res.Value.Entries(), "records", heldOf("w1", "w2", "w3"))
	if len(absent) != 0 || total != 3 {
		t.Fatalf("plan over an unchanged source = %d absent of %d", len(absent), total)
	}

	// w2 is no longer held.
	absent, total = planRetraction(res.Value.Entries(), "records", heldOf("w1", "w3"))
	if len(absent) != 1 || absent[0].SourceRef != facet.SourceRef("records", "w2") || total != 3 {
		t.Fatalf("plan after dropping w2 = %v of %d", absent, total)
	}
	// The report fetches only the planned facets, at the version in force.
	facets, err := facetsOf(ctx, l, absent)
	if err != nil {
		t.Fatalf("facetsOf: %v", err)
	}
	if len(facets) != 1 || facets[0].ID != absent[0].FacetID || facets[0].ArtifactVersion != 1 ||
		facets[0].Provenance.SourceRef != absent[0].SourceRef {
		t.Fatalf("facetsOf(w2) = %+v", facets)
	}
	if err := guardRetraction(len(absent), total, 2, 0.5); err != nil {
		t.Fatalf("guard refused a third: %v", err)
	}

	// Retracting it is one tombstone through the gate; the plan is then empty.
	defs, err := def.Bundled()
	if err != nil {
		t.Fatal(err)
	}
	g := gate.New(l, defs, gate.StaticScopes{}, clock.Real{}, "matt", gate.Limits{MaxPayloadBytes: config.DefaultMaxPayloadBytes, MaxRetries: config.DefaultCommitRetries})
	out, _, err := g.Retract(ctx, absent[0].FacetID)
	if err != nil || out.Outcome != gate.OutcomeTombstoned {
		t.Fatalf("Retract = %v %v", out.Outcome, err)
	}
	res, err = fold.Read(ctx, l)
	if err != nil {
		t.Fatalf("fold.Read: %v", err)
	}
	absent, total = planRetraction(res.Value.Entries(), "records", heldOf("w1", "w3"))
	if len(absent) != 0 || total != 2 || res.States[out.FacetID].Status != facet.StatusTombstoned {
		t.Errorf("after the tombstone: %d absent of %d, status %s", len(absent), total, res.States[out.FacetID].Status)
	}
}

// The guards: an empty source is refused whatever the fraction, and a source that
// lost more than allowed is refused with the fraction named.
func TestGuardRetraction(t *testing.T) {
	t.Parallel()
	if err := guardRetraction(3, 3, 0, 1.0); err == nil || !strings.Contains(err.Error(), "holds nothing") {
		t.Errorf("empty source: %v", err)
	}
	if err := guardRetraction(2, 10, 8, 0.1); err == nil || !strings.Contains(err.Error(), "max-fraction") {
		t.Errorf("over the fraction: %v", err)
	}
	if err := guardRetraction(1, 10, 9, 0.1); err != nil {
		t.Errorf("at the fraction: %v", err)
	}
}
