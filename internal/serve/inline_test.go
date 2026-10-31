package serve_test

import (
	"context"
	"errors"
	"testing"

	"github.com/mattjoyce/anam/internal/consent"
	"github.com/mattjoyce/anam/internal/fold"
	"github.com/mattjoyce/anam/internal/projection"
	"github.com/mattjoyce/anam/internal/query"
	"github.com/mattjoyce/anam/internal/receipt"
	"github.com/mattjoyce/anam/internal/serve"
	"github.com/mattjoyce/anam/internal/store/ops"
	"github.com/mattjoyce/anam/internal/store/ops/opstest"
)

const inlineBP = `{"types":["health.blood-pressure"],"statuses":["admitted","endorsed"],"max_classification":"Sensitive"}`

// answerThrough runs a query over the blood-pressure readings through the inline
// template and files it, as the daemon's query route does.
func answerThrough(t *testing.T, h *harness, tmpl projection.Template, dryRun bool) serve.Served {
	t.Helper()
	ctx := context.Background()
	head, err := h.l.Head(ctx)
	if err != nil {
		t.Fatal(err)
	}
	res, err := fold.ReadAt(ctx, h.l, head)
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
		t.Fatal(err)
	}
	prog := receipt.Program{RuleSet: rules.Name, RuleSetVersion: rules.Version, RuleSetHash: rules.Hash, Text: text, Relation: "high"}
	out, err := serve.PublishQuery(ctx, h.deps, tmpl, head, prog, served, ans, dryRun)
	if err != nil {
		t.Fatalf("PublishQuery: %v", err)
	}
	return out
}

// Card 211: a query through an inline template files the template's body by hash beside
// its receipt, the receipt says it is inline, and verify finds the body by that hash and
// replays the answer. A dry run files nothing.
func TestPublishQuery_InlineTemplateIsFiledAndReplayed(t *testing.T) {
	t.Parallel()
	h := medHarness(t)
	h.deps.Definitions = h.deps.Series.(query.Definitions)
	h.seed(t)
	ctx := context.Background()
	tmpl, err := projection.ParseInline([]byte(inlineBP))
	if err != nil {
		t.Fatal(err)
	}

	answerThrough(t, h, tmpl, true)
	if got, _ := h.db.List(ctx, receipt.InlineKind); len(got) != 0 {
		t.Fatalf("a dry run filed %d inline bodies", len(got))
	}

	out := answerThrough(t, h, tmpl, false)
	r := out.Receipt
	if !r.TemplateInline || r.Template != projection.InlineName || r.TemplateVersion != 1 || r.TemplateHash != tmpl.Hash {
		t.Fatalf("receipt = %+v, want inline, named by %s", r, tmpl.Hash)
	}
	if got, _ := h.db.List(ctx, receipt.InlineKind); len(got) != 1 || got[0] != tmpl.Hash {
		t.Fatalf("inline bodies = %v, want [%s]", got, tmpl.Hash)
	}
	if _, vr, err := serve.Verify(ctx, h.deps, out.ReceiptHash); err != nil || !vr.OK() {
		t.Fatalf("Verify = %v\n%s", err, vr.Summary())
	}
	// The same template asked again files the one body again, not a second.
	answerThrough(t, h, tmpl, false)
	if got, _ := h.db.List(ctx, receipt.InlineKind); len(got) != 1 {
		t.Errorf("a second query through the same template left %d bodies", len(got))
	}

	// The registered templates are not consulted: verify of an inline receipt needs
	// none of them.
	h.deps.Templates, _ = projection.NewRegistry()
	if _, vr, err := serve.Verify(ctx, h.deps, out.ReceiptHash); err != nil || !vr.OK() {
		t.Errorf("Verify with no registered templates = %v\n%s", err, vr.Summary())
	}
}

// A receipt whose inline body is not held cannot be replayed, and verify says the template is
// gone rather than blaming the ledger. A body altered under its hash is refused by the store.
func TestVerify_InlineBodyMissingOrAltered(t *testing.T) {
	t.Parallel()
	h := medHarness(t)
	h.deps.Definitions = h.deps.Series.(query.Definitions)
	h.seed(t)
	ctx := context.Background()
	tmpl, err := projection.ParseInline([]byte(inlineBP))
	if err != nil {
		t.Fatal(err)
	}

	// The receipt is derived with a dry run and filed alone: its body was never filed.
	dry := answerThrough(t, h, tmpl, true)
	b, err := dry.Receipt.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	hash, err := h.db.Put(ctx, receipt.Kind, b)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := serve.Verify(ctx, h.deps, hash); !errors.Is(err, serve.ErrTemplateMoved) {
		t.Errorf("Verify with the body missing = %v, want ErrTemplateMoved", err)
	}

	// Now the body is there, but altered: a row that no longer hashes to its key.
	if err := opstest.Corrupt(ctx, h.db, ops.KindInlineProjection, tmpl.Hash, []byte(`{"types":[]}`)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := serve.Verify(ctx, h.deps, hash); !errors.Is(err, ops.ErrCorrupt) {
		t.Errorf("Verify with the body altered = %v, want ErrCorrupt", err)
	}
}
