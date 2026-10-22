// Package mint cuts checkpoints: one content-addressed value of the live-only fold per
// bucket (ARCHITECTURE.md §7). A checkpoint exists to bound replay — it is a cache of
// the fold, never a second record — so everything here is derivable from the ledger
// and nothing here is a fact.
//
// Whether a bucket needs re-minting is decided by its value, not by bookkeeping about
// it: the bucket's value at the head watermark is built and its hash compared with the
// head checkpoint's. Equal means nothing a reader could see has changed, and nothing
// is written; different means a new version. A hash is exact where a watermark is only
// sufficient (journal: checkpoint-is-a-replay-floor — staleness is dedup'd by hash).
//
// A mint starts from the last run where it can (incremental.go): the buckets the tail
// after that run touched are the only ones whose value can differ, so only they are
// rebuilt and compared. The full mint folds the whole ledger and compares every
// bucket; it is the oracle the incremental one is tested against, and the repair if
// the log is ever wrong.
//
// The same rule covers the one awkward case: a facet whose fact-time moved between
// versions sits in a new bucket and has left its old one, so both hashes move and both
// buckets re-mint. No birth-bucket pinning, no second index.
//
// Failure and recovery (Armstrong): the value is written before its checkpoint row, and
// both are idempotent: a crash between them leaves an unreferenced, correctly keyed
// value that the re-run writes again (a no-op) and then references. A crash between
// buckets leaves the earlier buckets minted and the later ones for the re-run. No bucket
// is ever half-minted, because its value is one row and its checkpoint is one insert.
//
// A run that wrote ends by removing the checkpoints the newest run does not read
// (collect): a checkpoint is a cache, and a read that needs one that is gone folds past it.
package mint

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/mattjoyce/anam/internal/api"
	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/fold"
	"github.com/mattjoyce/anam/internal/span"
	"github.com/mattjoyce/anam/internal/store/ledger"
	"github.com/mattjoyce/anam/internal/store/ops"
)

// Kind is the kind of stored content checkpoint values are filed under.
const Kind = ops.KindCheckpoint

// Index is the checkpoint log the mint reads heads from and appends to.
type Index interface {
	PinLedger(ctx context.Context, ledgerID string) error
	SpanHeads(ctx context.Context) (map[string]ops.CheckpointRow, error)
	AppendCheckpoint(ctx context.Context, r ops.CheckpointRow) error
	AppendMintRun(ctx context.Context, w int64, ranAt string) error
	AppendCheckpointSize(ctx context.Context, span string, v, facets int) error
	LastMintRun(ctx context.Context, w int64) (int64, bool, error)
	LogSealed(ctx context.Context) (bool, error)
	CheckpointSizes(ctx context.Context) (map[ops.SpanVersion]int, error)
	CollectCheckpoints(ctx context.Context) error
}

// Store is where checkpoint values are written to and read back from: the operational
// store's content-addressed rows. Get checks the bytes against the digest that keys
// them.
type Store interface {
	Put(ctx context.Context, kind string, b []byte) (string, error)
	Get(ctx context.Context, kind, digest string) ([]byte, error)
	Size(ctx context.Context, kind, digest string) (int64, error)
}

// Ledger is the read surface the mint folds: fold's own, plus the ledger's identity so
// checkpoint rows cannot be appended to an index derived from another ledger.
type Ledger interface {
	fold.Ledger
	ID(ctx context.Context) (string, error)
}

// Clock supplies minted_at; tests freeze it.
type Clock interface{ Now() time.Time }

// Deps is everything a mint needs.
type Deps struct {
	Ledger  Ledger
	Index   Index
	Values  Store
	Buckets span.Buckets
	Clock   Clock
	// Budget, when positive, is the most bytes a bucket's value may hold before the
	// mint splits it: years into months, months into days (span.Layout). Zero mints
	// at the fixed width Buckets names.
	Budget int
}

// Outcome reports what happened to one bucket.
type Outcome struct {
	SpanID    string
	V         int
	CoversW   int64
	ValueHash string
	Len       int

	// Minted is true when a new version was cut; false when the head checkpoint's
	// hash already matched and nothing was written. V and CoversW describe the row
	// that stands after the run either way.
	Minted bool
}

// Result reports one mint run.
type Result struct {
	// W is the watermark every value was folded at.
	W ledger.Watermark

	// Outcomes holds one entry per bucket that holds a live facet or has ever been
	// minted, in bucket order.
	Outcomes []Outcome

	// Written is false for a dry run: everything was derived, nothing was published.
	Written bool

	// Full is true when the mint folded the whole ledger; false when it started from
	// the mint run at From and rebuilt only what the tail after it touched.
	Full bool
	From ledger.Watermark
}

// Minted counts the buckets that received a new version.
func (r Result) Minted() int {
	n := 0
	for _, o := range r.Outcomes {
		if o.Minted {
			n++
		}
	}
	return n
}

// Run mints every bucket whose value has changed since its last checkpoint, at the
// ledger's head watermark. It starts from the last mint run when it can, and folds the
// whole ledger when it cannot (fromLastRun). With dryRun set it derives everything and
// writes nothing — the same code path, so a dry run cannot flatter a real one.
func Run(ctx context.Context, d Deps, dryRun bool) (Result, error) {
	if err := d.pin(ctx); err != nil {
		return Result{}, err
	}
	res, ok, err := d.fromLastRun(ctx, dryRun)
	switch {
	case errors.Is(err, ops.ErrNotFound):
		// A checkpoint's value has gone: the full mint writes it again (settle).
	case err != nil:
		return res, err
	case ok:
		return d.collect(ctx, res, dryRun)
	}
	res, err = d.full(ctx, dryRun)
	if err != nil {
		return res, err
	}
	return d.collect(ctx, res, dryRun)
}

// RunFull is Run without the last run: it folds the whole ledger and compares every
// bucket's value with its head. Whatever the log holds, the checkpoints it leaves are
// the fold's.
func RunFull(ctx context.Context, d Deps, dryRun bool) (Result, error) {
	if err := d.pin(ctx); err != nil {
		return Result{}, err
	}
	res, err := d.full(ctx, dryRun)
	if err != nil {
		return res, err
	}
	return d.collect(ctx, res, dryRun)
}

// collect removes the checkpoints the newest run does not read, once a run that wrote
// has recorded itself. It is the run's last step and not part of what it minted: a
// failure leaves the run done and the log longer than it need be, and the next run
// collects. The error says so.
func (d Deps) collect(ctx context.Context, res Result, dryRun bool) (Result, error) {
	if dryRun {
		return res, nil
	}
	if err := d.Index.CollectCheckpoints(ctx); err != nil {
		return res, fmt.Errorf("mint: minted at W=%d, but removing the old checkpoints failed and the next mint will try again: %w", res.W, err)
	}
	return res, nil
}

// pin refuses a checkpoint log built from another ledger.
func (d Deps) pin(ctx context.Context) error {
	id, err := d.Ledger.ID(ctx)
	if err != nil {
		return err
	}
	return d.Index.PinLedger(ctx, id)
}

func (d Deps) full(ctx context.Context, dryRun bool) (Result, error) {
	res, err := fold.Read(ctx, d.Ledger)
	if err != nil {
		return Result{}, err
	}
	heads, err := d.Index.SpanHeads(ctx)
	if err != nil {
		return Result{}, err
	}
	parts, heads, err := d.partition(res.Value, heads)
	if err != nil {
		return Result{}, err
	}

	out := Result{W: res.W, Written: !dryRun, Full: true}
	for _, spanID := range spanIDs(parts, heads) {
		head, minted := heads[spanID]
		o, err := d.settle(ctx, res.W, spanID, parts[spanID], head, minted, dryRun)
		if err != nil {
			return Result{}, err
		}
		out.Outcomes = append(out.Outcomes, o)
	}
	return out, d.recordRun(ctx, res.W, dryRun)
}

// settle decides one bucket at w: unchanged when entries are the value its head
// checkpoint already holds, a new version otherwise.
func (d Deps) settle(ctx context.Context, w ledger.Watermark, spanID string, entries []fold.Entry,
	head ops.CheckpointRow, minted, dryRun bool,
) (Outcome, error) {
	value, err := fold.NewValue(entries)
	if err != nil {
		return Outcome{}, err
	}
	o := Outcome{SpanID: spanID, ValueHash: value.Hash(), Len: value.Len()}
	if minted && head.ValueHash == value.Hash() {
		o.V, o.CoversW = head.V, head.CoversW
		if dryRun {
			return o, nil
		}
		// A checkpoint minted before counts were recorded gets its count now; one
		// that has it keeps it.
		if err := d.Index.AppendCheckpointSize(ctx, spanID, o.V, o.Len); err != nil {
			return Outcome{}, err
		}
		// A checkpoint whose value has gone gets it back: the same bytes under the same
		// key, so the row that names it can be loaded again. No new version.
		if _, err := d.Values.Size(ctx, Kind, head.ValueHash); errors.Is(err, ops.ErrNotFound) {
			if _, err := d.Values.Put(ctx, Kind, value.Bytes()); err != nil {
				return Outcome{}, err
			}
		} else if err != nil {
			return Outcome{}, err
		}
		return o, nil
	}

	o.V, o.CoversW, o.Minted = head.V+1, int64(w), true
	if dryRun {
		return o, nil
	}
	if _, err := d.Values.Put(ctx, Kind, value.Bytes()); err != nil {
		return Outcome{}, err
	}
	row := ops.CheckpointRow{
		SpanID: spanID, V: o.V, CoversW: o.CoversW,
		ValueHash: o.ValueHash, MintedAt: clock.Format(d.Clock.Now()),
	}
	if err := d.Index.AppendCheckpoint(ctx, row); err != nil {
		return Outcome{}, err
	}
	if err := d.Index.AppendCheckpointSize(ctx, spanID, o.V, o.Len); err != nil {
		return Outcome{}, err
	}
	return o, nil
}

// recordRun records the run at w after its checkpoints, so a reader never starts from
// a run whose checkpoints are not all written. Every span was checked at w, minted or
// found unchanged.
func (d Deps) recordRun(ctx context.Context, w ledger.Watermark, dryRun bool) error {
	if dryRun {
		return nil
	}
	return d.Index.AppendMintRun(ctx, int64(w), clock.Format(d.Clock.Now()))
}

// Report runs a mint and reports it as the API's value, which the CLI renders whether
// it minted here or asked the daemon to.
func Report(ctx context.Context, d Deps, q api.MintQuery) (api.Mint, error) {
	run := Run
	if q.Full {
		run = RunFull
	}
	res, err := run(ctx, d, q.DryRun)
	if err != nil {
		return api.Mint{}, err
	}
	out := api.Mint{
		W: int64(res.W), Written: res.Written, Full: res.Full, From: int64(res.From),
		Outcomes: make([]api.MintOutcome, 0, len(res.Outcomes)),
	}
	for _, o := range res.Outcomes {
		out.Outcomes = append(out.Outcomes, api.MintOutcome{
			SpanID: o.SpanID, V: o.V, CoversW: o.CoversW, ValueHash: o.ValueHash, Len: o.Len, Minted: o.Minted,
		})
	}
	return out, nil
}

// spanIDs is the union of the buckets that hold a live facet and the buckets that have
// ever been minted, sorted. The second set matters when a bucket has emptied: its
// checkpoint must move to the empty value, or a composed read would still list the
// facets that left.
func spanIDs(parts map[string][]fold.Entry, heads map[string]ops.CheckpointRow) []string {
	seen := map[string]bool{}
	var ids []string
	for id := range parts {
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	for id := range heads {
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

// partition splits the value into the buckets to mint, and returns the heads that are
// still buckets. At a fixed width that is Partition. With a budget the layout is the
// one the log implies, any bucket over budget is split along the calendar until it
// fits or is a day, and the heads of buckets split earlier drop out: their children
// carry their facets now.
func (d Deps) partition(v fold.Value, heads map[string]ops.CheckpointRow,
) (map[string][]fold.Entry, map[string]ops.CheckpointRow, error) {
	if d.Budget <= 0 {
		parts, err := Partition(v, d.Buckets)
		return parts, heads, err
	}
	ids := make([]string, 0, len(heads))
	for id := range heads {
		ids = append(ids, id)
	}
	layout, err := span.LayoutOf(ids)
	if err != nil {
		return nil, nil, fmt.Errorf("mint: the checkpoint log cannot subdivide (%w); clear it and mint again", err)
	}
	parts, layout, err := subdivide(v, layout, d.Budget)
	if err != nil {
		return nil, nil, err
	}
	leaves := make(map[string]ops.CheckpointRow, len(heads))
	for id, row := range heads {
		b, _ := span.Parse(id) // LayoutOf has parsed every id
		if layout.Leaf(b) {
			leaves[id] = row
		}
	}
	return parts, leaves, nil
}

// subdivide partitions v by layout, splitting every bucket whose value is over budget
// bytes until none is or it is a day.
func subdivide(v fold.Value, layout span.Layout, budget int) (map[string][]fold.Entry, span.Layout, error) {
	parts, err := Partition(v, layout)
	if err != nil {
		return nil, layout, err
	}
	return split(parts, layout, budget)
}

// split divides every bucket of parts whose value is over budget bytes until none is or
// it is a day, and returns the layout that leaves. The size is the value's own
// serialisation, so the same facts always split the same way.
func split(parts map[string][]fold.Entry, layout span.Layout, budget int,
) (map[string][]fold.Entry, span.Layout, error) {
	for {
		var over []span.Bucket
		for id, entries := range parts {
			val, err := fold.NewValue(entries)
			if err != nil {
				return nil, layout, err
			}
			if len(val.Bytes()) <= budget {
				continue
			}
			b, err := span.Parse(id)
			if err != nil {
				return nil, layout, err
			}
			if next, ok := layout.Split(b); ok {
				layout = next
				over = append(over, b)
			}
		}
		if len(over) == 0 {
			return parts, layout, nil
		}
		for _, b := range over {
			entries := parts[b.ID()]
			delete(parts, b.ID())
			for _, e := range entries {
				t, err := clock.Parse(e.FactTime)
				if err != nil {
					return nil, layout, err
				}
				id := layout.Of(t).ID()
				parts[id] = append(parts[id], e)
			}
		}
	}
}

// Placer places a fact-time in a bucket: a fixed width (span.Buckets) or a
// subdivision (span.Layout).
type Placer interface {
	Of(t time.Time) span.Bucket
}

// Partition splits a value's entries by the bucket their fact-time falls in. Every
// entry lands in exactly one bucket, by arithmetic on fact_time and nothing else, so
// the partition is a function of the value and the width alone.
func Partition(v fold.Value, buckets Placer) (map[string][]fold.Entry, error) {
	parts := map[string][]fold.Entry{}
	for _, e := range v.Entries() {
		t, err := clock.Parse(e.FactTime)
		if err != nil {
			return nil, fmt.Errorf("mint: facet %s: fact_time %q: %w", e.FacetID, e.FactTime, err)
		}
		id := buckets.Of(t).ID()
		parts[id] = append(parts[id], e)
	}
	return parts, nil
}

// Compose stitches bucket values into one value — a derived span read (§4, §7). The
// result is byte-identical to a fold of the same facets, because NewValue imposes the
// one (fact_time, facet_id) order and buckets partition fact_time: composition is a
// union, never a merge.
func Compose(values ...fold.Value) (fold.Value, error) {
	var entries []fold.Entry
	for _, v := range values {
		entries = append(entries, v.Entries()...)
	}
	return fold.NewValue(entries)
}

// Load reads a checkpoint's value back from the store and rebuilds it, refusing bytes
// that do not reproduce the hash the row recorded. The store's own check covers the
// bytes against their key; this one covers the key against the row, so a row pointing
// at the wrong (intact) value is caught too.
func Load(ctx context.Context, d Deps, row ops.CheckpointRow) (fold.Value, error) {
	b, err := d.Values.Get(ctx, Kind, row.ValueHash)
	if err != nil {
		return fold.Value{}, err
	}
	var entries []fold.Entry
	if err := json.Unmarshal(b, &entries); err != nil {
		return fold.Value{}, fmt.Errorf("mint: checkpoint %s v%d: decode value: %w", row.SpanID, row.V, err)
	}
	v, err := fold.NewValue(entries)
	if err != nil {
		return fold.Value{}, err
	}
	if v.Hash() != row.ValueHash {
		return fold.Value{}, fmt.Errorf("mint: checkpoint %s v%d: value rebuilds to %s, row says %s",
			row.SpanID, row.V, v.Hash(), row.ValueHash)
	}
	return v, nil
}
