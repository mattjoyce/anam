package gate_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/gate"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

// Card 105: a facet's id is the source plus its identity values, without the type. Two
// types from one source whose identity values coincide would otherwise collide, and the
// second would be written over the first as a new version of another kind.

const pulseManifest = `{
  "meta": {
    "facet_type": "health.pulse",
    "definition_version": 1,
    "classification": "Normal",
    "validation_mode": "strict",
    "source_schema": "bp-csv/pulse",
    "time_semantics": {
      "fact_time": {"pointer": "/measured_at", "layout": "2006-01-02T15:04:05Z07:00", "zone": "UTC"}
    },
    "identity_key": ["/measured_at"]
  },
  "schema": {"type": "object", "required": ["measured_at", "bpm"],
    "properties": {"measured_at": {"type": "string", "minLength": 1}, "bpm": {"type": "number"}},
    "additionalProperties": false}
}`

func TestCommit_RefusesAFacetChangingType(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	l, err := ledger.Create(ctx, filepath.Join(t.TempDir(), "ledger.db"), person)
	if err != nil {
		t.Fatalf("ledger.Create: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })
	bp, err := def.Parse([]byte(bpManifest))
	if err != nil {
		t.Fatal(err)
	}
	pulse, err := def.Parse([]byte(pulseManifest))
	if err != nil {
		t.Fatal(err)
	}
	reg, err := def.NewRegistry(bp, pulse)
	if err != nil {
		t.Fatal(err)
	}
	g := gate.New(l, reg, gate.StaticScopes{"bp-csv": {"health.blood-pressure", "health.pulse"}}, clock.Real{}, person)

	first := bpEnvelope(bpPayload)
	b, _ := g.Resolve(first)
	if _, _, err := g.Commit(ctx, first, b); err != nil {
		t.Fatalf("admit the reading: %v", err)
	}
	head, _ := l.Head(ctx)

	// The same source and the same identity value, another type: the same facet id.
	second := facet.Envelope{
		Type: "health.pulse", Source: "bp-csv", Injector: "bp-csv",
		Payload: []byte(`{"measured_at": "2026-01-24T06:30:00Z", "bpm": 61}`),
	}
	b, _ = g.Resolve(second)
	_, _, err = g.Commit(ctx, second, b)
	var refused *gate.Refused
	if !errors.As(err, &refused) || !errors.Is(err, gate.ErrTypeChanged) {
		t.Fatalf("a facet changing type: %v, want a refusal with ErrTypeChanged", err)
	}
	for _, want := range []string{"health.blood-pressure", "health.pulse", facet.SourceRef("bp-csv", "2026-01-24T06:30:00Z")} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal does not name %q: %v", want, err)
		}
	}
	if after, _ := l.Head(ctx); after != head {
		t.Errorf("the refusal wrote: W %d → %d", head, after)
	}
	cur, err := l.CurrentVersion(ctx, facet.NewID(first.SourceRef))
	if err == nil && cur.Type != "health.blood-pressure" {
		t.Errorf("the reading is now a %s", cur.Type)
	}

	// In a batch, the refusal names the envelope and the rest is not written either.
	ok := bpEnvelope(`{"measured_at": "2026-01-25T06:30:00Z", "systolic": 120, "diastolic": 80}`)
	ok.SourceRef = facet.SourceRef("bp-csv", "2026-01-25T06:30:00Z")
	envs := []facet.Envelope{ok, second}
	bindings := make([]facet.Binding, len(envs))
	for i, env := range envs {
		bindings[i], _ = g.Resolve(env)
	}
	_, _, err = g.CommitBatch(ctx, envs, bindings)
	if !errors.As(err, &refused) || len(refused.Envelopes) != 1 || refused.Envelopes[0].Index != 1 {
		t.Fatalf("batch with a type change: %v", err)
	}
	if after, _ := l.Head(ctx); after != head {
		t.Errorf("the refused batch wrote: W %d → %d", head, after)
	}
}
