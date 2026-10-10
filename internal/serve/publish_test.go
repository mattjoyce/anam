package serve_test

import (
	"context"
	"errors"
	"testing"

	"github.com/mattjoyce/anam/internal/consent"
	"github.com/mattjoyce/anam/internal/fold"
	"github.com/mattjoyce/anam/internal/projection"
	"github.com/mattjoyce/anam/internal/receipt"
	"github.com/mattjoyce/anam/internal/serve"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

// narrowedBP is the bp template's value at w, narrowed by q: what a read publishes.
func (h *harness) narrowedBP(t *testing.T, w ledger.Watermark, q *receipt.Query) fold.Value {
	t.Helper()
	tmpl, err := h.deps.Templates.Lookup("bp")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	res, err := fold.ReadAt(context.Background(), h.l, w)
	if err != nil {
		t.Fatalf("ReadAt: %v", err)
	}
	v, err := projection.Apply(res.Value, tmpl, consent.Open{}, h.deps.Series)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	out, err := serve.Narrow(v, q, h.deps.Series)
	if err != nil {
		t.Fatalf("Narrow: %v", err)
	}
	return out
}

func TestPublish_ANarrowedReadVerifiesAfterACorrection(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.seed(t)
	ctx := context.Background()
	head, err := h.l.Head(ctx)
	if err != nil {
		t.Fatalf("Head: %v", err)
	}

	// Two of the three readings: the 24th and the 25th.
	q := &receipt.Query{From: "2026-01-24T00:00:00.000000000Z", Before: "2026-01-26T00:00:00.000000000Z", Type: "health.blood-pressure"}
	tmpl, _ := h.deps.Templates.Lookup("bp")
	value := h.narrowedBP(t, head, q)
	if value.Len() != 2 {
		t.Fatalf("the window holds %d readings, want 2", value.Len())
	}

	out, err := serve.Publish(ctx, h.deps, tmpl, head, q, value, false)
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if out.Receipt.Query == nil || *out.Receipt.Query != *q {
		t.Errorf("the receipt's query is %+v, want %+v", out.Receipt.Query, q)
	}
	if out.Receipt.ServedHash != value.Hash() || len(out.Receipt.FacetIDs) != 2 {
		t.Errorf("the receipt does not name the narrowed value: %s, %d facets", out.Receipt.ServedHash, len(out.Receipt.FacetIDs))
	}

	// A correction inside the window, after the read.
	h.admit(t, "bp-csv", "health.blood-pressure", "2026-01-24T06:30:00Z", `"systolic": 116, "diastolic": 74`)

	_, res, err := serve.Verify(ctx, h.deps, out.ReceiptHash)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !res.OK() {
		t.Fatalf("a narrowed receipt did not verify after a correction:\n%s", res.Summary())
	}

	// Verifying replayed the query: without it the whole projection, three readings,
	// would not have reproduced two.
	whole := h.narrowedBP(t, head, nil)
	if whole.Hash() == value.Hash() {
		t.Fatal("the narrowing changed nothing; the test is vacuous")
	}
}

func TestPublish_ASnapshotVerifies(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.seed(t)
	ctx := context.Background()
	head, _ := h.l.Head(ctx)

	q := &receipt.Query{At: "2026-01-25T23:59:59.999999999Z"}
	tmpl, _ := h.deps.Templates.Lookup("bp")
	value := h.narrowedBP(t, head, q)
	out, err := serve.Publish(ctx, h.deps, tmpl, head, q, value, false)
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if _, res, err := serve.Verify(ctx, h.deps, out.ReceiptHash); err != nil || !res.OK() {
		t.Fatalf("a snapshot receipt did not verify: %v\n%s", err, res.Summary())
	}
}

func TestPublish_AWholeProjectionIsTheSameReceiptAsServe(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.seed(t)
	ctx := context.Background()
	head, _ := h.l.Head(ctx)

	run, err := serve.Run(ctx, h.deps, "bp", true)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	tmpl, _ := h.deps.Templates.Lookup("bp")
	pub, err := serve.Publish(ctx, h.deps, tmpl, head, nil, h.narrowedBP(t, head, nil), true)
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if pub.ReceiptHash != run.ReceiptHash {
		t.Errorf("an unnarrowed publish filed %s, serve %s; they must be one receipt", pub.ReceiptHash, run.ReceiptHash)
	}
}

func TestNarrow_RefusesAnEmptyQueryAndTwoUpperBounds(t *testing.T) {
	t.Parallel()
	for name, q := range map[string]*receipt.Query{
		"empty":     {},
		"at+before": {At: "2026-01-25T00:00:00.000000000Z", Before: "2026-01-26T00:00:00.000000000Z"},
	} {
		if _, err := serve.Narrow(fold.Value{}, q, nil); !errors.Is(err, receipt.ErrMalformed) {
			t.Errorf("%s: err = %v, want ErrMalformed", name, err)
		}
	}
}
