package ops_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/store/ops"
)

// bound is part with a digest: the same part number under another digest is another
// part, not a retry.
func bound(id string, n, of int, digest string, facets ...string) ops.SnapshotPart {
	p := part(id, n, of, facets...)
	p.Digest = digest
	return p
}

// Card 107: a part is a value. Recorded once, the same number with the same digest
// adds nothing, before and after completion; with another digest it is refused, and
// the snapshot's membership, completion and watermarks do not move. The binding is in
// the file, so it holds across a reopen.
func TestSnapshot_PartIsBoundToItsDigest(t *testing.T) {
	t.Parallel()
	db, path := open(t)
	ctx := context.Background()

	if _, err := db.RecordSnapshotPart(ctx, bound("run", 1, 2, "d1", "a", "b"), t0, 1, ops.SnapshotRetention{Incomplete: config.DefaultSnapshotTTL, Complete: config.DefaultSnapshotKeep}); err != nil {
		t.Fatal(err)
	}
	// Changed while incomplete.
	_, err := db.RecordSnapshotPart(ctx, bound("run", 1, 2, "d1x", "a", "c"), t0, 2, ops.SnapshotRetention{Incomplete: config.DefaultSnapshotTTL, Complete: config.DefaultSnapshotKeep})
	if !errors.Is(err, ops.ErrSnapshotPartChanged) {
		t.Fatalf("changed part 1 = %v, want ErrSnapshotPartChanged", err)
	}
	// Exact while incomplete.
	s, err := db.RecordSnapshotPart(ctx, bound("run", 1, 2, "d1", "a", "b"), t0, 2, ops.SnapshotRetention{Incomplete: config.DefaultSnapshotTTL, Complete: config.DefaultSnapshotKeep})
	if err != nil || s.PartsAdmitted != 1 || s.Carried != 2 || s.Complete() {
		t.Fatalf("exact retry of part 1 = %+v, %v", s, err)
	}
	s, err = db.RecordSnapshotPart(ctx, bound("run", 2, 2, "d2", "c"), t0.Add(time.Minute), 3, ops.SnapshotRetention{Incomplete: config.DefaultSnapshotTTL, Complete: config.DefaultSnapshotKeep})
	if err != nil || !s.Complete() || s.Carried != 3 || s.CompletedW != 3 {
		t.Fatalf("part 2 = %+v, %v", s, err)
	}
	// Changed after completion: the one the audit reproduced.
	_, err = db.RecordSnapshotPart(ctx, bound("run", 2, 2, "d2x", "d"), t0.Add(2*time.Minute), 4, ops.SnapshotRetention{Incomplete: config.DefaultSnapshotTTL, Complete: config.DefaultSnapshotKeep})
	if !errors.Is(err, ops.ErrSnapshotPartChanged) {
		t.Fatalf("changed part 2 after completion = %v, want ErrSnapshotPartChanged", err)
	}
	// Exact after completion, across a reopen.
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = ops.Open(ctx, path, buckets(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	s, err = db.RecordSnapshotPart(ctx, bound("run", 2, 2, "d2", "c"), t0.Add(2*time.Minute), 4, ops.SnapshotRetention{Incomplete: config.DefaultSnapshotTTL, Complete: config.DefaultSnapshotKeep})
	if err != nil || !s.Complete() || s.Carried != 3 || s.PartsAdmitted != 2 || s.CompletedW != 3 ||
		!s.CompletedAt.Equal(t0.Add(time.Minute)) {
		t.Fatalf("exact retry of part 2 after completion = %+v, %v", s, err)
	}
	if _, err = db.RecordSnapshotPart(ctx, bound("run", 1, 2, "other", "a"), t0.Add(2*time.Minute), 4, ops.SnapshotRetention{Incomplete: config.DefaultSnapshotTTL, Complete: config.DefaultSnapshotKeep}); !errors.Is(err, ops.ErrSnapshotPartChanged) {
		t.Fatalf("changed part 1 after reopen = %v, want ErrSnapshotPartChanged", err)
	}
	carried, err := db.SnapshotFacets(ctx, "run")
	if err != nil || len(carried) != 3 || carried["d"] {
		t.Fatalf("carried = %v, %v", carried, err)
	}
	digest, ok, err := db.SnapshotPartDigest(ctx, "run", 2)
	if err != nil || !ok || digest != "d2" {
		t.Errorf("part 2 digest = %q, %v, %v", digest, ok, err)
	}
	if _, ok, err := db.SnapshotPartDigest(ctx, "run", 3); err != nil || ok {
		t.Errorf("part 3 held = %v, %v", ok, err)
	}
}
