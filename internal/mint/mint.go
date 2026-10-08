// Package mint cuts checkpoints: one content-addressed value of the live-only fold per
// bucket (ARCHITECTURE.md §7). A checkpoint exists to bound replay — it is a cache of
// the fold, never a second record — so everything here is derivable from the ledger
// and nothing here is a fact.
//
// Whether a bucket needs re-minting is decided by its value, not by bookkeeping about
// it: the bucket's value at the head watermark is built and its hash compared with the
// head checkpoint's. Equal means nothing a reader could see has changed, and nothing
// is written; different means a new version. The architecture's `W > Wᵥ` comparison is
// the cheap pre-filter an incremental mint would add in front of this; it is not needed
// while the mint folds the whole person, and a hash is exact where a watermark is only
// sufficient (journal: checkpoint-is-a-replay-floor — staleness is dedup'd by hash).
//
// The same rule covers the one awkward case: a facet whose fact-time moved between
// versions sits in a new bucket and has left its old one, so both hashes move and both
// buckets re-mint. No birth-bucket pinning, no second index.
//
// Failure and recovery (Armstrong): the value file is written before its checkpoint
// row, and both are idempotent — a crash between them leaves an unreferenced,
// correctly named file that the re-run writes again (a no-op) and then references. A
// crash between buckets leaves the earlier buckets minted and the later ones for the
// re-run. No bucket is ever half-minted, because its value is one file and its row is
// one insert.
package mint

import (
	"context"
	"encoding/json"
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

// Kind is the artifact-store subdirectory checkpoint values are filed under.
const Kind = "checkpoint"

// Index is the checkpoint log the mint reads heads from and appends to.
type Index interface {
	PinLedger(ctx context.Context, ledgerID string) error
	SpanHeads(ctx context.Context) (map[string]ops.CheckpointRow, error)
	AppendCheckpoint(ctx context.Context, r ops.CheckpointRow) error
}

// Store is the published-artifact tier the values are written to and read back from.
type Store interface {
	Put(kind string, b []byte) (string, error)
	Get(kind, digest string) ([]byte, error)
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
	Ledger    Ledger
	Index     Index
	Artifacts Store
	Buckets   span.Buckets
	Clock     Clock
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
// ledger's head watermark. With dryRun set it derives everything and writes nothing —
// the same code path, so a dry run cannot flatter a real one.
func Run(ctx context.Context, d Deps, dryRun bool) (Result, error) {
	id, err := d.Ledger.ID(ctx)
	if err != nil {
		return Result{}, err
	}
	if err := d.Index.PinLedger(ctx, id); err != nil {
		return Result{}, err
	}

	res, err := fold.Read(ctx, d.Ledger)
	if err != nil {
		return Result{}, err
	}
	heads, err := d.Index.SpanHeads(ctx)
	if err != nil {
		return Result{}, err
	}
	parts, err := Partition(res.Value, d.Buckets)
	if err != nil {
		return Result{}, err
	}

	out := Result{W: res.W, Written: !dryRun}
	for _, spanID := range spanIDs(parts, heads) {
		value, err := fold.NewValue(parts[spanID])
		if err != nil {
			return Result{}, err
		}
		head, minted := heads[spanID]
		o := Outcome{SpanID: spanID, ValueHash: value.Hash(), Len: value.Len()}
		if minted && head.ValueHash == value.Hash() {
			o.V, o.CoversW = head.V, head.CoversW
			out.Outcomes = append(out.Outcomes, o)
			continue
		}

		o.V, o.CoversW, o.Minted = head.V+1, int64(res.W), true
		out.Outcomes = append(out.Outcomes, o)
		if dryRun {
			continue
		}
		if _, err := d.Artifacts.Put(Kind, value.Bytes()); err != nil {
			return Result{}, err
		}
		row := ops.CheckpointRow{
			SpanID: spanID, V: o.V, CoversW: o.CoversW,
			ValueHash: o.ValueHash, MintedAt: clock.Format(d.Clock.Now()),
		}
		if err := d.Index.AppendCheckpoint(ctx, row); err != nil {
			return Result{}, err
		}
	}
	return out, nil
}

// Report runs a mint and reports it as the API's value, which the CLI renders whether
// it minted here or asked the daemon to.
func Report(ctx context.Context, d Deps, dryRun bool) (api.Mint, error) {
	res, err := Run(ctx, d, dryRun)
	if err != nil {
		return api.Mint{}, err
	}
	out := api.Mint{W: int64(res.W), Written: res.Written, Outcomes: make([]api.MintOutcome, 0, len(res.Outcomes))}
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

// Partition splits a value's entries by the bucket their fact-time falls in. Every
// entry lands in exactly one bucket, by arithmetic on fact_time and nothing else, so
// the partition is a function of the value and the width alone.
func Partition(v fold.Value, buckets span.Buckets) (map[string][]fold.Entry, error) {
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

// Load reads a checkpoint's value back from the artifact tier and rebuilds it,
// refusing a file whose bytes do not reproduce the hash the row recorded. The store's
// own check covers the file against its name; this one covers the name against the row,
// so a row pointing at the wrong (intact) file is caught too.
func Load(d Deps, row ops.CheckpointRow) (fold.Value, error) {
	b, err := d.Artifacts.Get(Kind, row.ValueHash)
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
