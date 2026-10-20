package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/mattjoyce/anam/internal/api"
	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/gate"
	"github.com/mattjoyce/anam/internal/retract"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

// maxRetractBody bounds a retraction's body: one source name. It is an invariant (a
// longer body is not this request), not a default.
const maxRetractBody = 4 << 10

// importerSources are the sources the importers file gives a token to. A retraction by
// hand is refused for one that has completed a snapshot, since the importer's resend
// reconciles it.
func (d *Daemon) importerSources() []string {
	var out []string
	for _, p := range d.principals {
		if p.Role == config.RoleImporter {
			out = append(out, p.Source)
		}
	}
	return out
}

// postFacetRetract tombstones one facet of the source the body names, by hand, as the
// operator (card 188, slice 3). The plan is the one `anam retract <source> --facet`
// applies (retract.PlanFacet), made inside the write so no writer lands between the
// plan and the tombstone. Like an importer's batch it takes the write queue within the
// request's budget and answers 503 at once when another write holds it. The browser
// guard wraps the whole mux, so a foreign page cannot reach it.
func (d *Daemon) postFacetRetract(w http.ResponseWriter, r *http.Request) {
	var req api.RetractRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRetractBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("retraction: %w", err))
		return
	}
	if err := atEOF(dec); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("retraction: %w", err))
		return
	}
	if req.Source == "" {
		writeError(w, http.StatusBadRequest, errors.New("retraction: source is required"))
		return
	}
	out, err := d.retractFacet(r.Context(), req.Source, r.PathValue("id"), d.timeout(r))

	switch {
	case errors.Is(err, ledger.ErrNotFound):
		writeError(w, http.StatusNotFound, err)
	case errors.Is(err, retract.ErrWrongSource), errors.Is(err, retract.ErrNotLive), errors.Is(err, retract.ErrReconciled):
		writeError(w, http.StatusConflict, err)
	case errors.Is(err, ErrBusy), errors.Is(err, ErrStopped), errors.Is(err, gate.ErrContention):
		w.Header().Set("Retry-After", d.retryAfter())
		writeError(w, http.StatusServiceUnavailable, err)
	case errors.Is(err, context.DeadlineExceeded):
		writeError(w, http.StatusGatewayTimeout, err)
	case err != nil:
		writeError(w, http.StatusInternalServerError, err)
	default:
		writeJSON(w, http.StatusOK, out)
	}
}

// retractFacet plans and applies the tombstone of one facet of source, inside the
// write queue within budget, and asks for a mint when it wrote one. The route and the
// Workbench's pages both call it. An error is the plan's, the gate's or the queue's,
// for the caller to map.
func (d *Daemon) retractFacet(ctx context.Context, source, id string, budget time.Duration) (api.Retraction, error) {
	var out api.Retraction
	err := d.TryWrite(ctx, budget, func(ctx context.Context, s Stores) error {
		plan, err := retract.PlanFacet(ctx, s.Ledger, s.Ops, d.importerSources(), source, id)
		if err != nil {
			return err
		}
		reg := d.defs.current.Load()
		// A tombstone admits nothing, so the gate needs no scopes for it.
		g := gate.New(s.Ledger, reg, gate.StaticScopes{}, clock.Real{}, d.cfg.PersonID, gateLimits(d.cfg))
		res, head, err := g.Retract(ctx, id)
		if err != nil {
			return err
		}
		out = api.Retraction{
			FacetID: id, Source: source, Type: string(plan.Facet.Type),
			ArtifactVersion: res.ArtifactVersion, Outcome: string(res.Outcome), W: int64(head),
		}
		if res.Outcome == gate.OutcomeTombstoned {
			out.Seq = int64(head)
			d.catchUp(ctx, s)
		}
		return nil
	})
	if err == nil && out.Seq != 0 {
		d.requestMint()
	}
	return out, err
}
