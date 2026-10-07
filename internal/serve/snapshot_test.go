package serve_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/gate"
	"github.com/mattjoyce/anam/internal/serve"
)

func medManifest(typ string, closes bool) string {
	return fmt.Sprintf(`{
  "meta": {
    "facet_type": %q, "definition_version": 1, "classification": "Sensitive",
    "validation_mode": "lenient", "source_schema": "records/%s",
    "time_semantics": {"fact_time": {"pointer": "/date", "layout": "2006-01-02", "zone": "UTC"}},
    "identity_key": ["/id"],
    "series": {"name": "med.regimen", "key": ["/drug", "/course"], "closes": %t}
  },
  "schema": {"required": ["/id", "/date", "/drug"]}
}`, typ, typ, closes)
}

// medHarness widens the harness to the regimen types and hands the serve path the
// Definitions' series rules, as app.serveDeps does.
func medHarness(t *testing.T) *harness {
	t.Helper()
	h := newHarness(t)
	var defs []def.Definition
	for _, m := range []string{bpManifest, sleepManifest,
		medManifest("med.started", false), medManifest("med.dose-changed", false), medManifest("med.stopped", true)} {
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
		"bp-csv": {"health.blood-pressure"}, "health-db": {"health.sleep"},
		"self-log": {"med.started", "med.dose-changed", "med.stopped"},
	}
	h.gate = gate.New(h.l, reg, scopes, clock.Real{}, person)
	h.deps.Series = reg
	return h
}

func (h *harness) med(t *testing.T, typ facet.Type, id, date, drug string, course int) {
	t.Helper()
	env := facet.Envelope{
		Type: typ, Source: "self-log", Injector: "self-log", SourceRef: facet.SourceRef("self-log", id),
		Payload: []byte(fmt.Sprintf(`{"id": %q, "date": %q, "drug": %q, "course": %d}`, id, date, drug, course)),
	}
	binding, err := h.gate.Resolve(env)
	if err != nil {
		t.Fatalf("Resolve %s: %v", id, err)
	}
	if _, _, err := h.gate.Commit(context.Background(), env, binding); err != nil {
		t.Fatalf("Commit %s: %v", id, err)
	}
}

// The meds projection serves the snapshot and a receipt proves it: re-deriving at
// the receipt's watermark, through the same template and the same series rules,
// reproduces the served bytes.
func TestRun_MedsServesTheSnapshotWithAReceipt(t *testing.T) {
	t.Parallel()
	h := medHarness(t)
	ctx := context.Background()

	h.med(t, "med.started", "m1", "2026-03-01", "ramipril", 1)
	h.med(t, "med.dose-changed", "m2", "2026-05-10", "ramipril", 1)
	h.med(t, "med.stopped", "m3", "2026-08-01", "ramipril", 1)
	h.med(t, "med.started", "m4", "2026-10-01", "ramipril", 2)
	h.med(t, "med.started", "m5", "2026-04-01", "atorvastatin", 1)

	out, err := serve.Run(ctx, h.deps, "meds", false)
	if err != nil {
		t.Fatalf("Run(meds): %v", err)
	}
	if len(out.Receipt.FacetIDs) != 2 {
		t.Errorf("meds served %d facets, want 2 (the two open courses)", len(out.Receipt.FacetIDs))
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
	if _, err := serve.Run(ctx, bare, "meds", true); err == nil {
		t.Error("Run(meds) without series rules succeeded")
	}
}
