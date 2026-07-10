package gate_test

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/gate"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

// The gate is the error kernel's logic; it is tested over the real ledger
// (AGENTS.md), one unique table per test, skipping cleanly when the endpoint is away.

const (
	endpoint = "http://192.168.20.4:8000"
	region   = "us-east-1"
	person   = "matt"
)

var reachable bool

func TestMain(m *testing.M) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if resp, err := http.DefaultClient.Do(req); err == nil {
		_ = resp.Body.Close()
		reachable = true
	}
	os.Exit(m.Run())
}

const bpManifest = `{
  "meta": {
    "facet_type": "health.blood-pressure",
    "definition_version": 1,
    "classification": "Normal",
    "validation_mode": "lenient",
    "source_schema": "bp-csv/reading",
    "time_semantics": {
      "event_time": {"pointer": "/measured_at", "layout": "2006-01-02T15:04:05Z07:00"}
    },
    "identity_key": ["/measured_at"]
  },
  "schema": {"required": ["/systolic", "/diastolic"]}
}`

// newGate wires a Gate over a fresh real ledger table.
func newGate(t *testing.T) (*gate.Gate, *ledger.Ledger) {
	t.Helper()
	if !reachable {
		t.Skipf("DynamoDB Local unreachable at %s", endpoint)
	}
	ctx := context.Background()

	l, err := ledger.New(ctx, endpoint, region, "anam_test_"+uuid.NewString(), person)
	if err != nil {
		t.Fatalf("ledger.New: %v", err)
	}
	if err := l.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}
	t.Cleanup(func() { _ = l.Drop(context.Background()) })

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
		SourceRef: "bp-csv:2026-01-24T06:30:00Z",
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
	res, err := g.Commit(ctx, env, binding)
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if res.Outcome != gate.OutcomeAdmitted || res.ArtifactVersion != 1 || res.W != 1 {
		t.Fatalf("first commit = %+v, want admitted v1 W=1", res)
	}

	// Same row, same bytes: a no-op, and the watermark must not move.
	res2, err := g.Commit(ctx, env, binding)
	if err != nil {
		t.Fatalf("re-Commit: %v", err)
	}
	if res2.Outcome != gate.OutcomeNoOp || res2.W != 1 {
		t.Fatalf("re-ingest = %+v, want no-op with W still 1", res2)
	}

	// Same row, corrected value: supersedes at v2; the old artifact stays.
	corrected := bpEnvelope(strings.Replace(bpPayload, "118", "121", 1))
	res3, err := g.Commit(ctx, corrected, binding)
	if err != nil {
		t.Fatalf("corrected Commit: %v", err)
	}
	if res3.Outcome != gate.OutcomeSuperseded || res3.ArtifactVersion != 2 || res3.W != 2 {
		t.Fatalf("corrected commit = %+v, want superseded v2 W=2", res3)
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

	// Retract: one more record-event, no new artifact.
	res4, err := g.Retract(ctx, res.FacetID)
	if err != nil {
		t.Fatalf("Retract: %v", err)
	}
	if res4.Outcome != gate.OutcomeTombstoned || res4.W != 3 {
		t.Fatalf("retract = %+v, want tombstoned W=3", res4)
	}

	// The stream tells the whole story in order.
	events, err := l.Replay(ctx, 0, 3)
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	kinds := make([]facet.RecordEventKind, len(events))
	for i, ev := range events {
		kinds[i] = ev.Kind
	}
	want := []facet.RecordEventKind{facet.EventAdmitted, facet.EventSuperseded, facet.EventTombstoned}
	for i := range want {
		if kinds[i] != want[i] {
			t.Fatalf("event stream = %v, want %v", kinds, want)
		}
	}
	if events[1].PriorArtifactVersion != 1 {
		t.Errorf("superseded event prior = %d, want 1", events[1].PriorArtifactVersion)
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
		if _, err := g.Commit(ctx, env, binding); !errors.Is(err, gate.ErrNotAuthorized) {
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
		if _, err := g2.Commit(ctx, env, binding); !errors.Is(err, gate.ErrNoDefinition) {
			t.Errorf("got %v, want ErrNoDefinition", err)
		}
	})
	t.Run("non-compliant payload", func(t *testing.T) {
		t.Parallel()
		env := bpEnvelope(`{"measured_at": "2026-01-24T06:30:00Z", "systolic": 118}`)
		if _, err := g.Commit(ctx, env, binding); !errors.Is(err, def.ErrNotCompliant) {
			t.Errorf("got %v, want ErrNotCompliant (missing diastolic)", err)
		}
	})
	t.Run("identity mismatch", func(t *testing.T) {
		t.Parallel()
		env := bpEnvelope(bpPayload)
		env.SourceRef = "bp-csv:some-other-key"
		if _, err := g.Commit(ctx, env, binding); !errors.Is(err, gate.ErrIdentityMismatch) {
			t.Errorf("got %v, want ErrIdentityMismatch", err)
		}
	})
	t.Run("malformed payload", func(t *testing.T) {
		t.Parallel()
		env := bpEnvelope(`{"measured_at": `)
		if _, err := g.Commit(ctx, env, binding); err == nil {
			t.Error("malformed payload committed")
		}
	})
	t.Run("assurance below floor", func(t *testing.T) {
		t.Parallel()
		low := facet.Binding{PersonID: person, Assurance: facet.AssuranceReview}
		if _, err := g.Commit(ctx, bpEnvelope(bpPayload), low); !errors.Is(err, gate.ErrAssuranceBelowFloor) {
			t.Errorf("got %v, want ErrAssuranceBelowFloor (review < medium default)", err)
		}
	})
	t.Run("retract unknown facet", func(t *testing.T) {
		t.Parallel()
		if _, err := g.Retract(ctx, "0000-not-a-facet"); !errors.Is(err, ledger.ErrNotFound) {
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

	if _, err := g.Commit(ctx, env, binding); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	res, err := g.Commit(ctx, env, binding)
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
	refs := []string{"bp-csv:2026-01-25T06:30:00Z", "bp-csv:2026-01-26T06:30:00Z"}

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
			_, err = g.Commit(ctx, env, binding)
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
