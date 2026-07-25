package fold_test

import (
	"context"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/fold"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

// An as-of read must return the artifact versions that were in force at the watermark,
// not the ones in force now. That distinction is invisible until a facet has been
// superseded, so this test builds the supersession and then reads on both sides of it.
//
// This is the criterion the whole receipt rests on: if ReadAt silently read head
// versions, every receipt would still "verify" at head and fail the moment anything
// moved — which is exactly when a receipt is worth having.
func TestReadAt_ReturnsTheVersionsInForceThen(t *testing.T) {
	t.Parallel()
	g, l := newHarness(t)
	ctx := context.Background()

	src := source{
		"2026-01-24T06:30:00Z": `"systolic": 118, "diastolic": 76`,
		"2026-01-25T06:30:00Z": `"systolic": 121, "diastolic": 79`,
	}
	src.ingest(t, g)

	before, err := l.Head(ctx)
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	original, err := fold.ReadAt(ctx, l, before)
	if err != nil {
		t.Fatalf("ReadAt(before): %v", err)
	}
	if original.Value.Len() != 2 {
		t.Fatalf("before the correction: %d live facets, want 2", original.Value.Len())
	}

	// The source corrects a reading and gains a third row: one supersession, one
	// admission. Both change the canonical.
	src["2026-01-24T06:30:00Z"] = `"systolic": 117, "diastolic": 75`
	src["2026-01-26T06:30:00Z"] = `"systolic": 119, "diastolic": 77`
	src.ingest(t, g)

	after, err := l.Head(ctx)
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	if after <= before {
		t.Fatalf("the ledger did not advance: W was %d, now %d", before, after)
	}

	// The as-of read, run now, must reproduce what the earlier read saw — byte for
	// byte, hash for hash.
	asOf, err := fold.ReadAt(ctx, l, before)
	if err != nil {
		t.Fatalf("ReadAt(before), after the correction: %v", err)
	}
	if asOf.Value.Hash() != original.Value.Hash() {
		t.Errorf("as-of hash moved after later ingest:\n got %s\nwant %s", asOf.Value.Hash(), original.Value.Hash())
	}
	if string(asOf.Value.Bytes()) != string(original.Value.Bytes()) {
		t.Errorf("as-of bytes moved after later ingest")
	}
	if asOf.Value.Len() != 2 {
		t.Errorf("as-of served %d facets, want the 2 that existed then", asOf.Value.Len())
	}

	// And it must hold the *old* content, not the new. This is the assertion that
	// fails if the fetch quietly asks for the current version.
	corrected := facetIDFor(t, original, "2026-01-24T06:30:00Z")
	for _, e := range asOf.Value.Entries() {
		if e.FacetID != corrected {
			continue
		}
		if got := string(e.Payload); !strings.Contains(got, `"systolic":118`) {
			t.Errorf("as-of returned the corrected payload, not the one in force at W=%d: %s", before, got)
		}
	}

	// The head read sees the correction and the new row.
	head, err := fold.ReadAt(ctx, l, after)
	if err != nil {
		t.Fatalf("ReadAt(after): %v", err)
	}
	if head.Value.Len() != 3 {
		t.Errorf("at head: %d live facets, want 3", head.Value.Len())
	}
	if head.Value.Hash() == original.Value.Hash() {
		t.Error("the canonical hash did not move across a correction and an admission")
	}
}

// Read is ReadAt at the head watermark; the two must not be able to disagree.
func TestReadAt_AtHeadEqualsRead(t *testing.T) {
	t.Parallel()
	g, l := newHarness(t)
	ctx := context.Background()

	source{
		"2026-01-24T06:30:00Z": `"systolic": 118, "diastolic": 76`,
		"2026-01-25T06:30:00Z": `"systolic": 121, "diastolic": 79`,
		"2026-01-26T06:30:00Z": `"systolic": 117, "diastolic": 75`,
	}.ingest(t, g)

	read, err := fold.Read(ctx, l)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	at, err := fold.ReadAt(ctx, l, read.W)
	if err != nil {
		t.Fatalf("ReadAt(head): %v", err)
	}

	if at.W != read.W {
		t.Errorf("W: ReadAt %d, Read %d", at.W, read.W)
	}
	if at.Value.Hash() != read.Value.Hash() {
		t.Errorf("hash: ReadAt %s, Read %s", at.Value.Hash(), read.Value.Hash())
	}
	if at.Value.Len() != read.Value.Len() {
		t.Errorf("len: ReadAt %d, Read %d", at.Value.Len(), read.Value.Len())
	}
}

// A watermark before anything was admitted is a real question with a real answer: the
// empty person. It must not be an error, because a receipt could legitimately name it.
func TestReadAt_GenesisIsEmpty(t *testing.T) {
	t.Parallel()
	g, l := newHarness(t)
	ctx := context.Background()

	source{"2026-01-24T06:30:00Z": `"systolic": 118, "diastolic": 76`}.ingest(t, g)

	res, err := fold.ReadAt(ctx, l, ledger.Watermark(0))
	if err != nil {
		t.Fatalf("ReadAt(0): %v", err)
	}
	if res.Value.Len() != 0 {
		t.Errorf("at W=0: %d live facets, want 0", res.Value.Len())
	}
	if len(res.States) != 0 {
		t.Errorf("at W=0: %d folded states, want 0", len(res.States))
	}
}

// facetIDFor finds the facet id a source row produced, by its source_ref.
func facetIDFor(t *testing.T, res fold.Result, measuredAt string) string {
	t.Helper()
	for _, e := range res.Value.Entries() {
		if e.SourceRef == "bp-csv:"+measuredAt {
			return e.FacetID
		}
	}
	t.Fatalf("no facet for %s in the canonical", measuredAt)
	return ""
}
