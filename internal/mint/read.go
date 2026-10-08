package mint

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/fold"
	"github.com/mattjoyce/anam/internal/span"
	"github.com/mattjoyce/anam/internal/store/ledger"
	"github.com/mattjoyce/anam/internal/store/ops"
)

// The read side of the mint: the canonical at a watermark, composed from the
// checkpoints a mint run left and the tail of events after it. This is what minting is
// for (ARCHITECTURE.md §7): a read costs the checkpoints it loads and its tail, never a
// replay from the beginning.

// ErrOtherLedger is returned when the checkpoint log was built from a different ledger
// than the one being read. Its checkpoints are values of another record.
var ErrOtherLedger = errors.New("mint: checkpoints belong to another ledger")

// CheckpointLog is what a composed read needs of the operational store.
type CheckpointLog interface {
	LedgerID(ctx context.Context) (string, error)
	LastMintRun(ctx context.Context, w int64) (int64, bool, error)
	CheckpointsAt(ctx context.Context, w int64) ([]ops.CheckpointRow, error)
}

// ReadLedger is the ledger surface a composed read needs: the tail, the artifacts the
// tail names, and the ledger's identity.
type ReadLedger interface {
	ID(ctx context.Context) (string, error)
	Replay(ctx context.Context, sinceW, uptoW ledger.Watermark) ([]facet.AnamEvent, error)
	Versions(ctx context.Context, refs []ledger.VersionRef) ([]facet.Facet, error)
}

// Composed is the canonical at W, and how it was reached.
type Composed struct {
	W     ledger.Watermark
	Value fold.Value
	// Run is the mint run the read started from; Checkpoints how many it loaded.
	Run         ledger.Watermark
	Checkpoints int
	// Tail is how many events after Run were applied, touching Touched facets.
	Tail, Touched int
	// Versions is the artifact version of each facet the tail touched and left live.
	// The facets taken from checkpoints are not in it: a checkpoint holds values, not
	// versions.
	Versions map[string]int
}

// ReadAt composes the canonical at w from the latest mint run at or below w and the
// events after it. ok is false when no run is at or below w, or the log has never been
// pinned to a ledger: there is nothing to start from, and the caller folds in full.
//
// A facet the tail touched is dropped from whichever checkpoint held it, wherever its
// fact-time put it then, and is added back at the version its last tail event names if
// that event leaves it live. That is the whole difference between a checkpoint at the
// run and the value at w, so the result is the fold at w (the oracle test holds it to
// that).
func ReadAt(ctx context.Context, l ReadLedger, log CheckpointLog, artifacts Store, w ledger.Watermark,
) (Composed, bool, error) {
	run, ok, err := startingRun(ctx, l, log, w)
	if err != nil || !ok {
		return Composed{}, false, err
	}
	rows, _, ok, err := leavesAt(ctx, log, run)
	if err != nil || !ok {
		return Composed{}, false, err
	}
	byID := map[string]fold.Entry{}
	for _, row := range rows {
		if err := loadInto(artifacts, row, byID); err != nil {
			return Composed{}, false, err
		}
	}

	tail, err := l.Replay(ctx, run, w)
	if err != nil {
		return Composed{}, false, err
	}
	versions, err := applyTail(ctx, l, tail, byID)
	if err != nil {
		return Composed{}, false, err
	}

	entries := make([]fold.Entry, 0, len(byID))
	for _, e := range byID {
		entries = append(entries, e)
	}
	value, err := fold.NewValue(entries)
	if err != nil {
		return Composed{}, false, err
	}
	return Composed{
		W: w, Value: value, Run: run, Checkpoints: len(rows), Tail: len(tail),
		Touched: len(versions.touched), Versions: versions.live,
	}, true, nil
}

// startingRun is the latest mint run at or below w, once the log is known to describe
// this ledger.
func startingRun(ctx context.Context, l ReadLedger, log CheckpointLog, w ledger.Watermark,
) (ledger.Watermark, bool, error) {
	pinned, err := log.LedgerID(ctx)
	if err != nil || pinned == "" {
		return 0, false, err
	}
	id, err := l.ID(ctx)
	if err != nil {
		return 0, false, err
	}
	if pinned != id {
		return 0, false, fmt.Errorf("%w: the log was built from %s, this is %s", ErrOtherLedger, pinned, id)
	}
	run, ok, err := log.LastMintRun(ctx, int64(w))
	return ledger.Watermark(run), ok, err
}

// leavesAt returns the checkpoints at run of the buckets the layout then held, and that
// layout. A bucket split since its checkpoint was minted has children with their own,
// so its row is history, not part of the value. ok is false for a log whose ids do
// not subdivide (a weekly width): the caller reads the full fold instead.
func leavesAt(ctx context.Context, log CheckpointLog, run ledger.Watermark,
) ([]ops.CheckpointRow, span.Layout, bool, error) {
	rows, err := log.CheckpointsAt(ctx, int64(run))
	if err != nil {
		return nil, span.Layout{}, false, err
	}
	ids := make([]string, len(rows))
	for i, r := range rows {
		ids[i] = r.SpanID
	}
	layout, err := span.LayoutOf(ids)
	if err != nil {
		return nil, span.Layout{}, false, nil
	}
	leaves := rows[:0:0]
	for _, r := range rows {
		b, _ := span.Parse(r.SpanID) // LayoutOf has parsed every id
		if layout.Leaf(b) {
			leaves = append(leaves, r)
		}
	}
	return leaves, layout, true, nil
}

// loadInto adds a checkpoint's entries to byID. The artifact store checks the file's
// bytes against the hash the row names, so a tampered or missing checkpoint fails the
// read rather than answering from something else.
func loadInto(artifacts Store, row ops.CheckpointRow, byID map[string]fold.Entry) error {
	b, err := artifacts.Get(Kind, row.ValueHash)
	if err != nil {
		return fmt.Errorf("mint: checkpoint %s v%d: %w", row.SpanID, row.V, err)
	}
	var entries []fold.Entry
	if err := json.Unmarshal(b, &entries); err != nil {
		return fmt.Errorf("mint: checkpoint %s v%d: decode value: %w", row.SpanID, row.V, err)
	}
	for _, e := range entries {
		byID[e.FacetID] = e
	}
	return nil
}

// tailVersions is what the tail left: every facet it touched, and the version of those
// it left live.
type tailVersions struct {
	touched map[string]bool
	live    map[string]int
}

// applyTail replaces every facet the tail touched with its state after the tail.
func applyTail(ctx context.Context, l ReadLedger, tail []facet.AnamEvent, byID map[string]fold.Entry,
) (tailVersions, error) {
	last := map[string]fold.State{}
	for _, ev := range tail {
		st, err := fold.StateAfter(ev)
		if err != nil {
			return tailVersions{}, err
		}
		last[ev.FacetID] = st
	}
	out := tailVersions{touched: map[string]bool{}, live: map[string]int{}}
	refs := make([]ledger.VersionRef, 0, len(last))
	for id, st := range last {
		out.touched[id] = true
		delete(byID, id)
		if st.Live() {
			refs = append(refs, ledger.VersionRef{FacetID: id, Version: st.Version})
			out.live[id] = st.Version
		}
	}
	facets, err := l.Versions(ctx, refs)
	if err != nil {
		return tailVersions{}, fmt.Errorf("%w: %v", fold.ErrCorruptStream, err)
	}
	for _, f := range facets {
		byID[f.ID] = fold.EntryOf(f, last[f.ID])
	}
	return out, nil
}
