package daemon

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/gate"
	"github.com/mattjoyce/anam/internal/store/ops"
)

// manyMonths is how many monthly buckets a store made slow to mint holds. A mint
// writes three rows for each bucket it cuts, so a full mint over this many takes far
// longer than shortTimeout on any machine, while seeding it costs a few batches.
const manyMonths = 120

// shortTimeout is a per-call timeout no full mint over manyMonths buckets finishes in:
// such a mint takes most of a second under -race on an idle machine, and seconds on a
// busy one. It is long enough for the mint to start.
const shortTimeout = 100 * time.Millisecond

// weighInOn is a weigh-in at 07:30 on day in Sydney, a date like 2001-02-15.
func weighInOn(t *testing.T, id, day string, version int) string {
	t.Helper()
	syd, err := time.LoadLocation("Australia/Sydney")
	if err != nil {
		t.Fatal(err)
	}
	at, err := time.ParseInLocation("2006-01-02 15:04", day+" 07:30", syd)
	if err != nil {
		t.Fatal(err)
	}
	local := at.Format("2006-01-02 15:04")
	return fmt.Sprintf(`{"type": "health.weight", "definition_version": %d,
		"payload": {"id": %q, "date": %q, "source_time": {"value": %q, "zone": "Australia/Sydney", "offset": %q, "basis": "source"}, "kg": 80}}`,
		version, id, local, local, at.Format("-07:00"))
}

// seedUnminted admits a weigh-in in each of months months, from January 1990, on a
// daemon that stops before it mints, as one stopped just after a batch, or a store
// carried with no mint runs, leaves it: the next daemon on cfg starts behind, with no
// run to start from, and its mint cuts every bucket. It returns the head.
func seedUnminted(t *testing.T, cfg config.Config, months int) int64 {
	t.Helper()
	cfg.MintQuiet = time.Hour // stopped long before the mint would run
	d, url, stop := serveImporters(t, cfg)
	weight, _ := d.defs.current.Load().Lookup("health.weight")
	var envs []string
	for i := range months {
		day := time.Date(1990, time.January, 15, 0, 0, 0, 0, time.UTC).AddDate(0, i, 0).Format("2006-01-02")
		envs = append(envs, weighInOn(t, fmt.Sprintf("w-%d", i), day, weight.Version))
	}
	for len(envs) > 0 {
		n := min(len(envs), gate.MaxBatch)
		call(t, "POST", url+"/v1/envelopes", importerToken, batch(envs[:n]...), http.StatusOK, nil)
		envs = envs[n:]
	}
	w := head(t, d)
	if err := stop(); err != nil {
		t.Fatalf("stop the seeding daemon: %v", err)
	}
	if w != int64(months) {
		t.Fatalf("head %d after %d weigh-ins", w, months)
	}
	return w
}

// copyStores is cfg on a copy of its ledger and operational store, in a directory of
// its own, so each attempt at a mint starts from the same files. The daemon on cfg
// must be stopped.
func copyStores(t *testing.T, cfg config.Config) config.Config {
	t.Helper()
	dir := t.TempDir()
	out := cfg
	out.LedgerPath = filepath.Join(dir, filepath.Base(cfg.LedgerPath))
	out.DBPath = filepath.Join(dir, filepath.Base(cfg.DBPath))
	out.LibraryDir = filepath.Join(dir, "documents")
	for from, to := range map[string]string{cfg.LedgerPath: out.LedgerPath, cfg.DBPath: out.DBPath} {
		for _, suffix := range []string{"", "-wal", "-shm"} {
			copyFile(t, from+suffix, to+suffix)
		}
	}
	return out
}

func copyFile(t *testing.T, from, to string) {
	t.Helper()
	src, err := os.Open(from)
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = src.Close() }()
	dst, err := os.OpenFile(to, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(dst, src); err != nil {
		t.Fatal(err)
	}
	if err := dst.Close(); err != nil {
		t.Fatal(err)
	}
}

// openIdle opens and runs a daemon on cfg that mints only when the test calls
// mintAtHead, with timeout and long_write_timeout as given. They are set once Open
// has returned, since Open spends timeout on its own reads; nothing has run yet.
// Catch-up at start has nothing to do: the seeding daemon caught up inside each
// batch. It returns once catch-up has run, with how to stop the daemon.
func openIdle(t *testing.T, cfg config.Config, timeout, long time.Duration) (*Daemon, func() error) {
	t.Helper()
	cfg.MintQuiet = time.Hour // the mint at start is requested but never runs
	d := open(t, cfg)
	d.cfg.Timeout, d.cfg.LongWriteTimeout = timeout, long
	stop := start(t, d)
	drain(t, d)
	return d, stop
}

// Card 268: the automatic mint runs under long_write_timeout, not the per-call
// timeout. Both its callers, the mint worker after a batch and mintIfBehind at start,
// run mintAtHead, which logs "daemon: minted" or "mint at head failed" as the run it
// returns says. On a store with no run and many buckets, a full mint takes far longer
// than timeout. It must finish, not be cut at timeout as the first mint on the
// carried schema-10 store was twice.
func TestAutomaticMintHasTheLongWriteBudget(t *testing.T) {
	t.Parallel()
	cfg := importerConfig(t)
	w := seedUnminted(t, cfg, manyMonths)
	d, _ := openIdle(t, cfg, shortTimeout, time.Minute)

	began := time.Now()
	run := d.mintAtHead()
	took := time.Since(began)
	if run.Error != "" {
		t.Fatalf("the automatic mint was cut short: %s", run.Error)
	}
	t.Logf("a full mint of %d buckets took %s against a timeout of %s", run.Buckets, took, shortTimeout)
	if !run.Full || run.W != w || run.Minted != manyMonths || run.Buckets != manyMonths {
		t.Errorf("mint = %+v, want a full mint of %d buckets at W=%d", *run, manyMonths, w)
	}
	if last, ok, err := d.readOps.LastMintRun(context.Background(), w); err != nil || !ok || last != w {
		t.Fatalf("last mint run = %d, %v, %v; want %d", last, ok, err, w)
	}
}

// Card 268: a mint that the long budget does cut short leaves the store as a cut
// mint always has. What it minted stays minted, it records no run, W does not move,
// and the next mint, a full one since there is still no run, cuts only the buckets
// the first did not reach and re-cuts none.
//
// The cut is made by long_write_timeout alone, so where it lands depends on how fast
// the machine is. Each attempt runs on a fresh copy of the seeded store and moves the
// budget toward the middle of the mint from what it saw: nothing minted means too
// short, a finished mint (or one cut only after it recorded its run) too long.
func TestMintCutShortResumes(t *testing.T) {
	t.Parallel()
	seeded := importerConfig(t)
	w := seedUnminted(t, seeded, manyMonths)
	ctx := context.Background()

	var cfg config.Config
	var partial []ops.CheckpointRow
	budget := 50 * time.Millisecond
	for attempt := 0; ; attempt++ {
		if attempt == 12 {
			t.Fatalf("no budget cut the mint partway; last tried %s", budget)
		}
		cfg = copyStores(t, seeded)
		d, stop := openIdle(t, cfg, testTimeout, budget)
		run := d.mintAtHead()
		rows, err := d.readOps.Checkpoints(ctx)
		if err != nil {
			t.Fatal(err)
		}
		_, recorded, err := d.readOps.LastMintRun(ctx, w)
		if err != nil {
			t.Fatal(err)
		}
		if got := head(t, d); got != w {
			t.Fatalf("W moved to %d after a mint, want %d", got, w)
		}
		if err := stop(); err != nil {
			t.Fatal(err)
		}
		switch {
		case run.Error == "" || recorded || len(rows) == manyMonths:
			budget /= 2
		case len(rows) == 0:
			budget *= 2
		default:
			partial = rows
		}
		if partial != nil {
			t.Logf("attempt %d: %s cut the mint after %d of %d buckets: %s", attempt, budget, len(rows), manyMonths, run.Error)
			break
		}
	}

	d, _ := openIdle(t, cfg, testTimeout, time.Minute)
	run := d.mintAtHead()
	if run.Error != "" {
		t.Fatalf("the resumed mint failed: %s", run.Error)
	}
	if !run.Full || run.Buckets != manyMonths || run.Minted != manyMonths-len(partial) {
		t.Errorf("resumed mint = %+v, want a full mint that cuts the %d buckets the cut one did not reach",
			*run, manyMonths-len(partial))
	}
	all, err := d.readOps.Checkpoints(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != manyMonths {
		t.Errorf("%d checkpoints after resuming, want %d", len(all), manyMonths)
	}
	for _, r := range all {
		if r.V != 1 {
			t.Errorf("bucket %s cut again, at v%d", r.SpanID, r.V)
		}
	}
	if last, ok, err := d.readOps.LastMintRun(ctx, w); err != nil || !ok || last != w {
		t.Fatalf("last mint run after resuming = %d, %v, %v; want %d", last, ok, err, w)
	}
	if got := head(t, d); got != w {
		t.Errorf("W moved to %d, want %d", got, w)
	}
}

// Card 268: the manual POST /v1/checkpoints keeps the request's budget, full or not:
// timeout, or Anam-Timeout when it is shorter, cuts a mint that outlasts it. With the
// long budget the mint would finish and answer 200 with a run recorded. A cut mint
// usually answers 504; a deadline that lands inside a store call can surface as
// that call's own error, so the test asks only that the mint did not finish.
func TestPostCheckpointsKeepsTheRequestBudget(t *testing.T) {
	t.Parallel()
	seeded := importerConfig(t)
	w := seedUnminted(t, seeded, manyMonths)
	tests := []struct {
		name    string
		timeout time.Duration
		header  string
		query   string
	}{
		{"timeout", shortTimeout, "", ""},
		{"timeout, full", shortTimeout, "", "?full=true"},
		{"Anam-Timeout shorter than timeout", testTimeout, shortTimeout.String(), ""},
		{"Anam-Timeout shorter than timeout, full", testTimeout, shortTimeout.String(), "?full=true"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			d, _ := openIdle(t, copyStores(t, seeded), tt.timeout, time.Minute)
			srv := httptest.NewServer(d.Handler())
			t.Cleanup(srv.Close)

			req, err := http.NewRequest("POST", srv.URL+"/v1/checkpoints"+tt.query, nil)
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
			t.Logf("POST /v1/checkpoints%s: %d", tt.query, resp.StatusCode)
			if resp.StatusCode == http.StatusOK {
				t.Fatalf("POST /v1/checkpoints%s: 200, a mint that outlasts the request's budget finished", tt.query)
			}
			if _, ok, err := d.readOps.LastMintRun(context.Background(), w); err != nil || ok {
				t.Errorf("a mint held to the request budget recorded a run: %v, %v", ok, err)
			}
		})
	}
}

// Card 268: while a write holds the queue, POST /v1/checkpoints answers busy after
// accept_wait, as before: the automatic mint's long budget is not the manual one's.
func TestPostCheckpointsBusyWithinAcceptWait(t *testing.T) {
	t.Parallel()
	const acceptWait = 100 * time.Millisecond
	cfg := newConfig(t)
	cfg.TokenFile = writeToken(t, t.TempDir(), testToken)
	cfg.AcceptWait = acceptWait
	d := open(t, cfg)
	start(t, d)
	srv := httptest.NewServer(d.Handler())
	t.Cleanup(srv.Close)
	hold(t, d)

	began := time.Now()
	call(t, "POST", srv.URL+"/v1/checkpoints", testToken, "", http.StatusServiceUnavailable, nil)
	// Room for scheduling, but well short of a wait of a second or of timeout.
	if took := time.Since(began); took < acceptWait || took > acceptWait+500*time.Millisecond {
		t.Errorf("busy after %s, want about accept_wait (%s)", took, acceptWait)
	}
}
