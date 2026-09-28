package ledger_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

// batchOf builds n commits following readW, with distinct source refs.
func batchOf(t *testing.T, readW ledger.Watermark, n int, prefix string) []ledger.Commit {
	t.Helper()
	cs := make([]ledger.Commit, n)
	for i := range cs {
		ref := fmt.Sprintf("bp-csv:%s-%03d", prefix, i)
		f := testFacet(t, ref, fmt.Sprintf(`{"systolic": %d}`, 110+i), 1)
		cs[i] = ledger.Commit{Facet: f, Event: admittedEvent(f, int64(readW)+int64(i)+1)}
	}
	return cs
}

// A batch is one transaction over a contiguous block of seqs. The invariant it must
// not weaken is `W ≡ seq` (§8): every anam-event still gets its own unique seq, the
// block has no gaps, and the watermark lands on the last of them.
func TestAdmitBatch_AllocatesAContiguousBlockOfSeqs(t *testing.T) {
	t.Parallel()
	l := newLedger(t)
	ctx := context.Background()

	const n = 20
	if err := l.AdmitBatch(ctx, 0, batchOf(t, 0, n, "a")); err != nil {
		t.Fatalf("AdmitBatch: %v", err)
	}

	head, err := l.Head(ctx)
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	if head != n {
		t.Errorf("watermark = %d after a batch of %d, want %d", head, n, n)
	}

	events, err := l.Replay(ctx, 0, head)
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	if len(events) != n {
		t.Fatalf("replayed %d events, want %d", len(events), n)
	}
	for i, ev := range events {
		if want := int64(i) + 1; ev.Seq != want {
			t.Errorf("event %d has seq %d, want %d — the block has a gap or a repeat", i, ev.Seq, want)
		}
		if ev.Kind != facet.EventAdmitted {
			t.Errorf("event %d kind %s, want admitted", i, ev.Kind)
		}
	}

	// A second batch must continue the block, not restart it.
	if err := l.AdmitBatch(ctx, head, batchOf(t, head, 5, "b")); err != nil {
		t.Fatalf("second AdmitBatch: %v", err)
	}
	if head2, err := l.Head(ctx); err != nil || head2 != n+5 {
		t.Errorf("watermark = %d (err %v) after a second batch, want %d", head2, err, n+5)
	}
}

// Every artifact in the block must be readable at its own version afterwards —
// the transaction is atomic, so a partially-visible batch would be corruption.
func TestAdmitBatch_WritesEveryArtifact(t *testing.T) {
	t.Parallel()
	l := newLedger(t)
	ctx := context.Background()

	cs := batchOf(t, 0, 10, "c")
	if err := l.AdmitBatch(ctx, 0, cs); err != nil {
		t.Fatalf("AdmitBatch: %v", err)
	}
	for _, c := range cs {
		got, err := l.Version(ctx, c.Facet.ID, 1)
		if err != nil {
			t.Errorf("%s: %v", c.Facet.Provenance.SourceRef, err)
			continue
		}
		if got.Descriptor.ContentHash != c.Facet.Descriptor.ContentHash {
			t.Errorf("%s: hash %s, want %s", c.Facet.Provenance.SourceRef,
				got.Descriptor.ContentHash, c.Facet.Descriptor.ContentHash)
		}
	}
}

// checkBatch is the kernel refusing to write something unsound, before a transaction
// is opened and with an error that names the item.
func TestAdmitBatch_RefusesAnUnsoundBatch(t *testing.T) {
	t.Parallel()
	l := newLedger(t)
	ctx := context.Background()

	t.Run("empty", func(t *testing.T) {
		if err := l.AdmitBatch(ctx, 0, nil); err == nil {
			t.Error("an empty batch was accepted")
		}
	})

	t.Run("seqs do not follow the watermark", func(t *testing.T) {
		cs := batchOf(t, 0, 3, "e")
		cs[2].Event.Seq = 99
		if err := l.AdmitBatch(ctx, 0, cs); err == nil {
			t.Error("a batch with a broken seq block was accepted")
		}
	})

	t.Run("the same artifact twice", func(t *testing.T) {
		cs := batchOf(t, 0, 2, "f")
		cs[1].Facet = cs[0].Facet
		if err := l.AdmitBatch(ctx, 0, cs); err == nil {
			t.Error("a batch naming one artifact twice was accepted")
		}
	})
}

// A stale watermark must be reported as such and nothing written, exactly as for a
// single Admit — the batch costs more rework, but the arm is the same.
func TestAdmitBatch_StaleWatermarkWritesNothing(t *testing.T) {
	t.Parallel()
	l := newLedger(t)
	ctx := context.Background()

	if err := l.AdmitBatch(ctx, 0, batchOf(t, 0, 3, "g")); err != nil {
		t.Fatalf("first AdmitBatch: %v", err)
	}
	// A writer that still believes the watermark is 0.
	stale := batchOf(t, 0, 3, "h")
	if err := l.AdmitBatch(ctx, 0, stale); !errors.Is(err, ledger.ErrStaleWatermark) {
		t.Fatalf("stale batch = %v, want ErrStaleWatermark", err)
	}
	if head, err := l.Head(ctx); err != nil || head != 3 {
		t.Errorf("watermark = %d (err %v) after a rejected batch, want 3", head, err)
	}
	for _, c := range stale {
		if _, err := l.Version(ctx, c.Facet.ID, 1); !errors.Is(err, ledger.ErrNotFound) {
			t.Errorf("%s was written by a cancelled batch", c.Facet.Provenance.SourceRef)
		}
	}
}

// A re-run after a crash re-presents facets that already landed. The batch is
// cancelled whole, so the conflict must name which ones so the caller can drop them
// and retry the remainder rather than giving up on the block.
func TestAdmitBatch_ReportsWhichCommitsAlreadyLanded(t *testing.T) {
	t.Parallel()
	l := newLedger(t)
	ctx := context.Background()

	first := batchOf(t, 0, 3, "i")
	if err := l.AdmitBatch(ctx, 0, first); err != nil {
		t.Fatalf("first AdmitBatch: %v", err)
	}

	// Re-present two of them alongside two new ones, at the correct watermark.
	head, _ := l.Head(ctx)
	mixed := []ledger.Commit{first[0], first[2]}
	mixed = append(mixed, batchOf(t, 0, 2, "j")...)
	for i := range mixed {
		mixed[i].Event.Seq = int64(head) + int64(i) + 1
	}

	err := l.AdmitBatch(ctx, head, mixed)
	var conflict *ledger.BatchConflict
	if !errors.As(err, &conflict) {
		t.Fatalf("AdmitBatch = %v, want *BatchConflict", err)
	}
	if len(conflict.Divergent) != 0 {
		t.Errorf("Divergent = %v, want none — the bytes are identical", conflict.Divergent)
	}
	if len(conflict.AlreadyWritten) != 2 ||
		conflict.AlreadyWritten[0] != 0 || conflict.AlreadyWritten[1] != 1 {
		t.Errorf("AlreadyWritten = %v, want [0 1]", conflict.AlreadyWritten)
	}
	if head2, err := l.Head(ctx); err != nil || head2 != head {
		t.Errorf("watermark moved to %d on a cancelled batch, want %d", head2, head)
	}
}

// Different bytes under an existing identity is a lost update, and must be told
// apart from a benign re-run even when both appear in one batch.
func TestAdmitBatch_SeparatesDivergentFromAlreadyWritten(t *testing.T) {
	t.Parallel()
	l := newLedger(t)
	ctx := context.Background()

	first := batchOf(t, 0, 2, "k")
	if err := l.AdmitBatch(ctx, 0, first); err != nil {
		t.Fatalf("first AdmitBatch: %v", err)
	}
	head, _ := l.Head(ctx)

	// Same identity as first[1], different payload, still claiming version 1.
	changed := testFacet(t, first[1].Facet.Provenance.SourceRef, `{"systolic": 999}`, 1)
	mixed := []ledger.Commit{
		{Facet: first[0].Facet, Event: admittedEvent(first[0].Facet, int64(head)+1)},
		{Facet: changed, Event: admittedEvent(changed, int64(head)+2)},
	}

	err := l.AdmitBatch(ctx, head, mixed)
	var conflict *ledger.BatchConflict
	if !errors.As(err, &conflict) {
		t.Fatalf("AdmitBatch = %v, want *BatchConflict", err)
	}
	if len(conflict.AlreadyWritten) != 1 || conflict.AlreadyWritten[0] != 0 {
		t.Errorf("AlreadyWritten = %v, want [0]", conflict.AlreadyWritten)
	}
	if len(conflict.Divergent) != 1 || conflict.Divergent[0] != 1 {
		t.Errorf("Divergent = %v, want [1]", conflict.Divergent)
	}
}

// KnownFacets is only trustworthy in the negative, and that is what the batch
// disposition relies on: absent means the facet has never been admitted, so its
// first artifact is version 1 without asking anything else.
func TestKnownFacets_IsATrustworthyNegative(t *testing.T) {
	t.Parallel()
	l := newLedger(t)
	ctx := context.Background()

	cs := batchOf(t, 0, 3, "m")
	if err := l.AdmitBatch(ctx, 0, cs); err != nil {
		t.Fatalf("AdmitBatch: %v", err)
	}

	absent := facet.NewID("bp-csv:never-seen")
	ids := []string{cs[0].Facet.ID, absent, cs[2].Facet.ID}

	known, err := l.KnownFacets(ctx, ids)
	if err != nil {
		t.Fatalf("KnownFacets: %v", err)
	}
	for _, id := range []string{cs[0].Facet.ID, cs[2].Facet.ID} {
		if !known[id] {
			t.Errorf("admitted facet %s reported unknown — a duplicate v1 would be written over it", id)
		}
	}
	if known[absent] {
		t.Error("an unadmitted facet was reported known")
	}

	if _, err := l.KnownFacets(ctx, nil); err != nil {
		t.Errorf("KnownFacets(nil) = %v, want no error", err)
	}
	// A repeated id is answered once and must not make the probe fail.
	if got, err := l.KnownFacets(ctx, []string{cs[0].Facet.ID, cs[0].Facet.ID}); err != nil {
		t.Errorf("KnownFacets with a repeated id: %v", err)
	} else if !got[cs[0].Facet.ID] {
		t.Error("a repeated id came back unknown")
	}
}
