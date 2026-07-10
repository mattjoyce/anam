package fold

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

// ErrOutpaced is returned when the ledger kept advancing between the replay and the
// artifact reads on every attempt. The read is simply re-run; nothing was written.
var ErrOutpaced = errors.New("fold: ledger advanced past the read on every attempt")

// readAttempts bounds the read-races-ingest retry. Reads are cheap and writes are
// rare; three losses in a row means something is churning and the caller should know.
const readAttempts = 3

// Ledger is the read surface the fold needs — the ledger's two access paths plus the
// head (interfaces at the point of use; *ledger.Ledger satisfies it).
type Ledger interface {
	Head(ctx context.Context) (ledger.Watermark, error)
	Replay(ctx context.Context, sinceW, uptoW ledger.Watermark) ([]facet.RecordEvent, error)
	CurrentVersion(ctx context.Context, facetID string) (facet.Facet, error)
}

// Result is one consistent read of the whole person at a watermark.
type Result struct {
	// W is the watermark the read is true at.
	W ledger.Watermark

	// States holds every logical facet's folded state — the dead included, which is
	// what lets a caller distinguish "tombstoned" from "never admitted".
	States map[string]State

	// Live holds the live facets at their current artifact versions, in
	// (event-time, id) order — the whole-person timeline, un-narrowed.
	Live []facet.Facet

	// Value is the canonical built from Live: the live-only value at W.
	Value Value
}

// Read produces the canonical at the head watermark: head → replay → fold → fetch
// each live artifact. The whole-person read is the default; narrowing is the
// caller's lens over Live, never a different read (§4).
//
// The fetch races ingest: a supersession landing between the replay and a
// CurrentVersion call returns an artifact version the folded events cannot have
// seen. That mismatch is detected and the read re-runs from a fresh head — bounded,
// like the gate's write retry. A version *behind* the fold is different: artifact
// and event are written atomically, so an event without its artifact means the
// ledger is corrupt, and the read stops rather than papers over it.
func Read(ctx context.Context, l Ledger) (Result, error) {
	for attempt := 0; attempt < readAttempts; attempt++ {
		head, err := l.Head(ctx)
		if err != nil {
			return Result{}, err
		}
		events, err := l.Replay(ctx, 0, head)
		if err != nil {
			return Result{}, err
		}
		states, err := Fold(events)
		if err != nil {
			return Result{}, err
		}

		live, outpaced, err := fetchLive(ctx, l, states)
		if err != nil {
			return Result{}, err
		}
		if outpaced {
			continue
		}

		entries := make([]Entry, len(live))
		for i, f := range live {
			entries[i] = entryOf(f, states[f.ID])
		}
		value, err := NewValue(entries)
		if err != nil {
			return Result{}, err
		}
		return Result{W: head, States: states, Live: live, Value: value}, nil
	}
	return Result{}, fmt.Errorf("%w: %d attempts", ErrOutpaced, readAttempts)
}

// fetchLive reads each live facet's current artifact and verifies it is the version
// the fold derived. outpaced reports the benign race (ledger moved on); an artifact
// behind its own events is returned as a corruption error instead.
func fetchLive(ctx context.Context, l Ledger, states map[string]State) (live []facet.Facet, outpaced bool, err error) {
	for id, st := range states {
		if !st.Live() {
			continue
		}
		f, err := l.CurrentVersion(ctx, id)
		if err != nil {
			return nil, false, err
		}
		switch {
		case f.ArtifactVersion == st.Version:
			live = append(live, f)
		case f.ArtifactVersion > st.Version:
			return nil, true, nil
		default:
			return nil, false, fmt.Errorf("%w: facet %s folded to v%d but the ledger holds v%d",
				ErrCorruptStream, id, st.Version, f.ArtifactVersion)
		}
	}
	sort.Slice(live, func(i, j int) bool {
		if !live[i].EventTime.Equal(live[j].EventTime) {
			return live[i].EventTime.Before(live[j].EventTime)
		}
		return live[i].ID < live[j].ID
	})
	return live, false, nil
}
