package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/mattjoyce/anam/internal/api"
	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/inspect"
	"github.com/mattjoyce/anam/internal/mint"
	"github.com/mattjoyce/anam/internal/read"
	"github.com/mattjoyce/anam/internal/serve"
	"github.com/mattjoyce/anam/internal/store/artifact"
)

// route is one pattern, the roles whose tokens open it, and its handler.
type route struct {
	pattern string
	roles   []config.Role
	handler http.Handler
}

// Who may call a route. Reading the record, and publishing a read, which writes
// artifacts and never the ledger, is every role's. Submitting is an importer's alone:
// an envelope is admitted as its importer's source, and the operator is no source.
// Running the daemon is the operator's, and so is the inspector, which shows the
// whole record unprojected.
var (
	anyRole      = []config.Role{config.RoleReader, config.RoleImporter, config.RoleOperator}
	importerOnly = []config.Role{config.RoleImporter}
	operatorOnly = []config.Role{config.RoleOperator}
)

// routes is every route but /healthz. Every handler is bounded by the configured
// timeout, and none waits on the write queue, so a busy ledger never holds a request.
func (d *Daemon) routes() []route {
	buckets, _ := d.cfg.Buckets() // Open has already accepted the width.
	inspector := inspect.Handler(inspect.Deps{
		Ledger: d.read, Timeout: d.cfg.Timeout, Buckets: buckets, Library: d.cfg.LibraryDir,
		Definitions: d.defs.current.Load, Held: d.heldDefinitions, Checkpoints: d.readOps,
		Projections: d.projections.current.Load, CachedReads: d.cfg.InspectCachedReads,
		Workbench: d.workbenchFor(),
	})
	f := func(pattern string, roles []config.Role, h http.HandlerFunc) route { return route{pattern, roles, h} }
	return []route{
		f("GET /v1/status", anyRole, d.getStatus),
		f("GET /v1/timeline", anyRole, d.getTimeline),
		f("GET /v1/changes", anyRole, d.getChanges),
		f("GET /v1/receipts", anyRole, d.getReceipts),
		f("POST /v1/receipts", anyRole, d.postReceipt),
		f("GET /v1/receipts/{hash}", anyRole, d.getReceipt),
		f("GET /v1/receipts/{hash}/verify", anyRole, d.verifyReceipt),
		f("GET /v1/query", anyRole, d.getQueryCatalogue),
		f("POST /v1/query", anyRole, d.postQuery),
		f("GET /v1/projections", anyRole, d.getProjections),
		f("GET /v1/projections/{name}", anyRole, d.getProjection),
		f("GET /v1/projections/{name}/{version}", anyRole, d.getProjection),
		f("GET /v1/checkpoints", anyRole, d.getCheckpoints),
		f("GET /v1/definitions", anyRole, d.getDefinitions),
		f("GET /v1/definitions/{type}", anyRole, d.getDefinition),
		f("GET /v1/definitions/{type}/{version}", anyRole, d.getDefinition),
		f("GET /v1/snapshots/{id}", anyRole, d.getSnapshot),

		f("POST /v1/envelopes", importerOnly, d.postEnvelopes),
		f("POST /v1/definitions", importerOnly, d.postDefinitions),
		f("POST /v1/documents", importerOnly, d.postDocument),

		f("POST /v1/projections", operatorOnly, d.postProjections),
		f("POST /v1/checkpoints", operatorOnly, d.postCheckpoints),
		f("POST /v1/backups", operatorOnly, d.postBackups),
		f("POST /v1/facets/{id}/retract", operatorOnly, d.postFacetRetract),
		f("POST /v1/entry", operatorOnly, d.postEntry),
		{"/", operatorOnly, inspector},
	}
}

func (d *Daemon) getCheckpoints(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), d.timeout(r))
	defer cancel()
	if err := d.index.wrong(); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	list, err := read.Checkpoints(ctx, d.readOps, d.cfg.DBPath)
	if err != nil {
		writeReadError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// mintDeps is a mint on the write handles, with the daemon's width and budget.
func (d *Daemon) mintDeps(s Stores) mint.Deps {
	buckets, _ := d.cfg.Buckets() // Open has already accepted the width.
	return mint.Deps{
		Ledger: s.Ledger, Index: s.Ops, Artifacts: d.artifacts, Buckets: buckets, Clock: clock.Real{},
		Budget: d.cfg.CheckpointBudget,
	}
}

// postCheckpoints mints, or with ?dry_run=true derives without publishing; ?full=true
// folds the whole ledger instead of starting from the last run. Minting
// writes the operational store and the artifacts, so it runs on the write queue. While
// another write holds the queue it answers 503 at once, rather than wait out a write
// that may take minutes. Once accepted, the mint
// is answered whenever it ends, so a 2xx or an error always says what happened.
func (d *Daemon) postCheckpoints(w http.ResponseWriter, r *http.Request) {
	var q api.MintQuery
	for name, into := range map[string]*bool{"dry_run": &q.DryRun, "full": &q.Full} {
		if v := r.URL.Query().Get(name); v != "" {
			var err error
			if *into, err = strconv.ParseBool(v); err != nil {
				writeError(w, http.StatusBadRequest, fmt.Errorf("%s %q: %w", name, v, err))
				return
			}
		}
	}
	var m api.Mint
	err := d.TryWrite(r.Context(), d.timeout(r), func(ctx context.Context, s Stores) error {
		var err error
		m, err = mint.Report(ctx, d.mintDeps(s), q)
		return err
	})
	switch {
	case errors.Is(err, ErrStopped), errors.Is(err, ErrBusy):
		writeError(w, http.StatusServiceUnavailable, err)
	case err != nil:
		writeReadError(w, err)
	default:
		writeJSON(w, http.StatusOK, m)
	}
}

func (d *Daemon) getReceipts(w http.ResponseWriter, _ *http.Request) {
	deps, err := read.ServeDeps(d.read, d.artifacts, d.readOps, d.defs.current.Load(), d.projections.current.Load())
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
	deps, err := read.ServeDeps(d.read, d.artifacts, d.readOps, d.defs.current.Load(), d.projections.current.Load())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	shown, err := read.Receipt(deps, r.PathValue("hash"))
	if err != nil {
		writeReadError(w, err)
		return
	}
	// A receipt published locally, or before the bound, can be too large to show
	// here; it is still on disk and still verifies.
	if err := read.Fit(shown.Entries, d.cfg.MaxReadBytes,
		"show it locally with anam receipt show, or publish a narrower read; verify needs no entries"); err != nil {
		writeReadError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, shown)
}

func (d *Daemon) verifyReceipt(w http.ResponseWriter, r *http.Request) {
	deps, err := read.ServeDeps(d.read, d.artifacts, d.readOps, d.defs.current.Load(), d.projections.current.Load())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), d.timeout(r))
	defer cancel()
	v, err := read.VerifyReceipt(ctx, deps, r.PathValue("hash"))
	if err != nil {
		writeReadError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

// postReceipt publishes a read: the timeline read the query describes, through its
// projection, filed with a receipt (card 87). The query is the timeline's, in the
// same parameters. Publishing writes artifacts only, so it takes no turn on the write
// queue and never waits on a write; the artifact store's writes are atomic (card
// 95), so two publishes of one value at once each leave a whole file.
func (d *Daemon) postReceipt(w http.ResponseWriter, r *http.Request) {
	q, err := api.TimelineQueryOf(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := namedZone(q.Zone); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	defs := d.defs.current.Load()
	deps, err := read.ServeDeps(d.read, d.artifacts, d.readOps, defs, d.projections.current.Load())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), d.timeout(r))
	defer cancel()
	src := read.Source{Ledger: d.read, Checkpoints: d.readOps, Artifacts: d.artifacts, Definitions: defs, Projections: d.projections.current.Load()}
	p, err := read.Publish(ctx, src, deps, q, d.cfg.MaxReadBytes)
	if err != nil {
		writeReadError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

// getTimeline is the CLI's timeline read, run here on the read handle. The query
// carries the reader's zone; the daemon has none of its own.
func (d *Daemon) getTimeline(w http.ResponseWriter, r *http.Request) {
	q, err := api.TimelineQueryOf(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := namedZone(q.Zone); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), d.timeout(r))
	defer cancel()
	src := read.Source{Ledger: d.read, Checkpoints: d.readOps, Artifacts: d.artifacts, Definitions: d.defs.current.Load(), Projections: d.projections.current.Load()}
	t, err := read.Timeline(ctx, src, q)
	if err != nil {
		writeReadError(w, err)
		return
	}
	// The read is done either way; what is bounded is the answer, and with it a
	// client's context (card 88). The refusal says what is left to narrow by.
	if err := read.FitTimeline(t, q, d.cfg.MaxReadBytes); err != nil {
		writeReadError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

// getChanges reads the anam-event index as it stands. The daemon catches the index
// up inside each batch's write, so a read never writes and never waits on a write;
// the answer carries W, which says when the index is behind.
func (d *Daemon) getChanges(w http.ResponseWriter, r *http.Request) {
	q := api.ChangesQueryOf(r.URL.Query())
	if err := namedZone(q.Zone); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), d.timeout(r))
	defer cancel()
	if err := d.index.wrong(); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	head, err := d.read.Head(ctx)
	if err != nil {
		writeLedgerError(w, err)
		return
	}
	c, err := read.Changes(ctx, d.readOps, int64(head), q)
	if err != nil {
		writeReadError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

// TimeoutHeader carries the caller's own deadline for a request. The daemon works to
// the shorter of it and its configured timeout, so it answers before the caller gives
// up, and the caller never misses an answer to a write that happened.
const TimeoutHeader = "Anam-Timeout"

// timeout is the bound for one request: the configured timeout, or the caller's if
// that is shorter.
func (d *Daemon) timeout(r *http.Request) time.Duration {
	t := d.cfg.Timeout
	if v, err := time.ParseDuration(r.Header.Get(TimeoutHeader)); err == nil && v > 0 && v < t {
		t = v
	}
	return t
}

// writeLedgerError answers a ledger that could not be read: 504 if it ran out of
// time, 503 otherwise.
func writeLedgerError(w http.ResponseWriter, err error) {
	if errors.Is(err, context.DeadlineExceeded) {
		writeError(w, http.StatusGatewayTimeout, err)
		return
	}
	writeError(w, http.StatusServiceUnavailable, err)
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
// 413 for an answer over max_read_bytes, with its measure, 504 for a read that ran
// out of time, 500 for anything else.
func writeReadError(w http.ResponseWriter, err error) {
	var tooLarge *read.TooLarge
	switch {
	case errors.Is(err, read.ErrQuery), errors.Is(err, artifact.ErrInvalid):
		writeError(w, http.StatusBadRequest, err)
	case errors.As(err, &tooLarge):
		writeJSON(w, http.StatusRequestEntityTooLarge,
			api.Error{Error: err.Error(), Code: api.CodeTooLarge, TooLarge: &tooLarge.TooLarge})
	case errors.Is(err, artifact.ErrNotFound):
		writeError(w, http.StatusNotFound, err)
	case errors.Is(err, read.ErrTooManyAbsent):
		writeError(w, http.StatusRequestEntityTooLarge, err)
	case errors.Is(err, serve.ErrTemplateMoved), errors.Is(err, serve.ErrOtherLedger):
		writeError(w, http.StatusConflict, err)
	case errors.Is(err, context.DeadlineExceeded):
		writeError(w, http.StatusGatewayTimeout, err)
	default:
		writeError(w, http.StatusInternalServerError, err)
	}
}

func (d *Daemon) getStatus(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), d.timeout(r))
	defer cancel()
	id, err := d.read.ID(ctx)
	if err != nil {
		writeLedgerError(w, err)
		return
	}
	head, err := d.read.Head(ctx)
	if err != nil {
		writeLedgerError(w, err)
		return
	}
	s := api.Status{
		Version: d.opts.Version, Person: d.cfg.PersonID, LedgerID: id, W: int64(head),
		Since: d.started, Sources: d.sources.statuses(),
	}
	writeJSON(w, http.StatusOK, s)
}

// writeJSON answers with v. HTML is not escaped: a payload's bytes reach the client as
// they are held, so the CLI renders them as it would from local files.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		slog.Warn("daemon: write response", "err", err)
	}
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, api.Error{Error: err.Error()})
}

func sortSources(s []api.SourceStatus) {
	sort.Slice(s, func(i, j int) bool { return s[i].Source < s[j].Source })
}
