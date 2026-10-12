package serve_test

import (
	"context"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/frame"
	"github.com/mattjoyce/anam/internal/receipt"
	"github.com/mattjoyce/anam/internal/serve"
)

// A publish whose query names fields serves a frame (card 138): the receipt's served
// hash is the frame's, its facet ids are still the whole facets', the artifact holds
// the frame, Show reads it back, and Verify re-derives it through the same cut.
func TestPublish_FieldsServeAFrameThatVerifies(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.seed(t)
	ctx := context.Background()
	head, err := h.l.Head(ctx)
	if err != nil {
		t.Fatalf("Head: %v", err)
	}

	q := &receipt.Query{Type: "health.blood-pressure", Fields: []string{"/systolic"}}
	tmpl, _ := h.deps.Templates.Lookup("bp")
	value := h.narrowedBP(t, head, q)
	if value.Len() != 3 {
		t.Fatalf("the type holds %d readings, want 3", value.Len())
	}

	out, err := serve.Publish(ctx, h.deps, tmpl, head, q, value, false)
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if out.Frame == nil || out.Frame.Len() != 3 {
		t.Fatalf("Publish with fields served no frame: %+v", out.Frame)
	}
	if out.Receipt.ServedHash != out.Frame.Hash() || out.Receipt.ServedHash == value.Hash() {
		t.Errorf("served_hash = %s, want the frame's %s, not the value's %s", out.Receipt.ServedHash, out.Frame.Hash(), value.Hash())
	}
	if len(out.Receipt.FacetIDs) != 3 || strings.Join(out.Receipt.Query.Fields, " ") != "/systolic" {
		t.Errorf("the receipt names %d facets and fields %v", len(out.Receipt.FacetIDs), out.Receipt.Query.Fields)
	}
	if s := string(out.Frame.Bytes()); strings.Contains(s, "diastolic") || !strings.Contains(s, "118") {
		t.Errorf("the frame holds the wrong cells: %s", s)
	}

	r, shown, err := serve.Show(serve.Deps{Artifacts: h.store}, out.ReceiptHash)
	if err != nil {
		t.Fatalf("Show: %v", err)
	}
	if shown.Frame == nil || shown.Entries != nil || shown.Frame.Hash() != r.ServedHash {
		t.Errorf("Show of a fields receipt: frame %v, entries %v", shown.Frame, shown.Entries)
	}
	if got := strings.Join(shown.Frame.Columns, " "); got != strings.Join(frame.Envelope, " ")+" /systolic" {
		t.Errorf("columns = %q", got)
	}

	_, res, err := serve.Verify(ctx, h.deps, out.ReceiptHash)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !res.Reproduced || !res.Intact {
		t.Errorf("a fields receipt did not verify: %+v", res)
	}
}

// Fields a payload lacks are null cells, never a refusal: the cut takes away and
// invents nothing.
func TestPublish_AFieldARowLacksIsNull(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.seed(t)
	ctx := context.Background()
	head, _ := h.l.Head(ctx)
	q := &receipt.Query{Type: "health.blood-pressure", Fields: []string{"/pulse"}}
	tmpl, _ := h.deps.Templates.Lookup("bp")
	out, err := serve.Publish(ctx, h.deps, tmpl, head, q, h.narrowedBP(t, head, q), true)
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	for _, row := range out.Frame.Rows {
		if string(row[len(row)-1]) != "null" {
			t.Errorf("a missing field came back as %s", row[len(row)-1])
		}
	}
}
