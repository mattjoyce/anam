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
	Replay(ctx context.Context, sinceW, uptoW ledger.Watermark) ([]facet.AnamEvent, error)
	Version(ctx context.Context, facetID string, version int) (facet.Facet, error)
	Versions(ctx context.Context, refs []ledger.VersionRef) ([]facet.Facet, error)
}

// Result is one consistent read of the whole person at a watermark.
type Result struct {
	// W is the watermark the read is true at.
	W ledger.Watermark

	// States holds every logical facet's folded state — the dead included, which is
	// what lets a caller distinguish "tombstoned" from "never admitted".
	States map[string]State

	// Live holds the live facets at the artifact versions in force at W, in
	// (fact-time, id) order — the whole-person timeline, un-narrowed.
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
// A missing artifact is still fatal. Artifact and anam-event are written in one
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
		entries[i] = EntryOf(f, states[f.ID])
	}
	value, err := NewValue(entries)
	if err != nil {
		return Result{}, err
	}
	return Result{W: w, States: states, Live: live, Value: value}, nil
}

// fetchLive reads each live facet at the exact version the fold derived, in batches
// rather than one query per facet: a read of the whole person fetches every live
// artifact, and a query each priced 14,000 facets at most of a second in SQL parsing
// alone.
func fetchLive(ctx context.Context, l Ledger, states map[string]State) ([]facet.Facet, error) {
	refs := make([]ledger.VersionRef, 0, len(states))
	for id, st := range states {
		if st.Live() {
			refs = append(refs, ledger.VersionRef{FacetID: id, Version: st.Version})
		}
	}
	live, err := l.Versions(ctx, refs)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCorruptStream, err)
	}

	sort.Slice(live, func(i, j int) bool {
		if !live[i].FactTime.Equal(live[j].FactTime) {
			return live[i].FactTime.Before(live[j].FactTime)
		}
		return live[i].ID < live[j].ID
	})
	return live, nil
}
