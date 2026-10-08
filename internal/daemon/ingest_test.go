package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// gatedIngest is an IngestFunc that announces each run and waits to be released. It
// stands in for capture, which this package does not know; app's tests run the real one.
type gatedIngest struct {
	started chan struct{}
	release chan error
	result  []SourceResult
}

func newGatedIngest() *gatedIngest {
	return &gatedIngest{
		started: make(chan struct{}, 8),
		release: make(chan error),
		result:  []SourceResult{{Source: "bp-csv", Offered: 3, Admitted: 3, Committed: 3}},
	}
}

func (g *gatedIngest) run(ctx context.Context, s Stores) ([]SourceResult, error) {
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
	if got := d.state(first); got != StateRunning {
		t.Fatalf("first run is %s, want running", got)
	}
	if got := d.state(second); got != StatePending {
		t.Fatalf("second run is %s, want pending", got)
	}

	g.release <- nil
	<-g.started
	if got := d.state(first); got != StateSucceeded {
		t.Fatalf("first run is %s, want succeeded", got)
	}
	g.release <- nil
	waitFor(t, "the pending run to finish", func() bool { return d.state(second) == StateSucceeded })

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
	waitFor(t, "the run to fail", func() bool { return d.state(id) == StateFailed })

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
	var op Operation
	if err := json.NewDecoder(resp.Body).Decode(&op); err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted || op.ID == "" ||
		resp.Header.Get("Location") != "/v1/operations/"+op.ID {
		t.Fatalf("POST /v1/ingest: %d %+v location %q", resp.StatusCode, op, resp.Header.Get("Location"))
	}

	<-g.started
	var status Status
	getJSON(t, srv.URL+"/v1/status", http.StatusOK, &status)
	if status.Ingest.Running != op.ID || status.Version != "v0.test" || status.LedgerID == "" {
		t.Fatalf("status = %+v", status)
	}
	g.release <- nil
	waitFor(t, "the run to finish", func() bool { return d.state(op.ID) == StateSucceeded })

	var got Operation
	getJSON(t, srv.URL+"/v1/operations/"+op.ID, http.StatusOK, &got)
	if got.State != StateSucceeded {
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
