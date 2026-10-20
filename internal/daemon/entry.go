package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/mattjoyce/anam/internal/api"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/gate"
)

// entryHeadroom is what a hand entry's body may hold beyond its payload: the type and
// the definition version. It is an invariant, not a default: the body is those two
// fields and one payload, and the payload is bounded by max_payload_bytes.
const entryHeadroom = 4 << 10

// postEntry admits one fact entered by hand, as the workbench principal (card 188,
// slice 2). It is a route and not an ingress: the one envelope it builds goes through
// admitBatch, the write the envelope route uses, so the gate holds it to the
// workbench's source and types and nothing here can admit what an importer could not.
//
// The client names the type, the Definition version it drew its form from, and the
// payload. The source and the injector are the workbench's, and the identity field is
// the gate's to fill from the Definition's identity_template. The operator's token
// opens the route, and the browser guard wraps the mux, so a foreign page cannot reach
// it. A mint requested by the write, the status line, and the index catch-up are
// admitBatch's, as for an importer.
func (d *Daemon) postEntry(w http.ResponseWriter, r *http.Request) {
	if d.workbench == nil {
		writeError(w, http.StatusNotFound, errors.New(
			"the importers file declares no workbench principal (source: workbench, with the types it may enter)"))
		return
	}
	req, err := decodeEntry(w, r, d.cfg.MaxPayloadBytes)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(w, http.StatusRequestEntityTooLarge, err)
			return
		}
		writeError(w, http.StatusBadRequest, err)
		return
	}
	out, err := d.enter(r.Context(), req, d.timeout(r))
	var refused *gate.Refused
	if errors.As(err, &refused) && errors.Is(err, gate.ErrDefinitionVersion) {
		// The form was drawn from a Definition that is no longer in force. The gate said
		// so inside the write, where a registration cannot land between check and
		// admission, so this is not a guess made beforehand.
		writeJSON(w, http.StatusConflict, refusalOf(refused))
		return
	}
	if d.writeAdmitError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// enter admits one entry as the workbench principal through admitBatch and says what
// the gate did. The route and the Workbench's pages both call it, so there is one
// way in; the workbench must be declared, which the caller has checked. An error is
// the gate's or the queue's, for the caller to map.
func (d *Daemon) enter(ctx context.Context, req api.EntryRequest, budget time.Duration) (api.Entry, error) {
	who := *d.workbench
	env := facet.Envelope{
		Type: facet.Type(req.Type), Source: who.Source, Injector: who.Source,
		Payload: req.Payload, DefinitionVersion: req.DefinitionVersion,
	}
	a, err := d.admitBatch(ctx, who, intake{envs: []facet.Envelope{env}}, budget)
	if err != nil {
		return api.Entry{}, err
	}
	res := a.Results[0]
	out := api.Entry{Outcome: res.Outcome, FacetID: res.FacetID, ArtifactVersion: res.ArtifactVersion, W: a.W}
	if a.Changed > 0 {
		// One envelope, one event, written under the write queue: it is the newest.
		out.Seq = a.W
	}
	return out, nil
}

// decodeEntry reads an entry's body: the three fields and nothing else, as one
// bounded value. A field it does not know, a source or an id among them, is refused.
func decodeEntry(w http.ResponseWriter, r *http.Request, maxPayloadBytes int) (api.EntryRequest, error) {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, int64(maxPayloadBytes)+entryHeadroom))
	dec.DisallowUnknownFields()
	var req api.EntryRequest
	if err := dec.Decode(&req); err != nil {
		return req, fmt.Errorf("entry: %w", err)
	}
	if err := atEOF(dec); err != nil {
		return req, fmt.Errorf("entry: %w", err)
	}
	switch {
	case req.Type == "":
		return req, errors.New("entry: type is required")
	case req.DefinitionVersion < 1:
		return req, fmt.Errorf("entry (%s): definition_version is required", req.Type)
	case !isObject(req.Payload):
		return req, fmt.Errorf("entry (%s): payload must be a JSON object", req.Type)
	}
	return req, nil
}

// isObject reports whether raw is a JSON object, not null, a scalar or an array.
func isObject(raw json.RawMessage) bool {
	var obj map[string]json.RawMessage
	return json.Unmarshal(raw, &obj) == nil && obj != nil
}
