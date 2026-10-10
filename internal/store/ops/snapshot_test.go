package ops_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mattjoyce/anam/internal/store/ops"
)

var t0 = time.Date(2026, 10, 10, 9, 0, 0, 0, time.UTC)

func part(id string, n, of int, facets ...string) ops.SnapshotPart {
	return ops.SnapshotPart{ID: id, Source: "withings", Types: []string{"health.measurement"}, Part: n, Parts: of, FacetIDs: facets}
}

// Parts arrive in any order and may repeat; the snapshot is complete when the last
// missing part lands, and what it carried is a set.
func TestSnapshot_CompletesWhenEveryPartIsAdmitted(t *testing.T) {
	t.Parallel()
	db, _ := open(t)
	ctx := context.Background()

	s, err := db.RecordSnapshotPart(ctx, part("run-1", 2, 3, "b", "c"), t0, 1)
	if err != nil || s.Complete() || s.PartsAdmitted != 1 || s.Carried != 2 {
		t.Fatalf("after part 2: %+v, %v", s, err)
	}
	if s, err = db.RecordSnapshotPart(ctx, part("run-1", 2, 3, "b", "c"), t0, 1); err != nil || s.PartsAdmitted != 1 || s.Carried != 2 {
		t.Fatalf("part 2 again: %+v, %v", s, err)
	}
	if s, err = db.RecordSnapshotPart(ctx, part("run-1", 1, 3, "a", "b"), t0, 1); err != nil || s.Complete() {
		t.Fatalf("after part 1: %+v, %v", s, err)
	}
	if _, ok, _ := db.NewestCompleteSnapshot(ctx, "withings"); ok {
		t.Fatal("an incomplete snapshot was offered as complete")
	}
	if s, err = db.RecordSnapshotPart(ctx, part("run-1", 3, 3, "d"), t0.Add(time.Minute), 9); err != nil || !s.Complete() || s.Carried != 4 {
		t.Fatalf("after part 3: %+v, %v", s, err)
	}
	got, ok, err := db.NewestCompleteSnapshot(ctx, "withings")
	if err != nil || !ok || got.ID != "run-1" || got.CompletedW != 9 {
		t.Fatalf("newest complete = %+v %v %v", got, ok, err)
	}
	carried, err := db.SnapshotFacets(ctx, "run-1")
	if err != nil || len(carried) != 4 || !carried["a"] || !carried["d"] {
		t.Fatalf("carried = %v, %v", carried, err)
	}
}

// A part that names a run already begun as another source, other types or another
// part count is refused: it is not the same run.
func TestSnapshot_RefusesAMismatchedPart(t *testing.T) {
	t.Parallel()
	db, _ := open(t)
	ctx := context.Background()
	if _, err := db.RecordSnapshotPart(ctx, part("run-1", 1, 2), t0, 1); err != nil {
		t.Fatal(err)
	}
	other := part("run-1", 2, 3)
	if _, err := db.RecordSnapshotPart(ctx, other, t0, 1); !errors.Is(err, ops.ErrSnapshotMismatch) {
		t.Errorf("another part count = %v, want ErrSnapshotMismatch", err)
	}
	other = part("run-1", 2, 2)
	other.Source = "bp-csv"
	if _, err := db.RecordSnapshotPart(ctx, other, t0, 1); !errors.Is(err, ops.ErrSnapshotMismatch) {
		t.Errorf("another source = %v, want ErrSnapshotMismatch", err)
	}
}

// An incomplete snapshot is dropped after a day; a source keeps its two newest
// complete ones.
func TestSnapshot_StaleAndOldAreDropped(t *testing.T) {
	t.Parallel()
	db, _ := open(t)
	ctx := context.Background()
	if _, err := db.RecordSnapshotPart(ctx, part("stale", 1, 2, "x"), t0, 1); err != nil {
		t.Fatal(err)
	}
	for i, id := range []string{"c1", "c2", "c3"} {
		if _, err := db.RecordSnapshotPart(ctx, part(id, 1, 1, "y"), t0.Add(25*time.Hour+time.Duration(i)*time.Minute), 1); err != nil {
			t.Fatal(err)
		}
	}
	for id, want := range map[string]bool{"stale": false, "c1": false, "c2": true, "c3": true} {
		if _, ok, err := db.Snapshot(ctx, id); err != nil || ok != want {
			t.Errorf("snapshot %s held = %v (%v), want %v", id, ok, err, want)
		}
	}
	if c, _ := db.SnapshotFacets(ctx, "stale"); len(c) != 0 {
		t.Errorf("a dropped snapshot kept %d facets", len(c))
	}
}
