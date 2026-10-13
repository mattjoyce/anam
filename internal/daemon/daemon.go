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

	"github.com/mattjoyce/anam/internal/catchup"
	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/inspect"
	"github.com/mattjoyce/anam/internal/store/artifact"
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

// writeTimeout bounds writing one response. The inspector's whole-record page is about
// 5 MB and renders in about a second, so this leaves a slow link plenty.
const writeTimeout = 2 * time.Minute

// shutdownGrace bounds how long shutdown waits for in-flight HTTP requests. Each
// request is already bounded by the configured timeout, so this only has to cover it.
const shutdownGrace = 10 * time.Second

// Stores are the write handles a write is given. They are the daemon's and stay open
// for its life: a write uses them and never closes them.
type Stores struct {
	Ledger *ledger.Ledger
	Ops    *ops.DB
}

// WriteFunc is one unit of write work. It runs alone: no other WriteFunc runs until it
// returns.
type WriteFunc func(ctx context.Context, s Stores) error

type job struct {
	fn       WriteFunc
	deadline time.Duration
	done     chan error
}

// Options are what the daemon is given rather than configured with.
type Options struct {
	// Ingest is the ingest the API runs. Nil means POST /v1/ingest fails.
	Ingest IngestFunc
	// Version is the build, reported by /v1/status.
	Version string
}

// Daemon owns the stores. Open it, then Run it; Run closes the stores when it returns.
type Daemon struct {
	cfg     config.Config
	opts    Options
	token   []byte
	started string
	writes  Stores
	read    *ledger.Ledger
	// readOps is a read-only handle on the operational store, so the routes that only
	// read it cannot write it.
	readOps *ops.DB
	// artifacts is the published-artifact tier: checkpoint values and receipts.
	artifacts *artifact.Store
	// index is whether the operational index is wrong rather than merely behind.
	index   indexState
	ingests *ingests
	// importers are who may post envelopes and Definitions; defs is what their
	// envelopes are admitted against.
	importers importers
	defs      *definitions
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

// Open opens both stores for writing and the ledger once more for reading, with the
// same checks ingest makes: the ledger must exist, belong to cfg's person and hold this
// build's schema, and the operational store must match its schema and bucket width.
// Any mismatch refuses to start.
//
// With a token file configured every request but /healthz must carry the token, and
// the daemon may listen on any address. Without one it listens on loopback only.
func Open(ctx context.Context, cfg config.Config, opts Options) (*Daemon, error) {
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
	imps, err := loadImporters(cfg.ImportersFile, tok)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
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
	defs, err := loadDefinitions(ctx, db)
	if err != nil {
		closeAll()
		return nil, err
	}
	projs, err := loadProjections(ctx, db)
	if err != nil {
		closeAll()
		return nil, err
	}
	readOps, err := ops.OpenReadOnly(ctx, cfg.DBPath)
	if err != nil {
		closeAll()
		return nil, err
	}
	artifacts, err := artifact.New(cfg.ArtifactDir)
	if err != nil {
		_ = readOps.Close()
		closeAll()
		return nil, err
	}
	return &Daemon{
		cfg:         cfg,
		opts:        opts,
		token:       tok,
		started:     clock.Format(time.Now()),
		writes:      Stores{Ledger: w, Ops: db},
		read:        r,
		readOps:     readOps,
		artifacts:   artifacts,
		ingests:     newIngests(),
		importers:   imps,
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

// Handler is the daemon's routes: /healthz open to all, and behind the token the /v1
// API and the inspector on everything else.
func (d *Daemon) Handler() http.Handler {
	buckets, _ := d.cfg.Buckets() // Open has already accepted the width.
	inner := http.NewServeMux()
	d.routes(inner)
	inner.Handle("/", inspect.Handler(inspect.Deps{
		Ledger: d.read, Timeout: d.cfg.Timeout, Buckets: buckets, Library: d.cfg.LibraryDir,
		Definitions: d.defs.current.Load, Held: d.heldDefinitions, Checkpoints: d.readOps,
		Projections: d.projections.current.Load,
	}))

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", d.healthz)
	// The importer routes sit outside the operator's token: an importer's token
	// opens these and nothing else.
	mux.Handle("POST /v1/envelopes", d.asImporter(d.postEnvelopes))
	mux.Handle("POST /v1/definitions", d.asImporter(d.postDefinitions))
	mux.Handle("GET /v1/definitions", d.asReader(d.getDefinitions))
	mux.Handle("GET /v1/definitions/{type}", d.asReader(d.getDefinition))
	mux.Handle("GET /v1/definitions/{type}/{version}", d.asReader(d.getDefinition))
	mux.Handle("GET /v1/snapshots/{id}", d.asReader(d.getSnapshot))
	mux.Handle("/", d.authorize(inner))
	return mux
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
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		// A response that cannot be written in this long is a client that has gone.
		// The one handler that holds a response longer, a POST /v1/ingest that asks
		// to wait, extends its own deadline by the wait it asked for.
		WriteTimeout: writeTimeout,
		IdleTimeout:  2 * time.Minute,
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
		_ = d.Write(ctx, d.cfg.IngestTimeout, func(ctx context.Context, s Stores) error {
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
	shutCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownGrace)
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
	return errors.Join(d.read.Close(), d.readOps.Close(), d.writes.Ledger.Close(), d.writes.Ops.Close(),
		d.artifacts.Close())
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
