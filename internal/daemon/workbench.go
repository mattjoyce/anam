package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/mattjoyce/anam/internal/api"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/gate"
	"github.com/mattjoyce/anam/internal/inspect"
	"github.com/mattjoyce/anam/internal/policy"
	"github.com/mattjoyce/anam/internal/retract"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

// hand is the Workbench the inspector's pages enter facets through (card 188, S4). It
// is the daemon's own write seen from the pages: Enter ends in the same enter the
// entry route calls, and Retract in the same retractFacet, so a form saves exactly
// what POST /v1/entry would, and the pages hold no token and read no file. The pages
// learn which types are enterable from here, not from the importers file.
type hand struct{ d *Daemon }

// workbenchFor is the Workbench to give the inspector: nil, so the pages only read,
// when the importers file declares no workbench principal.
func (d *Daemon) workbenchFor() inspect.Workbench {
	if d.workbench == nil {
		return nil
	}
	return hand{d}
}

// Source is the source the workbench enters facts as.
func (h hand) Source() string { return h.d.workbench.Source }

// Enters reports whether the workbench principal may enter facts of type t: the policy's
// answer for its allow-list, the same one the gate holds an entry to.
func (h hand) Enters(t facet.Type) bool { return policy.MaySubmitType(*h.d.workbench, t) }

// Enter admits one payload as the workbench within the daemon's request timeout.
func (h hand) Enter(ctx context.Context, t facet.Type, version int, payload json.RawMessage) (inspect.Entered, error) {
	out, err := h.d.enter(ctx, api.EntryRequest{Type: string(t), DefinitionVersion: version, Payload: payload}, h.d.cfg.Timeout)
	if err != nil {
		return inspect.Entered{}, h.declined(err)
	}
	return inspect.Entered{Outcome: out.Outcome, FacetID: out.FacetID, ArtifactVersion: out.ArtifactVersion, W: out.W}, nil
}

// Retract tombstones one facet of the workbench source.
func (h hand) Retract(ctx context.Context, facetID string) (inspect.Retracted, error) {
	out, err := h.d.retractFacet(ctx, h.d.workbench.Source, facetID, h.d.cfg.Timeout)
	if err != nil {
		return inspect.Retracted{}, h.declined(err)
	}
	return inspect.Retracted{FacetID: out.FacetID, Outcome: out.Outcome, ArtifactVersion: out.ArtifactVersion, W: out.W}, nil
}

// declined says why a write failed in the words a page shows, or returns err as it is
// when it is a failure of the daemon's own and not a decline. These are the same cases
// the routes answer 422, 409, 503 and 404.
func (h hand) declined(err error) error {
	var refused *gate.Refused
	switch {
	case errors.As(err, &refused) && errors.Is(err, gate.ErrDefinitionVersion):
		return &inspect.Declined{Kind: inspect.DeclinedStale, Message: refusalOf(refused).Error}
	case errors.As(err, &refused):
		return &inspect.Declined{Kind: inspect.DeclinedRefused, Message: refusalOf(refused).Error}
	case errors.Is(err, ledger.ErrNotFound):
		return &inspect.Declined{Kind: inspect.DeclinedMissing, Message: err.Error()}
	case errors.Is(err, retract.ErrWrongSource), errors.Is(err, retract.ErrNotLive), errors.Is(err, retract.ErrReconciled):
		return &inspect.Declined{Kind: inspect.DeclinedConflict, Message: err.Error()}
	case errors.Is(err, ErrBusy), errors.Is(err, ErrStopped), errors.Is(err, gate.ErrContention), errors.Is(err, context.DeadlineExceeded):
		return &inspect.Declined{Kind: inspect.DeclinedBusy, Message: fmt.Sprintf(
			"the write queue is busy or the daemon is stopping (%v); try again in %s seconds. The form keeps what you typed",
			err, h.d.retryAfter())}
	}
	return err
}
