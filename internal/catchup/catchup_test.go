package catchup_test

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/mattjoyce/anam/internal/catchup"
	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/fold"
	"github.com/mattjoyce/anam/internal/gate"
	"github.com/mattjoyce/anam/internal/span"
	"github.com/mattjoyce/anam/internal/store/ledger"
	"github.com/mattjoyce/anam/internal/store/ops"
)

// testBuckets is the bucketing catch-up stamps onto facet_index rows here. It is the
// default width, which is also what ops.Open pins the test database at.
func testBuckets(t *testing.T) span.Buckets {
	t.Helper()
	b, err := span.New(span.Default)
	if err != nil {
		t.Fatalf("span.New(%q): %v", span.Default, err)
	}
	return b
}

// Catch-up is tested over the real pair it bridges: a real ledger file and a real
// operational store file, both in the test's own temp directory.

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

func newHarness(t *testing.T) (*gate.Gate, *ledger.Ledger, *ops.DB) {
	t.Helper()
	ctx := context.Background()

	l, err := ledger.Create(ctx, filepath.Join(t.TempDir(), "ledger.db"), person)
	if err != nil {
		t.Fatalf("ledger.Create: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })

	db, err := ops.Open(ctx, filepath.Join(t.TempDir(), "tracking.db"), testBuckets(t))
	if err != nil {
		t.Fatalf("ops.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	d, err := def.Parse([]byte(bpManifest))
	if err != nil {
		t.Fatalf("def.Parse: %v", err)
	}
	reg, err := def.NewRegistry(d)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	scopes := gate.StaticScopes{"bp-csv": {"health.blood-pressure"}}
	return gate.New(l, reg, scopes, clock.Real{}, person, gate.Limits{MaxPayloadBytes: config.DefaultMaxPayloadBytes, MaxRetries: config.DefaultCommitRetries}), l, db
}

// noteHarness is newHarness with the bundled record.edge beside the readings, and a
// records source allowed to write it, so a test can assert an edge about a reading.
func noteHarness(t *testing.T) (*gate.Gate, *ledger.Ledger, *ops.DB) {
	t.Helper()
	ctx := context.Background()
	l, err := ledger.Create(ctx, filepath.Join(t.TempDir(), "ledger.db"), person)
	if err != nil {
		t.Fatalf("ledger.Create: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })
	db, err := ops.Open(ctx, filepath.Join(t.TempDir(), "tracking.db"), testBuckets(t))
	if err != nil {
		t.Fatalf("ops.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	bp, err := def.Parse([]byte(bpManifest))
	if err != nil {
		t.Fatalf("def.Parse: %v", err)
	}
	bundled, err := def.Bundled()
	if err != nil {
		t.Fatalf("def.Bundled: %v", err)
	}
	note, ok := bundled.Lookup(catchup.EdgeType)
	if !ok {
		t.Fatalf("no bundled %s", catchup.EdgeType)
	}
	reg, err := def.NewRegistry(bp, note)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	scopes := gate.StaticScopes{"bp-csv": {"health.blood-pressure"}, "records": {catchup.EdgeType}}
	return gate.New(l, reg, scopes, clock.Real{}, person, gate.Limits{MaxPayloadBytes: config.DefaultMaxPayloadBytes, MaxRetries: config.DefaultCommitRetries}), l, db
}

func commitNote(t *testing.T, g *gate.Gate, id, payload string) gate.Result {
	t.Helper()
	env := facet.Envelope{
		Type: catchup.EdgeType, Source: "records", Injector: "records",
		SourceRef: facet.SourceRef("records", id),
		Payload: []byte(`{"id": "` + id + `", "at": "2026-10-09 11:29", ` +
			`"source_time": {"value": "2026-10-09 11:29", "zone": "Australia/Sydney", "offset": "+11:00", "basis": "source"}, ` + payload + `}`),
	}
	binding, err := g.Resolve(env)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	res, _, err := g.Commit(context.Background(), env, binding)
	if err != nil {
		t.Fatalf("Commit note: %v", err)
	}
	return res
}

func commit(t *testing.T, g *gate.Gate, measuredAt, reading string) gate.Result {
	t.Helper()
	env := facet.Envelope{
		Type: "health.blood-pressure", Source: "bp-csv", Injector: "bp-csv",
		SourceRef: facet.SourceRef("bp-csv", measuredAt),
		Payload:   []byte(`{"measured_at": "` + measuredAt + `", ` + reading + `}`),
	}
	binding, err := g.Resolve(env)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	res, _, err := g.Commit(context.Background(), env, binding)
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	return res
}

// The derived tier catches up by replay, incrementally: a second sync resumes
// from the cursor, advances persisted states through the same fold, and the
// indexes end up agreeing with the canonical read.
func TestSync_IncrementalAndIdempotent(t *testing.T) {
	t.Parallel()
	g, l, db := newHarness(t)
	ctx := context.Background()

	resA := commit(t, g, "2026-01-24T06:30:00Z", `"systolic": 118, "diastolic": 76`)
	commit(t, g, "2026-01-25T06:30:00Z", `"systolic": 121, "diastolic": 79`)

	sync, err := catchup.Sync(ctx, l, db, testBuckets(t))
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if sync.From != 0 || sync.To != 2 {
		t.Fatalf("first sync = %+v, want 0→2", sync)
	}

	// Current already: a re-run is a no-op.
	sync2, err := catchup.Sync(ctx, l, db, testBuckets(t))
	if err != nil {
		t.Fatalf("re-Sync: %v", err)
	}
	if sync2.From != 2 || sync2.To != 2 {
		t.Fatalf("idle sync = %+v, want 2→2", sync2)
	}

	// More history: a correction and a retraction. The next sync replays only
	// (2, 4] and must land the same states the full fold derives.
	commit(t, g, "2026-01-24T06:30:00Z", `"systolic": 124, "diastolic": 80`)
	if _, _, err := g.Retract(ctx, resA.FacetID); err != nil {
		t.Fatalf("Retract: %v", err)
	}
	sync3, err := catchup.Sync(ctx, l, db, testBuckets(t))
	if err != nil {
		t.Fatalf("incremental Sync: %v", err)
	}
	if sync3.From != 2 || sync3.To != 4 {
		t.Fatalf("incremental sync = %+v, want 2→4", sync3)
	}

	states, err := db.FacetStates(ctx, []string{resA.FacetID})
	if err != nil {
		t.Fatalf("FacetStates: %v", err)
	}
	if got := states[resA.FacetID]; got != (fold.State{Version: 2, Status: facet.StatusTombstoned}) {
		t.Errorf("indexed state = %+v, want v2 tombstoned", got)
	}

	whole, err := fold.Read(ctx, l)
	if err != nil {
		t.Fatalf("fold.Read: %v", err)
	}
	for id, want := range whole.States {
		got, err := db.FacetStates(ctx, []string{id})
		if err != nil {
			t.Fatalf("FacetStates(%s): %v", id, err)
		}
		if got[id] != want {
			t.Errorf("index disagrees with the fold for %s: %+v vs %+v", id, got[id], want)
		}
	}
}

// changes_since is an anam-time question answered from the index (§9).
func TestChangesSince_OverTheRealPair(t *testing.T) {
	t.Parallel()
	g, l, db := newHarness(t)
	ctx := context.Background()

	before := clock.Format(time.Now().UTC().Add(-time.Minute))
	commit(t, g, "2026-02-01T06:30:00Z", `"systolic": 119, "diastolic": 77`)
	commit(t, g, "2026-02-02T06:30:00Z", `"systolic": 117, "diastolic": 74`)
	if _, err := catchup.Sync(ctx, l, db, testBuckets(t)); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	changes, err := db.ChangesSince(ctx, before, "")
	if err != nil {
		t.Fatalf("ChangesSince: %v", err)
	}
	if len(changes) != 2 {
		t.Fatalf("changes = %d, want 2", len(changes))
	}
	for i, c := range changes {
		if c.Kind != facet.EventAdmitted || c.Seq != int64(i)+1 {
			t.Errorf("change %d = %+v", i, c)
		}
	}

	// Nothing since the sync: an empty answer, not an error.
	after := clock.Format(time.Now().UTC())
	if late, _ := db.ChangesSince(ctx, after, ""); len(late) != 0 {
		t.Errorf("changes since now = %d, want 0", len(late))
	}
}

// The cursor ahead of the head is wrong, not behind — Sync must refuse it.
func TestSync_RefusesACursorAheadOfTheLedger(t *testing.T) {
	t.Parallel()
	g, l, db := newHarness(t)
	ctx := context.Background()

	commit(t, g, "2026-03-01T06:30:00Z", `"systolic": 120, "diastolic": 78`)
	if err := db.ApplyReplay(ctx, 0, 99, nil, nil, nil); err != nil {
		t.Fatalf("force cursor: %v", err)
	}

	_, err := catchup.Sync(ctx, l, db, testBuckets(t))
	if err == nil || !strings.Contains(err.Error(), "ahead") {
		t.Fatalf("Sync with cursor 99 over head 1 = %v, want ErrIndexAhead", err)
	}
}

// A rebuilt ledger holding the same facts reaches the same head, so the cursor alone
// reads the old index as current. Every anam-time in that index would be the old
// ledger's. The ledger pin is what refuses it.
func TestSync_RefusesAReplacedLedger(t *testing.T) {
	t.Parallel()
	g, l, db := newHarness(t)
	ctx := context.Background()

	commit(t, g, "2026-04-01T06:30:00Z", `"systolic": 120, "diastolic": 78`)
	commit(t, g, "2026-04-02T06:30:00Z", `"systolic": 122, "diastolic": 80`)
	if _, err := catchup.Sync(ctx, l, db, testBuckets(t)); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	if err := l.Drop(ctx); err != nil {
		t.Fatalf("Drop: %v", err)
	}
	if err := l.Init(ctx); err != nil {
		t.Fatalf("re-Init: %v", err)
	}
	commit(t, g, "2026-04-01T06:30:00Z", `"systolic": 120, "diastolic": 78`)
	commit(t, g, "2026-04-02T06:30:00Z", `"systolic": 122, "diastolic": 80`)

	_, err := catchup.Sync(ctx, l, db, testBuckets(t))
	if !errors.Is(err, ops.ErrLedgerChanged) {
		t.Fatalf("Sync against a rebuilt ledger = %v, want ErrLedgerChanged", err)
	}
}

// A batch commit is one change in Anam's knowledge, so its events share one anam-time.
// That makes anam_time non-unique — up to MaxBatch events on one instant — and
// changes_since is a half-open window on exactly that column.
//
// So the window's edges have to be right about ties, and a batch has to come back
// whole. A caller that took the last anam_time it saw as its next `since` would
// otherwise step over the rest of that instant's batch and call it "no changes".
func TestChangesSince_ReturnsABatchsSharedInstantWhole(t *testing.T) {
	t.Parallel()
	g, l, db := newHarness(t)
	ctx := context.Background()

	const n = 5
	envs := make([]facet.Envelope, n)
	bindings := make([]facet.Binding, n)
	for i := range envs {
		at := time.Date(2026, 5, 1, 6, i, 0, 0, time.UTC).Format(time.RFC3339)
		envs[i] = facet.Envelope{
			Type: "health.blood-pressure", Source: "bp-csv", Injector: "bp-csv",
			SourceRef: facet.SourceRef("bp-csv", at),
			Payload:   []byte(`{"measured_at": "` + at + `", "systolic": 118, "diastolic": 76}`),
		}
		b, err := g.Resolve(envs[i])
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		bindings[i] = b
	}

	before := clock.Format(time.Now().UTC().Add(-time.Minute))
	if _, _, err := g.CommitBatch(ctx, envs, bindings); err != nil {
		t.Fatalf("CommitBatch: %v", err)
	}
	if _, err := catchup.Sync(ctx, l, db, testBuckets(t)); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	changes, err := db.ChangesSince(ctx, before, "")
	if err != nil {
		t.Fatalf("ChangesSince: %v", err)
	}
	if len(changes) != n {
		t.Fatalf("changes = %d, want %d — the batch came back in pieces", len(changes), n)
	}

	// The premise: they really do share one instant, and seq is what orders them.
	instant := changes[0].AnamTime
	for i, c := range changes {
		if c.AnamTime != instant {
			t.Errorf("change %d is at %s, want the batch's shared %s", i, c.AnamTime, instant)
		}
		if c.Seq != int64(i)+1 {
			t.Errorf("change %d has seq %d — the tie is not broken by seq", i, c.Seq)
		}
	}

	// `since` is exclusive: asking from the batch's own instant excludes all of it,
	// rather than some of it.
	if after, err := db.ChangesSince(ctx, instant, ""); err != nil {
		t.Fatalf("ChangesSince at the instant: %v", err)
	} else if len(after) != 0 {
		t.Errorf("since the batch's own instant = %d changes, want 0 (the bound is exclusive)", len(after))
	}

	// `until` is inclusive, and takes the instant whole for the same reason.
	if upto, err := db.ChangesSince(ctx, before, instant); err != nil {
		t.Fatalf("ChangesSince up to the instant: %v", err)
	} else if len(upto) != n {
		t.Errorf("up to the batch's instant = %d changes, want %d (the bound is inclusive)", len(upto), n)
	}
}

// The edge index is derived from the edges' own payloads (card 104): a note about a
// reading is an edge touching it; a new version naming another reading moves the
// edge; a retraction removes it; and a fresh store replayed from genesis holds exactly
// what the incremental syncs left.
func TestSync_DerivesTheEdgeIndex(t *testing.T) {
	t.Parallel()
	g, l, db := noteHarness(t)
	ctx := context.Background()

	a := commit(t, g, "2026-01-24T06:30:00Z", `"systolic": 118, "diastolic": 76`)
	b := commit(t, g, "2026-01-25T06:30:00Z", `"systolic": 121, "diastolic": 79`)
	n := commitNote(t, g, "note:1", `"about": ["`+a.FacetID+`"], "text": "During a large work meeting, presenting."`)
	if _, err := catchup.Sync(ctx, l, db, testBuckets(t)); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	got, err := db.TouchingEdges(ctx, a.FacetID)
	if err != nil {
		t.Fatalf("TouchingEdges: %v", err)
	}
	if len(got.Edges) != 1 || got.Edges[0].EdgeID != n.FacetID || got.Edges[0].Kind != "about" ||
		!reflect.DeepEqual(got.Edges[0].To, []string{a.FacetID}) || got.Edges[0].From != nil || got.LastSeq != 3 {
		t.Fatalf("touching a = %+v, want the note about a at seq 3", got)
	}

	// A v2 writer: the note now names b on its to side and says its kind; a is free.
	commitNote(t, g, "note:1", `"kind": "about", "to": ["`+b.FacetID+`"], "text": "The next morning."`)
	if _, err := catchup.Sync(ctx, l, db, testBuckets(t)); err != nil {
		t.Fatalf("second Sync: %v", err)
	}
	if got, _ := db.TouchingEdges(ctx, a.FacetID); len(got.Edges) != 0 {
		t.Errorf("after the move, a is touched by %+v", got.Edges)
	}
	if got, _ := db.TouchingEdges(ctx, b.FacetID); len(got.Edges) != 1 || !reflect.DeepEqual(got.Edges[0].To, []string{b.FacetID}) {
		t.Errorf("after the move, b is touched by %+v", got.Edges)
	}

	// A second edge, a model's reading: from a draft (here, a reading stands in) to the note.
	commitNote(t, g, "interp:1", `"kind": "interpreted_from", "from": ["`+a.FacetID+`"], "to": ["`+n.FacetID+`"]`)
	if _, err := catchup.Sync(ctx, l, db, testBuckets(t)); err != nil {
		t.Fatalf("third Sync: %v", err)
	}
	byNote, _ := db.TouchingEdges(ctx, n.FacetID)
	if len(byNote.Edges) != 1 || byNote.Edges[0].Kind != "interpreted_from" || !reflect.DeepEqual(byNote.Edges[0].From, []string{a.FacetID}) {
		t.Errorf("the note is touched by %+v, want the interpretation from a", byNote.Edges)
	}

	// Retracting the note removes its edge; the interpretation still names it.
	if _, _, err := g.Retract(ctx, n.FacetID); err != nil {
		t.Fatalf("Retract: %v", err)
	}
	if _, err := catchup.Sync(ctx, l, db, testBuckets(t)); err != nil {
		t.Fatalf("fourth Sync: %v", err)
	}
	if got, _ := db.TouchingEdges(ctx, b.FacetID); len(got.Edges) != 0 {
		t.Errorf("after the retraction, b is touched by %+v", got.Edges)
	}

	// Provably derived: a fresh store replayed from genesis agrees, facet by facet.
	fresh, err := ops.Open(ctx, filepath.Join(t.TempDir(), "fresh.db"), testBuckets(t))
	if err != nil {
		t.Fatalf("ops.Open fresh: %v", err)
	}
	t.Cleanup(func() { _ = fresh.Close() })
	if _, err := catchup.Sync(ctx, l, fresh, testBuckets(t)); err != nil {
		t.Fatalf("Sync fresh: %v", err)
	}
	for _, id := range []string{a.FacetID, b.FacetID, n.FacetID} {
		inc, _ := db.TouchingEdges(ctx, id)
		full, _ := fresh.TouchingEdges(ctx, id)
		if !reflect.DeepEqual(inc, full) {
			t.Errorf("replay from genesis disagrees for %s: %+v vs %+v", id, full, inc)
		}
	}
}

// The edge index reads fields the bundled Definition of the edge type declares, and
// nothing else; the Definition is held to declaring them.
func TestEdgeType_DeclaresItsEnds(t *testing.T) {
	t.Parallel()
	bundled, err := def.Bundled()
	if err != nil {
		t.Fatal(err)
	}
	d, ok := bundled.Lookup(catchup.EdgeType)
	if !ok {
		t.Fatalf("no bundled %s", catchup.EdgeType)
	}
	for _, p := range []string{"/kind", "/from/0", "/to/0", "/about/0"} {
		if err := d.HasPointer(p); err != nil {
			t.Errorf("%s does not declare %s: %v", catchup.EdgeType, p, err)
		}
	}
}
