package read

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/fold"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

// FacetHistory is one facet's events, which absence needs to tell a facet untouched
// since a snapshot from one a later run sent again. *ledger.Ledger satisfies it.
type FacetHistory interface {
	FacetEvents(ctx context.Context, facetID string, uptoW ledger.Watermark) ([]facet.AnamEvent, error)
	Head(ctx context.Context) (ledger.Watermark, error)
}

// SnapshotScope is what a complete snapshot speaks for: one source's facets of some
// types, as they stood when the run began. StartedW is the watermark just after its
// first part was admitted.
type SnapshotScope struct {
	Source   string
	Types    []string
	Carried  map[string]bool
	StartedW ledger.Watermark
	// MaxJudged bounds how many uncarried live facets Absent will judge, each one a
	// query for that facet's events; past it Absent stops with ErrTooManyAbsent. Zero
	// is unbounded, which the CLI accepts because --max-fraction bounds what follows;
	// the API sets it so one request cannot run thousands of queries (card 100).
	MaxJudged int
}

// ErrTooManyAbsent is returned when a snapshot leaves more live facets uncarried than
// the scope allows judging in one read. The count is still answered; which ones is
// a question for the CLI, which runs it locally and unbounded.
var ErrTooManyAbsent = errors.New("read: too many uncarried facets to judge in one read")

// Absent is what a complete snapshot says the record holds and its source no longer
// does: the live facets of the snapshot's source and types that it did not carry, and
// whose last event is at or before the run began. A facet touched after that was sent
// by the run itself or by an incremental run beside it (card 94), so it is never
// called absent. live is how many facets of the source and types are live now.
// Entries are in timeline order.
//
// It reports and nothing else: a tombstone is an operator's deliberate step (card 17,
// card 78).
func Absent(ctx context.Context, src Source, hist FacetHistory, scope SnapshotScope) (absent []fold.Entry, live int, err error) {
	res, err := Canonical(ctx, src)
	if err != nil {
		return nil, 0, err
	}
	head, err := hist.Head(ctx)
	if err != nil {
		return nil, 0, err
	}
	types := make(map[facet.Type]bool, len(scope.Types))
	for _, t := range scope.Types {
		types[facet.Type(t)] = true
	}
	judged := 0
	for _, e := range res.Value.Entries() {
		if e.Source != scope.Source || !types[e.Type] {
			continue
		}
		live++
		if scope.Carried[e.FacetID] {
			continue
		}
		judged++
		if scope.MaxJudged > 0 && judged > scope.MaxJudged {
			return nil, live, fmt.Errorf("%w: more than %d of %s's %d live facets were not carried; run anam retract %s locally to list them",
				ErrTooManyAbsent, scope.MaxJudged, scope.Source, live, scope.Source)
		}
		events, err := hist.FacetEvents(ctx, e.FacetID, head)
		if err != nil {
			return nil, 0, err
		}
		if n := len(events); n > 0 && ledger.Watermark(events[n-1].Seq) > scope.StartedW {
			continue // touched since the run began: not its to judge
		}
		absent = append(absent, e)
	}
	sort.SliceStable(absent, func(i, j int) bool {
		if absent[i].FactTime != absent[j].FactTime {
			return absent[i].FactTime < absent[j].FactTime
		}
		return absent[i].FacetID < absent[j].FacetID
	})
	return absent, live, nil
}
