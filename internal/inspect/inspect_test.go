package inspect_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/inspect"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

// The inspector is exercised over a real seeded ledger (AGENTS.md: real components),
// through its Handler so no port is bound. Skips cleanly when DynamoDB Local is away.

const (
	endpoint = "http://192.168.20.4:8000"
	region   = "us-east-1"
	person   = "test-person"
)

var reachable bool

func TestMain(m *testing.M) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if resp, err := http.DefaultClient.Do(req); err == nil {
		_ = resp.Body.Close()
		reachable = true
	}
	os.Exit(m.Run())
}

// seededServer builds a ledger holding two facets — one healthy, one tombstoned —
// and returns an httptest server over the inspector plus the two facet ids.
func seededServer(t *testing.T) (*httptest.Server, string, string) {
	t.Helper()
	if !reachable {
		t.Skipf("DynamoDB Local unreachable at %s", endpoint)
	}
	ctx := context.Background()

	l, err := ledger.New(ctx, endpoint, region, "anam_test_"+uuid.NewString(), person)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := l.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}
	t.Cleanup(func() { _ = l.Drop(context.Background()) })

	bp := seedFacet(t, "bp-csv:2026-01-24T06:30:00", "health.blood-pressure",
		`{"systolic": 118, "diastolic": 76, "comment": "No filodipine"}`)
	if err := l.Admit(ctx, 0, bp, event(facet.EventAdmitted, bp, 1)); err != nil {
		t.Fatalf("Admit bp: %v", err)
	}

	hr := seedFacet(t, "health-db:hr:2026-07-05", "health.heart-rate", `{"resting": 54}`)
	if err := l.Admit(ctx, 1, hr, event(facet.EventAdmitted, hr, 2)); err != nil {
		t.Fatalf("Admit hr: %v", err)
	}
	tomb := event(facet.EventTombstoned, hr, 3)
	if err := l.Mark(ctx, 2, tomb); err != nil {
		t.Fatalf("Mark tombstone: %v", err)
	}

	srv := httptest.NewServer(inspect.Handler(l, 5*time.Second))
	t.Cleanup(srv.Close)
	return srv, bp.ID, hr.ID
}

func seedFacet(t *testing.T, sourceRef, ftype, payload string) facet.Facet {
	t.Helper()
	canonical, err := facet.Canonicalize([]byte(payload))
	if err != nil {
		t.Fatalf("canonicalize: %v", err)
	}
	et := time.Date(2026, 7, 5, 6, 30, 0, 0, time.UTC)
	return facet.Facet{
		ID: facet.NewID(sourceRef), Type: facet.Type(ftype), Confidence: 1,
		Classification: facet.ClassificationNormal, ArtifactVersion: 1, DefinitionVersion: 1,
		EventTime: et, RecordedTime: et, IngestedTime: et,
		Binding:     facet.Binding{PersonID: person, Assurance: facet.AssuranceHigh},
		Provenance:  facet.Provenance{Source: "test", Injector: "test", SourceRef: sourceRef},
		BirthStatus: facet.BornAdmitted,
		Descriptor: facet.Descriptor{
			SourceSchema: "test", SchemaVersion: 1, ContentHash: facet.ContentHash(canonical),
			PayloadKind: facet.PayloadVerbatim, MediaType: "application/json",
		},
		Payload: canonical,
	}
}

func event(kind facet.RecordEventKind, f facet.Facet, seq int64) facet.RecordEvent {
	return facet.RecordEvent{
		Kind: kind, FacetID: f.ID, RecordTime: time.Date(2026, 7, 10, 9, 0, 0, int(seq), time.UTC),
		Seq: seq, ArtifactVersion: 1,
	}
}

func get(t *testing.T, url string) (int, string) {
	t.Helper()
	resp, err := http.Get(url) //nolint:gosec // test URL from httptest
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return resp.StatusCode, string(body)
}

// Serve must exit cleanly when its context is cancelled — the CLI's Ctrl-C path.
func TestServe_ShutsDownOnContextCancel(t *testing.T) {
	t.Parallel()
	if !reachable {
		t.Skipf("DynamoDB Local unreachable at %s", endpoint)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)

	l, err := ledger.New(ctx, endpoint, region, "anam_test_"+uuid.NewString(), person)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := l.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}
	t.Cleanup(func() { _ = l.Drop(context.Background()) })

	go func() { done <- inspect.Serve(ctx, l, "127.0.0.1:0", time.Second) }()
	time.Sleep(200 * time.Millisecond) // let ListenAndServe bind
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Serve returned %v on cancel, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not shut down within 5s of cancel")
	}
}

func TestIndex_ListsFacetsWithWatermarkAndLastEvent(t *testing.T) {
	t.Parallel()
	srv, bpID, hrID := seededServer(t)

	code, body := get(t, srv.URL+"/")
	if code != http.StatusOK {
		t.Fatalf("GET / = %d", code)
	}
	for _, want := range []string{"W = 3", bpID, hrID, "tombstoned", "health.blood-pressure", "bucket:2026-W27"} {
		if !strings.Contains(body, want) {
			t.Errorf("index missing %q", want)
		}
	}
}

func TestFacetPage_ShowsVerbatimPayloadAndItsEvents(t *testing.T) {
	t.Parallel()
	srv, bpID, _ := seededServer(t)

	code, body := get(t, srv.URL+"/facet?id="+bpID)
	if code != http.StatusOK {
		t.Fatalf("GET /facet = %d", code)
	}
	for _, want := range []string{"No filodipine", "sha256:", "bp-csv:2026-01-24T06:30:00", "admitted"} {
		if !strings.Contains(body, want) {
			t.Errorf("facet page missing %q", want)
		}
	}
}

func TestFacetPage_UnknownIDIs404(t *testing.T) {
	t.Parallel()
	srv, _, _ := seededServer(t)

	if code, _ := get(t, srv.URL+"/facet?id=no-such-facet"); code != http.StatusNotFound {
		t.Errorf("GET /facet?id=missing = %d, want 404", code)
	}
	if code, _ := get(t, srv.URL+"/facet"); code != http.StatusBadRequest {
		t.Errorf("GET /facet without id = %d, want 400", code)
	}
}

func TestEvents_ListsTheStreamInSeqOrder(t *testing.T) {
	t.Parallel()
	srv, _, hrID := seededServer(t)

	code, body := get(t, srv.URL+"/events")
	if code != http.StatusOK {
		t.Fatalf("GET /events = %d", code)
	}
	if !strings.Contains(body, "W = 3") || !strings.Contains(body, hrID) {
		t.Error("events page missing watermark or facet link")
	}
	if strings.Index(body, ">1</td>") > strings.Index(body, ">3</td>") {
		t.Error("events not in seq order")
	}
}
