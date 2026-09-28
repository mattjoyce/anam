// Package catchup brings the operational store's derived tier up to the ledger's
// head by replay (ARCHITECTURE.md §8). It is the seam the substrate split created:
// the ledger and the operational store are two files, and no transaction spans two
// files. So the indexes are never written in the same breath as the ledger. They catch
// up afterwards, and a crash leaves them behind, never wrong.
//
// The replay is incremental. A facet_index row persists exactly a fold.State
// (current version + folded status), so new events advance the persisted states
// through fold.Apply — the same kind→status map the canonical fold uses, not a
// second one that could drift.
package catchup

import (
	"context"
	"errors"
	"fmt"

	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/fold"
	"github.com/mattjoyce/anam/internal/span"
	"github.com/mattjoyce/anam/internal/store/ledger"
	"github.com/mattjoyce/anam/internal/store/ops"
)

// ErrIndexAhead: the replay cursor exceeds the ledger's head. The derived tier
// claims to have seen events the ledger does not hold — that is not "behind"
// (fixable by replay); it is wrong, and wrong stops. The ledger pin catches a
// replaced ledger before the cursor is read, so reaching this means the ledger itself
// lost events.
var ErrIndexAhead = errors.New("catchup: replay cursor is ahead of the ledger head")

// Ledger is the read surface catch-up replays from.
type Ledger interface {
	ID(ctx context.Context) (string, error)
	Head(ctx context.Context) (ledger.Watermark, error)
	Replay(ctx context.Context, sinceW, uptoW ledger.Watermark) ([]facet.AnamEvent, error)
	Version(ctx context.Context, facetID string, version int) (facet.Facet, error)
}

// Ops is the derived tier catch-up writes to.
type Ops interface {
	PinLedger(ctx context.Context, ledgerID string) error
	LastSeq(ctx context.Context) (int64, error)
	FacetStates(ctx context.Context, ids []string) (map[string]fold.State, error)
	ApplyReplay(ctx context.Context, fromSeq, uptoSeq int64, events []ops.EventRow, facets []ops.FacetRow) error
}

// Result reports what a sync did.
type Result struct {
	// From is the cursor before the sync; To is the head it replayed to. Equal
	// means the tier was already current.
	From, To int64
}

// Sync replays anam-events in (cursor, head] into the indexes. Idempotent and
// crash-re-runnable: the rows and the cursor move in one transaction on the ops
// side, so a re-run resumes from wherever the last complete batch ended.
func Sync(ctx context.Context, l Ledger, o Ops, buckets span.Buckets) (Result, error) {
	// Which ledger, before how far. A cursor means nothing until it is known to count
	// this ledger's events.
	id, err := l.ID(ctx)
	if err != nil {
		return Result{}, err
	}
	if err := o.PinLedger(ctx, id); err != nil {
		return Result{}, err
	}

	last, err := o.LastSeq(ctx)
	if err != nil {
		return Result{}, err
	}
	head, err := l.Head(ctx)
	if err != nil {
		return Result{}, err
	}
	switch {
	case int64(head) == last:
		return Result{From: last, To: last}, nil
	case int64(head) < last:
		return Result{}, fmt.Errorf("%w: cursor %d, head %d", ErrIndexAhead, last, head)
	}

	events, err := l.Replay(ctx, ledger.Watermark(last), head)
	if err != nil {
		return Result{}, err
	}

	facets, err := advance(ctx, l, o, events, buckets)
	if err != nil {
		return Result{}, err
	}

	eventRows := make([]ops.EventRow, len(events))
	for i, ev := range events {
		eventRows[i] = ops.EventRow{
			Seq: ev.Seq, AnamTime: clock.Format(ev.AnamTime),
			Kind: ev.Kind, FacetID: ev.FacetID, ArtifactVersion: ev.ArtifactVersion,
		}
	}
	if err := o.ApplyReplay(ctx, last, int64(head), eventRows, facets); err != nil {
		return Result{}, err
	}
	return Result{From: last, To: int64(head)}, nil
}

// advance replays the new events over the persisted states and returns the changed
// facets' new index rows, each read at the version the replay derived. Ingest landing
// mid-sync is therefore invisible here: the artifact versions this replay names were
// written before it started and never change. Anything the new events did not reach is
// simply next run's work — the cursor moves with the rows, in one transaction.
func advance(ctx context.Context, l Ledger, o Ops, events []facet.AnamEvent, buckets span.Buckets) (rows []ops.FacetRow, err error) {
	var ids []string
	seen := map[string]bool{}
	for _, ev := range events {
		if !seen[ev.FacetID] {
			seen[ev.FacetID] = true
			ids = append(ids, ev.FacetID)
		}
	}

	states, err := o.FacetStates(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, ev := range events {
		cur, known := states[ev.FacetID]
		next, err := fold.Apply(cur, known, ev)
		if err != nil {
			return nil, err
		}
		states[ev.FacetID] = next
	}

	rows = make([]ops.FacetRow, 0, len(ids))
	for _, id := range ids {
		st := states[id]
		f, err := l.Version(ctx, id, st.Version)
		if err != nil {
			return nil, fmt.Errorf("%w: facet %s replayed to v%d: %v",
				fold.ErrCorruptStream, id, st.Version, err)
		}
		rows = append(rows, ops.FacetRow{
			FacetID: id, Type: f.Type,
			FactTime: clock.Format(f.FactTime), BucketID: buckets.Of(f.FactTime).ID(),
			Version: st.Version, Status: st.Status, ContentHash: f.Descriptor.ContentHash,
		})
	}
	return rows, nil
}
