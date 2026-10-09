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

// The gate is the error kernel's logic; it is tested over the real ledger
// (AGENTS.md), one ledger file per test.

const person = "matt"

const bpManifest = `{
  "meta": {
    "facet_type": "health.blood-pressure",
    "definition_version": 1,
    "classification": "Normal",
    "validation_mode": "strict",
    "source_schema": "bp-csv/reading",
    "time_semantics": {
      "fact_time": {"pointer": "/measured_at", "layout": "2006-01-02T15:04:05Z07:00", "zone": "UTC"}
    },
    "identity_key": ["/measured_at"]
  },
  "schema": {"type": "object", "required": ["measured_at", "systolic", "diastolic"],
    "properties": {"measured_at": {"type": "string", "minLength": 1}, "systolic": {"type": "number"}, "diastolic": {"type": "number"}, "comment": {"type": "string", "minLength": 1}},
    "additionalProperties": false}
}`

// newGate wires a Gate over a fresh real ledger.
func newGate(t *testing.T) (*gate.Gate, *ledger.Ledger) {
	t.Helper()
	ctx := context.Background()

	l, err := ledger.Create(ctx, filepath.Join(t.TempDir(), "ledger.db"), person)
	if err != nil {
		t.Fatalf("ledger.Create: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })

	d, err := def.Parse([]byte(bpManifest))
	if err != nil {
		t.Fatalf("def.Parse: %v", err)
	}
	reg, err := def.NewRegistry(d)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	scopes := gate.StaticScopes{"bp-csv": {"health.blood-pressure"}}
	return gate.New(l, reg, scopes, clock.Real{}, person), l
}

func bpEnvelope(payload string) facet.Envelope {
	return facet.Envelope{
		Type: "health.blood-pressure", Source: "bp-csv", Injector: "bp-csv",
		SourceRef: facet.SourceRef("bp-csv", "2026-01-24T06:30:00Z"),
		Payload:   []byte(payload),
	}
}

const bpPayload = `{"measured_at": "2026-01-24T06:30:00Z", "systolic": 118, "diastolic": 76, "comment": "No filodipine"}`

func TestResolve_TheThreeAssuranceArms(t *testing.T) {
	t.Parallel()
	g, _ := newGate(t)

	cases := []struct {
		asserted string
		want     facet.BindingAssurance
	}{
		{person, facet.AssuranceHigh},
		{"", facet.AssuranceMedium},
		{"someone-else", facet.AssuranceReview},
	}
	for _, tc := range cases {
		env := bpEnvelope(bpPayload)
		env.AssertedPerson = tc.asserted
		b, err := g.Resolve(env)
		if err != nil {
			t.Fatalf("Resolve(%q): %v", tc.asserted, err)
		}
		if b.Assurance != tc.want || b.PersonID != person {
			t.Errorf("Resolve(%q) = %+v, want assurance %s for %s", tc.asserted, b, tc.want, person)
		}
	}
}

// The full §6a choreography over one source row: admit, no-op re-ingest, supersede on
// a corrected value, retract as a pure status transition.
func TestCommit_AdmitNoopSupersedeRetract(t *testing.T) {
	t.Parallel()
	g, l := newGate(t)
	ctx := context.Background()
	env := bpEnvelope(bpPayload)
	binding, err := g.Resolve(env)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	// First sight: admitted at v1, W moves to 1.
	res, w, err := g.Commit(ctx, env, binding)
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if res.Outcome != gate.OutcomeAdmitted || res.ArtifactVersion != 1 || w != 1 {
		t.Fatalf("first commit = %+v at W=%d, want admitted v1 W=1", res, w)
	}

	// Same row, same bytes: a no-op, and the watermark must not move.
	res2, w2, err := g.Commit(ctx, env, binding)
	if err != nil {
		t.Fatalf("re-Commit: %v", err)
	}
	if res2.Outcome != gate.OutcomeNoOp || w2 != 1 {
		t.Fatalf("re-ingest = %+v at W=%d, want no-op with W still 1", res2, w2)
	}

	// Same row, corrected value: supersedes at v2; the old artifact stays.
	corrected := bpEnvelope(strings.Replace(bpPayload, "118", "121", 1))
	res3, w3, err := g.Commit(ctx, corrected, binding)
	if err != nil {
		t.Fatalf("corrected Commit: %v", err)
	}
	if res3.Outcome != gate.OutcomeSuperseded || res3.ArtifactVersion != 2 || w3 != 2 {
		t.Fatalf("corrected commit = %+v at W=%d, want superseded v2 W=2", res3, w3)
	}
	if res3.FacetID != res.FacetID {
		t.Fatalf("supersession changed identity: %s vs %s", res3.FacetID, res.FacetID)
	}

	current, err := l.CurrentVersion(ctx, res.FacetID)
	if err != nil {
		t.Fatalf("CurrentVersion: %v", err)
	}
	if current.ArtifactVersion != 2 || !strings.Contains(string(current.Payload.Bytes()), "121") {
		t.Errorf("current = v%d %s", current.ArtifactVersion, current.Payload.Bytes())
	}

	// Retract: one more anam-event, no new artifact.
	res4, w4, err := g.Retract(ctx, res.FacetID)
	if err != nil {
		t.Fatalf("Retract: %v", err)
	}
	if res4.Outcome != gate.OutcomeTombstoned || w4 != 3 {
		t.Fatalf("retract = %+v at W=%d, want tombstoned W=3", res4, w4)
	}

	// Retracting again is a no-op — the fold shows the facet already tombstoned, so
	// no second tombstone is appended and the watermark stays put (the R3 gap, closed).
	res5, w5, err := g.Retract(ctx, res.FacetID)
	if err != nil {
		t.Fatalf("re-Retract: %v", err)
	}
	if res5.Outcome != gate.OutcomeNoOp || w5 != 3 {
		t.Fatalf("re-retract = %+v at W=%d, want no-op with W still 3", res5, w5)
	}

	// The stream tells the whole story in order.
	events, err := l.Replay(ctx, 0, 3)
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	kinds := make([]facet.AnamEventKind, len(events))
	for i, ev := range events {
		kinds[i] = ev.Kind
	}
	want := []facet.AnamEventKind{facet.EventAdmitted, facet.EventSuperseded, facet.EventTombstoned}
	for i := range want {
		if kinds[i] != want[i] {
			t.Fatalf("event stream = %v, want %v", kinds, want)
		}
	}
	if events[1].PriorArtifactVersion != 1 {
		t.Errorf("superseded event prior = %d, want 1", events[1].PriorArtifactVersion)
	}
}

// Identity is the Definition's. An envelope may leave source_ref out and the gate
// composes it; one that carries it is checked and lands on the same facet. Either
// way the ledger holds the composed ref, so the two paths are one facet.
func TestCommit_ComposesASourceRefLeftOut(t *testing.T) {
	t.Parallel()
	composed := facet.SourceRef("bp-csv", "2026-01-24T06:30:00Z")
	tests := []struct {
		name  string
		first string // the source_ref the first envelope carries
		then  string // and the second, with the same payload
	}{
		{"left out, then left out", "", ""},
		{"left out, then carried", "", composed},
		{"carried, then left out", composed, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			g, l := newGate(t)
			ctx := context.Background()

			first := bpEnvelope(bpPayload)
			first.SourceRef = tt.first
			binding, err := g.Resolve(first)
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			res, _, err := g.Commit(ctx, first, binding)
			if err != nil {
				t.Fatalf("Commit: %v", err)
			}
			if res.Outcome != gate.OutcomeAdmitted || res.FacetID != facet.NewID(composed) {
				t.Fatalf("first = %+v, want admitted as %s", res, facet.NewID(composed))
			}
			f, err := l.CurrentVersion(ctx, res.FacetID)
			if err != nil {
				t.Fatalf("CurrentVersion: %v", err)
			}
			if f.Provenance.SourceRef != composed {
				t.Errorf("ledger holds source_ref %q, want %q", f.Provenance.SourceRef, composed)
			}

			then := bpEnvelope(bpPayload)
			then.SourceRef = tt.then
			res2, w, err := g.Commit(ctx, then, binding)
			if err != nil {
				t.Fatalf("second Commit: %v", err)
			}
			if res2.Outcome != gate.OutcomeNoOp || res2.FacetID != res.FacetID || w != 1 {
				t.Errorf("second = %+v at W=%d, want a no-op on the same facet at W=1", res2, w)
			}
		})
	}
}

func TestCommit_TypedRejections(t *testing.T) {
	t.Parallel()
	g, _ := newGate(t)
	ctx := context.Background()
	binding := facet.Binding{PersonID: person, Assurance: facet.AssuranceHigh}

	t.Run("unscoped source", func(t *testing.T) {
		t.Parallel()
		env := bpEnvelope(bpPayload)
		env.Source = "rogue"
		if _, _, err := g.Commit(ctx, env, binding); !errors.Is(err, gate.ErrNotAuthorized) {
			t.Errorf("got %v, want ErrNotAuthorized", err)
		}
	})
	t.Run("no definition", func(t *testing.T) {
		t.Parallel()
		// Scoped for the type, but no contract loaded for it: the definition check is
		// what fires. The rejection precedes any ledger call.
		reg, err := def.NewRegistry()
		if err != nil {
			t.Fatal(err)
		}
		g2 := gate.New(nil, reg, gate.StaticScopes{"bp-csv": {"health.sleep"}}, clock.Real{}, person)
		env := bpEnvelope(bpPayload)
		env.Type = "health.sleep"
		if _, _, err := g2.Commit(ctx, env, binding); !errors.Is(err, gate.ErrNoDefinition) {
			t.Errorf("got %v, want ErrNoDefinition", err)
		}
	})
	t.Run("non-compliant payload", func(t *testing.T) {
		t.Parallel()
		env := bpEnvelope(`{"measured_at": "2026-01-24T06:30:00Z", "systolic": 118}`)
		if _, _, err := g.Commit(ctx, env, binding); !errors.Is(err, def.ErrNotCompliant) {
			t.Errorf("got %v, want ErrNotCompliant (missing diastolic)", err)
		}
	})
	t.Run("identity mismatch", func(t *testing.T) {
		t.Parallel()
		env := bpEnvelope(bpPayload)
		env.SourceRef = "bp-csv:some-other-key"
		if _, _, err := g.Commit(ctx, env, binding); !errors.Is(err, gate.ErrIdentityMismatch) {
			t.Errorf("got %v, want ErrIdentityMismatch", err)
		}
	})
	t.Run("malformed payload", func(t *testing.T) {
		t.Parallel()
		env := bpEnvelope(`{"measured_at": `)
		if _, _, err := g.Commit(ctx, env, binding); err == nil {
			t.Error("malformed payload committed")
		}
	})
	t.Run("assurance below floor", func(t *testing.T) {
		t.Parallel()
		low := facet.Binding{PersonID: person, Assurance: facet.AssuranceReview}
		if _, _, err := g.Commit(ctx, bpEnvelope(bpPayload), low); !errors.Is(err, gate.ErrAssuranceBelowFloor) {
			t.Errorf("got %v, want ErrAssuranceBelowFloor (review < medium default)", err)
		}
	})
	t.Run("retract unknown facet", func(t *testing.T) {
		t.Parallel()
		if _, _, err := g.Retract(ctx, "0000-not-a-facet"); !errors.Is(err, ledger.ErrNotFound) {
			t.Errorf("got %v, want ErrNotFound", err)
		}
	})
}

// A worker that crashed after its transaction committed re-runs the same envelope:
// the gate must report a no-op, never a duplicate — observable crash-cleanness.
func TestCommit_CrashRerunIsANoOp(t *testing.T) {
	t.Parallel()
	g, l := newGate(t)
	ctx := context.Background()
	env := bpEnvelope(bpPayload)
	binding, _ := g.Resolve(env)

	if _, _, err := g.Commit(ctx, env, binding); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	res, _, err := g.Commit(ctx, env, binding)
	if err != nil {
		t.Fatalf("re-run: %v", err)
	}
	if res.Outcome != gate.OutcomeNoOp {
		t.Fatalf("re-run = %+v, want no-op", res)
	}

	head, err := l.Head(ctx)
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	if head != 1 {
		t.Errorf("W = %d after re-run, want 1 (no phantom write)", head)
	}
}

// Two gates over one ledger, interleaved: the loser of the watermark race must retry
// internally and land its write, never surface a conflict.
func TestCommit_ConcurrentWritersBothLand(t *testing.T) {
	t.Parallel()
	g, l := newGate(t)
	ctx := context.Background()

	payloads := []string{
		`{"measured_at": "2026-01-25T06:30:00Z", "systolic": 120, "diastolic": 78}`,
		`{"measured_at": "2026-01-26T06:30:00Z", "systolic": 117, "diastolic": 75}`,
	}
	refs := []string{facet.SourceRef("bp-csv", "2026-01-25T06:30:00Z"), facet.SourceRef("bp-csv", "2026-01-26T06:30:00Z")}

	errc := make(chan error, len(payloads))
	for i := range payloads {
		go func(payload, ref string) {
			env := bpEnvelope(payload)
			env.SourceRef = ref
			binding, err := g.Resolve(env)
			if err != nil {
				errc <- err
				return
			}
			_, _, err = g.Commit(ctx, env, binding)
			errc <- err
		}(payloads[i], refs[i])
	}
	for range payloads {
		if err := <-errc; err != nil {
			t.Fatalf("concurrent commit: %v", err)
		}
	}

	head, err := l.Head(ctx)
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	if head != 2 {
		t.Errorf("W = %d after two concurrent commits, want 2", head)
	}
}
