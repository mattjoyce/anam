package serve_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/consent"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/gate"
	"github.com/mattjoyce/anam/internal/projection"
	"github.com/mattjoyce/anam/internal/receipt"
	"github.com/mattjoyce/anam/internal/serve"
	"github.com/mattjoyce/anam/internal/store/artifact"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

// Serve is tested end to end over the real ledger and the real artifact tier
// (AGENTS.md: real components over mocks): a unique table and a temp directory per
// test, skipping cleanly when the endpoint is away.

const (
	endpoint = "http://192.168.20.4:8000"
	region   = "us-east-1"
	person   = "matt"
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

// Two definitions, so a projection has something to prune away. The bundled bp and
// sleep templates are the ones under test, so the types must be theirs.
const bpManifest = `{
  "meta": {
    "facet_type": "health.blood-pressure",
    "definition_version": 1,
    "classification": "Normal",
    "validation_mode": "lenient",
    "source_schema": "bp-csv/reading",
    "time_semantics": {
      "event_time": {"pointer": "/measured_at", "layout": "2006-01-02T15:04:05Z07:00"}
    },
    "identity_key": ["/measured_at"]
  },
  "schema": {"required": ["/systolic"]}
}`

const sleepManifest = `{
  "meta": {
    "facet_type": "health.sleep",
    "definition_version": 1,
    "classification": "Normal",
    "validation_mode": "lenient",
    "source_schema": "health-db/sleep",
    "time_semantics": {
      "event_time": {"pointer": "/measured_at", "layout": "2006-01-02T15:04:05Z07:00"}
    },
    "identity_key": ["/measured_at"]
  },
  "schema": {"required": ["/deep_minutes"]}
}`

type harness struct {
	gate  *gate.Gate
	l     *ledger.Ledger
	deps  serve.Deps
	dir   string
	store *artifact.Store
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	if !reachable {
		t.Skipf("DynamoDB Local unreachable at %s", endpoint)
	}
	ctx := context.Background()

	l, err := ledger.New(ctx, endpoint, region, "anam_test_"+uuid.NewString(), person)
	if err != nil {
		t.Fatalf("ledger.New: %v", err)
	}
	if err := l.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}
	t.Cleanup(func() { _ = l.Drop(context.Background()) })

	bp, err := def.Parse([]byte(bpManifest))
	if err != nil {
		t.Fatalf("def.Parse(bp): %v", err)
	}
	sleep, err := def.Parse([]byte(sleepManifest))
	if err != nil {
		t.Fatalf("def.Parse(sleep): %v", err)
	}
	defs, err := def.NewRegistry(bp, sleep)
	if err != nil {
		t.Fatalf("def.NewRegistry: %v", err)
	}
	scopes := gate.StaticScopes{
		"bp-csv":    {"health.blood-pressure"},
		"health-db": {"health.sleep"},
	}

	templates, err := projection.Bundled()
	if err != nil {
		t.Fatalf("projection.Bundled: %v", err)
	}
	dir := t.TempDir()
	store, err := artifact.New(dir)
	if err != nil {
		t.Fatalf("artifact.New: %v", err)
	}

	return &harness{
		gate:  gate.New(l, defs, scopes, clock.Real{}, person),
		l:     l,
		dir:   dir,
		store: store,
		deps: serve.Deps{
			Ledger:               l,
			Templates:            templates,
			Artifacts:            store,
			Gate:                 consent.Open{},
			Clock:                clock.Fixed{Instant: time.Date(2026, 7, 25, 9, 0, 0, 0, time.UTC)},
			DefinitionSetVersion: "sha256:" + strings.Repeat("d", 64),
		},
	}
}

// admit puts one row through the real gate, exactly as a capture worker would.
func (h *harness) admit(t *testing.T, source string, typ facet.Type, measuredAt, body string) {
	t.Helper()
	env := facet.Envelope{
		Type: typ, Source: source, Injector: source,
		SourceRef: source + ":" + measuredAt,
		Payload:   []byte(fmt.Sprintf(`{"measured_at": %q, %s}`, measuredAt, body)),
	}
	binding, err := h.gate.Resolve(env)
	if err != nil {
		t.Fatalf("Resolve %s: %v", measuredAt, err)
	}
	if _, err := h.gate.Commit(context.Background(), env, binding); err != nil {
		t.Fatalf("Commit %s: %v", measuredAt, err)
	}
}

// seed admits three blood-pressure readings and two sleep periods, so `bp` has
// something to serve and something to leave behind.
func (h *harness) seed(t *testing.T) {
	t.Helper()
	h.admit(t, "bp-csv", "health.blood-pressure", "2026-01-24T06:30:00Z", `"systolic": 118, "diastolic": 76`)
	h.admit(t, "bp-csv", "health.blood-pressure", "2026-01-25T06:30:00Z", `"systolic": 121, "diastolic": 79`)
	h.admit(t, "bp-csv", "health.blood-pressure", "2026-01-26T06:30:00Z", `"systolic": 117, "diastolic": 75`)
	h.admit(t, "health-db", "health.sleep", "2026-01-24T22:00:00Z", `"deep_minutes": 62`)
	h.admit(t, "health-db", "health.sleep", "2026-01-25T22:00:00Z", `"deep_minutes": 71`)
}

func TestRun_ServesAndPublishes(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.seed(t)
	ctx := context.Background()

	out, err := serve.Run(ctx, h.deps, "bp", false)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !out.Written {
		t.Error("a real serve reported nothing written")
	}
	if out.CanonicalLen != 5 {
		t.Errorf("canonical held %d facets, want 5", out.CanonicalLen)
	}
	if got := len(out.Receipt.FacetIDs); got != 3 {
		t.Errorf("bp served %d facets, want the 3 blood-pressure readings", got)
	}

	// The receipt is complete and says what it served.
	r := out.Receipt
	if err := r.Validate(); err != nil {
		t.Errorf("published receipt is incomplete: %v", err)
	}
	if r.Template != "bp" || r.TemplateVersion != 1 {
		t.Errorf("receipt names %s v%d", r.Template, r.TemplateVersion)
	}
	if r.Span != "whole-person" {
		t.Errorf("receipt span %q", r.Span)
	}
	if r.ServedAt != "2026-07-25T09:00:00.000000000Z" {
		t.Errorf("receipt served_at %q", r.ServedAt)
	}
	if r.DefinitionSetVersion != h.deps.DefinitionSetVersion {
		t.Errorf("receipt definition set %q", r.DefinitionSetVersion)
	}

	// served_hash addresses the bytes actually on disk.
	stored, err := h.store.Get(receipt.ServedKind, r.ServedHash)
	if err != nil {
		t.Fatalf("the served value is not in the store at its own hash: %v", err)
	}
	if artifact.Digest(stored) != r.ServedHash {
		t.Errorf("served_hash %s does not address the stored bytes", r.ServedHash)
	}

	// The receipt is itself content-addressed and retrievable.
	if _, err := h.store.Get(receipt.Kind, out.ReceiptHash); err != nil {
		t.Errorf("the receipt is not in the store at its own hash: %v", err)
	}

	// facet_ids is the served entries' ids, in served order — which is the canonical's
	// (event-time, id) order, and therefore reproducible.
	if !reflect.DeepEqual(r.FacetIDs, servedIDs(t, stored)) {
		t.Errorf("facet_ids %v is not the served value's own order %v", r.FacetIDs, servedIDs(t, stored))
	}
}

// Serving is a read. Being shown is not something Anam learned about a fact, so no
// record-event is appended and the watermark does not move (§9).
func TestRun_LeavesTheLedgerUntouched(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.seed(t)
	ctx := context.Background()

	before, err := h.l.Head(ctx)
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	eventsBefore, err := h.l.Replay(ctx, 0, before)
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}

	for _, name := range []string{"bp", "sleep", "bp"} {
		if _, err := serve.Run(ctx, h.deps, name, false); err != nil {
			t.Fatalf("Run(%s): %v", name, err)
		}
	}

	after, err := h.l.Head(ctx)
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	if after != before {
		t.Errorf("three serves moved W from %d to %d", before, after)
	}
	eventsAfter, err := h.l.Replay(ctx, 0, after)
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	if len(eventsAfter) != len(eventsBefore) {
		t.Errorf("serving appended %d record-events", len(eventsAfter)-len(eventsBefore))
	}
}

// Nothing changed, so nothing about the serve may change.
func TestRun_StableAcrossRepeatedServes(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.seed(t)
	ctx := context.Background()

	first, err := serve.Run(ctx, h.deps, "bp", false)
	if err != nil {
		t.Fatalf("first Run: %v", err)
	}
	second, err := serve.Run(ctx, h.deps, "bp", false)
	if err != nil {
		t.Fatalf("second Run: %v", err)
	}
	if first.Receipt.ServedHash != second.Receipt.ServedHash {
		t.Errorf("served hash moved with no ingest: %s then %s", first.Receipt.ServedHash, second.Receipt.ServedHash)
	}
	if first.ReceiptHash != second.ReceiptHash {
		t.Errorf("receipt hash moved with no ingest (the clock is frozen): %s then %s", first.ReceiptHash, second.ReceiptHash)
	}

	// And the tier holds one served file and one receipt, not two of each.
	for _, kind := range []string{receipt.ServedKind, receipt.Kind} {
		got, err := h.store.List(kind)
		if err != nil {
			t.Fatalf("List(%s): %v", kind, err)
		}
		if len(got) != 1 {
			t.Errorf("%s holds %d artifacts after two identical serves, want 1", kind, len(got))
		}
	}
}

// A dry run takes the same path and publishes nothing, so it cannot flatter a real one.
func TestRun_DryRunPublishesNothing(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.seed(t)
	ctx := context.Background()

	dry, err := serve.Run(ctx, h.deps, "bp", true)
	if err != nil {
		t.Fatalf("Run(dry): %v", err)
	}
	if dry.Written {
		t.Error("a dry run reported bytes written")
	}
	for _, kind := range []string{receipt.ServedKind, receipt.Kind} {
		got, err := h.store.List(kind)
		if err != nil {
			t.Fatalf("List(%s): %v", kind, err)
		}
		if len(got) != 0 {
			t.Errorf("a dry run left %d %s artifacts", len(got), kind)
		}
	}

	wet, err := serve.Run(ctx, h.deps, "bp", false)
	if err != nil {
		t.Fatalf("Run(wet): %v", err)
	}
	if dry.Receipt.ServedHash != wet.Receipt.ServedHash {
		t.Errorf("the dry run derived %s, the real one %s", dry.Receipt.ServedHash, wet.Receipt.ServedHash)
	}
}

func TestRun_UnknownProjection(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ctx := context.Background()

	if _, err := serve.Run(ctx, h.deps, "bloodpressure", false); !errors.Is(err, projection.ErrUnknown) {
		t.Errorf("Run(unknown): err = %v, want ErrUnknown", err)
	}
}

// --- the claim R7 exists to make ------------------------------------------------------

// A receipt must reproduce over a ledger that has moved on. This is the whole point:
// verifying at head proves nothing, because at head there is nothing to get wrong.
func TestVerify_ReproducesAfterTheLedgerAdvances(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.seed(t)
	ctx := context.Background()

	out, err := serve.Run(ctx, h.deps, "bp", false)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	servedW := out.Receipt.Watermark

	// The world moves: a correction to a served reading, and two new facts.
	h.admit(t, "bp-csv", "health.blood-pressure", "2026-01-24T06:30:00Z", `"systolic": 116, "diastolic": 74`)
	h.admit(t, "bp-csv", "health.blood-pressure", "2026-02-01T06:30:00Z", `"systolic": 120, "diastolic": 78`)
	h.admit(t, "health-db", "health.sleep", "2026-02-01T22:00:00Z", `"deep_minutes": 55`)

	head, err := h.l.Head(ctx)
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	if int64(head) <= servedW {
		t.Fatalf("the ledger did not advance past the serve: W was %d, now %d", servedW, head)
	}

	r, res, err := serve.Verify(ctx, h.deps, out.ReceiptHash)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !res.OK() {
		t.Fatalf("a sound receipt did not verify after later ingest:\n%s", res.Summary())
	}
	if !res.Reproduced {
		t.Errorf("Reproduced is false; re-derived %s, receipt claims %s", res.DerivedHash, r.ServedHash)
	}
	if !res.Intact {
		t.Errorf("Intact is false; on disk %s, receipt claims %s", res.StoredHash, r.ServedHash)
	}
	if r.Watermark != servedW {
		t.Errorf("the receipt's watermark moved: %d, want %d", r.Watermark, servedW)
	}

	// Serving now must differ — otherwise the reproduction above proved nothing.
	fresh, err := serve.Run(ctx, h.deps, "bp", false)
	if err != nil {
		t.Fatalf("Run (after ingest): %v", err)
	}
	if fresh.Receipt.ServedHash == out.Receipt.ServedHash {
		t.Error("the served hash did not move across a correction and a new reading; the reproduction test is vacuous")
	}
	if len(fresh.Receipt.FacetIDs) != 4 {
		t.Errorf("a fresh serve found %d bp facets, want 4", len(fresh.Receipt.FacetIDs))
	}

	// And the older receipt still verifies, now that a second serve exists alongside it.
	if _, res, err := serve.Verify(ctx, h.deps, out.ReceiptHash); err != nil || !res.OK() {
		t.Errorf("the original receipt stopped verifying after a second serve: %v\n%s", err, res.Summary())
	}
}

// One character, and verification must say so — and must say which of the two proofs
// broke, because a tampered file and a moved ledger are different accidents.
func TestVerify_DetectsATamperedArtifact(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.seed(t)
	ctx := context.Background()

	out, err := serve.Run(ctx, h.deps, "bp", false)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	path := filepath.Join(h.dir, receipt.ServedKind,
		strings.Replace(out.Receipt.ServedHash, "sha256:", "sha256-", 1)+".json")
	original, err := os.ReadFile(path) //nolint:gosec // test-owned temp path
	if err != nil {
		t.Fatalf("read served file: %v", err)
	}
	tampered := strings.Replace(string(original), `"systolic":118`, `"systolic":119`, 1)
	if tampered == string(original) {
		t.Fatalf("the tamper changed nothing; the served bytes are:\n%s", original)
	}
	if err := os.WriteFile(path, []byte(tampered), 0o600); err != nil {
		t.Fatalf("tamper: %v", err)
	}

	_, res, err := serve.Verify(ctx, h.deps, out.ReceiptHash)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if res.OK() {
		t.Fatal("a tampered artifact verified")
	}
	if !res.Reproduced {
		t.Error("the ledger is untouched; Reproduced should still hold — the two proofs must fail independently")
	}
	if res.Intact {
		t.Error("Intact held over a file that no longer hashes to its own address")
	}
	if res.StoredHash == out.Receipt.ServedHash {
		t.Error("the reported on-disk hash did not move")
	}
	if !strings.Contains(res.Summary(), "FAIL") || !strings.Contains(res.Summary(), "intact") {
		t.Errorf("the summary does not name the failing proof:\n%s", res.Summary())
	}
}

// A receipt whose served artifact was deleted is a different situation from one whose
// artifact was altered: the ledger can still honour it.
func TestVerify_MissingArtifactIsNotTampering(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.seed(t)
	ctx := context.Background()

	out, err := serve.Run(ctx, h.deps, "bp", false)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	path := filepath.Join(h.dir, receipt.ServedKind,
		strings.Replace(out.Receipt.ServedHash, "sha256:", "sha256-", 1)+".json")
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove served file: %v", err)
	}

	_, res, err := serve.Verify(ctx, h.deps, out.ReceiptHash)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !res.Reproduced {
		t.Error("the ledger still holds the facts; Reproduced should hold")
	}
	if res.Intact || res.StoredHash != "" {
		t.Errorf("an absent artifact should not read as intact: Intact=%v StoredHash=%q", res.Intact, res.StoredHash)
	}
	if !strings.Contains(res.Summary(), "not present") {
		t.Errorf("the summary does not distinguish absent from altered:\n%s", res.Summary())
	}
}

// The template is exactly what produced the pruning, so a moved template version
// cannot re-derive the same value. Saying "not reproduced" would blame the ledger for
// a change in the code.
func TestVerify_RefusesAMovedTemplateVersion(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.seed(t)
	ctx := context.Background()

	out, err := serve.Run(ctx, h.deps, "bp", false)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// A registry whose bp is v2 — the same template, a later revision.
	moved, err := projection.Parse([]byte(`{"name":"bp","version":2,"types":["health.blood-pressure"],` +
		`"statuses":["admitted","endorsed"],"max_classification":"Sensitive","from":"","to":""}`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	deps := h.deps
	deps.Templates = registryOf(t, moved)

	if _, _, err := serve.Verify(ctx, deps, out.ReceiptHash); !errors.Is(err, serve.ErrTemplateMoved) {
		t.Errorf("Verify under a moved template: err = %v, want ErrTemplateMoved", err)
	}
}

// Definitions govern capture, not the fold, so a moved definition set cannot change
// bytes already in the ledger. It is reported, and reproduction still holds.
func TestVerify_MovedDefinitionSetIsANoteNotAFailure(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.seed(t)
	ctx := context.Background()

	out, err := serve.Run(ctx, h.deps, "bp", false)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	deps := h.deps
	deps.DefinitionSetVersion = "sha256:" + strings.Repeat("e", 64)

	_, res, err := serve.Verify(ctx, deps, out.ReceiptHash)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !res.OK() {
		t.Errorf("a moved definition set failed verification:\n%s", res.Summary())
	}
	if len(res.Notes) != 1 || !strings.Contains(res.Notes[0], "definition set has moved") {
		t.Errorf("the move was not reported: %v", res.Notes)
	}
}

func TestVerify_UnknownReceipt(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ctx := context.Background()

	absent := artifact.Digest([]byte("never served"))
	if _, _, err := serve.Verify(ctx, h.deps, absent); !errors.Is(err, artifact.ErrNotFound) {
		t.Errorf("Verify(absent): err = %v, want ErrNotFound", err)
	}
}

func TestList_NewestFirst(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.seed(t)
	ctx := context.Background()

	// Two serves at two instants, so the ordering has something to order.
	early := h.deps
	early.Clock = clock.Fixed{Instant: time.Date(2026, 7, 20, 9, 0, 0, 0, time.UTC)}
	if _, err := serve.Run(ctx, early, "bp", false); err != nil {
		t.Fatalf("Run(early): %v", err)
	}
	late := h.deps
	late.Clock = clock.Fixed{Instant: time.Date(2026, 7, 25, 9, 0, 0, 0, time.UTC)}
	if _, err := serve.Run(ctx, late, "sleep", false); err != nil {
		t.Fatalf("Run(late): %v", err)
	}

	receipts, hashes, err := serve.List(h.deps)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(receipts) != 2 || len(hashes) != 2 {
		t.Fatalf("List returned %d receipts and %d hashes, want 2 and 2", len(receipts), len(hashes))
	}
	if receipts[0].Template != "sleep" || receipts[1].Template != "bp" {
		t.Errorf("List order: %s then %s, want sleep then bp (newest first)",
			receipts[0].Template, receipts[1].Template)
	}
	// The hashes must stay paired with their receipts through the sort.
	for i, r := range receipts {
		b, err := r.Bytes()
		if err != nil {
			t.Fatalf("Bytes: %v", err)
		}
		if got := artifact.Digest(b); got != hashes[i] {
			t.Errorf("receipt %d is paired with hash %s but hashes to %s", i, hashes[i], got)
		}
	}
}

func TestList_EmptyStore(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	receipts, hashes, err := serve.List(h.deps)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(receipts) != 0 || len(hashes) != 0 {
		t.Errorf("an empty store listed %d receipts", len(receipts))
	}
}

// registryOf builds a registry from templates, via the bundled one's public surface.
func registryOf(t *testing.T, templates ...projection.Template) *projection.Registry {
	t.Helper()
	r, err := projection.NewRegistry(templates...)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	return r
}

// servedIDs reads the facet ids out of a served value's bytes, so the receipt's list
// is checked against the file rather than against the same in-memory value that
// produced it.
func servedIDs(t *testing.T, servedBytes []byte) []string {
	t.Helper()
	var entries []struct {
		FacetID string `json:"facet_id"`
	}
	if err := json.Unmarshal(servedBytes, &entries); err != nil {
		t.Fatalf("unmarshal served value: %v", err)
	}
	ids := make([]string, 0, len(entries))
	for _, e := range entries {
		ids = append(ids, e.FacetID)
	}
	return ids
}
