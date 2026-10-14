package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/mattjoyce/anam/internal/api"
	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/gate"
	"github.com/mattjoyce/anam/internal/store/ledger"
	"github.com/mattjoyce/anam/internal/store/ops"
)

// maxSubmission bounds one batch's body: a full batch of the largest payloads the
// gate admits (max_payload_bytes), with room for the envelopes around them.
func maxSubmission(maxPayloadBytes int) int64 {
	return int64(gate.MaxBatch)*int64(maxPayloadBytes) + 1<<20
}

// gateLimits is the gate's share of the loaded config.
func gateLimits(cfg config.Config) gate.Limits {
	return gate.Limits{MaxPayloadBytes: cfg.MaxPayloadBytes, MaxRetries: cfg.CommitRetries}
}

// retryAfter is what a 503 asks its caller to wait (retry_after), in whole seconds as
// the header carries it.
func (d *Daemon) retryAfter() string {
	return strconv.Itoa(int(d.cfg.RetryAfter / time.Second))
}

// postEnvelopes admits one batch from an importer, whole or not at all. The importer
// is the source of every envelope and may submit only its own types: the gate's
// authenticated pillar holds it to them.
func (d *Daemon) postEnvelopes(w http.ResponseWriter, r *http.Request) {
	imp := importerOf(r)
	envs, snap, err := decodeSubmission(w, r, imp, d.cfg.Limits)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(w, http.StatusRequestEntityTooLarge, err)
			return
		}
		writeError(w, http.StatusBadRequest, err)
		return
	}

	var results []gate.Result
	var head ledger.Watermark
	var progress *api.SnapshotProgress
	var digest string
	if snap != nil {
		digest = partDigest(envs)
	}
	err = d.tryWriteWithin(d.cfg.ImporterWait, d.timeout(r), func(ctx context.Context, s Stores) error {
		// A part that does not belong to the run its id names, or that was already
		// recorded with other envelopes, is refused before anything is admitted.
		if err := checkSnapshot(ctx, s.Ops, imp.Source, snap, digest); err != nil {
			return err
		}
		// A reference to a document the library does not hold is refused before the
		// gate sees the batch, and nothing of it is admitted (card 102).
		if err := checkReferences(d.cfg.LibraryDir, envs); err != nil {
			return err
		}
		// Inside the write, where a registration cannot land between this and the
		// admission.
		reg := d.defs.current.Load()
		g := gate.New(s.Ledger, reg, gate.StaticScopes{imp.Source: imp.Types}, clock.Real{}, d.cfg.PersonID, gateLimits(d.cfg))
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
		if err == nil && snap != nil {
			// The ledger has the batch already; a failure here leaves the snapshot one
			// part short, so it never completes and nothing follows from it. The
			// importer resends the part, which admits as no-ops and records it. A
			// changed resend after such a crash is admitted as the part, since nothing
			// was recorded to compare it to: the protocol fixes a part's envelopes for
			// the life of its run id, and a run that reads its source again is a new
			// run (card 107).
			progress, err = recordSnapshotPart(ctx, s.Ops, imp.Source, snap, digest, results, head, time.Now(),
				ops.SnapshotRetention{Incomplete: d.cfg.SnapshotTTL, Complete: d.cfg.SnapshotKeep})
		}
		return err
	})

	var refused *gate.Refused
	switch {
	case errors.Is(err, ops.ErrSnapshotMismatch), errors.Is(err, ops.ErrSnapshotPartChanged):
		writeError(w, http.StatusConflict, err)
	case errors.As(err, &refused):
		writeJSON(w, http.StatusUnprocessableEntity, refusalOf(refused))
	case errors.Is(err, ErrBusy), errors.Is(err, ErrStopped), errors.Is(err, gate.ErrContention):
		w.Header().Set("Retry-After", d.retryAfter())
		writeError(w, http.StatusServiceUnavailable, err)
	case errors.Is(err, context.DeadlineExceeded):
		writeError(w, http.StatusGatewayTimeout, err)
	case err != nil:
		writeError(w, http.StatusInternalServerError, err)
	default:
		changed := changedBy(results)
		d.sources.saw(imp.Source, d.now(), changed > 0)
		if changed > 0 {
			d.requestMint()
		}
		a := admissionOf(results, head)
		a.Snapshot = progress
		writeJSON(w, http.StatusOK, a)
	}
}

// decodeSubmission reads a batch into envelopes from imp's source, and its snapshot
// label when it has one. Unknown fields are refused, a source among them: the token
// names the source. The body is one submission and nothing after it: the whole body
// is read, through the one bounded reader, before anything is queued, so a second
// value or garbage behind a valid batch is refused (400) and a body over
// maxSubmission is refused (413) however the excess is spread. The server's
// ReadTimeout bounds the read; a client that stalls is cut off there (card 109).
func decodeSubmission(w http.ResponseWriter, r *http.Request, imp config.Importer, limits config.Limits,
) ([]facet.Envelope, *api.SnapshotPart, error) {
	source := imp.Source
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxSubmission(limits.MaxPayloadBytes)))
	dec.DisallowUnknownFields()
	var sub api.Submission
	if err := dec.Decode(&sub); err != nil {
		return nil, nil, fmt.Errorf("submission: %w", err)
	}
	if err := atEOF(dec); err != nil {
		return nil, nil, fmt.Errorf("submission: %w", err)
	}
	switch n := len(sub.Envelopes); {
	case n == 0:
		return nil, nil, errors.New("submission holds no envelopes")
	case n > gate.MaxBatch:
		return nil, nil, fmt.Errorf("submission holds %d envelopes; a batch is at most %d", n, gate.MaxBatch)
	}
	if err := validSnapshot(sub.Snapshot, imp, sub.Envelopes, limits.MaxSnapshotParts); err != nil {
		return nil, nil, err
	}
	envs := make([]facet.Envelope, len(sub.Envelopes))
	for i, e := range sub.Envelopes {
		if e.DefinitionVersion < 1 {
			return nil, nil, fmt.Errorf("envelope %d (%s): definition_version is required", i, e.Type)
		}
		envs[i] = facet.Envelope{
			Type: facet.Type(e.Type), Source: source, Injector: source, SourceRef: e.SourceRef,
			Payload: e.Payload, DefinitionVersion: e.DefinitionVersion,
			AssertedPerson: e.AssertedPerson, SpanHint: e.SpanHint,
		}
	}
	return envs, sub.Snapshot, nil
}

// atEOF is nil once dec has nothing left but whitespace. Anything else is the error:
// a JSON token names a second value, a syntax error names garbage, and the reader's
// own error (a *http.MaxBytesError when the body ran over its bound) is kept whole.
func atEOF(dec *json.Decoder) error {
	tok, err := dec.Token()
	switch {
	case errors.Is(err, io.EOF):
		return nil
	case err != nil:
		return err
	default:
		return fmt.Errorf("one submission per request; found %v after it", tok)
	}
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
