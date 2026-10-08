package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"sort"
)

// The /v1 routes. Every handler is bounded by the configured timeout; none waits on
// the write queue, so a busy ledger never holds a request.
func (d *Daemon) routes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/ingest", d.postIngest)
	mux.HandleFunc("GET /v1/operations/{id}", d.getOperation)
	mux.HandleFunc("GET /v1/status", d.getStatus)
}

// postIngest asks for an ingest and answers 202 with the run that will take in what
// the sources hold now.
func (d *Daemon) postIngest(w http.ResponseWriter, _ *http.Request) {
	id, err := d.RequestIngest()
	if errors.Is(err, ErrStopped) {
		writeError(w, http.StatusServiceUnavailable, err)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	op, _ := d.Operation(id)
	w.Header().Set("Location", "/v1/operations/"+id)
	writeJSON(w, http.StatusAccepted, op)
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

// Status is the daemon's answer to "what state is the record in".
type Status struct {
	Version  string `json:"version"`
	Person   string `json:"person"`
	LedgerID string `json:"ledger_id"`
	W        int64  `json:"w"`
	// Since is when this daemon started; Sources covers only what it has seen since.
	Since   string         `json:"since"`
	Sources []SourceStatus `json:"sources"`
	Ingest  struct {
		Running string `json:"running,omitempty"`
		Pending string `json:"pending,omitempty"`
	} `json:"ingest"`
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
	s := Status{
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
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func sortSources(s []SourceStatus) {
	sort.Slice(s, func(i, j int) bool { return s[i].Source < s[j].Source })
}
