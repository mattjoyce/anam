package serve_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/consent"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/fold"
	"github.com/mattjoyce/anam/internal/gate"
	"github.com/mattjoyce/anam/internal/receipt"
	"github.com/mattjoyce/anam/internal/serve"
)

func courseEventManifest(typ string, closes bool) string {
	return fmt.Sprintf(`{
  "meta": {
    "facet_type": %q, "definition_version": 1, "classification": "Sensitive",
    "validation_mode": "strict", "source_schema": "records/%s",
    "time_semantics": {"fact_time": {"pointer": "/date", "layout": "2006-01-02", "zone": "UTC"}},
    "identity_key": ["/id"],
    "series": {"name": "test.course", "key": ["/name", "/course"], "closes": %t}
  },
  "schema": {"type": "object", "required": ["id", "date", "name", "course"],
    "properties": {"id": {"type": "string", "minLength": 1}, "date": {"type": "string", "minLength": 1}, "name": {"type": "string", "minLength": 1}, "course": {"type": "number"}, "dose": {"type": "string", "minLength": 1}},
    "additionalProperties": false}
}`, typ, typ, closes)
}

// courseHarness widens the harness to the course types and hands the serve path the
// Definitions' series rules, as app.serveDeps does.
func courseHarness(t *testing.T) *harness {
	t.Helper()
	h := newHarness(t)
	var defs []def.Definition
	for _, m := range []string{measureManifest, readingManifest,
		courseEventManifest("test.course.started", false), courseEventManifest("test.course.changed", false), courseEventManifest("test.course.stopped", true)} {
		d, err := def.Parse([]byte(m))
		if err != nil {
			t.Fatalf("def.Parse: %v", err)
		}
		defs = append(defs, d)
	}
	reg, err := def.NewRegistry(defs...)
	if err != nil {
		t.Fatalf("def.NewRegistry: %v", err)
	}
	scopes := gate.StaticScopes{
		"meter": {"test.measure"}, "logger": {"test.reading"},
		"self-log": {"test.course.started", "test.course.changed", "test.course.stopped"},
	}
	h.gate = gate.New(h.l, reg, scopes, clock.Real{}, person, gate.Limits{MaxPayloadBytes: config.DefaultMaxPayloadBytes, MaxRetries: config.DefaultCommitRetries})
	h.deps.Series = reg
	return h
}

func (h *harness) event(t *testing.T, typ facet.Type, id, date, name string, course int) {
	t.Helper()
	env := facet.Envelope{
		Type: typ, Source: "self-log", Injector: "self-log", SourceRef: facet.SourceRef("self-log", id),
		Payload: []byte(fmt.Sprintf(`{"id": %q, "date": %q, "name": %q, "course": %d}`, id, date, name, course)),
	}
	binding, err := h.gate.Resolve(env)
	if err != nil {
		t.Fatalf("Resolve %s: %v", id, err)
	}
	if _, _, err := h.gate.Commit(context.Background(), env, binding); err != nil {
		t.Fatalf("Commit %s: %v", id, err)
	}
}

// The course projection serves the snapshot and a receipt proves it: re-deriving at
// the receipt's watermark, through the same template and the same series rules,
// reproduces the served bytes.
func TestRun_MedsServesTheSnapshotWithAReceipt(t *testing.T) {
	t.Parallel()
	h := courseHarness(t)
	ctx := context.Background()

	h.event(t, "test.course.started", "m1", "2026-03-01", "alpha", 1)
	h.event(t, "test.course.changed", "m2", "2026-05-10", "alpha", 1)
	h.event(t, "test.course.stopped", "m3", "2026-08-01", "alpha", 1)
	h.event(t, "test.course.started", "m4", "2026-10-01", "alpha", 2)
	h.event(t, "test.course.started", "m5", "2026-04-01", "beta", 1)

	out, err := serve.Run(ctx, h.deps, "course", false)
	if err != nil {
		t.Fatalf("Run(course): %v", err)
	}
	if len(out.Receipt.FacetIDs) != 2 {
		t.Errorf("course served %d facets, want 2 (the two open courses)", len(out.Receipt.FacetIDs))
	}
	if out.CanonicalLen != 5 {
		t.Errorf("canonical has %d, want 5", out.CanonicalLen)
	}

	_, res, err := serve.Verify(ctx, h.deps, out.ReceiptHash)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !res.OK() {
		t.Errorf("receipt did not verify:\n%s", res.Summary())
	}

	// Without the series rules the serve path refuses the snapshot shape outright.
	bare := h.deps
	bare.Series = nil
	if _, err := serve.Run(ctx, bare, "course", true); err == nil {
		t.Error("Run(course) without series rules succeeded")
	}
}

// The course template at a past fact-time serves the courses running then, a course
// stopped after it included, and a receipt for that read verifies (card 172). The
// template's own snapshot is taken at the bound, not at the head and trimmed after:
// trimmed, a course closed by now is gone at every date.
func TestDerive_MedsAtAPastDateKeepsACourseStoppedLater(t *testing.T) {
	t.Parallel()
	h := courseHarness(t)
	ctx := context.Background()

	h.event(t, "test.course.started", "m1", "2024-09-13", "gamma", 1)
	h.event(t, "test.course.stopped", "m2", "2025-12-23", "gamma", 1)
	h.event(t, "test.course.started", "m3", "2024-08-16", "alpha", 1)

	tmpl, err := h.deps.Templates.Lookup("course")
	if err != nil {
		t.Fatalf("Lookup(course): %v", err)
	}
	head, err := h.l.Head(ctx)
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	res, err := fold.ReadAt(ctx, h.l, head)
	if err != nil {
		t.Fatalf("ReadAt: %v", err)
	}

	cases := []struct {
		at   string
		want int
	}{
		{"2024-01-01T23:59:59.999999999Z", 0}, // before either course
		{"2025-06-15T23:59:59.999999999Z", 2}, // both running; gamma stops in December
		{"2026-10-17T23:59:59.999999999Z", 1}, // gamma stopped
	}
	for _, c := range cases {
		q := &receipt.Query{At: c.at}
		v, err := serve.Derive(res.Value, tmpl, consent.Open{}, h.deps.Series, c.at)
		if err != nil {
			t.Fatalf("Derive at %s: %v", c.at[:10], err)
		}
		if v, err = serve.Narrow(v, q, h.deps.Series); err != nil {
			t.Fatalf("Narrow at %s: %v", c.at[:10], err)
		}
		if v.Len() != c.want {
			t.Errorf("course at %s served %d courses, want %d", c.at[:10], v.Len(), c.want)
		}
		if c.want == 0 {
			continue // an empty value has no receipt worth proving
		}
		out, err := serve.Publish(ctx, h.deps, tmpl, head, q, v, false)
		if err != nil {
			t.Fatalf("Publish at %s: %v", c.at[:10], err)
		}
		if _, vr, err := serve.Verify(ctx, h.deps, out.ReceiptHash); err != nil || !vr.OK() {
			t.Errorf("the receipt at %s did not verify: %v\n%s", c.at[:10], err, vr.Summary())
		}
	}
}
