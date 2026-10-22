package mint_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/mattjoyce/anam/internal/fold"
	"github.com/mattjoyce/anam/internal/mint"
	"github.com/mattjoyce/anam/internal/store/ledger"
	"github.com/mattjoyce/anam/internal/store/ops"
)

// Collecting old checkpoints ends a mint (card 215): a checkpoint a later one of its
// span subsumes and no receipt pins is removed, with the run it was read from, and every
// read stays the fold.

// A mint leaves the heads and nothing else, and a read at a watermark before the run
// that remains still answers, from the ledger.
func TestRun_LeavesOnlyTheCheckpointsStillNeeded(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.seed(t)
	first := h.run(t, false)

	// Three later mints, each re-opening July and August.
	var last mint.Result
	for i := 0; i < 3; i++ {
		h.bp(t, fmt.Sprintf("2026-07-%02dT08:00:00Z", 10+i), 120+i)
		h.bp(t, fmt.Sprintf("2026-08-%02dT08:00:00Z", 10+i), 130+i)
		last = h.run(t, false)
		if last.Collected.Checkpoints != 2 {
			t.Fatalf("mint %d removed %+v, want the two versions it subsumed", i+2, last.Collected)
		}
	}

	heads := h.heads(t)
	rows := h.rows(t)
	if len(rows) != len(heads) || len(rows) != 3 {
		t.Fatalf("the log holds %d rows for %d buckets, want 3 and 3", len(rows), len(heads))
	}
	if v := heads["bucket:2026-07"].V; v != 4 {
		t.Errorf("July is at v%d, want v4", v)
	}
	if h.files(t) != len(heads) {
		t.Errorf("%d checkpoint values remain, want one per head (%d)", h.files(t), len(heads))
	}
	if sealed, err := h.db.LogSealed(ctx); err != nil || !sealed {
		t.Errorf("LogSealed = %v, %v after a mint, want sealed", sealed, err)
	}

	// Every watermark answers as the fold does: from the run that remains at or after
	// it, and from the ledger before it.
	head, _ := h.l.Head(ctx)
	for w := ledger.Watermark(1); w <= head; w++ {
		want, err := fold.ReadAt(ctx, h.l, w)
		if err != nil {
			t.Fatal(err)
		}
		got, ok, err := mint.ReadAt(ctx, h.l, h.db, h.deps.Values, w)
		if err != nil {
			t.Fatalf("W=%d: %v", w, err)
		}
		if ok != (w >= last.W) {
			t.Fatalf("W=%d: composed=%v, want %v: the first run (%d) is gone, the newest is %d", w, ok, w >= last.W, first.W, last.W)
		}
		if ok && got.Value.Hash() != want.Value.Hash() {
			t.Fatalf("W=%d: composed %s, fold %s", w, got.Value.Hash(), want.Value.Hash())
		}
	}
}

// A receipt keeps the floor its watermark reads from: the run, and the checkpoints it
// takes, stay, and a read at that watermark still starts from them.
func TestRun_AReceiptKeepsItsFloor(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.seed(t)
	first := h.run(t, false)
	h.bp(t, "2026-07-20T08:00:00Z", 125)
	pinned := ledger.Watermark(first.W + 1) // between the first run and the next
	h.bp(t, "2026-07-21T08:00:00Z", 126)

	body := fmt.Sprintf(`{"watermark":%d,"served_hash":"sha256:x"}`, pinned)
	if _, err := h.db.Put(ctx, ops.KindReceipt, []byte(body)); err != nil {
		t.Fatal(err)
	}
	second := h.run(t, false)
	if second.Collected.Runs != 0 || second.Collected.Checkpoints != 0 {
		t.Fatalf("the mint removed %+v although a receipt reads from the first run", second.Collected)
	}
	if len(h.rows(t)) != 4 {
		t.Fatalf("the log holds %d rows, want July's two versions and the other buckets' one", len(h.rows(t)))
	}

	want, err := fold.ReadAt(ctx, h.l, pinned)
	if err != nil {
		t.Fatal(err)
	}
	got, ok, err := mint.ReadAt(ctx, h.l, h.db, h.deps.Values, pinned)
	if err != nil || !ok || got.Run != first.W || got.Value.Hash() != want.Value.Hash() {
		t.Fatalf("ReadAt at the receipt's watermark: ok=%v err=%v run=%d, want the first run (%d) and the fold's value", ok, err, got.Run, first.W)
	}

	// A later mint collects what the receipt does not read from, and no more.
	h.bp(t, "2026-07-22T08:00:00Z", 127)
	if third := h.run(t, false); third.Collected.Runs != 1 || third.Collected.Checkpoints != 1 {
		t.Errorf("the third mint removed %+v, want the second run and July's v2, subsumed by v3", third.Collected)
	}
}

// A dry run collects nothing, and a mint that crashed before its run was recorded
// leaves the log unsealed, so nothing is collected around it.
func TestRun_DryRunCollectsNothing(t *testing.T) {
	h := newHarness(t)
	h.seed(t)
	h.run(t, false)
	h.bp(t, "2026-07-20T08:00:00Z", 125)
	before := len(h.rows(t))
	dry := h.run(t, true)
	if !dry.Collected.None() || len(h.rows(t)) != before {
		t.Fatalf("a dry run collected %+v", dry.Collected)
	}
}

// straddle is a checkpoint log that, once a read has the rows of a run, lets a later mint
// run, as one running beside the read would. during runs on every call, so it is written
// to do its work once.
type straddle struct {
	mint.WindowLog
	during func()
}

func (s straddle) CheckpointsAt(ctx context.Context, w int64) ([]ops.CheckpointRow, error) {
	rows, err := s.WindowLog.CheckpointsAt(ctx, w)
	if s.during != nil {
		s.during()
	}
	return rows, err
}

// A read that took a run and its rows, and then met the collection that removed them,
// declines rather than fail or answer from the rows that were left. Its caller folds
// the ledger, which is the same value.
func TestReads_AMintCollectingMidReadFoldsInFull(t *testing.T) {
	for _, tc := range []struct {
		name string
		read func(h *harness, log straddle, w ledger.Watermark) (bool, error)
	}{
		{"ReadAt", func(h *harness, log straddle, w ledger.Watermark) (bool, error) {
			_, ok, err := mint.ReadAt(context.Background(), h.l, log, h.deps.Values, w)
			return ok, err
		}},
		{"ReadWindow", func(h *harness, log straddle, w ledger.Watermark) (bool, error) {
			_, ok, err := mint.ReadWindow(context.Background(), h.l, log, h.deps.Values, w, "", "")
			return ok, err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			h.seed(t)
			h.run(t, false)
			w, _ := h.l.Head(context.Background())
			done := false
			log := straddle{WindowLog: h.db, during: func() {
				if done {
					return
				}
				done = true
				h.bp(t, "2026-07-20T08:00:00Z", 125)
				h.run(t, false) // collects the run the read took
			}}
			ok, err := tc.read(h, log, w)
			if err != nil || ok {
				t.Fatalf("a read that straddled a collection: ok=%v err=%v, want it to decline", ok, err)
			}
			// And the next read, started after, takes the new run.
			if _, ok, err := mint.ReadAt(context.Background(), h.l, h.db, h.deps.Values, w+1); err != nil || !ok {
				t.Fatalf("a read after the collection: ok=%v err=%v", ok, err)
			}
		})
	}
}
