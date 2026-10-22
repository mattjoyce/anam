package mint_test

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/fold"
	"github.com/mattjoyce/anam/internal/mint"
	"github.com/mattjoyce/anam/internal/store/ops"
	"github.com/mattjoyce/anam/internal/store/ops/opstest"
)

// A mint that starts from the last run is held to the full mint as its oracle: the
// same ledger, two copies of the same checkpoint log, one minted from the last run
// and one by the full fold, must end with the same result and the same log.

var errCrash = errors.New("crashed before the run was recorded")

// crashIndex fails a mint before it records its run: at its nth checkpoint, or with
// n zero once every checkpoint is written, as a crash at that point would.
type crashIndex struct {
	mint.Index
	n    int
	seen *int
}

func (c crashIndex) AppendCheckpoint(ctx context.Context, r ops.CheckpointRow) error {
	if *c.seen++; c.n > 0 && *c.seen >= c.n {
		return errCrash
	}
	return c.Index.AppendCheckpoint(ctx, r)
}

func (crashIndex) AppendMintRun(context.Context, int64, string) error { return errCrash }

func TestRun_FromTheLastRunIsTheFullMint(t *testing.T) {
	sepID := facet.NewID(facet.SourceRef("bp-csv", "2026-09-03T06:30:00Z"))
	july := func(h *harness, t *testing.T, days ...int) {
		for _, d := range days {
			h.bp(t, fmt.Sprintf("2026-07-%02dT06:30:00Z", d), 110+d)
		}
	}
	cases := []struct {
		name         string
		before       func(h *harness, t *testing.T)
		budgetBefore int
		crash        func(h *harness, t *testing.T) // changes minted by a run that never recorded itself
		crashAt      int                            // the checkpoint that run crashed at; 0 for none
		budgetCrash  int
		after        func(h *harness, t *testing.T)
		budgetAfter  int
		wantFull     bool
	}{
		{name: "nothing new", before: (*harness).seed, after: func(*harness, *testing.T) {}},
		{name: "backfill", before: (*harness).seed,
			after: func(h *harness, t *testing.T) { h.bp(t, "2026-07-20T06:30:00Z", 123) }},
		{name: "fact-time moves across buckets and years",
			before: func(h *harness, t *testing.T) { h.seed(t); h.weight(t, "w-1", "2026-07-09T07:00:00Z", 81) },
			after: func(h *harness, t *testing.T) {
				h.weight(t, "w-1", "2026-08-09T07:00:00Z", 81)
				h.weight(t, "w-2", "2027-01-09T07:00:00Z", 80)
				h.weight(t, "w-2", "2023-01-09T07:00:00Z", 80)
			}},
		{name: "emptied bucket", before: (*harness).seed,
			after: func(h *harness, t *testing.T) {
				if _, _, err := h.gate.Retract(context.Background(), sepID); err != nil {
					t.Fatalf("Retract: %v", err)
				}
			}},
		{name: "growth splits a bucket",
			before: func(h *harness, t *testing.T) { h.seed(t) },
			after:  func(h *harness, t *testing.T) { july(h, t, 2, 3, 4, 5, 6, 7) }},
		{name: "budget lowered splits an unchanged bucket",
			before:       func(h *harness, t *testing.T) { h.seed(t); july(h, t, 2, 3, 4, 5, 6, 7) },
			budgetBefore: 100_000,
			after:        func(h *harness, t *testing.T) { h.bp(t, "2024-09-20T06:30:00Z", 120) }},
		{name: "last run at a fixed width",
			before:       func(h *harness, t *testing.T) { h.seed(t); july(h, t, 2, 3, 4) },
			budgetBefore: -1,
			after:        func(h *harness, t *testing.T) { h.bp(t, "2026-08-20T06:30:00Z", 120) }},
		{name: "a crashed run since the last", before: (*harness).seed,
			crash:    func(h *harness, t *testing.T) { h.bp(t, "2026-07-20T06:30:00Z", 123) },
			after:    func(h *harness, t *testing.T) { h.bp(t, "2026-08-20T06:30:00Z", 124) },
			wantFull: true},
		{name: "a crash partway through a split at the last run's W",
			before:       func(h *harness, t *testing.T) { h.seed(t); july(h, t, 2, 3, 4, 5, 6, 7) },
			budgetBefore: 100_000,
			crash:        func(*harness, *testing.T) {}, crashAt: 2,
			after:    func(*harness, *testing.T) {},
			wantFull: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			ctx := context.Background()
			twin := h.twin(t)
			setBudget := func(b int) {
				switch {
				case b < 0:
					h.deps.Budget = 0
				case b == 0:
					h.deps.Budget = 1000 // two readings fit, six do not
				default:
					h.deps.Budget = b
				}
			}
			both := func(index func(mint.Index) mint.Index) {
				for _, db := range []*ops.DB{h.db, twin} {
					d := h.deps
					d.Index, d.Values = index(db), db
					_, err := mint.RunFull(ctx, d, false)
					if err != nil && !errors.Is(err, errCrash) {
						t.Fatalf("RunFull: %v", err)
					}
				}
			}
			plain := func(i mint.Index) mint.Index { return i }

			setBudget(tc.budgetBefore)
			tc.before(h, t)
			both(plain)
			if tc.crash != nil {
				tc.crash(h, t)
				setBudget(tc.budgetCrash)
				both(func(i mint.Index) mint.Index { return crashIndex{Index: i, n: tc.crashAt, seen: new(int)} })
				readIsTheFold(t, h)
			}
			tc.after(h, t)
			setBudget(tc.budgetAfter)

			d := h.deps
			inc, err := mint.Run(ctx, d, false)
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			d.Index, d.Values = twin, twin
			full, err := mint.RunFull(ctx, d, false)
			if err != nil {
				t.Fatalf("RunFull: %v", err)
			}
			if inc.Full != tc.wantFull || !full.Full {
				t.Fatalf("paths: incremental run full=%v, want %v; full run full=%v", inc.Full, tc.wantFull, full.Full)
			}
			inc.Full, inc.From, full.Full, full.From = false, 0, false, 0
			if !reflect.DeepEqual(inc, full) {
				t.Fatalf("results differ\nfrom the last run: %+v\nfull:              %+v", inc, full)
			}
			sameLogs(t, h.db, twin, int64(inc.W))
		})
	}
}

// A dry run from the last run reports what a real one would mint, and writes nothing.
func TestRun_FromTheLastRunDryRunWritesNothing(t *testing.T) {
	h := newHarness(t)
	h.deps.Budget = 1000
	h.seed(t)
	h.run(t, false)
	h.bp(t, "2026-09-20T06:30:00Z", 123)
	rows := len(h.rows(t))

	dry := h.run(t, true)
	if dry.Full || dry.Written || dry.Minted() != 1 {
		t.Fatalf("dry run: full=%v written=%v minted=%d, want an unwritten run from the last minting 1",
			dry.Full, dry.Written, dry.Minted())
	}
	if len(h.rows(t)) != rows {
		t.Fatalf("dry run wrote checkpoint rows: %d, was %d", len(h.rows(t)), rows)
	}
	real := h.run(t, false)
	if !reflect.DeepEqual(outcomes(dry), outcomes(real)) {
		t.Fatalf("dry run %+v, real %+v", dry.Outcomes, real.Outcomes)
	}
}

// readIsTheFold holds a composed read at the head to the fold, or to declining.
func readIsTheFold(t *testing.T, h *harness) {
	t.Helper()
	ctx := context.Background()
	head, err := h.l.Head(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want, err := fold.ReadAt(ctx, h.l, head)
	if err != nil {
		t.Fatal(err)
	}
	got, ok, err := mint.ReadAt(ctx, h.l, h.db, h.deps.Values, head)
	if err != nil || (ok && got.Value.Hash() != want.Value.Hash()) {
		t.Fatalf("composed read after a crash: %v %v %d facets, fold %d", ok, err, got.Value.Len(), want.Value.Len())
	}
}

// twin opens a second operational store beside the harness's own, to mint the same
// ledger into.
func (h *harness) twin(t *testing.T) *ops.DB {
	t.Helper()
	db, err := ops.Open(context.Background(), filepath.Join(t.TempDir(), "tracking.db"), h.deps.Buckets)
	if err != nil {
		t.Fatalf("ops.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func sameLogs(t *testing.T, a, b *ops.DB, w int64) {
	t.Helper()
	ctx := context.Background()
	ra, err := a.Checkpoints(ctx)
	if err != nil {
		t.Fatal(err)
	}
	rb, err := b.Checkpoints(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(ra, rb) {
		t.Fatalf("checkpoint logs differ\nfrom the last run: %+v\nfull:              %+v", ra, rb)
	}
	sa, err := a.CheckpointSizes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	sb, err := b.CheckpointSizes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(sa, sb) {
		t.Fatalf("checkpoint sizes differ\nfrom the last run: %v\nfull:              %v", sa, sb)
	}
	runA, okA, errA := a.LastMintRun(ctx, w)
	runB, okB, errB := b.LastMintRun(ctx, w)
	if errA != nil || errB != nil || runA != runB || okA != okB || runA != w {
		t.Fatalf("mint runs: %d %v %v / %d %v %v, want %d", runA, okA, errA, runB, okB, errB, w)
	}
}

// loseCheckpointValue removes the value a bucket's head names, as a store carried
// across without its values would, and returns its digest.
func (h *harness) loseCheckpointValue(t *testing.T, spanID string) string {
	t.Helper()
	head, ok := h.heads(t)[spanID]
	if !ok {
		t.Fatalf("no head for %s", spanID)
	}
	if err := opstest.LoseCheckpointValue(context.Background(), h.db, head.ValueHash); err != nil {
		t.Fatal(err)
	}
	return head.ValueHash
}

// A checkpoint whose value has gone is written again by the next mint, from
// the last run or in full, with no new row; a dry run only reports.
func TestRun_RestoresAMissingCheckpointValue(t *testing.T) {
	for _, tc := range []struct {
		name     string
		run      func(context.Context, mint.Deps, bool) (mint.Result, error)
		ingest   bool
		wantFull bool
	}{
		{name: "full", run: mint.RunFull, wantFull: true},
		{name: "from the last run, nothing new", run: mint.Run, wantFull: true},
		{name: "from the last run, a tail elsewhere", run: mint.Run, ingest: true, wantFull: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			h.deps.Budget = 1000
			h.seed(t)
			h.run(t, false)
			digest := h.loseCheckpointValue(t, "bucket:2026-08")
			if tc.ingest {
				h.bp(t, "2026-09-20T06:30:00Z", 120)
			}
			rows := len(h.rows(t))

			dry, err := tc.run(context.Background(), h.deps, true)
			if err != nil {
				t.Fatalf("dry run: %v", err)
			}
			if _, err := h.db.Size(context.Background(), mint.Kind, digest); !errors.Is(err, ops.ErrNotFound) {
				t.Fatalf("a dry run wrote %s (%v)", digest, err)
			}
			res, err := tc.run(context.Background(), h.deps, false)
			if err != nil {
				t.Fatalf("mint: %v", err)
			}
			if res.Full != tc.wantFull || dry.Full != tc.wantFull {
				t.Fatalf("full = %v (dry %v), want %v", res.Full, dry.Full, tc.wantFull)
			}
			if _, err := h.db.Size(context.Background(), mint.Kind, digest); err != nil {
				t.Fatalf("the missing value was not restored: %v", err)
			}
			if aug := outcomes(res)["bucket:2026-08"]; aug.Minted || aug.V != 1 {
				t.Fatalf("August re-minted for a missing value: %+v", aug)
			}
			// Each bucket re-minted replaced the version it superseded.
			if want := rows + res.Minted() - res.Collected.Checkpoints; len(h.rows(t)) != want {
				t.Fatalf("checkpoint rows %d, want %d", len(h.rows(t)), want)
			}
			readIsTheFold(t, h)
		})
	}
}
