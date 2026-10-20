package serve_test

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/consent"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/gate"
	"github.com/mattjoyce/anam/internal/projection"
	"github.com/mattjoyce/anam/internal/serve"
	"github.com/mattjoyce/anam/internal/store/artifact"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

// courseManifest is a medication event Definition shaped like the bundled ones: a
// course keyed by drug and course, closed by a stop. Old and new type names share it,
// so a rename moves the type and nothing else (card 140).
func courseManifest(typ string, closes bool) string {
	return fmt.Sprintf(`{
  "meta": {
    "facet_type": %q,
    "definition_version": 1,
    "classification": "Sensitive",
    "validation_mode": "strict",
    "source_schema": "records/%s",
    "time_semantics": {"fact_time": {"pointer": "/date", "layout": "2006-01-02", "zone": "UTC"}},
    "identity_key": ["/id"],
    "series": {"name": "med.regimen", "key": ["/drug", "/course"], "closes": %t}
  },
  "schema": {"type": "object", "required": ["id", "date", "drug", "course"],
    "properties": {"id": {"type": "string", "minLength": 1}, "date": {"type": "string", "minLength": 1},
      "drug": {"type": "string", "minLength": 1}, "course": {"type": "number"}},
    "additionalProperties": false}
}`, typ, typ, closes)
}

// Card 187: a receipt served under meds v1 names the retired types (med.started,
// med.stopped). After card 147 the Definitions in force no longer name them, so the
// replay did not know that med.stopped closes a course, and the stopped course read
// as open: one more facet than was served. The receipt must still verify.
func TestVerify_ReceiptServedUnderRetiredTypesStillReproduces(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	l, err := ledger.Create(ctx, filepath.Join(t.TempDir(), "ledger.db"), person)
	if err != nil {
		t.Fatalf("ledger.Create: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })
	store, err := artifact.New(t.TempDir())
	if err != nil {
		t.Fatalf("artifact.New: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	var all []def.Definition
	for _, m := range []string{
		courseManifest("med.started", false), courseManifest("med.stopped", true),
		courseManifest("health.medication-started", false), courseManifest("health.medication-stopped", true),
	} {
		d, err := def.Parse([]byte(m))
		if err != nil {
			t.Fatalf("def.Parse: %v", err)
		}
		all = append(all, d)
	}
	before, err := def.NewRegistry(all...) // the Definitions in force when the receipt was served, plus the new names for the rename
	if err != nil {
		t.Fatalf("def.NewRegistry: %v", err)
	}
	scopes := gate.StaticScopes{"records": {"med.started", "med.stopped", "health.medication-started", "health.medication-stopped"}}
	g := gate.New(l, before, scopes, clock.Real{}, person,
		gate.Limits{MaxPayloadBytes: config.DefaultMaxPayloadBytes, MaxRetries: config.DefaultCommitRetries})

	var ids []string
	admit := func(typ facet.Type, id, date, drug string) {
		t.Helper()
		env := facet.Envelope{
			Type: typ, Source: "records", Injector: "records",
			SourceRef: facet.SourceRef("records", id),
			Payload:   []byte(fmt.Sprintf(`{"id": %q, "date": %q, "drug": %q, "course": 1}`, id, date, drug)),
		}
		b, err := g.Resolve(env)
		if err != nil {
			t.Fatalf("Resolve %s: %v", id, err)
		}
		res, _, err := g.Commit(ctx, env, b)
		if err != nil {
			t.Fatalf("Commit %s: %v", id, err)
		}
		ids = append(ids, res.FacetID)
	}
	// felodipine ran and stopped; ramipril is still on.
	admit("med.started", "records:med/felodipine/1/started", "2025-06-01", "felodipine")
	admit("med.stopped", "records:med/felodipine/1/stopped", "2025-12-22", "felodipine")
	admit("med.started", "records:med/ramipril/1/started", "2025-07-01", "ramipril")

	templates, err := projection.Bundled()
	if err != nil {
		t.Fatalf("projection.Bundled: %v", err)
	}
	v1, err := templates.Version("meds", 1)
	if err != nil {
		t.Fatalf("meds v1: %v", err)
	}
	served, err := projection.NewRegistry(v1)
	if err != nil {
		t.Fatalf("projection.NewRegistry: %v", err)
	}
	fixed := clock.Fixed{Instant: time.Date(2026, 10, 10, 9, 0, 0, 0, time.UTC)}
	deps := serve.Deps{
		Ledger: l, Templates: served, Artifacts: store, Gate: consent.Open{}, Clock: fixed,
		DefinitionSetVersion: "sha256:" + strings.Repeat("d", 64), Series: before, Definitions: before,
	}
	out, err := serve.Run(ctx, deps, "meds", false)
	if err != nil {
		t.Fatalf("Run meds v1: %v", err)
	}
	if out.Receipt.TemplateVersion != 1 || len(out.Receipt.FacetIDs) != 1 {
		t.Fatalf("served v%d with %d facets, want v1 with ramipril alone", out.Receipt.TemplateVersion, len(out.Receipt.FacetIDs))
	}

	// The rename: every live facet becomes the next version of itself under the new
	// name, after the receipt's watermark.
	to := map[facet.Type]facet.Type{
		"med.started": "health.medication-started", "med.stopped": "health.medication-stopped",
	}
	var renames []gate.Rename
	for _, id := range ids {
		f, err := l.CurrentVersion(ctx, id)
		if err != nil {
			t.Fatalf("CurrentVersion %s: %v", id, err)
		}
		renames = append(renames, gate.Rename{
			Env:     facet.Envelope{Type: to[f.Type], Source: f.Provenance.Source, Injector: "rename", Payload: f.Payload.Bytes()},
			Binding: f.Binding,
			From:    f.Type,
		})
	}
	if _, _, err := g.RenameBatch(ctx, renames); err != nil {
		t.Fatalf("RenameBatch: %v", err)
	}

	// Meds v2 is current, v1 is history, and the Definitions in force name only the
	// new types.
	now, err := def.NewRegistry(all[2], all[3])
	if err != nil {
		t.Fatalf("def.NewRegistry: %v", err)
	}
	deps.Templates, deps.Series, deps.Definitions = templates, now, now

	_, res, err := serve.Verify(ctx, deps, out.ReceiptHash)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !res.OK() {
		t.Fatalf("a receipt served under the retired type names did not verify:\n%s", res.Summary())
	}
	if !slices.Contains(res.Notes, "replayed meds v1 from the held history; v2 is current") {
		t.Errorf("the verdict does not say which version was replayed: %q", res.Notes)
	}
}
