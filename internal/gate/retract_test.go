package gate_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/fold"
	"github.com/mattjoyce/anam/internal/gate"
)

// The retraction check folds only the facet's own events. Among others whose events
// interleave with its own, it still decides exactly what the full fold decides: a
// first retraction tombstones, a second is a no-op, and retracting one facet leaves
// every other facet's state as it was.
func TestRetract_AmongOtherFacetsIsTheFullFoldsAnswer(t *testing.T) {
	t.Parallel()
	g, l := newGate(t)
	ctx := context.Background()

	admit := func(i int, systolic int) string {
		t.Helper()
		at := fmt.Sprintf("2026-01-%02dT06:30:00Z", i)
		env := facet.Envelope{
			Type: "health.blood-pressure", Source: "bp-csv", Injector: "bp-csv",
			SourceRef: facet.SourceRef("bp-csv", at),
			Payload:   []byte(fmt.Sprintf(`{"measured_at": %q, "systolic": %d, "diastolic": 76}`, at, systolic)),
		}
		b, err := g.Resolve(env)
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		res, _, err := g.Commit(ctx, env, b)
		if err != nil {
			t.Fatalf("Commit: %v", err)
		}
		return res.FacetID
	}

	var ids []string
	for i := 1; i <= 5; i++ {
		ids = append(ids, admit(i, 110+i))
	}
	admit(2, 130) // supersede the second, so its history has two events
	target := ids[1]

	retract := func(id string, want gate.Outcome) {
		t.Helper()
		res, _, err := g.Retract(ctx, id)
		if err != nil {
			t.Fatalf("Retract %s: %v", id, err)
		}
		if res.Outcome != want {
			t.Fatalf("Retract %s = %s, want %s", id, res.Outcome, want)
		}
	}
	retract(target, gate.OutcomeTombstoned)
	admit(6, 120) // more events after the tombstone
	retract(target, gate.OutcomeNoOp)
	retract(ids[3], gate.OutcomeTombstoned)

	full, err := fold.Read(ctx, l)
	if err != nil {
		t.Fatalf("fold.Read: %v", err)
	}
	for i, id := range ids {
		want := facet.StatusAdmitted
		if i == 1 || i == 3 {
			want = facet.StatusTombstoned
		}
		if got := full.States[id].Status; got != want {
			t.Errorf("facet %d is %s, want %s", i+1, got, want)
		}
	}
	if v := full.States[target].Version; v != 2 {
		t.Errorf("the tombstone landed on v%d, want the superseding v2", v)
	}
}
