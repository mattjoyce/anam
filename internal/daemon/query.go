package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/mattjoyce/anam/internal/api"
	"github.com/mattjoyce/anam/internal/query"
	"github.com/mattjoyce/anam/internal/read"
)

// The query routes (card 173): GET /v1/query is the introspection a program is
// written against, POST /v1/query runs one through a projection and answers with a
// receipt. Both read on the read-only handles and take no turn on the write queue; a
// query's receipt is written on the write handle, as a published read's is.

// querySource is the read's source with every held Definition body, for the
// catalogue and the per-version pointer warnings.
func (d *Daemon) querySource(ctx context.Context) (read.Source, error) {
	bodies, _, err := d.heldDefinitions(ctx)
	if err != nil {
		return read.Source{}, err
	}
	return read.Source{
		Ledger: d.read, Checkpoints: d.readOps, Values: d.readOps,
		Definitions: d.defs.current.Load(), Projections: d.projections.current.Load(), Held: query.Held(bodies),
	}, nil
}

func (d *Daemon) getQueryCatalogue(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), d.timeout(r))
	defer cancel()
	src, err := d.querySource(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	cat, err := read.Describe(src)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, cat)
}

// postQuery runs a program. The body is bounded as a manifest is: a program is text a
// person or an agent wrote, not a record.
func (d *Daemon) postQuery(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, int64(d.cfg.MaxManifestBytes)))
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, err)
		return
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	var q api.QueryRequest
	if err := dec.Decode(&q); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("query request: %w", err))
		return
	}
	if err := namedZone(q.Zone); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), d.timeout(r))
	defer cancel()
	src, err := d.querySource(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	deps, err := read.ServeDeps(d.read, d.writes.Ops, d.readOps, src.Definitions, src.Projections)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	deps.Reader = readerOf(principalOf(r))
	a, err := read.Query(ctx, src, deps, q, d.cfg.MaxReadBytes)
	if err != nil {
		d.writeFiledReadError(w, err)
		return
	}
	status := http.StatusCreated
	if !a.Written {
		status = http.StatusOK
	}
	writeJSON(w, status, a)
}
