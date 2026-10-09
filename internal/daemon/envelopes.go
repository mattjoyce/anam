package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/mattjoyce/anam/internal/api"
	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/gate"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

// maxSubmission bounds one batch's body: a full batch of the largest payloads the
// gate admits, with room for the envelopes around them.
const maxSubmission = gate.MaxBatch*300*1024 + 1<<20

// retryAfter is what a 503 asks an importer to wait, in seconds. A write holding the
// queue is usually an ingest, which takes tens of seconds.
const retryAfter = "30"

// postEnvelopes admits one batch from an importer, whole or not at all. The importer
// is the source of every envelope and may submit only its own types: the gate's
// authenticated pillar holds it to them.
func (d *Daemon) postEnvelopes(w http.ResponseWriter, r *http.Request) {
	imp := importerOf(r)
	envs, err := decodeSubmission(w, r, imp.Source)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(w, http.StatusRequestEntityTooLarge, err)
			return
		}
		writeError(w, http.StatusBadRequest, err)
		return
	}

	reg := d.defs.current.Load()
	var results []gate.Result
	var head ledger.Watermark
	err = d.TryWrite(d.timeout(r), func(ctx context.Context, s Stores) error {
		g := gate.New(s.Ledger, reg, gate.StaticScopes{imp.Source: imp.Types}, clock.Real{}, d.cfg.PersonID)
		bindings := make([]facet.Binding, len(envs))
		for i, env := range envs {
			// A malformed envelope binds to nothing; the gate's first check refuses it
			// again and names it with the rest.
			bindings[i], _ = g.Resolve(env)
		}
		var err error
		results, head, err = g.CommitBatch(ctx, envs, bindings)
		if err == nil && changedBy(results) > 0 {
			d.catchUp(ctx, s)
		}
		return err
	})

	var refused *gate.Refused
	switch {
	case errors.As(err, &refused):
		writeJSON(w, http.StatusUnprocessableEntity, refusalOf(refused))
	case errors.Is(err, ErrBusy), errors.Is(err, ErrStopped), errors.Is(err, gate.ErrContention):
		w.Header().Set("Retry-After", retryAfter)
		writeError(w, http.StatusServiceUnavailable, err)
	case errors.Is(err, context.DeadlineExceeded):
		writeError(w, http.StatusGatewayTimeout, err)
	case err != nil:
		writeError(w, http.StatusInternalServerError, err)
	default:
		changed := changedBy(results)
		d.ingests.sawImporter(imp.Source, d.now(), changed > 0)
		if changed > 0 {
			d.requestMint()
		}
		writeJSON(w, http.StatusOK, admissionOf(results, head))
	}
}

// decodeSubmission reads a batch into envelopes from source. Unknown fields are
// refused, a source among them: the token names the source.
func decodeSubmission(w http.ResponseWriter, r *http.Request, source string) ([]facet.Envelope, error) {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxSubmission))
	dec.DisallowUnknownFields()
	var sub api.Submission
	if err := dec.Decode(&sub); err != nil {
		return nil, fmt.Errorf("submission: %w", err)
	}
	switch n := len(sub.Envelopes); {
	case n == 0:
		return nil, errors.New("submission holds no envelopes")
	case n > gate.MaxBatch:
		return nil, fmt.Errorf("submission holds %d envelopes; a batch is at most %d", n, gate.MaxBatch)
	}
	envs := make([]facet.Envelope, len(sub.Envelopes))
	for i, e := range sub.Envelopes {
		if e.DefinitionVersion < 1 {
			return nil, fmt.Errorf("envelope %d (%s): definition_version is required", i, e.Type)
		}
		envs[i] = facet.Envelope{
			Type: facet.Type(e.Type), Source: source, Injector: source, SourceRef: e.SourceRef,
			Payload: e.Payload, DefinitionVersion: e.DefinitionVersion,
			AssertedPerson: e.AssertedPerson, SpanHint: e.SpanHint,
		}
	}
	return envs, nil
}

func changedBy(results []gate.Result) int {
	n := 0
	for _, r := range results {
		if r.Outcome == gate.OutcomeAdmitted || r.Outcome == gate.OutcomeSuperseded {
			n++
		}
	}
	return n
}

func admissionOf(results []gate.Result, head ledger.Watermark) api.Admission {
	a := api.Admission{W: int64(head), Changed: changedBy(results), Results: make([]api.EnvelopeResult, len(results))}
	for i, r := range results {
		a.Results[i] = api.EnvelopeResult{
			Index: i, Outcome: string(r.Outcome), FacetID: r.FacetID, ArtifactVersion: r.ArtifactVersion,
		}
	}
	return a
}

func refusalOf(r *gate.Refused) api.Refusal {
	out := api.Refusal{Error: "batch refused, nothing admitted: " + r.Error(),
		Refused: make([]api.RefusedEnvelope, len(r.Envelopes))}
	for i, e := range r.Envelopes {
		out.Refused[i] = api.RefusedEnvelope{Index: e.Index, Type: string(e.Type), Reason: e.Err.Error()}
	}
	return out
}

// sawImporter records a batch from an importer in the daemon's view of its sources.
func (in *ingests) sawImporter(source, at string, admitted bool) {
	in.mu.Lock()
	defer in.mu.Unlock()
	st := in.sources[source]
	if st == nil {
		st = &api.SourceStatus{Source: source}
		in.sources[source] = st
	}
	st.LastScanAt = at
	if admitted {
		st.LastAdmissionAt = at
	}
}
