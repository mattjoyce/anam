package ops_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/fold"
	"github.com/mattjoyce/anam/internal/store/ops"
)

func eventRow(seq int64, rt string, kind facet.AnamEventKind, id string, v int) ops.EventRow {
	return ops.EventRow{Seq: seq, AnamTime: rt, Kind: kind, FacetID: id, ArtifactVersion: v}
}

func facetRow(id string, v int, status facet.Status) ops.FacetRow {
	return ops.FacetRow{
		FacetID: id, Type: "health.blood-pressure",
		FactTime: "2026-07-05T06:30:00.000000000Z", BucketID: "bucket:2026-W27",
		Version: v, Status: status, ContentHash: "sha256:aaa",
	}
}

func TestApplyReplay_MovesRowsAndCursorTogether(t *testing.T) {
	t.Parallel()
	d, _ := open(t)
	ctx := context.Background()

	last, err := d.LastSeq(ctx)
	if err != nil || last != 0 {
		t.Fatalf("fresh LastSeq = %d, %v; want 0", last, err)
	}

	batch1 := []ops.EventRow{
		eventRow(1, "2026-07-10T09:00:00.000000000Z", facet.EventAdmitted, "a", 1),
		eventRow(2, "2026-07-10T09:01:00.000000000Z", facet.EventAdmitted, "b", 1),
	}
	if err := d.ApplyReplay(ctx, 0, 2, batch1,
		[]ops.FacetRow{facetRow("a", 1, facet.StatusAdmitted), facetRow("b", 1, facet.StatusAdmitted)}, nil); err != nil {
		t.Fatalf("ApplyReplay: %v", err)
	}
	if last, _ := d.LastSeq(ctx); last != 2 {
		t.Fatalf("LastSeq after batch = %d, want 2", last)
	}

	// A second batch supersedes a: the row is upserted, not duplicated.
	batch2 := []ops.EventRow{eventRow(3, "2026-07-10T09:02:00.000000000Z", facet.EventSuperseded, "a", 2)}
	if err := d.ApplyReplay(ctx, 2, 3, batch2,
		[]ops.FacetRow{facetRow("a", 2, facet.StatusAdmitted)}, nil); err != nil {
		t.Fatalf("second ApplyReplay: %v", err)
	}

	states, err := d.FacetStates(ctx, []string{"a", "b", "never-seen"})
	if err != nil {
		t.Fatalf("FacetStates: %v", err)
	}
	if got := states["a"]; got != (fold.State{Version: 2, Status: facet.StatusAdmitted}) {
		t.Errorf("a = %+v, want v2 admitted", got)
	}
	if got := states["b"]; got != (fold.State{Version: 1, Status: facet.StatusAdmitted}) {
		t.Errorf("b = %+v", got)
	}
	if _, ok := states["never-seen"]; ok {
		t.Error("an unseen facet has a state")
	}

	// A batch whose fromSeq is stale must be refused whole — the tier moves in
	// complete steps or not at all.
	err = d.ApplyReplay(ctx, 2, 4, []ops.EventRow{eventRow(4, "2026-07-10T09:03:00.000000000Z", facet.EventTombstoned, "a", 2)}, nil, nil)
	if err == nil {
		t.Fatal("stale-cursor batch accepted")
	}
	if last, _ := d.LastSeq(ctx); last != 3 {
		t.Errorf("cursor moved by a refused batch: %d", last)
	}
}

func TestChangesSince_BoundsAreAnamTime(t *testing.T) {
	t.Parallel()
	d, _ := open(t)
	ctx := context.Background()

	events := []ops.EventRow{
		eventRow(1, "2026-07-10T09:00:00.000000000Z", facet.EventAdmitted, "a", 1),
		eventRow(2, "2026-07-10T10:00:00.000000000Z", facet.EventAdmitted, "b", 1),
		eventRow(3, "2026-07-10T11:00:00.000000000Z", facet.EventTombstoned, "a", 1),
	}
	if err := d.ApplyReplay(ctx, 0, 3, events, []ops.FacetRow{
		facetRow("a", 1, facet.StatusTombstoned), facetRow("b", 1, facet.StatusAdmitted),
	}, nil); err != nil {
		t.Fatalf("ApplyReplay: %v", err)
	}

	// (since, until]: strictly after 09:00, up to and including 11:00.
	got, err := d.ChangesSince(ctx, "2026-07-10T09:00:00.000000000Z", "2026-07-10T11:00:00.000000000Z")
	if err != nil {
		t.Fatalf("ChangesSince: %v", err)
	}
	if len(got) != 2 || got[0].Seq != 2 || got[1].Seq != 3 {
		t.Fatalf("changes = %+v, want seqs 2,3", got)
	}
	if got[1].Kind != facet.EventTombstoned {
		t.Errorf("kind = %s, want tombstoned", got[1].Kind)
	}

	// Open-ended until.
	all, err := d.ChangesSince(ctx, "0000", "")
	if err != nil {
		t.Fatalf("open ChangesSince: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("open-ended changes = %d, want 3", len(all))
	}
}

// The edge index (card 104): a changed facet's edge rows are replaced whole by the
// ones the batch carries, so an edge that moves its ends leaves no stale row, a
// tombstoned edge leaves none, and the edges touching a facet come back whole with
// the cursor they are current to.
func TestApplyReplay_ReplacesAnEdgesEnds(t *testing.T) {
	t.Parallel()
	d, _ := open(t)
	ctx := context.Background()

	note := func(v int, status facet.Status) ops.FacetRow {
		r := facetRow("n", v, status)
		r.Type = "record.note"
		r.FactTime = "2026-07-06T06:30:00.000000000Z"
		return r
	}
	if err := d.ApplyReplay(ctx, 0, 1,
		[]ops.EventRow{eventRow(1, "2026-07-10T09:00:00.000000000Z", facet.EventAdmitted, "n", 1)},
		[]ops.FacetRow{note(1, facet.StatusAdmitted)},
		[]ops.EdgeRow{{EdgeID: "n", Kind: "about", Side: ops.SideTo, FacetID: "a"}, {EdgeID: "n", Kind: "about", Side: ops.SideTo, FacetID: "b"}}); err != nil {
		t.Fatalf("ApplyReplay: %v", err)
	}
	got, err := d.TouchingEdges(ctx, "a")
	if err != nil {
		t.Fatalf("TouchingEdges: %v", err)
	}
	want := ops.Touching{LastSeq: 1, Edges: []ops.Edge{{EdgeID: "n", Kind: "about", FactTime: "2026-07-06T06:30:00.000000000Z", To: []string{"a", "b"}}}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("touching a = %+v, want %+v", got, want)
	}

	// v2 names b and c, with a from side: a is no longer touched, c is, b still.
	if err := d.ApplyReplay(ctx, 1, 2,
		[]ops.EventRow{eventRow(2, "2026-07-10T09:01:00.000000000Z", facet.EventSuperseded, "n", 2)},
		[]ops.FacetRow{note(2, facet.StatusAdmitted)},
		[]ops.EdgeRow{{EdgeID: "n", Kind: "interpreted_from", Side: ops.SideFrom, FacetID: "c"}, {EdgeID: "n", Kind: "interpreted_from", Side: ops.SideTo, FacetID: "b"}}); err != nil {
		t.Fatalf("second ApplyReplay: %v", err)
	}
	if got, _ := d.TouchingEdges(ctx, "a"); len(got.Edges) != 0 || got.LastSeq != 2 {
		t.Errorf("after the move, a is touched by %+v", got)
	}
	got, _ = d.TouchingEdges(ctx, "c")
	want = ops.Touching{LastSeq: 2, Edges: []ops.Edge{{EdgeID: "n", Kind: "interpreted_from", FactTime: "2026-07-06T06:30:00.000000000Z", From: []string{"c"}, To: []string{"b"}}}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("touching c = %+v, want %+v", got, want)
	}

	// Tombstoned: the batch carries no rows for it, and none remain.
	if err := d.ApplyReplay(ctx, 2, 3,
		[]ops.EventRow{eventRow(3, "2026-07-10T09:02:00.000000000Z", facet.EventTombstoned, "n", 2)},
		[]ops.FacetRow{note(2, facet.StatusTombstoned)}, nil); err != nil {
		t.Fatalf("third ApplyReplay: %v", err)
	}
	for _, id := range []string{"a", "b", "c"} {
		if got, _ := d.TouchingEdges(ctx, id); len(got.Edges) != 0 {
			t.Errorf("after the tombstone, %s is touched by %+v", id, got.Edges)
		}
	}
}
