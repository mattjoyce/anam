package mint

import (
	"context"
	"sort"

	"github.com/mattjoyce/anam/internal/fold"
	"github.com/mattjoyce/anam/internal/span"
	"github.com/mattjoyce/anam/internal/store/ledger"
	"github.com/mattjoyce/anam/internal/store/ops"
)

// The mint from the last run. A run at R checked every bucket, so the checkpoints at R
// are the value at R, and the only buckets whose value can differ at W are the ones the
// events after R touched (dirty, as a narrow read finds them). Those are rebuilt from
// their checkpoint and the tail, split if they are now over budget, and settled exactly
// as the full mint settles a bucket. Every other bucket is unchanged without being
// decoded, and the run is still a run that checked every bucket.
//
// One more kind of bucket can need work with no event touching it: one whose
// checkpoint is over the budget, because the budget was lowered or the last run minted
// at a fixed width. Its file's size says so without reading it, and it is rebuilt too.
//
// Failure and recovery (Armstrong): a mint that crashes leaves checkpoints no recorded
// run accounts for. Their covers_w cannot say so, since a run at the last run's W
// writes rows at that W too; the digest of the checkpoints the run took can
// (ops.LogSealed). The next mint finds the log unsealed and folds in full, which settles
// every bucket against the fold whatever the log holds, and records a run whose digest
// matches the log again. A checkpoint value that has gone is met
// as ops.ErrNotFound, which Run answers with the full mint, which writes the
// value again.

// fromLastRun mints from the last run at or below the head. ok is false when it
// cannot, and the caller folds in full: without a budget, the fixed width (and any
// weekly log) mints in full; without a run there is nothing to start from; and an
// unsealed log holds rows from a mint that crashed.
func (d Deps) fromLastRun(ctx context.Context, dryRun bool) (Result, bool, error) {
	if d.Budget <= 0 {
		return Result{}, false, nil
	}
	w, err := d.Ledger.Head(ctx)
	if err != nil {
		return Result{}, false, err
	}
	if sealed, err := d.Index.LogSealed(ctx); err != nil || !sealed {
		return Result{}, false, err
	}
	r, ok, err := d.Index.LastMintRun(ctx, int64(w))
	if err != nil || !ok {
		return Result{}, false, err
	}
	run := ledger.Watermark(r)
	heads, err := d.Index.SpanHeads(ctx)
	if err != nil {
		return Result{}, false, err
	}
	ids := make([]string, 0, len(heads))
	for id := range heads {
		ids = append(ids, id)
	}
	layout, err := span.LayoutOf(ids)
	if err != nil {
		return Result{}, false, nil // the full mint says why the log cannot subdivide
	}
	leaves := map[string]ops.CheckpointRow{}
	for id, row := range heads {
		b, _ := span.Parse(id) // LayoutOf has parsed every id
		if layout.Leaf(b) {
			leaves[id] = row
		}
	}

	tail, err := d.Ledger.Replay(ctx, run, w)
	if err != nil {
		return Result{}, false, err
	}
	dd, err := dirty(ctx, d.Ledger, layout, tail)
	if err != nil {
		return Result{}, false, err
	}
	parts, err := d.rebuild(ctx, leaves, dd)
	if err != nil {
		return Result{}, false, err
	}
	parts, layout, err = split(parts, layout, d.Budget)
	if err != nil {
		return Result{}, false, err
	}

	out := Result{W: w, Written: !dryRun, From: run}
	sizes, err := d.Index.CheckpointSizes(ctx)
	if err != nil {
		return Result{}, false, err
	}
	for _, id := range settledIDs(parts, leaves, layout) {
		head, minted := leaves[id]
		var o Outcome
		if entries, rebuilt := parts[id]; rebuilt {
			o, err = d.settle(ctx, w, id, entries, head, minted, dryRun)
		} else {
			o, err = d.unchanged(ctx, head, sizes, dryRun)
		}
		if err != nil {
			return Result{}, false, err
		}
		out.Outcomes = append(out.Outcomes, o)
	}
	return out, true, d.recordRun(ctx, w, dryRun)
}

// rebuild returns the entries at W of every bucket the tail dirtied or whose
// checkpoint is over budget: the checkpoint's entries less the facets the tail
// touched, plus the tail's own. A dirtied bucket with no checkpoint and nothing live
// is left out, as the full mint, which never sees it, leaves it out.
func (d Deps) rebuild(ctx context.Context, leaves map[string]ops.CheckpointRow, dd dirtied) (map[string][]fold.Entry, error) {
	ids := map[string]bool{}
	for id := range dd.buckets {
		ids[id] = true
	}
	for id, row := range leaves {
		if ids[id] {
			continue
		}
		n, err := d.Values.Size(ctx, Kind, row.ValueHash)
		if err != nil {
			return nil, err
		}
		if n > int64(d.Budget) {
			ids[id] = true
		}
	}

	parts := map[string][]fold.Entry{}
	for id := range ids {
		row, minted := leaves[id]
		entries := []fold.Entry{}
		if minted {
			v, err := Load(ctx, d, row)
			if err != nil {
				return nil, err
			}
			for _, e := range v.Entries() {
				if !dd.touched[e.FacetID] {
					entries = append(entries, e)
				}
			}
		}
		entries = append(entries, dd.entries[id]...)
		if minted || len(entries) > 0 {
			parts[id] = entries
		}
	}
	return parts, nil
}

// unchanged reports a bucket no event touched: its head stands. Its facet count comes
// from the log, or from decoding it once when the log predates counts.
func (d Deps) unchanged(ctx context.Context, head ops.CheckpointRow, sizes map[ops.SpanVersion]int, dryRun bool,
) (Outcome, error) {
	o := Outcome{SpanID: head.SpanID, V: head.V, CoversW: head.CoversW, ValueHash: head.ValueHash}
	n, ok := sizes[ops.SpanVersion{SpanID: head.SpanID, V: head.V}]
	if ok {
		o.Len = n
		return o, nil
	}
	v, err := Load(ctx, d, head)
	if err != nil {
		return Outcome{}, err
	}
	o.Len = v.Len()
	if !dryRun {
		if err := d.Index.AppendCheckpointSize(ctx, head.SpanID, head.V, o.Len); err != nil {
			return Outcome{}, err
		}
	}
	return o, nil
}

// settledIDs is every bucket rebuilt, and every checkpointed bucket still a leaf after
// the splits, sorted as the full mint sorts them.
func settledIDs(parts map[string][]fold.Entry, leaves map[string]ops.CheckpointRow, layout span.Layout) []string {
	ids := make([]string, 0, len(leaves))
	for id := range parts {
		ids = append(ids, id)
	}
	for id := range leaves {
		if _, rebuilt := parts[id]; rebuilt {
			continue
		}
		b, _ := span.Parse(id) // LayoutOf has parsed every id
		if layout.Leaf(b) {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}
