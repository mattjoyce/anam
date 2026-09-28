package serve_test

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/consent"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/gate"
	"github.com/mattjoyce/anam/internal/projection"
	"github.com/mattjoyce/anam/internal/serve"
	"github.com/mattjoyce/anam/internal/store/artifact"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

// Serve is tested end to end over the real ledger and the real artifact tier
// (AGENTS.md: real components over mocks): a ledger file and an artifact directory
// per test, both under the test's temp directory.

const person = "matt"

// Two definitions, so a projection has something to prune away. The bundled bp and
// sleep templates are the ones under test, so the types must be theirs.
const bpManifest = `{
  "meta": {
    "facet_type": "health.blood-pressure",
    "definition_version": 1,
    "classification": "Normal",
    "validation_mode": "lenient",
    "source_schema": "bp-csv/reading",
    "time_semantics": {
      "fact_time": {"pointer": "/measured_at", "layout": "2006-01-02T15:04:05Z07:00"}
    },
    "identity_key": ["/measured_at"]
  },
  "schema": {"required": ["/systolic"]}
}`

const sleepManifest = `{
  "meta": {
    "facet_type": "health.sleep",
    "definition_version": 1,
    "classification": "Normal",
    "validation_mode": "lenient",
    "source_schema": "health-db/sleep",
    "time_semantics": {
      "fact_time": {"pointer": "/measured_at", "layout": "2006-01-02T15:04:05Z07:00"}
    },
    "identity_key": ["/measured_at"]
  },
  "schema": {"required": ["/deep_minutes"]}
}`

type harness struct {
	gate  *gate.Gate
	l     *ledger.Ledger
	deps  serve.Deps
	dir   string
	store *artifact.Store
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	ctx := context.Background()

	l, err := ledger.Create(ctx, filepath.Join(t.TempDir(), "ledger.db"), person)
	if err != nil {
		t.Fatalf("ledger.Create: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })

	bp, err := def.Parse([]byte(bpManifest))
	if err != nil {
		t.Fatalf("def.Parse(bp): %v", err)
	}
	sleep, err := def.Parse([]byte(sleepManifest))
	if err != nil {
		t.Fatalf("def.Parse(sleep): %v", err)
	}
	defs, err := def.NewRegistry(bp, sleep)
	if err != nil {
		t.Fatalf("def.NewRegistry: %v", err)
	}
	scopes := gate.StaticScopes{
		"bp-csv":    {"health.blood-pressure"},
		"health-db": {"health.sleep"},
	}

	templates, err := projection.Bundled()
	if err != nil {
		t.Fatalf("projection.Bundled: %v", err)
	}
	dir := t.TempDir()
	store, err := artifact.New(dir)
	if err != nil {
		t.Fatalf("artifact.New: %v", err)
	}

	return &harness{
		gate:  gate.New(l, defs, scopes, clock.Real{}, person),
		l:     l,
		dir:   dir,
		store: store,
		deps: serve.Deps{
			Ledger:               l,
			Templates:            templates,
			Artifacts:            store,
			Gate:                 consent.Open{},
			Clock:                clock.Fixed{Instant: time.Date(2026, 7, 25, 9, 0, 0, 0, time.UTC)},
			DefinitionSetVersion: "sha256:" + strings.Repeat("d", 64),
		},
	}
}

// admit puts one row through the real gate, exactly as a capture worker would.
func (h *harness) admit(t *testing.T, source string, typ facet.Type, measuredAt, body string) {
	t.Helper()
	env := facet.Envelope{
		Type: typ, Source: source, Injector: source,
		SourceRef: source + ":" + measuredAt,
		Payload:   []byte(fmt.Sprintf(`{"measured_at": %q, %s}`, measuredAt, body)),
	}
	binding, err := h.gate.Resolve(env)
	if err != nil {
		t.Fatalf("Resolve %s: %v", measuredAt, err)
	}
	if _, _, err := h.gate.Commit(context.Background(), env, binding); err != nil {
		t.Fatalf("Commit %s: %v", measuredAt, err)
	}
}

// seed admits three blood-pressure readings and two sleep periods, so `bp` has
// something to serve and something to leave behind.
func (h *harness) seed(t *testing.T) {
	t.Helper()
	h.admit(t, "bp-csv", "health.blood-pressure", "2026-01-24T06:30:00Z", `"systolic": 118, "diastolic": 76`)
	h.admit(t, "bp-csv", "health.blood-pressure", "2026-01-25T06:30:00Z", `"systolic": 121, "diastolic": 79`)
	h.admit(t, "bp-csv", "health.blood-pressure", "2026-01-26T06:30:00Z", `"systolic": 117, "diastolic": 75`)
	h.admit(t, "health-db", "health.sleep", "2026-01-24T22:00:00Z", `"deep_minutes": 62`)
	h.admit(t, "health-db", "health.sleep", "2026-01-25T22:00:00Z", `"deep_minutes": 71`)
}
