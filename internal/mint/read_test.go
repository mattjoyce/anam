package mint_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/fold"
	"github.com/mattjoyce/anam/internal/mint"
	"github.com/mattjoyce/anam/internal/store/artifact"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

// The oracle: at every watermark, a read composed from checkpoints is the full fold,
// byte for byte, across the cases that make composition hard. A facet's fact-time
// moves it into another bucket, a backfill reopens an old bucket, a retraction
// empties one, and a facet is corrected twice in one tail. Watermarks before the
// first mint run have nothing to start from and are the full fold's.
func TestReadAt_IsTheFoldAtEveryWatermark(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.seed(t)
	moved := h.weight(t, "w-1", "2026-07-09T07:00:00Z", 81.2)
	alone := h.bp(t, "2026-05-20T06:30:00Z", 125) // May's only reading
	first := h.run(t, false).W

	// The tail after the first run.
	h.weight(t, "w-1", "2026-08-09T07:00:00Z", 81.2) // moves July → August
	h.bp(t, "2026-07-02T06:30:00Z", 122)             // backfill into July
	if _, _, err := h.gate.Retract(ctx, alone); err != nil {
		t.Fatalf("retract: %v", err)
	}
	h.weight(t, "w-2", "2026-09-01T07:00:00Z", 80.0)
	h.weight(t, "w-2", "2026-09-01T07:00:00Z", 80.4) // corrected
	h.weight(t, "w-2", "2026-09-02T07:00:00Z", 80.4) // and moved within September
	second := h.run(t, false).W
	h.bp(t, "2026-09-20T06:30:00Z", 116) // a tail after the second run

	head, err := h.l.Head(ctx)
	if err != nil {
		t.Fatal(err)
	}
	store := h.deps.Artifacts
	for w := ledger.Watermark(1); w <= head; w++ {
		want, err := fold.ReadAt(ctx, h.l, w)
		if err != nil {
			t.Fatal(err)
		}
		got, ok, err := mint.ReadAt(ctx, h.l, h.db, store, w)
		if err != nil {
			t.Fatalf("W=%d: %v", w, err)
		}
		if ok != (w >= first) {
			t.Fatalf("W=%d: composed=%v, want %v (first run at %d)", w, ok, w >= first, first)
		}
		if !ok {
			continue
		}
		if got.Value.Hash() != want.Value.Hash() {
			t.Fatalf("W=%d: composed %s (%d facets), fold %s (%d facets); run %d, tail %d",
				w, got.Value.Hash(), got.Value.Len(), want.Value.Hash(), want.Value.Len(), got.Run, got.Tail)
		}
		for id, v := range got.Versions {
			if want.States[id].Version != v {
				t.Errorf("W=%d: %s at v%d, fold says v%d", w, id, v, want.States[id].Version)
			}
		}
		wantRun := first
		if w >= second {
			wantRun = second
		}
		if got.Run != wantRun || got.Tail != int(w-wantRun) {
			t.Errorf("W=%d: started from run %d with tail %d, want run %d", w, got.Run, got.Tail, wantRun)
		}
	}
	_ = moved
}

// A checkpoint log built from another ledger describes another record. Composing from
// it would answer with someone else's facts, so the read refuses.
func TestReadAt_RefusesAnotherLedgersCheckpoints(t *testing.T) {
	h := newHarness(t)
	h.seed(t)
	h.run(t, false)
	other := newHarness(t)
	other.seed(t)
	head, _ := other.l.Head(context.Background())
	_, _, err := mint.ReadAt(context.Background(), other.l, h.db, h.deps.Artifacts, head)
	if !errors.Is(err, mint.ErrOtherLedger) {
		t.Fatalf("ReadAt over another ledger's log: %v", err)
	}
}

// A checkpoint whose file was altered fails the read: the store checks the bytes
// against the hash the row names, and nothing falls back to another answer.
func TestReadAt_RefusesATamperedCheckpoint(t *testing.T) {
	h := newHarness(t)
	h.seed(t)
	h.run(t, false)
	files, err := filepath.Glob(filepath.Join(h.dir, mint.Kind, "*.json"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no checkpoint files: %v", err)
	}
	b, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(files[0], []byte(strings.Replace(string(b), "118", "181", 1)+" "), 0o600); err != nil {
		t.Fatal(err)
	}
	head, _ := h.l.Head(context.Background())
	_, _, err = mint.ReadAt(context.Background(), h.l, h.db, h.deps.Artifacts, head)
	if !errors.Is(err, artifact.ErrCorrupt) {
		t.Fatalf("ReadAt over a tampered checkpoint: %v", err)
	}
}
