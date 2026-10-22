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
	"github.com/mattjoyce/anam/internal/store/ops"
)

// route is one pattern, the roles whose tokens open it, and its handler.
type route struct {
	pattern string
	roles   []config.Role
	handler http.Handler
}

// Who may call a route. Reading the record, and publishing a read or saving a
// selection, which write rows and never the ledger, is every role's. Submitting is an
// importer's alone: an envelope is admitted as its importer's source, and the operator
// is no source.
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
	index := guardedIndex{ops: d.readOps, state: &d.index}
	inspector := inspect.Handler(inspect.Deps{
		Ledger: d.read, Timeout: d.cfg.Timeout, Buckets: buckets, Library: d.cfg.LibraryDir,
		Definitions: d.defs.current.Load, Held: d.heldDefinitions, Checkpoints: index, Edges: index,
		Projections: d.projections.current.Load, CachedReads: d.cfg.InspectCachedReads,
		Workbench: d.workbenchFor(), MaxReadBytes: d.cfg.MaxReadBytes, Selections: d.selections,
	})
	f := func(pattern string, roles []config.Role, h http.HandlerFunc) route { return route{pattern, roles, h} }
	return []route{
		f("GET /v1/status", anyRole, d.getStatus),
		f("GET /v1/timeline", anyRole, d.getTimeline),
		f("GET /v1/changes", anyRole, d.getChanges),
		f("GET /v1/coverage", anyRole, d.getCoverage),
		f("GET /v1/receipts", anyRole, d.getReceipts),
		f("POST /v1/receipts", anyRole, d.postReceipt),
		f("GET /v1/receipts/{hash}", anyRole, d.getReceipt),
		f("GET /v1/receipts/{hash}/verify", anyRole, d.verifyReceipt),
		f("GET /v1/receipts/{hash}/facets/{facet_id}", anyRole, d.getReceiptFacet),
		f("POST /v1/selections", anyRole, d.postSelection),
		f("GET /v1/selections/{code}", anyRole, d.getSelection),
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
		f("POST /v1/hold", operatorOnly, d.postHold),
		f("DELETE /v1/hold", operatorOnly, d.deleteHold),
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
		Ledger: s.Ledger, Index: s.Ops, Values: s.Ops, Buckets: buckets, Clock: clock.Real{},
		Budget: d.cfg.CheckpointBudget,
	}
}

// postCheckpoints mints, or with ?dry_run=true derives without publishing; ?full=true
// folds the whole ledger instead of starting from the last run. Minting
// writes the operational store, so it runs on the write queue. While
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

// getReceipts lists the receipts, newest first: all of them, or those that name a
// principal (?principal=) and were served in a window (?since=, ?until=, with ?zone=
// for a bare date), which is the answer to who read what (card 216).
func (d *Daemon) getReceipts(w http.ResponseWriter, r *http.Request) {
	q := api.ReceiptsQueryOf(r.URL.Query())
	if err := namedZone(q.Zone); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	deps, err := read.ServeDeps(d.read, d.readOps, d.readOps, d.defs.current.Load(), d.projections.current.Load())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), d.timeout(r))
	defer cancel()
	list, err := read.Receipts(ctx, deps, d.cfg.DBPath, q)
	if err != nil {
		writeReadError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// getReceipt answers a receipt and the value it served, a page at a time (card 92):
// the first page without a cursor, the page ?cursor= names with one. Every page is
// cut from the served value, so a receipt published locally, or before the bound,
// pages like any other and still verifies.
func (d *Daemon) getReceipt(w http.ResponseWriter, r *http.Request) {
	deps, err := read.ServeDeps(d.read, d.readOps, d.readOps, d.defs.current.Load(), d.projections.current.Load())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if !d.mayOpenReceipt(w, r, deps) {
		return
	}
	shown, err := read.ReceiptPage(r.Context(), deps, r.PathValue("hash"), r.URL.Query().Get("cursor"), d.cfg.MaxReadBytes)
	if err != nil {
		writeReadError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, shown)
}

// getReceiptFacet answers one facet as the receipt served it (card 91), with the
// live edges touching it from the edge index (card 104). The two parts carry their
// own watermarks: the entry is the receipt's, the links are the index's cursor when
// they were read, and a wrong index refuses the whole answer rather than show the
// entry with links absent, which would read as none.
func (d *Daemon) getReceiptFacet(w http.ResponseWriter, r *http.Request) {
	deps, err := read.ServeDeps(d.read, d.readOps, d.readOps, d.defs.current.Load(), d.projections.current.Load())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if !d.mayOpenReceipt(w, r, deps) {
		return
	}
	if err := d.index.wrong(); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	shown, err := read.Facet(r.Context(), deps, r.PathValue("hash"), r.PathValue("facet_id"))
	if err != nil {
		writeReadError(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), d.timeout(r))
	defer cancel()
	touching, err := d.readOps.TouchingEdges(ctx, shown.Entry.FacetID)
	if err != nil {
		writeReadError(w, err)
		return
	}
	shown.Links = read.LinksOf(touching, shown.Entry.FacetID)
	if err := read.Fit(shown.Links.Edges, d.cfg.MaxReadBytes,
		"ask the record through POST /v1/query with linked/2, which pages by the query's own narrowing"); err != nil {
		writeReadError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, shown)
}

func (d *Daemon) verifyReceipt(w http.ResponseWriter, r *http.Request) {
	deps, err := read.ServeDeps(d.read, d.readOps, d.readOps, d.defs.current.Load(), d.projections.current.Load())
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
// same parameters. Publishing writes the receipt and the served value, two idempotent
// inserts, and never the ledger or the index, so it takes no turn on the write queue.
// It writes on the write handle, SQLite lets one writer in at a time, and two publishes
// of one value at once each leave the one row (card 95).
func (d *Daemon) postReceipt(w http.ResponseWriter, r *http.Request) {
	d.publishRead(w, r, http.StatusCreated)
}

// publishRead files the read r describes and answers with it, under status. The
// receipt names the principal when the principal's token records every read (card
// 216); for any other it names nobody, as every receipt did before.
func (d *Daemon) publishRead(w http.ResponseWriter, r *http.Request, status int) {
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
	deps, err := read.ServeDeps(d.read, d.writes.Ops, d.readOps, defs, d.projections.current.Load())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	deps.Reader = readerOf(principalOf(r))
	ctx, cancel := context.WithTimeout(r.Context(), d.timeout(r))
	defer cancel()
	src := read.Source{Ledger: d.read, Checkpoints: d.readOps, Values: d.readOps, Definitions: defs, Projections: d.projections.current.Load()}
	p, err := read.Publish(ctx, src, deps, q, d.cfg.MaxReadBytes)
	if err != nil {
		d.writeFiledReadError(w, err)
		return
	}
	writeJSON(w, status, p)
}

// readerOf is the name a receipt filed for p carries: p's, when its token records every
// read, else none.
func readerOf(p config.Principal) string {
	if p.Records() {
		return p.Name
	}
	return ""
}

// mayOpenReceipt reports whether the caller may open the value of the receipt the
// request names, and answers 403 when it may not. A token that records every read
// opens only receipts that name it: opening another's would be a read of facets no
// receipt of its own records. Every other token opens any receipt, as before.
func (d *Daemon) mayOpenReceipt(w http.ResponseWriter, r *http.Request, deps serve.Deps) bool {
	who := principalOf(r)
	if !who.Records() {
		return true
	}
	ctx, cancel := context.WithTimeout(r.Context(), d.timeout(r))
	defer cancel()
	reader, err := read.ReceiptReader(ctx, deps, r.PathValue("hash"))
	switch {
	case err != nil:
		writeReadError(w, err)
		return false
	case reader != who.Name:
		writeError(w, http.StatusForbidden, fmt.Errorf(
			"%s files a receipt for every read, so it opens only the receipts that name it; read through a projection to file one", who.Name))
		return false
	}
	return true
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
	if who := principalOf(r); who.Records() {
		// The read is filed as a receipt and answered from what was filed, once, so the
		// reader holds exactly what the receipt names (card 216). The answer is the
		// timeline's own fields with the receipt beside them, and a read too large for
		// one answer comes a page at a time, as a published read does.
		if q.Projection == "" || q.Explain {
			writeError(w, http.StatusBadRequest, fmt.Errorf(
				"%s files a receipt for every read, and a receipt is what a projection served: name a projection, and leave out explain", who.Name))
			return
		}
		d.publishRead(w, r, http.StatusOK)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), d.timeout(r))
	defer cancel()
	src := read.Source{Ledger: d.read, Checkpoints: d.readOps, Values: d.readOps, Definitions: d.defs.current.Load(), Projections: d.projections.current.Load()}
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
// with code cursor_invalid for a cursor that names no page of its receipt, 404 for a stored value that is not there, 409 for a receipt this daemon cannot honour,
// 413 for an answer over max_read_bytes, with its measure, 504 for a read that ran
// out of time, 500 for anything else.
// writeFiledReadError answers a read that files a receipt as it is served. Filing writes
// the operational store on the write handle outside the write queue, so another writer
// holding the file past its busy timeout fails the read; that is 503 with Retry-After,
// as a busy queue is, and nothing was filed or served. Anything else is writeReadError's.
func (d *Daemon) writeFiledReadError(w http.ResponseWriter, err error) {
	if errors.Is(err, ops.ErrBusy) {
		w.Header().Set("Retry-After", d.retryAfter())
		writeError(w, http.StatusServiceUnavailable, err)
		return
	}
	writeReadError(w, err)
}

func writeReadError(w http.ResponseWriter, err error) {
	var tooLarge *read.TooLarge
	switch {
	case errors.Is(err, read.ErrCursor):
		writeJSON(w, http.StatusBadRequest, api.Error{Error: err.Error(), Code: api.CodeCursorInvalid})
	case errors.Is(err, read.ErrQuery), errors.Is(err, ops.ErrInvalid):
		writeError(w, http.StatusBadRequest, err)
	case errors.As(err, &tooLarge):
		writeJSON(w, http.StatusRequestEntityTooLarge,
			api.Error{Error: err.Error(), Code: api.CodeTooLarge, TooLarge: &tooLarge.TooLarge})
	case errors.Is(err, ops.ErrNotFound), errors.Is(err, read.ErrNotServed):
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
	held, err := heldNames(ctx, d.readOps)
	if err != nil {
		writeReadError(w, err)
		return
	}
	s := api.Status{
		Version: d.opts.Version, Person: d.cfg.PersonID, LedgerID: id, W: int64(head),
		Since: d.started, Sources: d.sources.statuses(), Held: held,
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

// getCoverage counts what the record holds at the head or at a named W, type by type
// and source by source (card 90), on the read handles and under the request's bound.
func (d *Daemon) getCoverage(w http.ResponseWriter, r *http.Request) {
	q, err := api.CoverageQueryOf(r.URL.Query())
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
	src := read.Source{Ledger: d.read, Checkpoints: d.readOps, Values: d.readOps, Definitions: d.defs.current.Load(), Projections: d.projections.current.Load()}
	c, err := read.Coverage(ctx, src, q)
	if err != nil {
		writeReadError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}
