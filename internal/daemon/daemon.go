// Package daemon is Anam as one long-running process: the only owner of the ledger and
// the operational store while it runs (docs/API.md §2, storage and process ownership).
//
// It holds one write handle to each store and lets exactly one write run at a time,
// through a queue with a single worker. Reads go to a separate read-only ledger handle
// and may run concurrently. The inspector is mounted on the same listener, and
// /healthz says whether the daemon can read its ledger.
//
// A write runs on the daemon's own context with its own deadline, never on the context
// of whoever asked for it: an HTTP request's context ends when its response is sent,
// and a write cut off there would be cut off for a reason that has nothing to do with
// the write. Shutdown stops taking requests and new writes, lets the running write
// finish or reach its deadline, then closes the stores. A crash at any point leaves
// the ledger as commit already guarantees: whole batches or nothing.
package daemon

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/mattjoyce/anam/internal/catalog"
	"github.com/mattjoyce/anam/internal/catchup"
	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/inspect"
	"github.com/mattjoyce/anam/internal/selection"
	"github.com/mattjoyce/anam/internal/store/ledger"
	"github.com/mattjoyce/anam/internal/store/ops"
	"github.com/mattjoyce/anam/internal/token"
)

// ErrStopped is returned by Write once shutdown has begun. The write was not run.
var ErrStopped = errors.New("daemon: stopping, write not accepted")

// ErrBusy is returned by TryWrite while another write holds the queue. The write was
// not run.
var ErrBusy = errors.New("daemon: a write is running")

// ErrNotLoopback is returned by Open for a listen address beyond loopback with no
// token configured. The inspector serves the whole record unprojected and the
// library's documents, and the API can write, so neither may be reachable from the
// network without a token.
var ErrNotLoopback = errors.New("daemon: listen address is not loopback")

// Stores are the write handles a write is given. They are the daemon's and stay open
// for its life: a write uses them and never closes them.
type Stores struct {
	Ledger *ledger.Ledger
	Ops    *ops.DB
}

// WriteFunc is one unit of write work. It runs alone: no other WriteFunc runs until it
// returns.
type WriteFunc func(ctx context.Context, s Stores) error

// job is one write offered to the queue. caller is the context of whoever offered it;
// until gives the deadline the write runs to from the moment the worker takes it.
type job struct {
	fn     WriteFunc
	caller context.Context
	until  func(started time.Time) time.Time
	done   chan error
}

// Options are what the daemon is given rather than configured with.
type Options struct {
	// Version is the build, reported by /v1/status and shown in the Workbench's frame.
	Version string
	// GitCommit and BuildDate are the rest of the build's identity, shown beside the
	// version in the Workbench's frame.
	GitCommit string
	BuildDate string
}

// Daemon owns the stores. Open it, then Run it; Run closes the stores when it returns.
type Daemon struct {
	cfg  config.Config
	opts Options
	// tokenless is a daemon with no token_file: on loopback, and every request the
	// operator's that presents no principal's token.
	tokenless bool
	started   string
	writes    Stores
	read      *ledger.Ledger
	// readOps is a read-only handle on the operational store, so the routes that only
	// read it cannot write it.
	readOps *ops.DB
	// catalog is the provisioned catalog loaded at Open: what the guard persisted,
	// what the registries are built on, and the set version every receipt names.
	catalog *catalog.Catalog
	// selections are the saved selections (card 206), filed in the operational store.
	selections *selection.Store
	// index is whether the operational index is wrong rather than merely behind.
	index indexState
	// sources is what the daemon has seen of each importer since it started.
	sources *sources
	// principals are who may call the daemon, each with a role; defs is what
	// importers' envelopes are admitted against.
	principals principals
	// workbench is the principal the importers file declares for hand entry, nil
	// when it declares none. It has no token: POST /entry submits as it.
	workbench *config.Principal
	// file is the importers file's whole list, which the policy reads for the sources
	// it declares.
	file []config.Principal
	defs *definitions
	// projections are the templates reads select from and receipts verify against:
	// bundled, and those the operator registered (card 125).
	projections *projections
	// mints asks the mint worker to mint at the head. It holds one request, so any
	// number of batches landing during a mint cost one more mint, not one each.
	mints chan struct{}

	jobs chan job
	quit chan struct{}
	stop sync.Once
}

// Open opens both stores for writing and the ledger once more for reading. The ledger
// must exist, belong to cfg's person and hold this build's schema; the operational
// store must match its schema and bucket width; and this build's bundled Definitions
// must not re-key what the store last saw. Any mismatch refuses to start.
//
// With a token file configured every request but /healthz must carry the token, and
// the daemon may listen on any address. Without one it listens on loopback only.
func Open(ctx context.Context, cfg config.Config, opts Options) (*Daemon, error) {
	// Load has checked the limits; a Config built another way is checked here, so a
	// zero wait or bound never reaches a request.
	if err := cfg.Limits.Validate(); err != nil {
		return nil, fmt.Errorf("daemon: %w", err)
	}
	var tok []byte
	if cfg.TokenFile != "" {
		t, err := token.Read(cfg.TokenFile)
		if err != nil {
			return nil, fmt.Errorf("daemon: %w", err)
		}
		tok = t
	} else if err := loopbackOnly(cfg.Listen); err != nil {
		return nil, err
	}
	buckets, err := cfg.Buckets()
	if err != nil {
		return nil, err
	}
	// Loaded and validated once, before any store is opened: the principals, the
	// guard, the registries and every receipt read this one catalog.
	cat, err := catalog.Load(ctx, "", "")
	if err != nil {
		return nil, err
	}
	bundled := cat.Definitions()
	whom, err := loadPrincipals(cfg.ImportersFile, tok, bundled)
	if err != nil {
		return nil, err
	}

	// Each phase of the opening has a deadline of its own, so one that runs long under
	// load does not spend the next one's.
	parent := ctx
	ctx, cancel := context.WithTimeout(parent, cfg.Timeout)
	defer cancel()

	w, err := ledger.Open(ctx, cfg.LedgerPath, cfg.PersonID)
	if err != nil {
		return nil, err
	}
	r, err := ledger.OpenReadOnly(ctx, cfg.LedgerPath, cfg.PersonID)
	if err != nil {
		_ = w.Close()
		return nil, err
	}
	db, err := ops.Open(ctx, cfg.DBPath, buckets)
	if err != nil {
		_ = r.Close()
		_ = w.Close()
		return nil, err
	}
	closeAll := func() { _ = db.Close(); _ = r.Close(); _ = w.Close() }
	if err := checkIndex(ctx, r, db); err != nil {
		closeAll()
		return nil, err
	}
	ctx, cancel = context.WithTimeout(parent, cfg.Timeout)
	defer cancel()
	if err := guardBundled(ctx, db, cat); err != nil {
		closeAll()
		return nil, err
	}
	defs, err := loadDefinitions(ctx, db, bundled)
	if err != nil {
		closeAll()
		return nil, err
	}
	projs, err := loadProjections(ctx, db, cat.Projections(), cfg.DefaultProjection)
	if err != nil {
		closeAll()
		return nil, err
	}
	readOps, err := ops.OpenReadOnly(ctx, cfg.DBPath)
	if err != nil {
		closeAll()
		return nil, err
	}
	return &Daemon{
		cfg:         cfg,
		opts:        opts,
		tokenless:   tok == nil,
		started:     clock.Format(time.Now()),
		writes:      Stores{Ledger: w, Ops: db},
		read:        r,
		readOps:     readOps,
		selections:  selection.NewStore(db),
		sources:     newSources(),
		principals:  whom.byToken,
		workbench:   whom.workbench,
		file:        whom.file,
		catalog:     cat,
		defs:        defs,
		projections: projs,
		mints:       make(chan struct{}, 1),
		jobs:        make(chan job),
		quit:        make(chan struct{}),
	}, nil
}

// checkIndex refuses an operational store whose index is wrong for this ledger:
// derived from another ledger, or claiming events this one does not hold. Behind is
// fine; the first write catches it up. Wrong would be served as an answer, so the
// daemon does not start.
func checkIndex(ctx context.Context, l *ledger.Ledger, db *ops.DB) error {
	id, err := l.ID(ctx)
	if err != nil {
		return err
	}
	if err := db.PinLedger(ctx, id); err != nil {
		return err
	}
	last, err := db.LastSeq(ctx)
	if err != nil {
		return err
	}
	head, err := l.Head(ctx)
	if err != nil {
		return err
	}
	if last > int64(head) {
		return fmt.Errorf("%w: cursor %d, head %d", catchup.ErrIndexAhead, last, head)
	}
	return nil
}

// indexState holds why the operational index is wrong, once a catch-up finds that it
// is. Behind is not recorded: a reader sees it in the answer.
type indexState struct {
	mu  sync.Mutex
	err error
}

func (s *indexState) set(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.err = err
}

func (s *indexState) wrong() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

// guardedIndex is the operational index as the inspector reads it, for the checkpoint
// strip and the facet page's links: the read-only ops handle behind the daemon's
// index state, so an index a catch-up has found wrong refuses with
// inspect.ErrIndexWrong instead of answering from rows that cannot be trusted. The
// inspector knows nothing else of "wrong": the facet page refuses on it as GET
// /v1/receipts/{hash}/facets/{facet_id} does, and the timeline draws the strip red.
type guardedIndex struct {
	ops   *ops.DB
	state *indexState
}

func (g guardedIndex) wrong() error {
	if err := g.state.wrong(); err != nil {
		return fmt.Errorf("%w: %w", inspect.ErrIndexWrong, err)
	}
	return nil
}

func (g guardedIndex) TouchingEdges(ctx context.Context, facetID string) (ops.Touching, error) {
	if err := g.wrong(); err != nil {
		return ops.Touching{}, err
	}
	return g.ops.TouchingEdges(ctx, facetID)
}

func (g guardedIndex) SpanHeads(ctx context.Context) (map[string]ops.CheckpointRow, error) {
	if err := g.wrong(); err != nil {
		return nil, err
	}
	return g.ops.SpanHeads(ctx)
}

func (g guardedIndex) LastMintRun(ctx context.Context, w int64) (int64, bool, error) {
	if err := g.wrong(); err != nil {
		return 0, false, err
	}
	return g.ops.LastMintRun(ctx, w)
}

func (g guardedIndex) LogSealed(ctx context.Context) (bool, error) {
	if err := g.wrong(); err != nil {
		return false, err
	}
	return g.ops.LogSealed(ctx)
}

// Handler is the daemon's routes: /healthz and the API's description open to all,
// and every other route behind a token whose role the route admits (routes).
func (d *Daemon) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", d.healthz)
	mux.HandleFunc(openAPIPath, d.getOpenAPI)
	for _, rt := range d.routes() {
		mux.Handle(rt.pattern, d.admit(rt.roles, rt.handler))
	}
	// Before the token: a browser on another site is refused whatever credential it
	// carries, since a cached one is exactly what it would spend (card 108).
	return d.browserGuard(mux)
}

// healthz is ready when the ledger's head can be read within the deadline. The ledger's
// person and schema were checked at Open, and the file cannot change schema under a
// running daemon, so a readable head is the remaining question. It answers ready or
// not and nothing more.
func (d *Daemon) healthz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), d.cfg.Timeout)
	defer cancel()
	if _, err := d.read.Head(ctx); err != nil {
		slog.Warn("healthz: ledger head unreadable", "err", err)
		http.Error(w, "not ready", http.StatusServiceUnavailable)
		return
	}
	_, _ = fmt.Fprintln(w, "ready")
}

// Run serves on the configured address until ctx ends, then shuts down in order: stop
// accepting connections and let in-flight requests finish, stop accepting writes, let
// the running write finish or reach its deadline, close the stores. It returns nil on
// a clean shutdown.
func (d *Daemon) Run(ctx context.Context) error {
	ln, err := net.Listen("tcp", d.cfg.Listen)
	if err != nil {
		return errors.Join(fmt.Errorf("daemon: listen %s: %w", d.cfg.Listen, err), d.closeStores())
	}
	return d.Serve(ctx, ln)
}

// Serve is Run on a listener the caller has opened.
func (d *Daemon) Serve(ctx context.Context, ln net.Listener) error {
	srv := &http.Server{
		Handler:           d.Handler(),
		ReadHeaderTimeout: d.cfg.HTTPReadHeaderTimeout,
		ReadTimeout:       d.cfg.HTTPReadTimeout,
		// A response that cannot be written in this long is a client that has gone.
		WriteTimeout: d.cfg.HTTPWriteTimeout,
		IdleTimeout:  d.cfg.HTTPIdleTimeout,
	}

	var worker sync.WaitGroup
	worker.Add(2)
	go func() {
		defer worker.Done()
		// Writes outlive the signal that starts shutdown; each is bounded by its own
		// deadline instead.
		d.work(context.WithoutCancel(ctx))
	}()
	go func() {
		defer worker.Done()
		d.mintOnRequest()
	}()

	// The index may be behind a ledger that was written while no daemon ran. Catch it
	// up as the first write, so reads of it start current. Its error is ignored on
	// purpose: catchUp records a wrong index itself, and the only other outcome is
	// ErrStopped or the daemon's context ending, when there is nothing left to catch
	// up for.
	go func() {
		_ = d.Write(ctx, d.cfg.LongWriteTimeout, func(ctx context.Context, s Stores) error {
			d.catchUp(ctx, s)
			return nil
		})
		d.mintIfBehind(ctx)
	}()

	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()
	slog.Info("daemon: serving", "addr", ln.Addr().String(), "ledger", d.cfg.LedgerPath, "person", d.cfg.PersonID)

	var serveErr error
	select {
	case <-ctx.Done():
	case serveErr = <-errc:
	}

	// Stop taking writes first: callers waiting on a run are answered now rather than
	// holding Shutdown up, and the worker leaves once the running write returns.
	d.stop.Do(func() { close(d.quit) })
	shutCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), d.cfg.ShutdownGrace)
	defer cancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		serveErr = errors.Join(serveErr, fmt.Errorf("daemon: shutdown: %w", err))
	}
	worker.Wait()

	if err := d.closeStores(); err != nil {
		serveErr = errors.Join(serveErr, err)
	}
	if errors.Is(serveErr, http.ErrServerClosed) {
		serveErr = nil
	}
	slog.Info("daemon: stopped")
	return serveErr
}

func (d *Daemon) closeStores() error {
	return errors.Join(d.read.Close(), d.readOps.Close(), d.writes.Ledger.Close(), d.writes.Ops.Close())
}

// Ready asks the daemon listening on addr whether it is ready. An address bound to
// every interface is asked on loopback.
func Ready(ctx context.Context, addr string) error {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("daemon: ready: %q: %w", addr, err)
	}
	if ip := net.ParseIP(host); host == "" || (ip != nil && ip.IsUnspecified()) {
		host = "127.0.0.1"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+net.JoinHostPort(host, port)+"/healthz", nil)
	if err != nil {
		return fmt.Errorf("daemon: ready: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("daemon: ready: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("daemon: ready: /healthz answered %s", resp.Status)
	}
	return nil
}
