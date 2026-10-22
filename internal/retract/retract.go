// Package retract plans the retraction of one facet by hand (card 188, slice 3).
//
// An importer's source is reconciled by its snapshots: the plan for it is a set
// comparison, and `anam retract <source>` tombstones what a complete resend dropped.
// A source nobody resends (workbench, where a person types a record in; conversation,
// where a chat saves a note) has no snapshot to compare with, so a wrong entry is
// undone by naming the one facet. This package decides whether that is allowed. It
// writes nothing: the tombstone is gate.Retract, and the CLI and the daemon both call
// it with the plan this package returns.
package retract

import (
	"context"
	"errors"
	"fmt"

	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/fold"
	"github.com/mattjoyce/anam/internal/store/ledger"
	"github.com/mattjoyce/anam/internal/store/ops"
)

// The three refusals. Each names the rule that fired, so an operator or a form can
// say why, and a caller can tell them apart with errors.Is.
var (
	// ErrWrongSource is a facet that belongs to a source other than the one named.
	// The source is a guard: a button on one source's page cannot tombstone another's.
	ErrWrongSource = errors.New("retract: the facet belongs to another source")
	// ErrNotLive is a facet whose current status is not live. Status is only ever
	// decided by folding the facet's anam-events.
	ErrNotLive = errors.New("retract: the facet is not live")
	// ErrReconciled is a source an importer declares and has sent a complete
	// snapshot for. Its facets are reconciled by the importer's next resend, so a
	// hand retraction would be undone or contradicted by it.
	ErrReconciled = errors.New("retract: the source is reconciled by an importer's snapshot")
)

// Ledger is what the plan reads of the ledger. *ledger.Ledger satisfies it, on either
// handle.
type Ledger interface {
	CurrentVersion(ctx context.Context, facetID string) (facet.Facet, error)
	Head(ctx context.Context) (ledger.Watermark, error)
	FacetEvents(ctx context.Context, facetID string, uptoW ledger.Watermark) ([]facet.AnamEvent, error)
}

// Snapshots is what the plan reads of the operational store: whether a source has
// ever completed a snapshot. *ops.DB satisfies it.
type Snapshots interface {
	NewestCompleteSnapshot(ctx context.Context, source string) (ops.Snapshot, bool, error)
}

// Plan is what retracting one facet would do: the facet at the version in force, and
// its status now. The tombstone would mark that version and move the watermark by one.
type Plan struct {
	// Facet is the facet at its newest version, the one the tombstone marks.
	Facet facet.Facet
	// Status is the facet's status now, decided by folding its anam-events. It is live.
	Status facet.Status
}

// PlanFacet decides whether facetID may be retracted by hand as a facet of source, and
// returns what that would do. outright is the policy's answer for the source
// (policy.MayRetract): when it is false an importer declares the source, and a source
// that has completed a snapshot is refused (ErrReconciled), a facet
// of any other source is refused (ErrWrongSource), and a facet that is not live is
// refused (ErrNotLive). A facet the ledger does not hold is ledger.ErrNotFound.
// Nothing is written.
func PlanFacet(ctx context.Context, l Ledger, snaps Snapshots, outright bool, source, facetID string) (Plan, error) {
	f, err := l.CurrentVersion(ctx, facetID)
	if err != nil {
		return Plan{}, err
	}
	if f.Provenance.Source != source {
		return Plan{}, fmt.Errorf("%w: %s is a %s from source %s, not %s",
			ErrWrongSource, facetID, f.Type, f.Provenance.Source, source)
	}
	if !outright {
		snap, ok, err := snaps.NewestCompleteSnapshot(ctx, source)
		if err != nil {
			return Plan{}, err
		}
		if ok {
			return Plan{}, fmt.Errorf("%w: %s is declared in the importers file and snapshot %s completed; "+
				"retract it by the importer's next complete resend, or retire the source", ErrReconciled, source, snap.ID)
		}
	}
	head, err := l.Head(ctx)
	if err != nil {
		return Plan{}, err
	}
	events, err := l.FacetEvents(ctx, facetID, head)
	if err != nil {
		return Plan{}, err
	}
	states, err := fold.Fold(events)
	if err != nil {
		return Plan{}, err
	}
	st := states[facetID]
	if !st.Live() {
		return Plan{}, fmt.Errorf("%w: %s is a %s (v%d) and its status is %q",
			ErrNotLive, facetID, f.Type, f.ArtifactVersion, st.Status)
	}
	return Plan{Facet: f, Status: st.Status}, nil
}
