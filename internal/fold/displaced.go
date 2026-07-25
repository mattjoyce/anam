package fold

import (
	"context"
	"fmt"
	"sort"

	"github.com/mattjoyce/anam/internal/facet"
)

// Displacement is one artifact version that is *not* in force at a watermark, together
// with why it is not.
//
// These are the versions a projection never sees. The canonical is live-only by
// construction (§9), so nothing here ever reaches a template and none of it can carry
// an exclusion reason from one — "superseded" is a standing of its own, not a verdict.
// Anyone asking "what did this projection stand on, and what has moved since?" needs
// both halves, which is why this lives beside the fold rather than in whichever viewer
// asked first.
type Displacement struct {
	Facet   facet.Facet
	Version int

	// Reason distinguishes the two ways a version stops being in force: a later
	// version displaced it, or the whole facet is no longer live.
	Reason string
}

// Displaced lists every artifact version below each facet's current one, plus the
// current version of any facet that is not live.
//
// It asks the record-event stream nothing extra: a facet folded to v3 has necessarily
// had v1 and v2 displaced, and the fold already derived that 3. Only the artifacts
// themselves are fetched, which is what lets a caller show the reading that was
// corrected rather than merely that something was.
//
// The reads are one round-trip per displaced version. That is proportional to how much
// the record has been corrected, not to how large it is — a person with no corrections
// pays nothing here.
func Displaced(ctx context.Context, l Ledger, res Result) ([]Displacement, error) {
	var out []Displacement
	for id, st := range res.States {
		if !st.Live() {
			f, err := l.Version(ctx, id, st.Version)
			if err != nil {
				return nil, fmt.Errorf("fold: displaced %s v%d: %w", id, st.Version, err)
			}
			out = append(out, Displacement{Facet: f, Version: st.Version, Reason: "not live: " + string(st.Status)})
		}
		for n := 1; n < st.Version; n++ {
			f, err := l.Version(ctx, id, n)
			if err != nil {
				return nil, fmt.Errorf("fold: displaced %s v%d: %w", id, n, err)
			}
			out = append(out, Displacement{Facet: f, Version: n, Reason: fmt.Sprintf("displaced by v%d", n+1)})
		}
	}

	// States is a map, so insertion order is arbitrary. Impose the canonical's order
	// with version breaking ties, so a facet's history reads oldest-first in place.
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		switch {
		case !a.Facet.EventTime.Equal(b.Facet.EventTime):
			return a.Facet.EventTime.Before(b.Facet.EventTime)
		case a.Facet.ID != b.Facet.ID:
			return a.Facet.ID < b.Facet.ID
		default:
			return a.Version < b.Version
		}
	})
	return out, nil
}
