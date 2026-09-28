package fold_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/fold"
	"github.com/mattjoyce/anam/internal/gate"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

// The fold's integration tests run over the real ledger and the real gate
// (AGENTS.md): one ledger file per test.

const person = "matt"

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
  "schema": {"required": ["/systolic", "/diastolic"]}
}`

func newHarness(t *testing.T) (*gate.Gate, *ledger.Ledger) {
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

// source stands in for the system of record: measured_at → the row's reading.
// Ingesting it captures every row it currently holds, exactly as a capture worker
// re-scanning the real CSV would.
type source map[string]string

func (s source) ingest(t *testing.T, g *gate.Gate) map[string]gate.Result {
	t.Helper()
	results := make(map[string]gate.Result, len(s))
	for measuredAt, reading := range s {
		env := facet.Envelope{
			Type: "health.blood-pressure", Source: "bp-csv", Injector: "bp-csv",
			SourceRef: "bp-csv:" + measuredAt,
			Payload:   []byte(fmt.Sprintf(`{"measured_at": %q, %s}`, measuredAt, reading)),
		}
		binding, err := g.Resolve(env)
		if err != nil {
			t.Fatalf("Resolve %s: %v", measuredAt, err)
		}
		res, _, err := g.Commit(context.Background(), env, binding)
		if err != nil {
			t.Fatalf("Commit %s: %v", measuredAt, err)
		}
		results[measuredAt] = res
	}
	return results
}

// The executable specification of "Anam owns no fact" (MVP.md §7): live a history —
// admissions, a correction, a retraction — then drop the ledger, init, re-ingest what
// the source now holds, and the canonical fold is byte-identical. The value's hash is
// the equality oracle; the watermark is allowed to differ, because the ledger's
// bookkeeping is Anam-authored and the value deliberately excludes it.
func TestRebuildFromSource_IdenticalCanonical(t *testing.T) {
	t.Parallel()
	g, l := newHarness(t)
	ctx := context.Background()

	src := source{
		"2026-01-24T06:30:00Z": `"systolic": 118, "diastolic": 76`,
		"2026-01-25T06:30:00Z": `"systolic": 121, "diastolic": 79`,
		"2026-01-26T06:30:00Z": `"systolic": 117, "diastolic": 75`,
	}
	results := src.ingest(t, g)

	// Re-ingesting an unchanged source is a no-op: same watermark, same value.
	before, err := fold.Read(ctx, l)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	src.ingest(t, g)
	after, err := fold.Read(ctx, l)
	if err != nil {
		t.Fatalf("Read after re-ingest: %v", err)
	}
	if after.W != before.W || after.Value.Hash() != before.Value.Hash() {
		t.Fatalf("re-ingest moved the canonical: W %d→%d, hash %s→%s",
			before.W, after.W, before.Value.Hash(), after.Value.Hash())
	}

	// The source moves on: one row corrected, one row removed (and retracted, as the
	// capture path would on seeing the deletion). Each change moves the hash.
	src["2026-01-25T06:30:00Z"] = `"systolic": 124, "diastolic": 80`
	src.ingest(t, g)
	corrected, err := fold.Read(ctx, l)
	if err != nil {
		t.Fatalf("Read after correction: %v", err)
	}
	if corrected.Value.Hash() == before.Value.Hash() {
		t.Fatal("a corrected reading did not move the canonical hash")
	}

	removed := results["2026-01-26T06:30:00Z"].FacetID
	delete(src, "2026-01-26T06:30:00Z")
	if _, _, err := g.Retract(ctx, removed); err != nil {
		t.Fatalf("Retract: %v", err)
	}
	lived, err := fold.Read(ctx, l)
	if err != nil {
		t.Fatalf("Read after retract: %v", err)
	}
	if lived.Value.Hash() == corrected.Value.Hash() {
		t.Fatal("a retraction did not move the canonical hash")
	}
	if lived.Value.Len() != 2 {
		t.Fatalf("live set holds %d facets, want 2", lived.Value.Len())
	}
	if st := lived.States[removed]; st.Status != facet.StatusTombstoned {
		t.Fatalf("retracted facet folded to %+v, want tombstoned", st)
	}

	// Switch Anam off and lose nothing: drop the ledger, init, re-ingest.
	if err := l.Drop(ctx); err != nil {
		t.Fatalf("Drop: %v", err)
	}
	if err := l.Init(ctx); err != nil {
		t.Fatalf("re-Init: %v", err)
	}
	src.ingest(t, g)

	rebuilt, err := fold.Read(ctx, l)
	if err != nil {
		t.Fatalf("Read after rebuild: %v", err)
	}
	if rebuilt.Value.Hash() != lived.Value.Hash() {
		t.Errorf("rebuild diverged:\n lived   %s\n rebuilt %s\n lived bytes:   %s\n rebuilt bytes: %s",
			lived.Value.Hash(), rebuilt.Value.Hash(), lived.Value.Bytes(), rebuilt.Value.Bytes())
	}
	// The bookkeeping is allowed to differ — the lived history superseded and
	// tombstoned its way to W=5, the rebuild admits two facts and stops at W=2.
	if rebuilt.W == lived.W {
		t.Errorf("expected the rebuilt watermark to differ (lived %d, rebuilt %d) — if it matches, the oracle may be hashing bookkeeping", lived.W, rebuilt.W)
	}
}

// Read must hand back a value consistent with its own watermark: the live facets, at
// the folded versions, in fact-time order.
func TestRead_WholePersonAtTheHead(t *testing.T) {
	t.Parallel()
	g, l := newHarness(t)
	ctx := context.Background()

	src := source{
		"2026-02-02T07:00:00Z": `"systolic": 119, "diastolic": 77`,
		"2026-01-31T06:45:00Z": `"systolic": 122, "diastolic": 81`,
	}
	src.ingest(t, g)
	src["2026-02-02T07:00:00Z"] = `"systolic": 116, "diastolic": 74`
	src.ingest(t, g)

	res, err := fold.Read(ctx, l)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if res.W != 3 {
		t.Errorf("W = %d, want 3 (two admissions, one supersession)", res.W)
	}
	if len(res.Live) != 2 {
		t.Fatalf("live = %d facets, want 2", len(res.Live))
	}
	if !res.Live[0].FactTime.Before(res.Live[1].FactTime) {
		t.Error("live facets are not in fact-time order")
	}
	if res.Live[1].ArtifactVersion != 2 {
		t.Errorf("superseded facet served at v%d, want v2", res.Live[1].ArtifactVersion)
	}
	if res.Value.Len() != 2 {
		t.Errorf("value holds %d entries, want 2", res.Value.Len())
	}
	if res.Value.Entries()[0].FactTime > res.Value.Entries()[1].FactTime {
		t.Error("value entries are not in fact-time order")
	}
}
