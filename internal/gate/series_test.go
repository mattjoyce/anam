package gate_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/gate"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

const medStartedManifest = `{
  "meta": {
    "facet_type": "med.started",
    "definition_version": 1,
    "classification": "Sensitive",
    "validation_mode": "lenient",
    "source_schema": "records/med.started",
    "time_semantics": {"fact_time": {"pointer": "/date", "layout": "2006-01-02", "zone": "UTC"}},
    "identity_key": ["/id"],
    "series": {"name": "med.regimen", "key": ["/drug", "/course"]}
  },
  "schema": {"required": ["/id", "/date", "/drug"]}
}`

func newMedGate(t *testing.T) (*gate.Gate, *ledger.Ledger) {
	t.Helper()
	ctx := context.Background()
	l, err := ledger.Create(ctx, filepath.Join(t.TempDir(), "ledger.db"), person)
	if err != nil {
		t.Fatalf("ledger.Create: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })
	d, err := def.Parse([]byte(medStartedManifest))
	if err != nil {
		t.Fatalf("def.Parse: %v", err)
	}
	reg, err := def.NewRegistry(d)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	return gate.New(l, reg, gate.StaticScopes{"self-log": {"med.started"}}, clock.Real{}, person), l
}

func medEnvelope(payload string) facet.Envelope {
	return facet.Envelope{
		Type: "med.started", Source: "self-log", Injector: "self-log",
		SourceRef: facet.SourceRef("self-log", "m1"), Payload: []byte(payload),
	}
}

// The gate stamps the series at admission, like classification: the ledger row
// carries it, and no reader has to open the payload to group on it.
func TestCommit_StampsTheSeries(t *testing.T) {
	t.Parallel()
	g, l := newMedGate(t)
	ctx := context.Background()

	env := medEnvelope(`{"id": "m1", "date": "2026-03-01", "drug": "ramipril", "course": 1, "dose": "5mg"}`)
	binding, err := g.Resolve(env)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	res, _, err := g.Commit(ctx, env, binding)
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	f, err := l.CurrentVersion(ctx, res.FacetID)
	if err != nil {
		t.Fatalf("CurrentVersion: %v", err)
	}
	if want := facet.Stamp("med.regimen", "ramipril", "1"); f.Series != want {
		t.Errorf("stamped series = %q, want %q", f.Series, want)
	}
}

// A row that cannot say which series it belongs to fails the facet-compliant pillar,
// and nothing is written.
func TestCommit_RefusesARowWithoutItsSeriesKey(t *testing.T) {
	t.Parallel()
	g, l := newMedGate(t)
	ctx := context.Background()

	env := medEnvelope(`{"id": "m1", "date": "2026-03-01", "drug": "ramipril"}`)
	binding, err := g.Resolve(env)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if _, _, err := g.Commit(ctx, env, binding); !errors.Is(err, def.ErrNotCompliant) {
		t.Fatalf("Commit without course = %v, want ErrNotCompliant", err)
	}
	if head, _ := l.Head(ctx); head != 0 {
		t.Errorf("head = %d after a refusal, want 0", head)
	}
}
