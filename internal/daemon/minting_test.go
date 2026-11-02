package daemon

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/store/ops"
)

// slowClock is a mint's clock that sleeps before each reading. A mint reads it once
// for every bucket it cuts and once for the run, so a mint over n buckets takes more
// than n+1 times per, however small the ledger: a mint longer than a budget, made
// without a ledger large enough to be slow.
type slowClock struct{ per time.Duration }

func (c slowClock) Now() time.Time {
	time.Sleep(c.per)
	return time.Now()
}

// logBuffer is the default logger's output, safe to read while the daemon writes it.
type logBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (l *logBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.Write(p)
}

func (l *logBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.String()
}

// captureLogs sends the default logger to a buffer until the test ends. A test that
// calls it must not be parallel. Call it before start, so the daemon stops before the
// logger is put back.
func captureLogs(t *testing.T) *logBuffer {
	t.Helper()
	logs := &logBuffer{}
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(logs, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return logs
}

// waitMint is waitFor with room for a mint made slow on purpose, on a machine the
// gate keeps busy.
func waitMint(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// weighInOn is a weigh-in on day, a date in Sydney's summer time, so each month's
// falls in its own bucket.
func weighInOn(id, day string, version int) string {
	at := day + " 07:30"
	return fmt.Sprintf(`{"type": "health.weight", "definition_version": %d,
		"payload": {"id": %q, "date": %q, "source_time": {"value": %q, "zone": "Australia/Sydney", "offset": "+11:00", "basis": "source"}, "kg": 80}}`,
		version, id, at, at)
}

// seedUnminted admits a weigh-in in each of the first months of six on a daemon that
// stops before it mints, as one stopped just after a batch, or a store carried with
// no mint runs, leaves it: the next daemon on cfg starts behind and mints every
// bucket. It returns the head.
func seedUnminted(t *testing.T, cfg config.Config, months int) int64 {
	t.Helper()
	cfg.MintQuiet = time.Hour // stopped long before the mint would run
	d, url, stop := serveImporters(t, cfg)
	weight, _ := d.defs.current.Load().Lookup("health.weight")
	var envs []string
	for i, day := range []string{"2025-10-15", "2025-11-15", "2025-12-15", "2026-01-15", "2026-02-15", "2026-03-15"}[:months] {
		envs = append(envs, weighInOn(fmt.Sprintf("w-%d", i), day, weight.Version))
	}
	call(t, "POST", url+"/v1/envelopes", importerToken, batch(envs...), http.StatusOK, nil)
	w := head(t, d)
	if err := stop(); err != nil {
		t.Fatalf("stop the seeding daemon: %v", err)
	}
	if w != int64(len(envs)) {
		t.Fatalf("head %d after %d weigh-ins", w, len(envs))
	}
	return w
}

// Card 268: the automatic mint runs under long_write_timeout, not the per-call
// timeout. A daemon that starts behind mints from its start (mintIfBehind, through
// the mint worker), and this mint takes longer than timeout. It must finish, not be
// cut at timeout as the first mint on the carried schema-10 store was twice.
// Not parallel: it swaps the default logger.
func TestAutomaticMintHasTheLongWriteBudget(t *testing.T) {
	logs := captureLogs(t)
	cfg := importerConfig(t)
	w := seedUnminted(t, cfg, 2)

	cfg.LongWriteTimeout = time.Minute
	d := open(t, cfg)
	// Open spends timeout on its own reads, so it is shortened once the daemon is
	// open: under -race on a busy machine those reads may need more than this.
	d.cfg.Timeout = 500 * time.Millisecond
	d.mintClock = slowClock{per: 600 * time.Millisecond} // each bucket alone outlasts timeout
	start(t, d)

	waitMint(t, "the mint at start to end", func() bool {
		out := logs.String()
		return strings.Contains(out, "daemon: minted") || strings.Contains(out, "mint at head failed")
	})
	if out := logs.String(); strings.Contains(out, "mint at head failed") {
		t.Fatalf("the automatic mint was cut short:\n%s", out)
	}
	if run, ok, err := d.readOps.LastMintRun(context.Background(), w); err != nil || !ok || run != w {
		t.Fatalf("last mint run = %d, %v, %v; want %d", run, ok, err, w)
	}
}

// Card 268: a mint that the long budget does cut short leaves the store as a cut
// mint always has. What it minted stays minted, it records no run, W does not move,
// and the next mint resumes: it cuts only the buckets the first did not reach, and
// re-cuts none. Not parallel: it swaps the default logger.
func TestMintCutShortResumes(t *testing.T) {
	logs := captureLogs(t)
	cfg := importerConfig(t)
	w := seedUnminted(t, cfg, 6)

	cut := cfg
	cut.LongWriteTimeout = time.Second
	d := open(t, cut)
	d.mintClock = slowClock{per: 300 * time.Millisecond} // six buckets and a run take 2.1 s
	stop := start(t, d)
	waitMint(t, "the mint at start to be cut short", func() bool {
		return strings.Contains(logs.String(), "mint at head failed")
	})
	ctx := context.Background()
	partial, err := d.readOps.Checkpoints(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := d.readOps.LastMintRun(ctx, w); err != nil || ok {
		t.Fatalf("a cut mint recorded a run: %v, %v", ok, err)
	}
	if got := head(t, d); got != w {
		t.Fatalf("W moved to %d after a cut mint, want %d", got, w)
	}
	if err := stop(); err != nil {
		t.Fatal(err)
	}

	d2 := open(t, cfg)
	start(t, d2)
	waitMint(t, "the resumed mint", func() bool { return strings.Contains(logs.String(), "daemon: minted") })
	if run, ok, err := d2.readOps.LastMintRun(ctx, w); err != nil || !ok || run != w {
		t.Fatalf("last mint run after resuming = %d, %v, %v; want %d", run, ok, err, w)
	}
	all, err := d2.readOps.Checkpoints(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(partial) == 0 || len(partial) >= len(all) {
		t.Fatalf("the cut mint wrote %d of %d buckets, want some but not all", len(partial), len(all))
	}
	if !versionsAre1(all) {
		t.Errorf("a bucket the cut mint reached was cut again: %+v", all)
	}
	want := fmt.Sprintf("minted=%d buckets=%d", len(all)-len(partial), len(all))
	if out := logs.String(); !strings.Contains(out, want) {
		t.Errorf("the resumed mint did not log %q:\n%s", want, out)
	}
	if got := head(t, d2); got != w {
		t.Errorf("W moved to %d, want %d", got, w)
	}
}

func versionsAre1(rows []ops.CheckpointRow) bool {
	for _, r := range rows {
		if r.V != 1 {
			return false
		}
	}
	return true
}

// Card 268: the manual POST /v1/checkpoints keeps the request's budget, full or not:
// timeout, or Anam-Timeout when it is shorter, cuts a mint that outlasts it (504).
func TestPostCheckpointsKeepsTheRequestBudget(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		timeout time.Duration
		header  string
	}{
		{"timeout", 150 * time.Millisecond, ""},
		{"Anam-Timeout shorter than timeout", testTimeout, "150ms"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := importerConfig(t)
			w := seedUnminted(t, cfg, 2)
			cfg.LongWriteTimeout = time.Minute
			cfg.MintQuiet = time.Hour // only the POST mints
			d := open(t, cfg)
			d.cfg.Timeout = tt.timeout // after Open, which spends timeout on its own reads
			d.mintClock = slowClock{per: 200 * time.Millisecond}
			start(t, d)
			srv := httptest.NewServer(d.Handler())
			t.Cleanup(srv.Close)
			drain(t, d) // catch-up has run

			req, err := http.NewRequest("POST", srv.URL+"/v1/checkpoints?full=true", nil)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Authorization", "Bearer "+testToken)
			if tt.header != "" {
				req.Header.Set(TimeoutHeader, tt.header)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			_ = resp.Body.Close()
			if resp.StatusCode != http.StatusGatewayTimeout {
				t.Fatalf("POST /v1/checkpoints?full=true: %d, want 504", resp.StatusCode)
			}
			if _, ok, err := d.readOps.LastMintRun(context.Background(), w); err != nil || ok {
				t.Errorf("a mint cut at the request budget recorded a run: %v, %v", ok, err)
			}
		})
	}
}

// Card 268: while a write holds the queue, POST /v1/checkpoints answers busy within
// accept_wait, as before: the automatic mint's long budget is not the manual one's.
func TestPostCheckpointsBusyWithinAcceptWait(t *testing.T) {
	t.Parallel()
	cfg := newConfig(t)
	cfg.TokenFile = writeToken(t, t.TempDir(), testToken)
	cfg.AcceptWait = 100 * time.Millisecond
	d := open(t, cfg)
	start(t, d)
	srv := httptest.NewServer(d.Handler())
	t.Cleanup(srv.Close)
	hold(t, d)

	began := time.Now()
	call(t, "POST", srv.URL+"/v1/checkpoints", testToken, "", http.StatusServiceUnavailable, nil)
	if took := time.Since(began); took > 2*time.Second {
		t.Errorf("busy after %s, want about accept_wait (%s)", took, cfg.AcceptWait)
	}
}
