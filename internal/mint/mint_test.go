package mint_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/fold"
	"github.com/mattjoyce/anam/internal/gate"
	"github.com/mattjoyce/anam/internal/mint"
	"github.com/mattjoyce/anam/internal/span"
	"github.com/mattjoyce/anam/internal/store/ledger"
	"github.com/mattjoyce/anam/internal/store/ops"
)

// The mint is tested over the real pair it writes across (AGENTS.md: real components
// over mocks): a ledger file and an operational store file, each under the test's own
// temp directory.

const person = "matt"

// bp identifies a reading by when it was taken, so its fact-time can never move.
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
  "schema": {"type": "object", "required": ["measured_at", "systolic"],
    "properties": {"measured_at": {"type": "string", "minLength": 1}, "systolic": {"type": "number"}},
    "additionalProperties": false}
}`

// weight identifies a measurement by the source's own id, with the date a separate
// field — the Withings shape — so a revision can carry the same facet into another
// bucket. That is the case the architecture flags and the card makes a criterion.
const weightManifest = `{
  "meta": {
    "facet_type": "health.weight",
    "definition_version": 1,
    "classification": "Normal",
    "validation_mode": "strict",
    "source_schema": "scale/measure",
    "time_semantics": {
      "fact_time": {"pointer": "/measured_at", "layout": "2006-01-02T15:04:05Z07:00", "zone": "UTC"}
    },
    "identity_key": ["/id"]
  },
  "schema": {"type": "object", "required": ["id", "measured_at", "kg"],
    "properties": {"id": {"type": "string", "minLength": 1}, "measured_at": {"type": "string", "minLength": 1}, "kg": {"type": "number"}},
    "additionalProperties": false}
}`

type harness struct {
	gate *gate.Gate
	l    *ledger.Ledger
	db   *ops.DB
	deps mint.Deps
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	ctx := context.Background()

	buckets, err := span.New(span.Monthly)
	if err != nil {
		t.Fatalf("span.New: %v", err)
	}
	l, err := ledger.Create(ctx, filepath.Join(t.TempDir(), "ledger.db"), person)
	if err != nil {
		t.Fatalf("ledger.Create: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })

	db, err := ops.Open(ctx, filepath.Join(t.TempDir(), "tracking.db"), buckets)
	if err != nil {
		t.Fatalf("ops.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	bp, err := def.Parse([]byte(bpManifest))
	if err != nil {
		t.Fatalf("def.Parse(bp): %v", err)
	}
	weight, err := def.Parse([]byte(weightManifest))
	if err != nil {
		t.Fatalf("def.Parse(weight): %v", err)
	}
	defs, err := def.NewRegistry(bp, weight)
	if err != nil {
		t.Fatalf("def.NewRegistry: %v", err)
	}
	scopes := gate.StaticScopes{
		"bp-csv": {"health.blood-pressure"},
		"scale":  {"health.weight"},
	}

	return &harness{
		gate: gate.New(l, defs, scopes, clock.Real{}, person, gate.Limits{MaxPayloadBytes: config.DefaultMaxPayloadBytes, MaxRetries: config.DefaultCommitRetries}),
		l:    l,
		db:   db,
		deps: mint.Deps{
			Ledger: l, Index: db, Values: db, Buckets: buckets,
			Clock: clock.Fixed{Instant: time.Date(2026, 10, 6, 9, 0, 0, 0, time.UTC)},
		},
	}
}

// admit puts one row through the real gate and returns the facet id it bound to.
func (h *harness) admit(t *testing.T, source string, typ facet.Type, ref, payload string) string {
	t.Helper()
	env := facet.Envelope{
		Type: typ, Source: source, Injector: source, SourceRef: ref, Payload: []byte(payload),
	}
	binding, err := h.gate.Resolve(env)
	if err != nil {
		t.Fatalf("Resolve %s: %v", ref, err)
	}
	if _, _, err := h.gate.Commit(context.Background(), env, binding); err != nil {
		t.Fatalf("Commit %s: %v", ref, err)
	}
	return facet.NewID(ref)
}

func (h *harness) bp(t *testing.T, measuredAt string, systolic int) string {
	t.Helper()
	return h.admit(t, "bp-csv", "health.blood-pressure", facet.SourceRef("bp-csv", measuredAt),
		fmt.Sprintf(`{"measured_at": %q, "systolic": %d}`, measuredAt, systolic))
}

func (h *harness) weight(t *testing.T, id, measuredAt string, kg float64) string {
	t.Helper()
	return h.admit(t, "scale", "health.weight", facet.SourceRef("scale", id),
		fmt.Sprintf(`{"id": %q, "measured_at": %q, "kg": %g}`, id, measuredAt, kg))
}

// seed admits readings across three months, with two in July and one each in the
// other two.
func (h *harness) seed(t *testing.T) {
	t.Helper()
	h.bp(t, "2026-07-01T06:30:00Z", 118)
	h.bp(t, "2026-07-15T06:30:00Z", 121)
	h.bp(t, "2026-08-10T06:30:00Z", 117)
	h.bp(t, "2026-09-03T06:30:00Z", 119)
}

func (h *harness) run(t *testing.T, dryRun bool) mint.Result {
	t.Helper()
	res, err := mint.Run(context.Background(), h.deps, dryRun)
	if err != nil {
		t.Fatalf("mint.Run: %v", err)
	}
	return res
}

func (h *harness) heads(t *testing.T) map[string]ops.CheckpointRow {
	t.Helper()
	heads, err := h.db.SpanHeads(context.Background())
	if err != nil {
		t.Fatalf("SpanHeads: %v", err)
	}
	return heads
}

func (h *harness) rows(t *testing.T) []ops.CheckpointRow {
	t.Helper()
	rows, err := h.db.Checkpoints(context.Background())
	if err != nil {
		t.Fatalf("Checkpoints: %v", err)
	}
	return rows
}

// files is how many checkpoint values the store holds.
func (h *harness) files(t *testing.T) int {
	t.Helper()
	digests, err := h.db.List(context.Background(), mint.Kind)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	return len(digests)
}

func outcomes(res mint.Result) map[string]mint.Outcome {
	m := map[string]mint.Outcome{}
	for _, o := range res.Outcomes {
		m[o.SpanID] = o
	}
	return m
}

// The first mint cuts one value per bucket; a second with nothing new writes
// nothing and reports the same hashes.
func TestRun_FirstMintThenIdempotent(t *testing.T) {
	h := newHarness(t)
	h.seed(t)

	first := h.run(t, false)
	if got, want := first.Minted(), 3; got != want {
		t.Fatalf("first mint: minted %d buckets, want %d: %+v", got, want, first.Outcomes)
	}
	o := outcomes(first)
	if o["bucket:2026-07"].Len != 2 || o["bucket:2026-08"].Len != 1 || o["bucket:2026-09"].Len != 1 {
		t.Fatalf("bucket sizes wrong: %+v", first.Outcomes)
	}
	for _, oc := range first.Outcomes {
		if oc.V != 1 || oc.CoversW != int64(first.W) {
			t.Errorf("%s: v%d covers %d, want v1 covering %d", oc.SpanID, oc.V, oc.CoversW, first.W)
		}
	}
	if n := h.files(t); n != 3 {
		t.Fatalf("checkpoint values: %d, want 3", n)
	}

	second := h.run(t, false)
	if second.Minted() != 0 {
		t.Fatalf("second mint cut %d buckets, want 0: %+v", second.Minted(), second.Outcomes)
	}
	if len(h.rows(t)) != 3 {
		t.Fatalf("checkpoint rows after second mint: %d, want 3", len(h.rows(t)))
	}
	for id, a := range o {
		if b := outcomes(second)[id]; b.ValueHash != a.ValueHash || b.V != 1 {
			t.Errorf("%s: hash or version moved with nothing new: %+v -> %+v", id, a, b)
		}
	}
}

// A facet ingested later with an old fact-time re-opens its bucket, and only that one.
func TestRun_BackfillReopensOneBucket(t *testing.T) {
	h := newHarness(t)
	h.seed(t)
	before := outcomes(h.run(t, false))

	h.bp(t, "2026-07-20T06:30:00Z", 123)
	after := h.run(t, false)
	if after.Minted() != 1 {
		t.Fatalf("minted %d buckets, want 1: %+v", after.Minted(), after.Outcomes)
	}
	o := outcomes(after)
	if jul := o["bucket:2026-07"]; !jul.Minted || jul.V != 2 || jul.Len != 3 || jul.CoversW != int64(after.W) {
		t.Errorf("July: %+v, want minted v2 of 3 facets at W=%d", jul, after.W)
	}
	for _, id := range []string{"bucket:2026-08", "bucket:2026-09"} {
		if o[id].Minted || o[id].V != 1 || o[id].ValueHash != before[id].ValueHash {
			t.Errorf("%s moved on a July backfill: %+v", id, o[id])
		}
	}
	// July's first version is subsumed by the second and no receipt pins it, so the
	// mint that cut the second removed it: the log holds one row per bucket.
	if len(h.rows(t)) != 3 || h.files(t) != 3 {
		t.Fatalf("checkpoint rows and values: %d and %d, want 3 and 3", len(h.rows(t)), h.files(t))
	}
	if after.Collected.Checkpoints != 1 || after.Collected.Values != 1 {
		t.Errorf("the mint removed %+v, want July's first version and its value", after.Collected)
	}
}

// A new version whose fact-time falls in another bucket re-mints both buckets, and a
// composed read counts the facet once — in its new bucket, gone from its old one.
func TestRun_FactTimeMovesBetweenBuckets(t *testing.T) {
	h := newHarness(t)
	h.seed(t)
	id := h.weight(t, "w-1", "2026-07-09T07:00:00Z", 81.2)
	h.run(t, false)

	// The scale revises the measurement's date: same identity, new content.
	h.weight(t, "w-1", "2026-08-09T07:00:00Z", 81.2)
	res := h.run(t, false)
	if res.Minted() != 2 {
		t.Fatalf("minted %d buckets, want 2 (July and August): %+v", res.Minted(), res.Outcomes)
	}
	o := outcomes(res)
	if !o["bucket:2026-07"].Minted || !o["bucket:2026-08"].Minted || o["bucket:2026-09"].Minted {
		t.Fatalf("wrong buckets re-minted: %+v", res.Outcomes)
	}

	composed := h.composeAll(t)
	count := 0
	for _, e := range composed.Entries() {
		if e.FacetID == id {
			count++
			if e.FactTime[:7] != "2026-08" {
				t.Errorf("moved facet composed at %s, want August", e.FactTime)
			}
		}
	}
	if count != 1 {
		t.Fatalf("moved facet appears %d times in the composed read, want once", count)
	}

	full, err := fold.Read(context.Background(), h.l)
	if err != nil {
		t.Fatalf("fold.Read: %v", err)
	}
	if composed.Hash() != full.Value.Hash() {
		t.Fatalf("composed %s != full fold %s", composed.Hash(), full.Value.Hash())
	}
}

// A read over a range composes bucket values and matches a full fold narrowed to the
// same range.
func TestCompose_MatchesTheFoldOverARange(t *testing.T) {
	h := newHarness(t)
	h.seed(t)
	h.weight(t, "w-2", "2026-08-20T07:00:00Z", 80.4)
	h.run(t, false)
	heads := h.heads(t)

	var values []fold.Value
	for _, id := range []string{"bucket:2026-07", "bucket:2026-08"} {
		v, err := mint.Load(context.Background(), h.deps, heads[id])
		if err != nil {
			t.Fatalf("Load %s: %v", id, err)
		}
		values = append(values, v)
	}
	composed, err := mint.Compose(values...)
	if err != nil {
		t.Fatalf("Compose: %v", err)
	}

	full, err := fold.Read(context.Background(), h.l)
	if err != nil {
		t.Fatalf("fold.Read: %v", err)
	}
	var narrowed []fold.Entry
	for _, e := range full.Value.Entries() {
		if e.FactTime >= "2026-07" && e.FactTime < "2026-09" {
			narrowed = append(narrowed, e)
		}
	}
	want, err := fold.NewValue(narrowed)
	if err != nil {
		t.Fatalf("NewValue: %v", err)
	}
	if composed.Len() != 4 || composed.Hash() != want.Hash() {
		t.Fatalf("composed %d facets %s; fold over the range gives %d facets %s",
			composed.Len(), composed.Hash(), want.Len(), want.Hash())
	}
}

// Retracting a bucket's only facet re-mints it as the empty value, so a composed
// read stops listing the facet. The bucket is found through its head, not through
// the fold, which no longer knows it.
func TestRun_EmptiedBucketMintsTheEmptyValue(t *testing.T) {
	h := newHarness(t)
	h.seed(t)
	sep := facet.NewID(facet.SourceRef("bp-csv", "2026-09-03T06:30:00Z"))
	h.run(t, false)

	if _, _, err := h.gate.Retract(context.Background(), sep); err != nil {
		t.Fatalf("Retract: %v", err)
	}
	res := h.run(t, false)
	o := outcomes(res)
	if s := o["bucket:2026-09"]; !s.Minted || s.V != 2 || s.Len != 0 {
		t.Fatalf("September after retract: %+v, want minted v2 of 0 facets", s)
	}
	if res.Minted() != 1 {
		t.Fatalf("minted %d buckets, want 1: %+v", res.Minted(), res.Outcomes)
	}
	empty, err := mint.Load(context.Background(), h.deps, h.heads(t)["bucket:2026-09"])
	if err != nil {
		t.Fatalf("Load empty: %v", err)
	}
	if empty.Len() != 0 {
		t.Fatalf("empty checkpoint holds %d facets", empty.Len())
	}
}

// A dry run derives the same outcomes and publishes nothing.
func TestRun_DryRunPublishesNothing(t *testing.T) {
	h := newHarness(t)
	h.seed(t)

	dry := h.run(t, true)
	if dry.Written || dry.Minted() != 3 {
		t.Fatalf("dry run: written=%v minted=%d, want false and 3", dry.Written, dry.Minted())
	}
	if len(h.rows(t)) != 0 || h.files(t) != 0 {
		t.Fatalf("dry run wrote %d rows and %d values", len(h.rows(t)), h.files(t))
	}

	real := h.run(t, false)
	for id, d := range outcomes(dry) {
		if r := outcomes(real)[id]; r.ValueHash != d.ValueHash || r.V != d.V {
			t.Errorf("%s: dry run %+v, real %+v", id, d, r)
		}
	}
}

// Minting leaves the ledger untouched: a checkpoint is not something Anam learned.
func TestRun_LeavesTheLedgerUntouched(t *testing.T) {
	h := newHarness(t)
	h.seed(t)
	before, err := h.l.Head(context.Background())
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	h.run(t, false)
	after, err := h.l.Head(context.Background())
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	if before != after {
		t.Fatalf("watermark moved from %d to %d on a mint", before, after)
	}
}

// Load refuses bytes that hash to their key but are not a value's serialisation. The
// store's check covers bytes against key; this covers bytes against the value they
// claim to be, so a hand-written or re-encoded value is caught at the row.
func TestLoad_RefusesBytesThatAreNotTheValueTheyClaim(t *testing.T) {
	h := newHarness(t)
	// Valid JSON for the empty value, but not NewValue's bytes for it.
	loose := []byte("[ ]")
	digest, err := h.deps.Values.Put(context.Background(), mint.Kind, loose)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	row := ops.CheckpointRow{SpanID: "bucket:2026-01", V: 1, CoversW: 0, ValueHash: digest, MintedAt: "x"}
	if _, err := mint.Load(context.Background(), h.deps, row); err == nil {
		t.Fatal("Load accepted bytes that are not the value's serialisation")
	}
}

// composeAll loads every head and composes them: the whole-person read from
// checkpoints alone.
func (h *harness) composeAll(t *testing.T) fold.Value {
	t.Helper()
	var values []fold.Value
	for _, row := range h.heads(t) {
		v, err := mint.Load(context.Background(), h.deps, row)
		if err != nil {
			t.Fatalf("Load %s: %v", row.SpanID, err)
		}
		values = append(values, v)
	}
	composed, err := mint.Compose(values...)
	if err != nil {
		t.Fatalf("Compose: %v", err)
	}
	return composed
}
