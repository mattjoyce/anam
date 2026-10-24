package serve_test

import (
	"context"
	"errors"
	"strings"
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
	_, shown, err := serve.Show(ctx, h.deps, out.ReceiptHash)
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

// A query receipt is verified under the text its hash names, never the version
// alone. Version 1 was served as three different texts; a receipt cut under each
// still verifies, one cut under the bundled v2 does, and one whose hash no held
// text has is a moved rule set.
func TestVerifyQuery_SelectsTheRuleSetByHash(t *testing.T) {
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
	bundled, err := query.Bundled()
	if err != nil {
		t.Fatal(err)
	}
	// The text asks for current/2, so the rules in it decide the answer's meaning.
	const text = `instant(At) :- entry(_, "health.blood-pressure", At, _, _, _, _).
standing(E, At) :- current(E, At).`

	tests := []struct {
		name string
		hash string
	}{
		{"the bundled v2", bundled.Hash},
		{"v1 as first served", "sha256:bd62040288c0e5b4e6de576c13618b065714736ed6c628106b73b0769c32727d"},
		{"v1 over record.note, the one live receipt's text", "sha256:e29634877441262c765923739d4d2cb49ae7e15d93b537dd972ebc68bb7c026d"},
		{"v1 over record.edge", "sha256:f8a1a46bce8701e37c18e92340e5ce7ab082d3f791ec98c0dbacbc61802a2220"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rules, ok, err := query.HeldRuleSet(tt.hash)
			if err != nil || !ok {
				t.Fatalf("HeldRuleSet = ok %v, %v", ok, err)
			}
			ans, err := query.RunWith(ctx, rules, served, query.Versions(res.Live), h.deps.Definitions, text, "standing", 0)
			if err != nil {
				t.Fatalf("RunWith: %v", err)
			}
			if ans.Len() == 0 {
				t.Fatal("the answer is empty; the receipt would prove nothing")
			}
			prog := receipt.Program{RuleSet: rules.Name, RuleSetVersion: rules.Version, RuleSetHash: rules.Hash, Text: text, Relation: "standing"}
			out, err := serve.PublishQuery(ctx, h.deps, tmpl, head, prog, served, ans, false)
			if err != nil {
				t.Fatal(err)
			}
			_, vr, err := serve.Verify(ctx, h.deps, out.ReceiptHash)
			if err != nil {
				t.Fatalf("Verify: %v", err)
			}
			if !vr.OK() {
				t.Errorf("a receipt under %s v%d did not verify:\n%s", rules.Name, rules.Version, vr.Summary())
			}
		})
	}

	t.Run("a hash no held text has", func(t *testing.T) {
		rules, _, _ := query.HeldRuleSet(bundled.Hash)
		ans, err := query.RunWith(ctx, rules, served, query.Versions(res.Live), h.deps.Definitions, text, "standing", 0)
		if err != nil {
			t.Fatal(err)
		}
		// Right version, wrong hash: the version alone must not select a text.
		prog := receipt.Program{RuleSet: "anam", RuleSetVersion: query.BundledVersion, Text: text, Relation: "standing",
			RuleSetHash: "sha256:" + "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}
		out, err := serve.PublishQuery(ctx, h.deps, tmpl, head, prog, served, ans, false)
		if err != nil {
			t.Fatal(err)
		}
		_, _, err = serve.Verify(ctx, h.deps, out.ReceiptHash)
		if !errors.Is(err, serve.ErrRuleSetMoved) {
			t.Fatalf("Verify = %v, want ErrRuleSetMoved", err)
		}
		if !strings.Contains(err.Error(), "rule set has moved") {
			t.Errorf("the message does not say the rule set has moved: %v", err)
		}
	})
}
