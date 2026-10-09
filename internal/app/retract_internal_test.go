package app

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/capture/records"
	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/fold"
	"github.com/mattjoyce/anam/internal/gate"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

const threeWeights = `document: {name: diary.md, sha256: "0000000000000000000000000000000000000000000000000000000000000000"}
extracted: {method: by hand, version: 1}
records:
  - {type: health.weight, id: w1, date: "2026-01-01 07:00", kg: 80.1}
  - {type: health.weight, id: w2, date: "2026-01-02 07:00", kg: 80.2}
  - {type: health.weight, id: w3, date: "2026-01-03 07:00", kg: 80.3}
`

func heldBy(t *testing.T, dir string) map[string]bool {
	t.Helper()
	defs, err := def.Bundled()
	if err != nil {
		t.Fatalf("def.Bundled: %v", err)
	}
	envs, err := records.Scan(dir, "matt", defs)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	held, err := heldIDs([]scanned{{source: records.Source, envs: envs}})
	if err != nil {
		t.Fatalf("heldIDs: %v", err)
	}
	return held
}

// A row deleted from the source is reported, and only tombstoned on purpose; a
// re-run after the tombstone has nothing left to do (card 17).
func TestRetract_PlansOnlyWhatTheSourceDropped(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir := t.TempDir()
	writeDoc(t, filepath.Join(dir, "diary.yaml"), threeWeights)

	l, err := ledger.Create(ctx, filepath.Join(t.TempDir(), "ledger.db"), "matt")
	if err != nil {
		t.Fatalf("ledger.Create: %v", err)
	}
	defer func() { _ = l.Close() }()
	defs, err := def.Bundled()
	if err != nil {
		t.Fatalf("def.Bundled: %v", err)
	}
	g := gate.New(l, defs, mvpScopes, clock.Real{}, "matt")
	envs, err := records.Scan(dir, "matt", defs)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	for _, env := range envs {
		b, _ := g.Resolve(env)
		if _, _, err := g.Commit(ctx, env, b); err != nil {
			t.Fatalf("Commit: %v", err)
		}
	}

	// Nothing dropped: nothing planned.
	res, err := fold.Read(ctx, l)
	if err != nil {
		t.Fatalf("fold.Read: %v", err)
	}
	absent, total := planRetraction(res.Live, records.Source, heldBy(t, dir))
	if len(absent) != 0 || total != 3 {
		t.Fatalf("plan over an unchanged source = %d absent of %d", len(absent), total)
	}

	// w2 is deleted from the source.
	writeDoc(t, filepath.Join(dir, "diary.yaml"), strings.Replace(threeWeights,
		`  - {type: health.weight, id: w2, date: "2026-01-02 07:00", kg: 80.2}`+"\n", "", 1))
	absent, total = planRetraction(res.Live, records.Source, heldBy(t, dir))
	if len(absent) != 1 || absent[0].Provenance.SourceRef != facet.SourceRef("records", "w2") || total != 3 {
		t.Fatalf("plan after deleting w2 = %v of %d", absent, total)
	}
	if err := guardRetraction(len(absent), total, 2, 0.5); err != nil {
		t.Fatalf("guard refused a third: %v", err)
	}

	// Retracting it is one tombstone through the gate; the plan is then empty.
	out, _, err := g.Retract(ctx, absent[0].ID)
	if err != nil || out.Outcome != gate.OutcomeTombstoned {
		t.Fatalf("Retract = %v %v", out.Outcome, err)
	}
	res, err = fold.Read(ctx, l)
	if err != nil {
		t.Fatalf("fold.Read: %v", err)
	}
	absent, total = planRetraction(res.Live, records.Source, heldBy(t, dir))
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

// A worker that leaves source_ref to the gate is still held: retraction composes the
// ref from the Definition, as the gate does, rather than reading it off the envelope.
// Were it read off, every such facet would look dropped from its source.
func TestHeldIDs_ComposesTheRefTheGateAdmits(t *testing.T) {
	t.Parallel()
	env := facet.Envelope{
		Type: "doc.reference", Source: "documents", Injector: "documents",
		Payload: []byte(`{"path":"scans/bp.jpg","sha256":"00","media_type":"image/jpeg","bytes":10,"date":"2024-09-16"}`),
	}
	held, err := heldIDs([]scanned{{source: "documents", envs: []facet.Envelope{env}}})
	if err != nil {
		t.Fatalf("heldIDs: %v", err)
	}
	if want := facet.NewID(facet.SourceRef("documents", "scans/bp.jpg")); len(held) != 1 || !held[want] {
		t.Errorf("held = %v, want only %s", held, want)
	}
}
