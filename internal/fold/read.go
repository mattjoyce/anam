package fold

import (
	"context"
	"fmt"
	"sort"

	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

// Ledger is the read surface the fold needs — the ledger's replay path, its versioned
// artifact lookup, and the head (interfaces at the point of use; *ledger.Ledger
// satisfies it).
type Ledger interface {
	Head(ctx context.Context) (ledger.Watermark, error)
	Replay(ctx context.Context, sinceW, uptoW ledger.Watermark) ([]facet.RecordEvent, error)
	Version(ctx context.Context, facetID string, version int) (facet.Facet, error)
}

// Result is one consistent read of the whole person at a watermark.
type Result struct {
	// W is the watermark the read is true at.
	W ledger.Watermark

	// States holds every logical facet's folded state — the dead included, which is
	// what lets a caller distinguish "tombstoned" from "never admitted".
	States map[string]State

	// Live holds the live facets at the artifact versions in force at W, in
	// (event-time, id) order — the whole-person timeline, un-narrowed.
	Live []facet.Facet

	// Value is the canonical built from Live: the live-only value at W.
	Value Value
}

// Read produces the canonical at the head watermark.
//
// The whole-person read is the default; narrowing is the caller's lens over Live,
// never a different read (§4).
func Read(ctx context.Context, l Ledger) (Result, error) {
	head, err := l.Head(ctx)
	if err != nil {
		return Result{}, err
	}
	return ReadAt(ctx, l, head)
}

// ReadAt produces the canonical as it stood at watermark w: replay → fold → fetch each
// live facet at the version the fold derived. Pass the head watermark for the current
// value, a historical one for the as-of value — same fold, different upper bound (§9).
//
// There is no retry here, and there used to be one. While the fetch asked the ledger
// for each facet's *current* version, a supersession landing between the replay and
// the fetch returned an artifact the folded events could not have seen, and the read
// had to notice and start again. Asking instead for the version the fold derived
// removes the race rather than handling it: an artifact version is immutable once
// written, so the answer does not depend on when it is asked. The two questions —
// which facets are live, and which version of each — were braided into one lookup;
// separating them is what makes an as-of read possible at all, and the retry loop was
// the cost of the braid.
//
// A missing artifact is still fatal. Artifact and record-event are written in one
// transaction, so an event whose artifact is absent means the ledger is corrupt, and
// the read stops rather than papering over it.
func ReadAt(ctx context.Context, l Ledger, w ledger.Watermark) (Result, error) {
	events, err := l.Replay(ctx, 0, w)
	if err != nil {
		return Result{}, err
	}
	states, err := Fold(events)
	if err != nil {
		return Result{}, err
	}

	live, err := fetchLive(ctx, l, states)
	if err != nil {
		return Result{}, err
	}

	entries := make([]Entry, len(live))
	for i, f := range live {
		entries[i] = entryOf(f, states[f.ID])
	}
	value, err := NewValue(entries)
	if err != nil {
		return Result{}, err
	}
	return Result{W: w, States: states, Live: live, Value: value}, nil
}

// fetchWorkers bounds the concurrent artifact reads. The whole-person read is one
// fetch per live facet; serially that priced a 659-facet person at minutes. The reads
// are independent and strongly consistent, so ordering is imposed afterwards by sort,
// not by the fetch.
const fetchWorkers = 16

// fetchLive reads each live facet at the exact version the fold derived.
func fetchLive(ctx context.Context, l Ledger, states map[string]State) ([]facet.Facet, error) {
	ids := make([]string, 0, len(states))
	for id, st := range states {
		if st.Live() {
			ids = append(ids, id)
		}
	}

	type fetched struct {
		f   facet.Facet
		err error
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel() // stops in-flight workers once a bad result lands

	work := make(chan string)
	results := make(chan fetched)
	for range fetchWorkers {
		go func() {
			for id := range work {
				f, err := l.Version(ctx, id, states[id].Version)
				if err != nil {
					err = fmt.Errorf("%w: facet %s folded to v%d: %v",
						ErrCorruptStream, id, states[id].Version, err)
				}
				select {
				case results <- fetched{f: f, err: err}:
				case <-ctx.Done():
					return
				}
			}
		}()
	}
	go func() {
		defer close(work)
		for _, id := range ids {
			select {
			case work <- id:
			case <-ctx.Done():
				return
			}
		}
	}()

	live := make([]facet.Facet, 0, len(ids))
	for range ids {
		r := <-results
		if r.err != nil {
			return nil, r.err
		}
		live = append(live, r.f)
	}

	sort.Slice(live, func(i, j int) bool {
		if !live[i].EventTime.Equal(live[j].EventTime) {
			return live[i].EventTime.Before(live[j].EventTime)
		}
		return live[i].ID < live[j].ID
	})
	return live, nil
}
