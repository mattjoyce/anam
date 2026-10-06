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
	envs, err := records.Scan(dir, "matt")
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	held := map[string]bool{}
	for _, env := range envs {
		held[facet.NewID(env.SourceRef)] = true
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
	envs, err := records.Scan(dir, "matt")
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
	if len(absent) != 1 || absent[0].Provenance.SourceRef != "records:w2" || total != 3 {
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
