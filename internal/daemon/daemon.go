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

	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/inspect"
	"github.com/mattjoyce/anam/internal/store/ledger"
	"github.com/mattjoyce/anam/internal/store/ops"
)

// ErrStopped is returned by Write once shutdown has begun. The write was not run.
var ErrStopped = errors.New("daemon: stopping, write not accepted")

// ErrNotLoopback is returned by Open for a listen address beyond loopback with no
// token configured. The inspector serves the whole record unprojected and the
// library's documents, and the API can write, so neither may be reachable from the
// network without a token.
var ErrNotLoopback = errors.New("daemon: listen address is not loopback")

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
	ingests *ingests

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
	var token []byte
	if cfg.TokenFile != "" {
		t, err := readToken(cfg.TokenFile)
		if err != nil {
			return nil, err
		}
		token = t
	} else if err := loopbackOnly(cfg.Listen); err != nil {
		return nil, err
	}
	buckets, err := cfg.Buckets()
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
	return &Daemon{
		cfg:     cfg,
		opts:    opts,
		token:   token,
		started: clock.Format(time.Now()),
		writes:  Stores{Ledger: w, Ops: db},
		read:    r,
		ingests: newIngests(),
		jobs:    make(chan job),
		quit:    make(chan struct{}),
	}, nil
}

// loopbackOnly refuses any listen address that a host other than this one can reach.
// An empty host means every interface, so it is refused too.
func loopbackOnly(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("daemon: listen %q: %w", addr, err)
	}
	if host == "localhost" {
		return nil
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return nil
	}
	return fmt.Errorf("%w: %q (set token_file to listen beyond this host)", ErrNotLoopback, addr)
}

// Handler is the daemon's routes: /healthz open to all, and behind the token the /v1
// API and the inspector on everything else.
func (d *Daemon) Handler() http.Handler {
	buckets, _ := d.cfg.Buckets() // Open has already accepted the width.
	inner := http.NewServeMux()
	d.routes(inner)
	inner.Handle("/", inspect.Handler(d.read, d.cfg.Timeout, buckets, d.cfg.LibraryDir))

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", d.healthz)
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

// Write runs fn alone, under deadline, and returns its error. ctx bounds only the wait
// to be accepted and the wait for the result: once fn starts it runs on the daemon's
// context, so a caller that gives up does not cut a write short. Write returns
// ErrStopped once shutdown has begun.
func (d *Daemon) Write(ctx context.Context, deadline time.Duration, fn WriteFunc) error {
	j := job{fn: fn, deadline: deadline, done: make(chan error, 1)}
	select {
	case d.jobs <- j:
	case <-d.quit:
		return ErrStopped
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case err := <-j.done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// work is the single write worker. It returns when quit closes, but never while a
// write is running: the select is only reached between writes.
func (d *Daemon) work(base context.Context) {
	for {
		// Shutdown wins over a write offered at the same moment.
		select {
		case <-d.quit:
			return
		default:
		}
		select {
		case <-d.quit:
			return
		case j := <-d.jobs:
			ctx, cancel := context.WithTimeout(base, j.deadline)
			j.done <- j.fn(ctx, d.writes)
			cancel()
		}
	}
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
	return d.serve(ctx, ln)
}

func (d *Daemon) serve(ctx context.Context, ln net.Listener) error {
	srv := &http.Server{Handler: d.Handler(), ReadHeaderTimeout: 5 * time.Second}

	var worker sync.WaitGroup
	worker.Add(1)
	go func() {
		defer worker.Done()
		// Writes outlive the signal that starts shutdown; each is bounded by its own
		// deadline instead.
		d.work(context.WithoutCancel(ctx))
	}()

	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()
	slog.Info("daemon: serving", "addr", ln.Addr().String(), "ledger", d.cfg.LedgerPath, "person", d.cfg.PersonID)

	var serveErr error
	select {
	case <-ctx.Done():
	case serveErr = <-errc:
	}

	shutCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownGrace)
	defer cancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		serveErr = errors.Join(serveErr, fmt.Errorf("daemon: shutdown: %w", err))
	}
	d.stop.Do(func() { close(d.quit) })
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
	return errors.Join(d.read.Close(), d.writes.Ledger.Close(), d.writes.Ops.Close())
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
