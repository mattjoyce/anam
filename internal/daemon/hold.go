package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/mattjoyce/anam/internal/api"
	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/policy"
	"github.com/mattjoyce/anam/internal/store/ops"
)

// maxHoldBody bounds a hold request's body: a list of source names, or all. It is an
// invariant (a longer body is not this request), not a default.
const maxHoldBody = 4 << 10

// ErrHeld is the refusal a held source's submission gets: the operator held its
// admission, so nothing of it is admitted.
var ErrHeld = errors.New("daemon: admission is held")

// heldError says which source is held, or that the hold could not be read.
type heldError struct {
	Source string
	Cause  error
}

func (e *heldError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("admission is held for %s: the hold could not be read (%v); nothing was admitted", e.Source, e.Cause)
	}
	return fmt.Sprintf("admission is held for %s: the operator held it; nothing was admitted", e.Source)
}

func (e *heldError) Unwrap() error { return ErrHeld }

// checkHold refuses a submission from a held source (policy.Held). It runs inside the
// write, where the hold and the admission are one order: a hold written before this
// write is seen, and one written after it is not. A hold that cannot be read refuses
// the submission too: an admission that cannot see a hold must not admit.
//
// The workbench is never held (policy.Holdable), and reads no hold.
func checkHold(ctx context.Context, db *ops.DB, who config.Principal) error {
	if !policy.Holdable(who) {
		return nil
	}
	holds, err := db.Holds(ctx)
	if err != nil {
		return &heldError{Source: who.Source, Cause: err}
	}
	if policy.Held(policy.Request{Caller: who, Hold: holdOf(holds)}) {
		return &heldError{Source: who.Source}
	}
	return nil
}

// holdOf is the hold rows as the policy reads them.
func holdOf(rows []ops.Hold) policy.Hold {
	var h policy.Hold
	for _, r := range rows {
		if r.Source == ops.HoldAll {
			h.All = true
			continue
		}
		h.Sources = append(h.Sources, r.Source)
	}
	return h
}

// postHold holds admission for the sources the body names, or for every source when
// the body says all. Holding a source that is held already changes nothing: the hold
// keeps the moment it was first set.
func (d *Daemon) postHold(w http.ResponseWriter, r *http.Request) {
	var req api.HoldRequest
	if err := decodeHold(w, r, &req); err != nil {
		writeHoldBodyError(w, err)
		return
	}
	sources, err := heldSources(req.Sources, req.All)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	var held []string
	err = d.tryWriteWithin(r.Context(), d.cfg.ImporterWait, d.timeout(r), func(ctx context.Context, s Stores) error {
		now := d.now()
		for _, src := range sources {
			if err := s.Ops.SetHold(ctx, src, now); err != nil {
				return err
			}
		}
		names, err := heldNames(ctx, s.Ops)
		held = names
		return err
	})
	if d.writeHoldError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, api.Holds{Held: held})
}

// deleteHold releases the holds the body names, or every hold when the body names
// none or asks for all. Releasing a source that is not held changes nothing.
func (d *Daemon) deleteHold(w http.ResponseWriter, r *http.Request) {
	var req api.HoldReleaseRequest
	if err := decodeHold(w, r, &req); err != nil {
		writeHoldBodyError(w, err)
		return
	}
	sources := make([]string, 0, len(req.Sources)+1)
	for _, src := range req.Sources {
		if src = strings.TrimSpace(src); src != "" {
			sources = append(sources, src)
		}
	}
	if req.All || len(sources) == 0 {
		sources = append(sources, ops.HoldAll)
	}

	var held []string
	err := d.tryWriteWithin(r.Context(), d.cfg.ImporterWait, d.timeout(r), func(ctx context.Context, s Stores) error {
		for _, src := range sources {
			if err := s.Ops.ReleaseHold(ctx, src); err != nil {
				return err
			}
		}
		names, err := heldNames(ctx, s.Ops)
		held = names
		return err
	})
	if d.writeHoldError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, api.Holds{Held: held})
}

// decodeHold reads a hold request's body into v, refusing fields the request does not
// define. An empty body is not an error here: what it means is the route's to say.
func decodeHold(w http.ResponseWriter, r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxHoldBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

// writeHoldBodyError answers a body that could not be read: 413 over the bound, 400
// for anything else.
func writeHoldBodyError(w http.ResponseWriter, err error) {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		writeError(w, http.StatusRequestEntityTooLarge, err)
		return
	}
	writeError(w, http.StatusBadRequest, err)
}

// writeHoldError answers an error a hold write returned, and reports whether there
// was one. A write queue that is busy, or stopping, is 503 with Retry-After.
func (d *Daemon) writeHoldError(w http.ResponseWriter, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, ErrBusy), errors.Is(err, ErrStopped):
		w.Header().Set("Retry-After", d.retryAfter())
		writeError(w, http.StatusServiceUnavailable, err)
	default:
		writeError(w, http.StatusInternalServerError, err)
	}
	return true
}

// heldSources is what a hold request names: the sources listed, or every source when
// it says all. The workbench is refused: a hold names an importer's source, and a
// hand entry is the operator's own.
func heldSources(sources []string, all bool) ([]string, error) {
	out := make([]string, 0, len(sources)+1)
	for _, src := range sources {
		src = strings.TrimSpace(src)
		if src == "" {
			continue
		}
		if !policy.MayHold(src) {
			return nil, fmt.Errorf("%s is never held: a hand entry is the operator's own", config.WorkbenchSource)
		}
		out = append(out, src)
	}
	if all {
		out = append(out, ops.HoldAll)
	}
	if len(out) == 0 {
		return nil, errors.New("hold: name the sources to hold, or ask for all of them")
	}
	return out, nil
}

// heldNames is the sources the store holds, by name.
func heldNames(ctx context.Context, db *ops.DB) ([]string, error) {
	holds, err := db.Holds(ctx)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(holds))
	for _, h := range holds {
		names = append(names, h.Source)
	}
	return names, nil
}
