package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"time"

	"github.com/mattjoyce/anam/internal/api"
	"github.com/mattjoyce/anam/internal/read"
	"github.com/mattjoyce/anam/internal/serve"
	"github.com/mattjoyce/anam/internal/store/artifact"
)

// The /v1 routes. Every handler is bounded by the configured timeout, and none waits
// on the write queue, so a busy ledger never holds a request. The one exception is a
// POST /v1/ingest that asks to wait, bounded by the wait it asked for.
func (d *Daemon) routes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/ingest", d.postIngest)
	mux.HandleFunc("GET /v1/operations/{id}", d.getOperation)
	mux.HandleFunc("GET /v1/status", d.getStatus)
	mux.HandleFunc("GET /v1/timeline", d.getTimeline)
	mux.HandleFunc("GET /v1/changes", d.getChanges)
	mux.HandleFunc("GET /v1/receipts", d.getReceipts)
	mux.HandleFunc("GET /v1/receipts/{hash}", d.getReceipt)
	mux.HandleFunc("GET /v1/receipts/{hash}/verify", d.verifyReceipt)
}

func (d *Daemon) getReceipts(w http.ResponseWriter, _ *http.Request) {
	deps, err := read.ServeDeps(d.read, d.artifacts)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	list, err := read.Receipts(deps, d.cfg.ArtifactDir)
	if err != nil {
		writeReadError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (d *Daemon) getReceipt(w http.ResponseWriter, r *http.Request) {
	deps, err := read.ServeDeps(d.read, d.artifacts)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	shown, err := read.Receipt(deps, r.PathValue("hash"))
	if err != nil {
		writeReadError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, shown)
}

func (d *Daemon) verifyReceipt(w http.ResponseWriter, r *http.Request) {
	deps, err := read.ServeDeps(d.read, d.artifacts)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), d.cfg.Timeout)
	defer cancel()
	v, err := read.VerifyReceipt(ctx, deps, r.PathValue("hash"))
	if err != nil {
		writeReadError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

// getTimeline is the CLI's timeline read, run here on the read handle. The query
// carries the reader's zone; the daemon has none of its own.
func (d *Daemon) getTimeline(w http.ResponseWriter, r *http.Request) {
	q, err := api.TimelineQueryOf(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("explain: %w", err))
		return
	}
	if err := namedZone(q.Zone); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), d.cfg.Timeout)
	defer cancel()
	t, err := read.Timeline(ctx, d.read, q)
	if err != nil {
		writeReadError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

// getChanges reads the anam-event index as it stands. The daemon catches the index
// up inside each ingest's write, so a read never writes and never waits on a write;
// the answer carries W, which says when the index is behind.
func (d *Daemon) getChanges(w http.ResponseWriter, r *http.Request) {
	q := api.ChangesQueryOf(r.URL.Query())
	if err := namedZone(q.Zone); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), d.cfg.Timeout)
	defer cancel()
	head, err := d.read.Head(ctx)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err)
		return
	}
	c, err := read.Changes(ctx, d.writes.Ops, int64(head), q)
	if err != nil {
		writeReadError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

// namedZone refuses Local: on the daemon it would mean the daemon's zone, and the
// reader's is the one that counts.
func namedZone(zone string) error {
	if zone == "Local" {
		return errors.New("zone Local names the daemon's zone, not the reader's: send an IANA name")
	}
	return nil
}

// writeReadError answers a failed read: 400 for a query that cannot be read as asked,
// 404 for an artifact that is not there, 409 for a receipt this daemon cannot honour,
// 504 for a read that ran out of time, 500 for anything else.
func writeReadError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, read.ErrQuery), errors.Is(err, artifact.ErrInvalid):
		writeError(w, http.StatusBadRequest, err)
	case errors.Is(err, artifact.ErrNotFound):
		writeError(w, http.StatusNotFound, err)
	case errors.Is(err, serve.ErrTemplateMoved), errors.Is(err, serve.ErrOtherLedger):
		writeError(w, http.StatusConflict, err)
	case errors.Is(err, context.DeadlineExceeded):
		writeError(w, http.StatusGatewayTimeout, err)
	default:
		writeError(w, http.StatusInternalServerError, err)
	}
}

// postIngest asks for an ingest and answers 202 with the run that will take in what
// the sources hold now.
//
// With ?wait=<duration> it holds the response until that run finishes, for at most
// that long and never past ingest_timeout, and then answers 200 for a run that
// succeeded and 500 for one that failed, with the operation either way. A run still
// going when the wait ends answers 202 as without it. This is for a scheduler that
// judges a job by how it ends: the run itself is the same either way.
func (d *Daemon) postIngest(w http.ResponseWriter, r *http.Request) {
	var wait time.Duration
	if v := r.URL.Query().Get("wait"); v != "" {
		var err error
		if wait, err = time.ParseDuration(v); err != nil || wait <= 0 {
			writeError(w, http.StatusBadRequest, fmt.Errorf("wait %q: want a positive duration such as 300s", v))
			return
		}
		wait = min(wait, d.cfg.IngestTimeout)
	}

	id, err := d.RequestIngest()
	if errors.Is(err, ErrStopped) {
		writeError(w, http.StatusServiceUnavailable, err)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.Header().Set("Location", "/v1/operations/"+id)
	if wait == 0 {
		op, _ := d.Operation(id)
		writeJSON(w, http.StatusAccepted, op)
		return
	}
	// The server's write deadline is shorter than a long wait; this response is
	// allowed the wait plus the usual margin.
	if err := http.NewResponseController(w).SetWriteDeadline(time.Now().Add(wait + writeTimeout)); err != nil {
		slog.Warn("daemon: extend write deadline", "err", err)
	}
	ctx, cancel := context.WithTimeout(r.Context(), wait)
	defer cancel()
	op, _ := d.WaitOperation(ctx, id)
	switch op.State {
	case api.StateSucceeded:
		writeJSON(w, http.StatusOK, op)
	case api.StateFailed:
		writeJSON(w, http.StatusInternalServerError, op)
	default:
		writeJSON(w, http.StatusAccepted, op)
	}
}

func (d *Daemon) getOperation(w http.ResponseWriter, r *http.Request) {
	op, ok := d.Operation(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound,
			errors.New("no such operation here; operations are forgotten on restart, so read the ledger"))
		return
	}
	writeJSON(w, http.StatusOK, op)
}

func (d *Daemon) getStatus(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), d.cfg.Timeout)
	defer cancel()
	id, err := d.read.ID(ctx)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err)
		return
	}
	head, err := d.read.Head(ctx)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err)
		return
	}
	s := api.Status{
		Version: d.opts.Version, Person: d.cfg.PersonID, LedgerID: id, W: int64(head),
		Since: d.started, Sources: d.ingests.sourceStatuses(),
	}
	s.Ingest.Running, s.Ingest.Pending = d.ingests.ids()
	writeJSON(w, http.StatusOK, s)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Warn("daemon: write response", "err", err)
	}
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, api.Error{Error: err.Error()})
}

func sortSources(s []api.SourceStatus) {
	sort.Slice(s, func(i, j int) bool { return s[i].Source < s[j].Source })
}
