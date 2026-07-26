package ledger_test

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

// The tests run against real DynamoDB Local (AGENTS.md: real components over mocks),
// on a unique table per run so parallel packages and -shuffle=on cannot share state.
// TestMain probes the endpoint and skips the package cleanly when it is unreachable.

const (
	endpoint = "http://192.168.20.4:8000"
	region   = "us-east-1"
	person   = "test-person"
)

var reachable bool

func TestMain(m *testing.M) {
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if resp, err := http.DefaultClient.Do(req); err == nil {
		_ = resp.Body.Close() // probe only; the body is not read
		reachable = true      // DynamoDB Local answers 400 to a bare GET; alive is alive
	}
	os.Exit(m.Run())
}

// newLedger creates an initialised Ledger on a fresh uniquely-named table and
// registers its teardown.
func newLedger(t *testing.T) *ledger.Ledger {
	t.Helper()
	if !reachable {
		t.Skipf("DynamoDB Local unreachable at %s — start it (unraid) to run ledger tests", endpoint)
	}

	table := "anam_test_" + uuid.NewString()
	ctx := context.Background()

	l, err := ledger.New(ctx, endpoint, region, table, person)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := l.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}
	t.Cleanup(func() {
		if err := l.Drop(context.Background()); err != nil {
			t.Errorf("drop table %s: %v", table, err)
		}
	})
	return l
}

// testFacet builds a valid facet from a payload, with identity derived the way the
// gate will derive it: id = uuidv5(source_ref), hash over the canonical bytes.
func testFacet(t *testing.T, sourceRef, payload string, version int) facet.Facet {
	t.Helper()

	canonical, err := facet.Canonicalize([]byte(payload))
	if err != nil {
		t.Fatalf("canonicalize: %v", err)
	}
	factTime := time.Date(2026, 7, 5, 6, 30, 0, 0, time.UTC)
	return facet.Facet{
		ID:                facet.NewID(sourceRef),
		Type:              facet.Type("health.blood-pressure"),
		Confidence:        1.0,
		Classification:    facet.ClassificationNormal,
		ArtifactVersion:   version,
		DefinitionVersion: 1,
		FactTime:          factTime,
		Binding: facet.Binding{
			PersonID:  person,
			Assurance: facet.AssuranceHigh,
		},
		Provenance: facet.Provenance{
			Source:    "bp-csv",
			Injector:  "bp-csv",
			SourceRef: sourceRef,
		},
		Descriptor: facet.Descriptor{
			SourceSchema:  "bp-csv/reading",
			SchemaVersion: 1,
			ContentHash:   facet.ContentHash(canonical),
			PayloadKind:   facet.PayloadVerbatim,
			MediaType:     "application/json",
		},
		Payload: canonical,
	}
}

func admittedEvent(f facet.Facet, seq int64) facet.AnamEvent {
	return facet.AnamEvent{
		Kind:            facet.EventAdmitted,
		FacetID:         f.ID,
		AnamTime:        time.Date(2026, 7, 10, 9, 0, 0, int(seq), time.UTC),
		Seq:             seq,
		ArtifactVersion: f.ArtifactVersion,
	}
}

// On a -sharedDb endpoint, tables from earlier schemas survive indefinitely. Init on
// a table whose key schema differs must refuse loudly, not treat "already exists" as
// "already correct" — a silently divergent table is a corrupt kernel.
func TestInit_RefusesMismatchedSchema(t *testing.T) {
	t.Parallel()
	if !reachable {
		t.Skipf("DynamoDB Local unreachable at %s", endpoint)
	}
	ctx := context.Background()
	table := "anam_test_" + uuid.NewString()

	// Create a pk-only table straight through the wire API, bypassing the package.
	createBody := fmt.Sprintf(`{
		"TableName": %q,
		"BillingMode": "PAY_PER_REQUEST",
		"AttributeDefinitions": [{"AttributeName": "pk", "AttributeType": "S"}],
		"KeySchema": [{"AttributeName": "pk", "KeyType": "HASH"}]
	}`, table)
	rawDynamo(t, "CreateTable", createBody)

	l, err := ledger.New(ctx, endpoint, region, table, person)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() {
		if err := l.Drop(context.Background()); err != nil {
			t.Errorf("drop table %s: %v", table, err)
		}
	})

	if err := l.Init(ctx); !errors.Is(err, ledger.ErrSchemaMismatch) {
		t.Fatalf("Init on a pk-only table = %v, want ErrSchemaMismatch", err)
	}
}

// rawDynamo issues one DynamoDB Local wire call outside the SDK. Local does not
// authenticate, so the Authorization header only needs the right shape.
//
// The budget is deliberately loose. DynamoDB Local is one process backed by one file,
// shared by every package in a `-race -shuffle=on` run that is creating and dropping
// its own table; a CreateTable that takes half a second alone can take many seconds
// behind that queue. A tight budget here does not catch a slow server, it catches a
// busy one — and reports it as a broken test. The deadline still exists, because a
// hung call must fail rather than hang; it is just sized for contention rather than
// for an idle machine.
const rawDynamoBudget = 30 * time.Second

func rawDynamo(t *testing.T, target, body string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), rawDynamoBudget)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/x-amz-json-1.0")
	req.Header.Set("X-Amz-Target", "DynamoDB_20120810."+target)
	req.Header.Set("Authorization",
		"AWS4-HMAC-SHA256 Credential=fake/20260710/us-east-1/dynamodb/aws4_request, SignedHeaders=content-type;x-amz-target, Signature=fake")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("raw %s: %v", target, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("raw %s: HTTP %d: %s", target, resp.StatusCode, b)
	}
}

func TestInit_IsIdempotentAndSeedsTheCounter(t *testing.T) {
	t.Parallel()
	l := newLedger(t)
	ctx := context.Background()

	// Init ran once in newLedger; a second run must be a no-op, not an error.
	if err := l.Init(ctx); err != nil {
		t.Fatalf("second Init: %v", err)
	}

	w, err := l.Head(ctx)
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	if w != 0 {
		t.Errorf("fresh ledger: W = %d, want 0", w)
	}
}

func TestAdmit_RoundTripsByteIdentical(t *testing.T) {
	t.Parallel()
	l := newLedger(t)
	ctx := context.Background()

	payload := `{"systolic": 118, "diastolic": 76, "pulse": 61, "comment": "No filodipine"}`
	f := testFacet(t, "bp-csv:2026-01-24T06:30:00", payload, 1)

	if err := l.Admit(ctx, 0, f, admittedEvent(f, 1)); err != nil {
		t.Fatalf("Admit: %v", err)
	}

	got, err := l.CurrentVersion(ctx, f.ID)
	if err != nil {
		t.Fatalf("CurrentVersion: %v", err)
	}
	if string(got.Payload.Bytes()) != string(f.Payload.Bytes()) {
		t.Errorf("payload did not round-trip byte-identical:\n got %s\nwant %s",
			got.Payload.Bytes(), f.Payload.Bytes())
	}
	if got.Descriptor.ContentHash != f.Descriptor.ContentHash {
		t.Errorf("content hash: got %s, want %s", got.Descriptor.ContentHash, f.Descriptor.ContentHash)
	}
	if !got.FactTime.Equal(f.FactTime) {
		t.Errorf("fact time: got %s, want %s", got.FactTime, f.FactTime)
	}

	w, err := l.Head(ctx)
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	if w != 1 {
		t.Errorf("after one admit: W = %d, want 1", w)
	}
}

// A crashed-and-rerun commit is the crash-clean property: the rerun sees its own
// earlier write and reports ErrAlreadyWritten, never a duplicate or a bare conflict.
func TestAdmit_RerunIsIdempotentNoOp(t *testing.T) {
	t.Parallel()
	l := newLedger(t)
	ctx := context.Background()

	f := testFacet(t, "bp-csv:2026-01-25T06:30:00", `{"systolic": 121}`, 1)
	if err := l.Admit(ctx, 0, f, admittedEvent(f, 1)); err != nil {
		t.Fatalf("first Admit: %v", err)
	}

	// The re-run: the worker crashed before recording success, re-read W (now 1),
	// and re-attempts the same artifact with the next seq.
	err := l.Admit(ctx, 1, f, admittedEvent(f, 2))
	if !errors.Is(err, ledger.ErrAlreadyWritten) {
		t.Fatalf("re-run Admit = %v, want ErrAlreadyWritten", err)
	}

	// The no-op must not have advanced the watermark.
	w, err := l.Head(ctx)
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	if w != 1 {
		t.Errorf("after idempotent re-run: W = %d, want 1", w)
	}
}

// The same identity with different bytes is a lost update, not idempotency — the
// arm of the error map that must never be collapsed into "already written".
func TestAdmit_DivergentContentIsRefused(t *testing.T) {
	t.Parallel()
	l := newLedger(t)
	ctx := context.Background()

	f1 := testFacet(t, "bp-csv:2026-01-26T06:30:00", `{"systolic": 118}`, 1)
	if err := l.Admit(ctx, 0, f1, admittedEvent(f1, 1)); err != nil {
		t.Fatalf("Admit: %v", err)
	}

	f2 := testFacet(t, "bp-csv:2026-01-26T06:30:00", `{"systolic": 999}`, 1)
	err := l.Admit(ctx, 1, f2, admittedEvent(f2, 2))
	if !errors.Is(err, ledger.ErrDivergentArtifact) {
		t.Fatalf("divergent Admit = %v, want ErrDivergentArtifact", err)
	}
}

// A writer that read a stale watermark must lose, and must be told to retry — not
// told its artifact exists (it does not).
func TestAdmit_StaleWatermarkLoses(t *testing.T) {
	t.Parallel()
	l := newLedger(t)
	ctx := context.Background()

	f1 := testFacet(t, "bp-csv:2026-01-27T06:30:00", `{"systolic": 118}`, 1)
	if err := l.Admit(ctx, 0, f1, admittedEvent(f1, 1)); err != nil {
		t.Fatalf("Admit: %v", err)
	}

	// A second writer that read W=0 before the first commit landed.
	f2 := testFacet(t, "bp-csv:2026-01-28T06:30:00", `{"systolic": 122}`, 1)
	err := l.Admit(ctx, 0, f2, admittedEvent(f2, 1))
	if !errors.Is(err, ledger.ErrStaleWatermark) {
		t.Fatalf("stale Admit = %v, want ErrStaleWatermark", err)
	}

	// The loser retries with the fresh head, as the gate will.
	if err := l.Admit(ctx, 1, f2, admittedEvent(f2, 2)); err != nil {
		t.Fatalf("retry after stale: %v", err)
	}
}

// A tombstone is an anam-event and nothing else (§6a): Mark advances the watermark
// with no artifact written.
func TestMark_TombstoneAdvancesWatermarkWithoutArtifact(t *testing.T) {
	t.Parallel()
	l := newLedger(t)
	ctx := context.Background()

	f := testFacet(t, "bp-csv:2026-01-29T06:30:00", `{"systolic": 130}`, 1)
	if err := l.Admit(ctx, 0, f, admittedEvent(f, 1)); err != nil {
		t.Fatalf("Admit: %v", err)
	}

	tomb := facet.AnamEvent{
		Kind:            facet.EventTombstoned,
		FacetID:         f.ID,
		AnamTime:        time.Date(2026, 7, 10, 10, 0, 0, 0, time.UTC),
		Seq:             2,
		ArtifactVersion: 1,
	}
	if err := l.Mark(ctx, 1, tomb); err != nil {
		t.Fatalf("Mark: %v", err)
	}

	w, err := l.Head(ctx)
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	if w != 2 {
		t.Errorf("after admit+tombstone: W = %d, want 2", w)
	}

	// The artifact is untouched: still version 1, still the same bytes.
	got, err := l.CurrentVersion(ctx, f.ID)
	if err != nil {
		t.Fatalf("CurrentVersion: %v", err)
	}
	if got.ArtifactVersion != 1 {
		t.Errorf("tombstone must not write an artifact: version = %d, want 1", got.ArtifactVersion)
	}
}

// Replay returns exactly the half-open window (since, upto], in seq order — the scan
// every derived state is rebuilt from, and the one an as-of read bounds.
func TestReplay_WindowAndOrder(t *testing.T) {
	t.Parallel()
	l := newLedger(t)
	ctx := context.Background()

	var facets []facet.Facet
	for i := 0; i < 4; i++ {
		f := testFacet(t, fmt.Sprintf("bp-csv:2026-02-0%dT06:30:00", i+1),
			fmt.Sprintf(`{"systolic": %d}`, 110+i), 1)
		if err := l.Admit(ctx, ledger.Watermark(i), f, admittedEvent(f, int64(i)+1)); err != nil {
			t.Fatalf("Admit %d: %v", i, err)
		}
		facets = append(facets, f)
	}

	// Everything since genesis.
	all, err := l.Replay(ctx, 0, 4)
	if err != nil {
		t.Fatalf("Replay(0,4]: %v", err)
	}
	if len(all) != 4 {
		t.Fatalf("Replay(0,4] returned %d events, want 4", len(all))
	}
	for i, ev := range all {
		if ev.Seq != int64(i)+1 {
			t.Errorf("event %d out of order: seq %d", i, ev.Seq)
		}
	}

	// A bounded window: as-of seq 3, having already seen 1.
	window, err := l.Replay(ctx, 1, 3)
	if err != nil {
		t.Fatalf("Replay(1,3]: %v", err)
	}
	if len(window) != 2 || window[0].Seq != 2 || window[1].Seq != 3 {
		t.Errorf("Replay(1,3] = %+v, want seqs [2 3]", window)
	}
	if window[0].FacetID != facets[1].ID {
		t.Errorf("Replay(1,3] first event facet = %s, want %s", window[0].FacetID, facets[1].ID)
	}

	// An empty or inverted window is empty, not an error.
	if none, err := l.Replay(ctx, 4, 4); err != nil || len(none) != 0 {
		t.Errorf("Replay(4,4] = %v, %v — want empty, nil", none, err)
	}
}

// The seq a caller stamps must follow the watermark it read; the ledger refuses the
// pair rather than writing an anam-event whose key disagrees with the counter.
func TestAdmit_SeqMustFollowWatermark(t *testing.T) {
	t.Parallel()
	l := newLedger(t)
	ctx := context.Background()

	f := testFacet(t, "bp-csv:2026-03-01T06:30:00", `{"systolic": 118}`, 1)
	if err := l.Admit(ctx, 0, f, admittedEvent(f, 7)); err == nil {
		t.Fatal("Admit with seq 7 against W=0 succeeded, want refusal")
	}
}
