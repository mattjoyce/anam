package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mattjoyce/anam/internal/api"
)

// gatedIngest is an IngestFunc that announces each run and waits to be released. It
// stands in for capture, which this package does not know; app's tests run the real one.
type gatedIngest struct {
	started chan struct{}
	release chan error
	result  []api.SourceResult
}

func newGatedIngest() *gatedIngest {
	return &gatedIngest{
		started: make(chan struct{}, 8),
		release: make(chan error),
		result:  []api.SourceResult{{Source: "bp-csv", Offered: 3, Admitted: 3, Committed: 3}},
	}
}

func (g *gatedIngest) run(ctx context.Context, s Stores) ([]api.SourceResult, error) {
	g.started <- struct{}{}
	err := <-g.release
	return g.result, err
}

func openWith(t *testing.T, opts Options) *Daemon {
	t.Helper()
	d, err := Open(context.Background(), newConfig(t), opts)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	return d
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func (d *Daemon) state(id string) string {
	op, _ := d.Operation(id)
	return op.State
}

func mustRequest(t *testing.T, d *Daemon) string {
	t.Helper()
	id, err := d.RequestIngest()
	if err != nil {
		t.Fatalf("request ingest: %v", err)
	}
	return id
}

// A request while a run is in progress joins the pending run, never the running one,
// and never starts a third.
func TestIngestRequestsJoinThePendingRun(t *testing.T) {
	g := newGatedIngest()
	d := openWith(t, Options{Ingest: g.run})
	start(t, d)

	first := mustRequest(t, d)
	<-g.started
	second, third := mustRequest(t, d), mustRequest(t, d)
	if second == first {
		t.Fatal("a request during a run joined the running run, which has already read its sources")
	}
	if third != second {
		t.Fatalf("a third request made another run (%s), want the pending %s", third, second)
	}
	if got := d.state(first); got != api.StateRunning {
		t.Fatalf("first run is %s, want running", got)
	}
	if got := d.state(second); got != api.StatePending {
		t.Fatalf("second run is %s, want pending", got)
	}

	g.release <- nil
	<-g.started
	if got := d.state(first); got != api.StateSucceeded {
		t.Fatalf("first run is %s, want succeeded", got)
	}
	g.release <- nil
	waitFor(t, "the pending run to finish", func() bool { return d.state(second) == api.StateSucceeded })

	select {
	case <-g.started:
		t.Fatal("a third run started")
	case <-time.After(20 * time.Millisecond):
	}

	op, _ := d.Operation(first)
	if op.WBefore == nil || op.WAfter == nil {
		t.Fatalf("a finished run carries no W range: %+v", op)
	}
	if len(op.Sources) != 1 || op.Sources[0].Admitted != 3 {
		t.Fatalf("sources = %+v", op.Sources)
	}
	st := d.ingests.sourceStatuses()
	if len(st) != 1 || st[0].LastScanAt == "" || st[0].LastAdmissionAt == "" {
		t.Fatalf("source status = %+v", st)
	}
}

// A failed run keeps what it reported before failing: batches that committed are in
// the ledger whatever the error says.
func TestFailedIngestKeepsItsTallies(t *testing.T) {
	g := newGatedIngest()
	d := openWith(t, Options{Ingest: g.run})
	start(t, d)

	id := mustRequest(t, d)
	<-g.started
	g.release <- errors.New("commit batch at 500: ledger busy")
	waitFor(t, "the run to fail", func() bool { return d.state(id) == api.StateFailed })

	op, _ := d.Operation(id)
	if op.Error == "" || len(op.Sources) != 1 || op.WAfter == nil {
		t.Fatalf("failed run lost its record: %+v", op)
	}
}

func TestIngestOverHTTP(t *testing.T) {
	g := newGatedIngest()
	d := openWith(t, Options{Ingest: g.run, Version: "v0.test"})
	stop := start(t, d)
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/v1/ingest", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	var op api.Operation
	if err := json.NewDecoder(resp.Body).Decode(&op); err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted || op.ID == "" ||
		resp.Header.Get("Location") != "/v1/operations/"+op.ID {
		t.Fatalf("POST /v1/ingest: %d %+v location %q", resp.StatusCode, op, resp.Header.Get("Location"))
	}

	<-g.started
	var status api.Status
	getJSON(t, srv.URL+"/v1/status", http.StatusOK, &status)
	if status.Ingest.Running != op.ID || status.Version != "v0.test" || status.LedgerID == "" {
		t.Fatalf("status = %+v", status)
	}
	g.release <- nil
	waitFor(t, "the run to finish", func() bool { return d.state(op.ID) == api.StateSucceeded })

	var got api.Operation
	getJSON(t, srv.URL+"/v1/operations/"+op.ID, http.StatusOK, &got)
	if got.State != api.StateSucceeded {
		t.Fatalf("operation = %+v", got)
	}
	getJSON(t, srv.URL+"/v1/operations/op-gone", http.StatusNotFound, &map[string]string{})

	if err := stop(); err != nil {
		t.Fatal(err)
	}
	resp, err = http.Post(srv.URL+"/v1/ingest", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("POST after shutdown: %d, want 503", resp.StatusCode)
	}
}

func getJSON(t *testing.T, url string, want int, into any) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != want {
		t.Fatalf("GET %s: %d, want %d", url, resp.StatusCode, want)
	}
	if err := json.NewDecoder(resp.Body).Decode(into); err != nil {
		t.Fatalf("GET %s: decode: %v", url, err)
	}
}

func TestIngestWait(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		release    error
		wantStatus int
		wantState  string
		wantSumm   string
	}{
		{"a run that succeeds", "?wait=5s", nil, http.StatusOK, api.StateSucceeded, "ledger changed: 3 facets admitted or superseded"},
		{"a run that fails", "?wait=5s", errors.New("source gone"), http.StatusInternalServerError, api.StateFailed, ""},
		{"a wait that ends first", "?wait=30ms", nil, http.StatusAccepted, "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := newGatedIngest()
			d := openWith(t, Options{Ingest: g.run})
			start(t, d)
			srv := httptest.NewServer(d.Handler())
			defer srv.Close()

			go func() {
				<-g.started
				if tt.wantStatus == http.StatusAccepted {
					time.Sleep(100 * time.Millisecond)
				}
				g.release <- tt.release
			}()
			resp, err := http.Post(srv.URL+"/v1/ingest"+tt.query, "", nil)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = resp.Body.Close() }()
			var op api.Operation
			if err := json.NewDecoder(resp.Body).Decode(&op); err != nil {
				t.Fatal(err)
			}
			// An unfinished run may still be pending or already running; either is
			// what a wait that ends first should report.
			stateOK := op.State == tt.wantState || (tt.wantState == "" && !op.Finished())
			if resp.StatusCode != tt.wantStatus || !stateOK || op.Summary != tt.wantSumm {
				t.Fatalf("got %d %s %q, want %d %s %q",
					resp.StatusCode, op.State, op.Summary, tt.wantStatus, tt.wantState, tt.wantSumm)
			}
			waitFor(t, "the run to finish", func() bool { o, _ := d.Operation(op.ID); return o.Finished() })
		})
	}
}

func TestIngestWaitRefusesANonsenseDuration(t *testing.T) {
	d := openWith(t, Options{Ingest: newGatedIngest().run})
	t.Cleanup(func() { _ = d.closeStores() })
	for _, q := range []string{"?wait=soon", "?wait=-1s", "?wait=0s"} {
		rec := httptest.NewRecorder()
		d.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/ingest"+q, nil))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("POST /v1/ingest%s: %d, want 400", q, rec.Code)
		}
	}
}

// Shutdown answers a caller waiting on a run at once, rather than holding Shutdown up
// until its grace runs out and then reporting an error.
func TestShutdownAnswersAWaitingCaller(t *testing.T) {
	g := newGatedIngest()
	d := openWith(t, Options{Ingest: g.run})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	ran := make(chan error, 1)
	go func() { ran <- d.Serve(ctx, ln) }()

	answered := make(chan int, 1)
	go func() {
		resp, err := http.Post("http://"+ln.Addr().String()+"/v1/ingest?wait=1m", "", nil)
		if err != nil {
			answered <- -1
			return
		}
		_ = resp.Body.Close()
		answered <- resp.StatusCode
	}()
	<-g.started

	began := time.Now()
	cancel()
	select {
	case code := <-answered:
		if code != http.StatusAccepted {
			t.Fatalf("waiting caller got %d at shutdown, want 202", code)
		}
		if waited := time.Since(began); waited > 2*time.Second {
			t.Fatalf("waiting caller was answered after %s", waited)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown left a waiting caller hanging")
	}

	g.release <- nil
	if err := <-ran; err != nil {
		t.Fatalf("run: %v", err)
	}
}

// Minting waits on the write queue, so while an ingest holds it the answer is a 503
// at once that names the run, never a request held for the length of the ingest.
func TestMintWhileAnIngestRunsIsBusy(t *testing.T) {
	g := newGatedIngest()
	d := openWith(t, Options{Ingest: g.run})
	start(t, d)
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()

	id := mustRequest(t, d)
	<-g.started
	began := time.Now()
	resp, err := http.Post(srv.URL+"/v1/checkpoints", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	var e api.Error
	_ = json.NewDecoder(resp.Body).Decode(&e)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable || !strings.Contains(e.Error, id) {
		t.Fatalf("mint during an ingest: %d %q, want 503 naming %s", resp.StatusCode, e.Error, id)
	}
	if waited := time.Since(began); waited > time.Second {
		t.Errorf("the busy answer took %s", waited)
	}
	g.release <- nil
	waitFor(t, "the run to finish", func() bool { return d.state(id) == api.StateSucceeded })

	resp, err = http.Post(srv.URL+"/v1/checkpoints?dry_run=true", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("mint after the ingest: %d", resp.StatusCode)
	}
}
