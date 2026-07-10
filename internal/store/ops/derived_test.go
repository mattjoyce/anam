package ops_test

import (
	"context"
	"testing"

	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/fold"
	"github.com/mattjoyce/anam/internal/store/ops"
)

func eventRow(seq int64, rt string, kind facet.RecordEventKind, id string, v int) ops.EventRow {
	return ops.EventRow{Seq: seq, RecordTime: rt, Kind: kind, FacetID: id, ArtifactVersion: v}
}

func facetRow(id string, v int, status facet.Status) ops.FacetRow {
	return ops.FacetRow{
		FacetID: id, Type: "health.blood-pressure",
		EventTime: "2026-07-05T06:30:00.000000000Z", BucketID: "bucket:2026-W27",
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
		[]ops.FacetRow{facetRow("a", 1, facet.StatusAdmitted), facetRow("b", 1, facet.StatusAdmitted)}); err != nil {
		t.Fatalf("ApplyReplay: %v", err)
	}
	if last, _ := d.LastSeq(ctx); last != 2 {
		t.Fatalf("LastSeq after batch = %d, want 2", last)
	}

	// A second batch supersedes a: the row is upserted, not duplicated.
	batch2 := []ops.EventRow{eventRow(3, "2026-07-10T09:02:00.000000000Z", facet.EventSuperseded, "a", 2)}
	if err := d.ApplyReplay(ctx, 2, 3, batch2,
		[]ops.FacetRow{facetRow("a", 2, facet.StatusAdmitted)}); err != nil {
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
	err = d.ApplyReplay(ctx, 2, 4, []ops.EventRow{eventRow(4, "2026-07-10T09:03:00.000000000Z", facet.EventTombstoned, "a", 2)}, nil)
	if err == nil {
		t.Fatal("stale-cursor batch accepted")
	}
	if last, _ := d.LastSeq(ctx); last != 3 {
		t.Errorf("cursor moved by a refused batch: %d", last)
	}
}

func TestChangesSince_BoundsAreRecordTime(t *testing.T) {
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
	}); err != nil {
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
