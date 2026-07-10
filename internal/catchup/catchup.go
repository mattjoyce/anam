// Package catchup brings the operational store's derived tier up to the ledger's
// head by replay (ARCHITECTURE.md §8). It is the seam the substrate split created:
// there is no transaction across DynamoDB and SQLite, so the indexes are never
// written in the same breath as the ledger — they catch up afterwards, and a crash
// leaves them behind, never wrong.
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
// (fixable by replay); it is wrong, likely a cursor pointed at the wrong table,
// and wrong stops.
var ErrIndexAhead = errors.New("catchup: replay cursor is ahead of the ledger head")

// ErrOutpaced: ingest kept landing between the replay and the artifact reads on
// every attempt. Nothing was written; re-run.
var ErrOutpaced = errors.New("catchup: ledger advanced past the sync on every attempt")

const syncAttempts = 3

// Ledger is the read surface catch-up replays from.
type Ledger interface {
	Head(ctx context.Context) (ledger.Watermark, error)
	Replay(ctx context.Context, sinceW, uptoW ledger.Watermark) ([]facet.RecordEvent, error)
	CurrentVersion(ctx context.Context, facetID string) (facet.Facet, error)
}

// Ops is the derived tier catch-up writes to.
type Ops interface {
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

// Sync replays record-events in (cursor, head] into the indexes. Idempotent and
// crash-re-runnable: the rows and the cursor move in one transaction on the ops
// side, so a re-run resumes from wherever the last complete batch ended.
func Sync(ctx context.Context, l Ledger, o Ops) (Result, error) {
	for attempt := 0; attempt < syncAttempts; attempt++ {
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

		facets, outpaced, err := advance(ctx, l, o, events)
		if err != nil {
			return Result{}, err
		}
		if outpaced {
			continue
		}

		eventRows := make([]ops.EventRow, len(events))
		for i, ev := range events {
			eventRows[i] = ops.EventRow{
				Seq: ev.Seq, RecordTime: clock.Format(ev.RecordTime),
				Kind: ev.Kind, FacetID: ev.FacetID, ArtifactVersion: ev.ArtifactVersion,
			}
		}
		if err := o.ApplyReplay(ctx, last, int64(head), eventRows, facets); err != nil {
			return Result{}, err
		}
		return Result{From: last, To: int64(head)}, nil
	}
	return Result{}, fmt.Errorf("%w: %d attempts", ErrOutpaced, syncAttempts)
}

// advance replays the new events over the persisted states and returns the changed
// facets' new index rows. outpaced reports an artifact read that saw a version the
// events cannot have — ingest landed mid-sync; the caller re-runs from a fresh head.
func advance(ctx context.Context, l Ledger, o Ops, events []facet.RecordEvent) (rows []ops.FacetRow, outpaced bool, err error) {
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
		return nil, false, err
	}
	for _, ev := range events {
		cur, known := states[ev.FacetID]
		next, err := fold.Apply(cur, known, ev)
		if err != nil {
			return nil, false, err
		}
		states[ev.FacetID] = next
	}

	rows = make([]ops.FacetRow, 0, len(ids))
	for _, id := range ids {
		st := states[id]
		f, err := l.CurrentVersion(ctx, id)
		if err != nil {
			return nil, false, err
		}
		switch {
		case f.ArtifactVersion > st.Version:
			return nil, true, nil
		case f.ArtifactVersion < st.Version:
			return nil, false, fmt.Errorf("%w: facet %s replayed to v%d but the ledger holds v%d",
				fold.ErrCorruptStream, id, st.Version, f.ArtifactVersion)
		}
		rows = append(rows, ops.FacetRow{
			FacetID: id, Type: f.Type,
			EventTime: clock.Format(f.EventTime), BucketID: span.Of(f.EventTime).ID(),
			Version: st.Version, Status: st.Status, ContentHash: f.Descriptor.ContentHash,
		})
	}
	return rows, false, nil
}
