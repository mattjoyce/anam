package mint_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/fold"
	"github.com/mattjoyce/anam/internal/mint"
	"github.com/mattjoyce/anam/internal/store/ledger"
	"github.com/mattjoyce/anam/internal/store/ops"
	"github.com/mattjoyce/anam/internal/store/ops/opstest"
)

// The oracle: at every watermark, a read composed from checkpoints is the full fold,
// byte for byte, across the cases that make composition hard. A facet's fact-time
// moves it into another bucket, a backfill reopens an old bucket, a retraction
// empties one, and a facet is corrected twice in one tail. A watermark before the
// newest mint run has nothing to start from, since a mint collects the runs it
// supersedes, and answers from the full fold: so it is checked at each stage, while
// the run it starts from is the newest, and again once a later run has taken its place.
func TestReadAt_IsTheFoldAtEveryWatermark(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.seed(t)
	h.weight(t, "w-1", "2026-07-09T07:00:00Z", 81.2)
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
	h.checkReads(t, 1, first, nil)

	second := h.run(t, false).W
	h.bp(t, "2026-09-20T06:30:00Z", 116) // a tail after the second run
	h.checkReads(t, 1, second, nil)
}

// checkReads holds ReadAt, and ReadWindow over each window given, to the full fold at
// every watermark from 1 to the ledger's head. run is the newest mint run, the only one
// the log keeps: a read from it on composes from it and its tail, and a read before it
// has no run and says so. Either way the value is the fold's.
func (h *harness) checkReads(t *testing.T, from, run ledger.Watermark, windows [][2]string) {
	t.Helper()
	ctx := context.Background()
	head, err := h.l.Head(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for w := from; w <= head; w++ {
		want, err := fold.ReadAt(ctx, h.l, w)
		if err != nil {
			t.Fatal(err)
		}
		got, ok, err := mint.ReadAt(ctx, h.l, h.db, h.deps.Values, w)
		if err != nil {
			t.Fatalf("W=%d: %v", w, err)
		}
		if ok != (w >= run) {
			t.Fatalf("W=%d: composed=%v, want %v (newest run at %d)", w, ok, w >= run, run)
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
		if got.Run != run || got.Tail != int(w-run) {
			t.Errorf("W=%d: started from run %d with tail %d, want run %d", w, got.Run, got.Tail, run)
		}
		for _, win := range windows {
			gotWin, ok, err := mint.ReadWindow(ctx, h.l, h.db, h.deps.Values, w, win[0], win[1])
			if err != nil || !ok {
				t.Fatalf("W=%d %v: ok=%v %v", w, win, ok, err)
			}
			if gotWin.Hash != want.Value.Hash() || gotWin.Live != want.Value.Len() {
				t.Fatalf("W=%d %v: whole %s/%d, fold %s/%d", w, win, gotWin.Hash, gotWin.Live, want.Value.Hash(), want.Value.Len())
			}
			if a, b := inWindow(gotWin.Value.Entries(), win), inWindow(want.Value.Entries(), win); !sameIDs(a, b) {
				t.Fatalf("W=%d %v: window %v, fold %v", w, win, a, b)
			}
		}
	}
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
	_, _, err := mint.ReadAt(context.Background(), other.l, h.db, h.deps.Values, head)
	if !errors.Is(err, mint.ErrOtherLedger) {
		t.Fatalf("ReadAt over another ledger's log: %v", err)
	}
}

// A checkpoint whose value was altered fails the read: the store checks the bytes
// against the hash the row names, and nothing falls back to another answer.
func TestReadAt_RefusesATamperedCheckpoint(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.seed(t)
	h.run(t, false)
	digests, err := h.db.List(ctx, mint.Kind)
	if err != nil || len(digests) == 0 {
		t.Fatalf("no checkpoint values: %v", err)
	}
	b, err := h.db.Get(ctx, mint.Kind, digests[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := opstest.Corrupt(ctx, h.db, mint.Kind, digests[0], []byte(strings.Replace(string(b), "118", "181", 1)+" ")); err != nil {
		t.Fatal(err)
	}
	head, _ := h.l.Head(ctx)
	_, _, err = mint.ReadAt(ctx, h.l, h.db, h.deps.Values, head)
	if !errors.Is(err, ops.ErrCorrupt) {
		t.Fatalf("ReadAt over a tampered checkpoint: %v", err)
	}
}

// The narrow read's oracle: at every watermark from the newest run on, and for windows
// that cut across the buckets the tail dirtied, the whole hash and live count are the
// full fold's, and the window's entries are the full fold's entries in that window.
func TestReadWindow_IsTheFoldInTheWindow(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.seed(t)
	h.weight(t, "w-1", "2026-07-09T07:00:00Z", 81.2)
	alone := h.bp(t, "2026-05-20T06:30:00Z", 125)
	first := h.run(t, false).W
	h.weight(t, "w-1", "2026-08-09T07:00:00Z", 81.2)
	h.bp(t, "2026-07-02T06:30:00Z", 122)
	if _, _, err := h.gate.Retract(ctx, alone); err != nil {
		t.Fatal(err)
	}
	h.weight(t, "w-2", "2026-10-01T07:00:00Z", 80.0) // a bucket no checkpoint has

	windows := [][2]string{
		{"", ""},
		{"2026-07-01T00:00:00.000000000Z", "2026-08-01T00:00:00.000000000Z"},
		{"2026-08-01T00:00:00.000000000Z", ""},
		{"", "2026-06-01T00:00:00.000000000Z"},
		{"2026-09-15T00:00:00.000000000Z", "2026-10-15T00:00:00.000000000Z"},
	}
	h.checkReads(t, first, first, windows)
	second := h.run(t, false).W
	h.weight(t, "w-2", "2026-09-30T07:00:00Z", 80.0) // moved back, after the run
	h.checkReads(t, 1, second, windows)
}

func inWindow(entries []fold.Entry, win [2]string) []string {
	var ids []string
	for _, e := range entries {
		if (win[0] == "" || e.FactTime >= win[0]) && (win[1] == "" || e.FactTime < win[1]) {
			ids = append(ids, e.FacetID+"@"+e.FactTime+"#"+e.ContentHash)
		}
	}
	return ids
}

func sameIDs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// With a budget the mint subdivides: a sparse year stays one bucket, a dense month is
// split into days, and later growth splits further. Reads composed across those
// splits, whole and windowed, are still the full fold at every watermark.
func TestSubdivision_SplitsByBytesAndReadsAreStillTheFold(t *testing.T) {
	h := newHarness(t)
	h.deps.Budget = 1000 // two readings fit, six do not
	h.bp(t, "2024-03-01T06:30:00Z", 120)
	h.bp(t, "2024-11-01T06:30:00Z", 121)
	for d := 1; d <= 6; d++ {
		h.bp(t, fmt.Sprintf("2026-07-%02dT06:30:00Z", d), 110+d)
	}
	first := h.run(t, false)
	ids := map[string]bool{}
	for _, o := range first.Outcomes {
		ids[o.SpanID] = true
	}
	if !ids["bucket:2024"] || !ids["bucket:2026-07-03"] || ids["bucket:2026"] || ids["bucket:2026-07"] {
		t.Fatalf("first layout: %v", ids)
	}

	// Growth after the run: August fills and must split; a correction moves a weight
	// from a split day back into the unsplit 2024.
	h.weight(t, "w-1", "2026-07-02T08:00:00Z", 80)
	for d := 1; d <= 6; d++ {
		h.bp(t, fmt.Sprintf("2026-08-%02dT06:30:00Z", d), 100+d)
	}
	h.weight(t, "w-1", "2024-05-02T08:00:00Z", 80)
	windows := [][2]string{
		{"2026-08-01T00:00:00.000000000Z", "2026-09-01T00:00:00.000000000Z"},
		{"2024-01-01T00:00:00.000000000Z", "2025-01-01T00:00:00.000000000Z"},
		{"2026-07-02T00:00:00.000000000Z", "2026-07-03T00:00:00.000000000Z"},
	}
	h.checkReads(t, first.W, first.W, windows)
	second := h.run(t, false)
	ids = map[string]bool{}
	for _, o := range second.Outcomes {
		ids[o.SpanID] = true
	}
	if !ids["bucket:2026-08-04"] || ids["bucket:2026-08"] {
		t.Fatalf("second layout did not split August: %v", ids)
	}
	h.bp(t, "2026-08-09T06:30:00Z", 99) // a tail after the second run
	h.checkReads(t, 1, second.W, windows)
}
