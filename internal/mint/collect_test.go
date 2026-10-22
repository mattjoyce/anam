package mint_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/mattjoyce/anam/internal/fold"
	"github.com/mattjoyce/anam/internal/mint"
	"github.com/mattjoyce/anam/internal/store/ledger"
	"github.com/mattjoyce/anam/internal/store/ops"
	"github.com/mattjoyce/anam/internal/store/ops/opstest"
)

// Checkpoints are a cache (card 215). A mint ends by removing what the newest run does
// not read, and a read that needs a checkpoint that is gone folds the ledger instead of
// composing from what is left.

// A mint leaves each bucket's head and nothing else, and a read at an older watermark
// still answers, from the ledger.
func TestRun_LeavesOnlyWhatTheNewestRunReads(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.seed(t)
	h.run(t, false)

	// Three later mints, each re-opening July and August.
	var last mint.Result
	for i := 0; i < 3; i++ {
		h.bp(t, fmt.Sprintf("2026-07-%02dT08:00:00Z", 10+i), 120+i)
		h.bp(t, fmt.Sprintf("2026-08-%02dT08:00:00Z", 10+i), 130+i)
		last = h.run(t, false)
	}

	heads, rows := h.heads(t), h.rows(t)
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

	// Every watermark answers as the fold does: from the newest run at or after it, and
	// from the ledger before it, where the older runs no longer have their rows.
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
			t.Fatalf("W=%d: composed=%v, want %v (newest run %d)", w, ok, w >= last.W, last.W)
		}
		if ok && got.Value.Hash() != want.Value.Hash() {
			t.Fatalf("W=%d: composed %s, fold %s", w, got.Value.Hash(), want.Value.Hash())
		}
	}
}

// A dry run removes nothing.
func TestRun_DryRunCollectsNothing(t *testing.T) {
	h := newHarness(t)
	h.seed(t)
	h.run(t, false)
	h.bp(t, "2026-07-20T08:00:00Z", 125)
	before := len(h.rows(t))
	h.run(t, true)
	if len(h.rows(t)) != before {
		t.Fatalf("a dry run changed the log from %d rows to %d", before, len(h.rows(t)))
	}
}

// hurt is a way a cache loses an entry.
type hurt struct {
	name string
	do   func(t *testing.T, h *harness)
}

// The ways a checkpoint can go: a value of the newest run, a row of it, the head of one
// bucket while an older version of it stays, and a row that had been collected.
func hurts() []hurt {
	return []hurt{
		{"a value gone", func(t *testing.T, h *harness) {
			if err := opstest.LoseCheckpointValue(context.Background(), h.db, h.heads(t)["bucket:2026-08"].ValueHash); err != nil {
				t.Fatal(err)
			}
		}},
		{"a bucket's only row gone", func(t *testing.T, h *harness) {
			if err := opstest.LoseCheckpointRow(context.Background(), h.db, "bucket:2026-09", 1); err != nil {
				t.Fatal(err)
			}
		}},
		{"a head gone, an older version of it left", func(t *testing.T, h *harness) {
			// The run reads July's second version. Put the first back as the pick: the
			// log is what an older mint left, and a read that composed from it would
			// carry the July of an earlier watermark.
			head := h.heads(t)["bucket:2026-07"]
			if head.V < 2 {
				t.Fatal("July has no second version to lose")
			}
			if err := opstest.LoseCheckpointRow(context.Background(), h.db, head.SpanID, head.V); err != nil {
				t.Fatal(err)
			}
		}},
	}
}

// readyForHurt is a harness minted twice, July having moved on between, with July's first
// version put back in the log beside its second, as a log that had not yet collected has it.
func readyForHurt(t *testing.T) *harness {
	t.Helper()
	h := newHarness(t)
	h.seed(t)
	h.run(t, false)
	h.bp(t, "2026-07-20T08:00:00Z", 125)
	h.run(t, false)
	// Collection left July at v2 alone. Put v1 back, covering the first run's W, so that
	// losing v2 leaves an older version behind for the plain query to find.
	first := h.heads(t)["bucket:2026-07"]
	v1 := ops.CheckpointRow{SpanID: first.SpanID, V: 1, CoversW: 1, ValueHash: first.ValueHash, MintedAt: "t"}
	if err := h.db.AppendCheckpoint(context.Background(), v1); err != nil {
		t.Fatal(err)
	}
	return h
}

// A read that cannot find every checkpoint its run saw declines, whichever way it is
// short, and the value it would otherwise give is never partial: ReadAt and ReadWindow
// both report no run to start from, and the fold is the answer.
func TestReads_ACacheThatLostAnEntryDeclines(t *testing.T) {
	for _, tc := range hurts() {
		t.Run(tc.name, func(t *testing.T) {
			h := readyForHurt(t)
			ctx := context.Background()
			head, _ := h.l.Head(ctx)
			if _, ok, err := mint.ReadAt(ctx, h.l, h.db, h.deps.Values, head); err != nil || !ok {
				t.Fatalf("before the loss: ok=%v err=%v, want a composed read", ok, err)
			}
			tc.do(t, h)
			if _, ok, err := mint.ReadAt(ctx, h.l, h.db, h.deps.Values, head); err != nil || ok {
				t.Errorf("ReadAt after the loss: ok=%v err=%v, want it to decline", ok, err)
			}
			if _, ok, err := mint.ReadWindow(ctx, h.l, h.db, h.deps.Values, head, "", ""); err != nil || ok {
				t.Errorf("ReadWindow after the loss: ok=%v err=%v, want it to decline", ok, err)
			}
		})
	}
}

// valuesThatVanish is a store that loses a checkpoint's value between a read choosing its
// run and loading the value, as a cache entry removed beside the read would be.
type valuesThatVanish struct {
	mint.Store
	h    *harness
	done bool
}

func (v *valuesThatVanish) Get(ctx context.Context, kind, digest string) ([]byte, error) {
	if !v.done {
		v.done = true
		if err := opstest.LoseCheckpointValue(ctx, v.h.db, digest); err != nil {
			return nil, err
		}
	}
	return v.Store.Get(ctx, kind, digest)
}

// rowsThatVanish is a checkpoint log that loses a row after a read chose its run and
// before the read took the run's checkpoints.
type rowsThatVanish struct {
	mint.WindowLog
	during func()
}

func (r *rowsThatVanish) RunCheckpoints(ctx context.Context, run int64) ([]ops.CheckpointRow, bool, error) {
	if r.during != nil {
		r.during()
		r.during = nil
	}
	return r.WindowLog.RunCheckpoints(ctx, run)
}

// A value or a row that goes after the read chose its run declines as well, and a mint
// that comes after writes the value back.
func TestReads_ALossAfterTheRunWasChosenDeclines(t *testing.T) {
	ctx := context.Background()
	reads := map[string]func(h *harness, log mint.WindowLog, values mint.Store, w ledger.Watermark) (bool, error){
		"ReadAt": func(h *harness, log mint.WindowLog, values mint.Store, w ledger.Watermark) (bool, error) {
			_, ok, err := mint.ReadAt(ctx, h.l, log, values, w)
			return ok, err
		},
		"ReadWindow": func(h *harness, log mint.WindowLog, values mint.Store, w ledger.Watermark) (bool, error) {
			_, ok, err := mint.ReadWindow(ctx, h.l, log, values, w, "", "")
			return ok, err
		},
	}
	for name, read := range reads {
		t.Run(name+", a value", func(t *testing.T) {
			h := newHarness(t)
			h.seed(t)
			h.run(t, false)
			w, _ := h.l.Head(ctx)
			ok, err := read(h, h.db, &valuesThatVanish{Store: h.deps.Values, h: h}, w)
			if err != nil || ok {
				t.Fatalf("a value lost mid-read: ok=%v err=%v, want it to decline", ok, err)
			}
			// The next mint finds the value gone and writes it back; the read composes again.
			h.run(t, false)
			if ok, err := read(h, h.db, h.deps.Values, w); err != nil || !ok {
				t.Fatalf("after the mint: ok=%v err=%v, want a composed read", ok, err)
			}
		})
		t.Run(name+", a row", func(t *testing.T) {
			h := newHarness(t)
			h.seed(t)
			h.run(t, false)
			w, _ := h.l.Head(ctx)
			log := &rowsThatVanish{WindowLog: h.db, during: func() {
				if err := opstest.LoseCheckpointRow(ctx, h.db, "bucket:2026-08", 1); err != nil {
					t.Error(err)
				}
			}}
			ok, err := read(h, log, h.deps.Values, w)
			if err != nil || ok {
				t.Fatalf("a row lost mid-read: ok=%v err=%v, want it to decline", ok, err)
			}
		})
	}
}

// A mint writes back a value the log names and the store has lost, and the read that
// declined composes again. It does so whether the mint starts from the last run or folds
// in full, with nothing new or with a tail elsewhere.
func TestRun_WritesBackAValueTheCacheLost(t *testing.T) {
	for _, tc := range []struct {
		name   string
		run    func(context.Context, mint.Deps, bool) (mint.Result, error)
		ingest bool
	}{
		{"full", mint.RunFull, false},
		{"from the last run", mint.Run, false},
		{"from the last run, with a tail", mint.Run, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			h.deps.Budget = 1000
			h.seed(t)
			h.run(t, false)
			ctx := context.Background()
			digest := h.loseCheckpointValue(t, "bucket:2026-08")
			if tc.ingest {
				h.bp(t, "2026-09-20T06:30:00Z", 120)
			}
			head, _ := h.l.Head(ctx)
			if _, ok, _ := mint.ReadAt(ctx, h.l, h.db, h.deps.Values, head); ok {
				t.Fatal("a read composed from a log with a value missing")
			}
			if _, err := tc.run(ctx, h.deps, false); err != nil {
				t.Fatalf("mint: %v", err)
			}
			if _, err := h.db.Size(ctx, mint.Kind, digest); err != nil {
				t.Fatalf("the value was not written back: %v", err)
			}
			want, err := fold.ReadAt(ctx, h.l, head)
			if err != nil {
				t.Fatal(err)
			}
			got, ok, err := mint.ReadAt(ctx, h.l, h.db, h.deps.Values, head)
			if err != nil || !ok || got.Value.Hash() != want.Value.Hash() {
				t.Fatalf("after the mint: ok=%v err=%v, want a composed read equal to the fold", ok, err)
			}
		})
	}
}
