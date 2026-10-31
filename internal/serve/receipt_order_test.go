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

// errBodyRefused is the failure refusingTemplateBodies gives filing a template body.
var errBodyRefused = errors.New("template body refused")

// refusingTemplateBodies is a store that fails to file a template body, as a crash or
// a full disk would between the body and the receipt.
type refusingTemplateBodies struct{ serve.Store }

func (s refusingTemplateBodies) Put(ctx context.Context, kind string, body []byte) (string, error) {
	if kind == receipt.InlineKind {
		return "", errBodyRefused
	}
	return s.Store.Put(ctx, kind, body)
}

// A receipt is filed only after the template body it names: if the body cannot be
// filed, no receipt is, so no receipt exists that its own store cannot replay.
func TestPublish_FilesNoReceiptWhenItsTemplateBodyFails(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.seed(t)
	ctx := context.Background()

	deps := h.deps
	deps.Published = refusingTemplateBodies{deps.Published}
	if _, err := serve.Run(ctx, deps, "bp", false); !errors.Is(err, errBodyRefused) {
		t.Fatalf("Run = %v, want the body's failure", err)
	}
	if got, err := h.db.List(ctx, receipt.Kind); err != nil || len(got) != 0 {
		t.Errorf("a publish whose body failed filed %d receipts, %v", len(got), err)
	}
}

// The same holds for a query's receipt.
func TestPublishQuery_FilesNoReceiptWhenItsTemplateBodyFails(t *testing.T) {
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
	prog := receipt.Program{RuleSet: rules.Name, RuleSetVersion: rules.Version, RuleSetHash: rules.Hash, Text: text, Relation: "high"}

	deps := h.deps
	deps.Published = refusingTemplateBodies{deps.Published}
	if _, err := serve.PublishQuery(ctx, deps, tmpl, head, prog, served, ans, false); !errors.Is(err, errBodyRefused) {
		t.Fatalf("PublishQuery = %v, want the body's failure", err)
	}
	if got, err := h.db.List(ctx, receipt.Kind); err != nil || len(got) != 0 {
		t.Errorf("a query publish whose body failed filed %d receipts, %v", len(got), err)
	}
}
