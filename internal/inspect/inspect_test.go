package inspect_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/inspect"
	"github.com/mattjoyce/anam/internal/span"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

// defaultBuckets is the bucketing the inspector renders at here. The bucket ids the
// pages show are derived from it rather than spelled out, so a change of default width
// is a width change and not a test failure.
//
// The error is discarded because span.Default is a declared width: New can only refuse
// it if the constants contradict each other, and then the zero Buckets it returns
// yields "bucket:0000-00" and every assertion below fails by name.
var defaultBuckets, _ = span.New(span.Default)

// The inspector is exercised over a real seeded ledger (AGENTS.md: real components),
// through its Handler so no port is bound.

const person = "test-person"

// seededServer builds a ledger holding two facets — one healthy, one tombstoned —
// and returns an httptest server over the inspector plus the two facet ids.
func seededServer(t *testing.T) (*httptest.Server, string, string) {
	t.Helper()
	ctx := context.Background()

	l, err := ledger.Create(ctx, filepath.Join(t.TempDir(), "ledger.db"), person)
	if err != nil {
		t.Fatalf("ledger.Create: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })

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

	// A generous request timeout: latency is not under test here.
	srv := httptest.NewServer(inspect.Handler(l, 30*time.Second, defaultBuckets))
	t.Cleanup(srv.Close)
	return srv, bp.ID, hr.ID
}

func seedFacet(t *testing.T, sourceRef, ftype, payload string) facet.Facet {
	t.Helper()
	if _, err := facet.Canonicalize([]byte(payload)); err != nil {
		t.Fatalf("canonicalize: %v", err)
	}
	return seedFacetNoT(sourceRef, ftype, payload)
}

// seedFacetNoT is the same fixture without a *testing.T, for use inside a sync.Once
// where a t.Fatalf would report against whichever test arrived first. The payloads are
// package constants, so a canonicalisation failure is a bug in this file, not data.
func seedFacetNoT(sourceRef, ftype, payload string) facet.Facet {
	canonical, err := facet.Canonicalize([]byte(payload))
	if err != nil {
		panic("inspect_test: fixture payload is not canonicalisable: " + err.Error())
	}
	et := time.Date(2026, 7, 5, 6, 30, 0, 0, time.UTC)
	return facet.Facet{
		ID: facet.NewID(sourceRef), Type: facet.Type(ftype), Confidence: 1,
		Classification: facet.ClassificationNormal, ArtifactVersion: 1, DefinitionVersion: 1,
		FactTime:   et,
		Binding:    facet.Binding{PersonID: person, Assurance: facet.AssuranceHigh},
		Provenance: facet.Provenance{Source: "test", Injector: "test", SourceRef: sourceRef},
		Descriptor: facet.Descriptor{
			SourceSchema: "test", SchemaVersion: 1, ContentHash: facet.ContentHash(canonical),
			PayloadKind: facet.PayloadVerbatim, MediaType: "application/json",
		},
		Payload: canonical,
	}
}

func event(kind facet.AnamEventKind, f facet.Facet, seq int64) facet.AnamEvent {
	return facet.AnamEvent{
		Kind: kind, FacetID: f.ID, AnamTime: time.Date(2026, 7, 10, 9, 0, 0, int(seq), time.UTC),
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
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)

	l, err := ledger.Create(ctx, filepath.Join(t.TempDir(), "ledger.db"), person)
	if err != nil {
		t.Fatalf("ledger.Create: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })

	go func() { done <- inspect.Serve(ctx, l, "127.0.0.1:0", time.Second, defaultBuckets) }()
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

func TestFacetList_ShowsFoldedStatus(t *testing.T) {
	t.Parallel()
	srv, bpID, hrID := seededServer(t)

	code, body := get(t, srv.URL+"/facets")
	if code != http.StatusOK {
		t.Fatalf("GET /facets = %d", code)
	}
	// The bucket id is derived, not spelled out: the list must show whichever bucket
	// the seeded fact-time falls in, so changing span.Default is a width change and
	// not a test failure.
	bucket := defaultBuckets.Of(time.Date(2026, 7, 5, 6, 30, 0, 0, time.UTC)).ID()
	for _, want := range []string{"W = 3", bpID, hrID, "tombstoned", "health.blood-pressure", bucket} {
		if !strings.Contains(body, want) {
			t.Errorf("facet list missing %q", want)
		}
	}
}

// The landing page draws both surfaces under their own names (§7): the capture
// band is the ledger — everything, the dead ghosted — and the composed strip is
// the canonical, live-only. The tombstoned facet must appear in the first and be
// absent from the second.
func TestTimeline_DrawsLedgerBandAndCanonicalStrip(t *testing.T) {
	t.Parallel()
	srv, bpID, hrID := seededServer(t)

	code, body := get(t, srv.URL+"/")
	if code != http.StatusOK {
		t.Fatalf("GET / = %d", code)
	}
	for _, want := range []string{"<svg", `class="pt dead"`, "canonical sha256:", "W = 3", bpID, hrID} {
		if !strings.Contains(body, want) {
			t.Errorf("timeline missing %q", want)
		}
	}

	stripAt := strings.Index(body, "compose — the canonical")
	if stripAt < 0 {
		t.Fatal("no composed-context section")
	}
	strip := body[stripAt:]
	if !strings.Contains(strip, bpID) {
		t.Error("live facet absent from the canonical strip")
	}
	if strings.Contains(strip, hrID) {
		t.Error("tombstoned facet leaked into the canonical strip — the strip must be live-only")
	}
}

// Narrowing is a lens over the same read: a span that excludes the bp facet
// composes an empty strip while the header still names the whole-person value.
func TestTimeline_SpanNarrowsTheStrip(t *testing.T) {
	t.Parallel()
	srv, bpID, _ := seededServer(t)

	// Both seeded facets sit at 2026-07-05; this span misses them.
	code, body := get(t, srv.URL+"/?from=2026-01-01&to=2026-01-31")
	if code != http.StatusOK {
		t.Fatalf("GET /?from&to = %d", code)
	}
	stripAt := strings.Index(body, "compose — the canonical")
	if stripAt < 0 {
		t.Fatal("no composed-context section")
	}
	if strings.Contains(body[stripAt:], bpID) {
		t.Error("facet outside the span composed into the strip")
	}
	if code, _ := get(t, srv.URL+"/?from=2026-07-09&to=2026-07-01"); code != http.StatusBadRequest {
		t.Errorf("empty window = %d, want 400", code)
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
