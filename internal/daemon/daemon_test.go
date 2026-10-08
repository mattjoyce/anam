package daemon

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/span"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

// newConfig is a config over a real, initialised ledger under the test's own directory.
func newConfig(t *testing.T) config.Config {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Config{
		PersonID:    "matt",
		LedgerPath:  filepath.Join(dir, "ledger.db"),
		DBPath:      filepath.Join(dir, "tracking.db"),
		ArtifactDir: filepath.Join(dir, "artifacts"),
		LibraryDir:  filepath.Join(dir, "documents"),
		Zone:        "UTC",
		Timeout:     5 * time.Second,
		BucketWidth: span.Default,
		Listen:      "127.0.0.1:0",
	}
	l, err := ledger.Create(context.Background(), cfg.LedgerPath, cfg.PersonID)
	if err != nil {
		t.Fatalf("create ledger: %v", err)
	}
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	return cfg
}

func open(t *testing.T, cfg config.Config) *Daemon {
	t.Helper()
	d, err := Open(context.Background(), cfg, Options{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	return d
}

// start runs the daemon on a loopback listener until the returned stop is called;
// stop returns what Run returned.
func start(t *testing.T, d *Daemon) (stop func() error) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	errc := make(chan error, 1)
	go func() { errc <- d.serve(ctx, ln) }()
	var once sync.Once
	var runErr error
	stop = func() error {
		once.Do(func() { cancel(); runErr = <-errc })
		return runErr
	}
	t.Cleanup(func() { _ = stop() })
	return stop
}

func TestOpenRefuses(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*config.Config)
		want   error
	}{
		{"no ledger file", func(c *config.Config) { c.LedgerPath += ".missing" }, ledger.ErrNotInitialised},
		{"another person's ledger", func(c *config.Config) { c.PersonID = "someone-else" }, ledger.ErrPersonMismatch},
		{"every interface", func(c *config.Config) { c.Listen = ":8787" }, ErrNotLoopback},
		{"all-zeros address", func(c *config.Config) { c.Listen = "0.0.0.0:8787" }, ErrNotLoopback},
		{"a LAN address", func(c *config.Config) { c.Listen = "192.168.86.45:8787" }, ErrNotLoopback},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := newConfig(t)
			tt.mutate(&cfg)
			_, err := Open(context.Background(), cfg, Options{})
			if !errors.Is(err, tt.want) {
				t.Fatalf("Open: got %v, want %v", err, tt.want)
			}
		})
	}
}

func TestOpenAcceptsLoopback(t *testing.T) {
	for _, addr := range []string{"127.0.0.1:0", "localhost:0", "[::1]:0"} {
		t.Run(addr, func(t *testing.T) {
			cfg := newConfig(t)
			cfg.Listen = addr
			d := open(t, cfg)
			if err := d.closeStores(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestHandlerServesHealthAndInspector(t *testing.T) {
	d := open(t, newConfig(t))
	t.Cleanup(func() { _ = d.closeStores() })
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()

	for _, tt := range []struct{ path, body string }{
		{"/healthz", "ready\n"},
		{"/", ""},
	} {
		resp, err := http.Get(srv.URL + tt.path)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET %s: status %d: %s", tt.path, resp.StatusCode, b)
		}
		if tt.body != "" && string(b) != tt.body {
			t.Fatalf("GET %s: body %q, want %q", tt.path, b, tt.body)
		}
	}
}

func TestHealthzNotReadyWhenLedgerUnreadable(t *testing.T) {
	d := open(t, newConfig(t))
	t.Cleanup(func() { _ = d.writes.Ledger.Close(); _ = d.writes.Ops.Close() })
	if err := d.read.Close(); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	d.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status %d, want 503", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "ledger") {
		t.Fatalf("healthz says more than ready or not: %q", rec.Body.String())
	}
}

func TestWritesRunOneAtATime(t *testing.T) {
	d := open(t, newConfig(t))
	stop := start(t, d)

	var active, most atomic.Int32
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := d.Write(context.Background(), time.Second, func(ctx context.Context, s Stores) error {
				n := active.Add(1)
				defer active.Add(-1)
				for m := most.Load(); n > m && !most.CompareAndSwap(m, n); m = most.Load() {
				}
				time.Sleep(2 * time.Millisecond)
				_, err := s.Ledger.Head(ctx)
				return err
			})
			if err != nil {
				t.Errorf("write: %v", err)
			}
		}()
	}
	wg.Wait()
	if got := most.Load(); got != 1 {
		t.Fatalf("%d writes ran at once, want 1", got)
	}
	if err := stop(); err != nil {
		t.Fatalf("run: %v", err)
	}
}

func TestWriteDeadline(t *testing.T) {
	d := open(t, newConfig(t))
	start(t, d)

	err := d.Write(context.Background(), 20*time.Millisecond, func(ctx context.Context, _ Stores) error {
		<-ctx.Done()
		return ctx.Err()
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("got %v, want the write's own deadline", err)
	}
}

// A caller that gives up stops waiting; the write it started does not stop with it.
func TestCallerGivingUpDoesNotCutTheWrite(t *testing.T) {
	d := open(t, newConfig(t))
	start(t, d)

	started, release := make(chan struct{}), make(chan struct{})
	finished := make(chan error, 1)
	caller, giveUp := context.WithCancel(context.Background())
	go func() {
		_ = d.Write(caller, time.Second, func(ctx context.Context, _ Stores) error {
			close(started)
			<-release
			finished <- ctx.Err()
			return nil
		})
	}()
	<-started
	giveUp()
	close(release)
	if err := <-finished; err != nil {
		t.Fatalf("the write saw its context end with the caller's: %v", err)
	}
}

// Shutdown during a write lets it finish on open stores, refuses new writes, and only
// then closes the stores.
func TestShutdownDuringWrite(t *testing.T) {
	d := open(t, newConfig(t))
	stop := start(t, d)

	started, release := make(chan struct{}), make(chan struct{})
	wrote := make(chan error, 1)
	go func() {
		wrote <- d.Write(context.Background(), 5*time.Second, func(ctx context.Context, s Stores) error {
			close(started)
			<-release
			if err := ctx.Err(); err != nil {
				return err
			}
			_, err := s.Ledger.Head(ctx)
			return err
		})
	}()
	<-started

	stopped := make(chan error, 1)
	go func() { stopped <- stop() }()
	select {
	case err := <-stopped:
		t.Fatalf("Run returned with a write still running: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	close(release)
	if err := <-wrote; err != nil {
		t.Fatalf("the running write did not finish cleanly: %v", err)
	}
	if err := <-stopped; err != nil {
		t.Fatalf("run: %v", err)
	}
	if err := d.Write(context.Background(), time.Second, func(context.Context, Stores) error {
		t.Error("a write ran after shutdown")
		return nil
	}); !errors.Is(err, ErrStopped) {
		t.Fatalf("write after shutdown: got %v, want ErrStopped", err)
	}
}
