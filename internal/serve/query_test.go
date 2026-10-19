package serve_test

import (
	"context"
	"errors"
	"testing"

	"github.com/mattjoyce/anam/internal/consent"
	"github.com/mattjoyce/anam/internal/fold"
	"github.com/mattjoyce/anam/internal/query"
	"github.com/mattjoyce/anam/internal/receipt"
	"github.com/mattjoyce/anam/internal/serve"
)

// A query receipt replays: the same program over the projection's value at the
// receipt's watermark reproduces the answer's hash, and the answer shows from the
// artifact alone. A receipt answered under other rules cannot replay here.
func TestPublishQuery_VerifiesAndShows(t *testing.T) {
	t.Parallel()
	h := medHarness(t)
	h.deps.Definitions = h.deps.Series.(query.Definitions)
	h.seed(t)
	ctx := context.Background()

	head, err := h.l.Head(ctx)
	if err != nil {
		t.Fatal(err)
	}
	res, err := fold.ReadAt(ctx, h.l, head)
	if err != nil {
		t.Fatal(err)
	}
	tmpl, err := h.deps.Templates.Lookup("bp")
	if err != nil {
		t.Fatal(err)
	}
	served, err := serve.Derive(res.Value, tmpl, consent.Open{}, h.deps.Series, "")
	if err != nil {
		t.Fatal(err)
	}
	const text = `high(E, S) :- attr(E, "/systolic", S), S > 120.`
	ans, rules, err := query.Run(ctx, served, query.Versions(res.Live), h.deps.Definitions, text, "high", 0)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if ans.Len() != 1 {
		t.Fatalf("high = %d tuples, want 1", ans.Len())
	}
	prog := receipt.Program{RuleSet: rules.Name, RuleSetVersion: rules.Version, RuleSetHash: rules.Hash, Text: text, Relation: "high"}

	dry, err := serve.PublishQuery(ctx, h.deps, tmpl, head, prog, served, ans, true)
	if err != nil || dry.Written {
		t.Fatalf("dry run = written %v, %v", dry.Written, err)
	}
	out, err := serve.PublishQuery(ctx, h.deps, tmpl, head, prog, served, ans, false)
	if err != nil || !out.Written {
		t.Fatalf("PublishQuery = written %v, %v", out.Written, err)
	}
	if out.ReceiptHash != dry.ReceiptHash || out.Receipt.ServedHash != ans.Hash() || len(out.Receipt.FacetIDs) != 3 {
		t.Errorf("receipt = %+v (hash %s, dry %s)", out.Receipt, out.ReceiptHash, dry.ReceiptHash)
	}

	_, vr, err := serve.Verify(ctx, h.deps, out.ReceiptHash)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !vr.OK() {
		t.Errorf("the query receipt did not verify:\n%s", vr.Summary())
	}
	_, shown, err := serve.Show(h.deps, out.ReceiptHash)
	if err != nil {
		t.Fatalf("Show: %v", err)
	}
	if shown.Answer == nil || shown.Answer.Hash() != ans.Hash() || shown.Answer.Relation != "high" {
		t.Errorf("Show = %+v, want the answer with its hash", shown.Answer)
	}

	// Under other rules the answer cannot be re-derived, and verify says so rather
	// than blaming the ledger.
	moved := prog
	moved.RuleSetHash = "sha256:" + "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	other, err := serve.PublishQuery(ctx, h.deps, tmpl, head, moved, served, ans, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := serve.Verify(ctx, h.deps, other.ReceiptHash); !errors.Is(err, serve.ErrRuleSetMoved) {
		t.Errorf("Verify under other rules = %v, want ErrRuleSetMoved", err)
	}
}
