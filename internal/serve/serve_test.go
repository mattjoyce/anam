package serve_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/projection"
	"github.com/mattjoyce/anam/internal/receipt"
	"github.com/mattjoyce/anam/internal/serve"
	"github.com/mattjoyce/anam/internal/store/ops"
	"github.com/mattjoyce/anam/internal/store/ops/opstest"
)

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
	stored, err := h.db.Get(context.Background(), receipt.ServedKind, r.ServedHash)
	if err != nil {
		t.Fatalf("the served value is not in the store at its own hash: %v", err)
	}
	if ops.Digest(stored) != r.ServedHash {
		t.Errorf("served_hash %s does not address the stored bytes", r.ServedHash)
	}

	// The receipt is itself content-addressed and retrievable.
	if _, err := h.db.Get(context.Background(), receipt.Kind, out.ReceiptHash); err != nil {
		t.Errorf("the receipt is not in the store at its own hash: %v", err)
	}

	// facet_ids is the served entries' ids, in served order — which is the canonical's
	// (fact-time, id) order, and therefore reproducible.
	if !reflect.DeepEqual(r.FacetIDs, servedIDs(t, stored)) {
		t.Errorf("facet_ids %v is not the served value's own order %v", r.FacetIDs, servedIDs(t, stored))
	}
}

// Serving is a read. Being shown is not something Anam learned about a fact, so no
// anam-event is appended and the watermark does not move (§9).
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
		t.Errorf("serving appended %d anam-events", len(eventsAfter)-len(eventsBefore))
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
		got, err := h.db.List(context.Background(), kind)
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
		got, err := h.db.List(context.Background(), kind)
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

	original, err := h.db.Raw(ctx, receipt.ServedKind, out.Receipt.ServedHash)
	if err != nil {
		t.Fatalf("read the served value: %v", err)
	}
	tampered := strings.Replace(string(original), `"systolic":118`, `"systolic":119`, 1)
	if tampered == string(original) {
		t.Fatalf("the tamper changed nothing; the served bytes are:\n%s", original)
	}
	if err := opstest.Corrupt(ctx, h.db, receipt.ServedKind, out.Receipt.ServedHash, []byte(tampered)); err != nil {
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

// A receipt whose served value is not there is a different situation from one whose
// value was altered: the ledger can still honour it.
func TestVerify_MissingServedValueIsNotTampering(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.seed(t)
	ctx := context.Background()

	// A dry run derives the receipt and writes nothing; filing the receipt alone leaves
	// one whose served value is not there.
	out, err := serve.Run(ctx, h.deps, "bp", true)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	rb, err := out.Receipt.Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}
	if _, err := h.db.Put(ctx, receipt.Kind, rb); err != nil {
		t.Fatalf("Put the receipt: %v", err)
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
// A receipt files the body of the template it was served through, so it verifies from
// that body once the version's file has left the catalog, and the verdict says so. A
// receipt with no filed body, as every one served before bodies were filed, cannot be
// re-derived without the file, and the error says so and what to put back. A version
// the catalog holds under a different body is refused, filed body or not: whether a
// version may change its body is open question §6, which this rung leaves as it was.
func TestVerify_ReplaysTheFiledBodyOnceTheCatalogMovesOn(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.seed(t)
	ctx := context.Background()

	out, err := serve.Run(ctx, h.deps, "bp", false)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	parse := func(version int, statuses string) projection.Template {
		t.Helper()
		tmpl, err := projection.Parse([]byte(fmt.Sprintf(`{"name":"bp","version":%d,"types":["health.blood-pressure"],`+
			`"statuses":[%s],"max_classification":"Sensitive","from":"","to":""}`, version, statuses)))
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		return tmpl
	}

	t.Run("the version's file has left", func(t *testing.T) {
		deps := h.deps
		deps.Templates = registryOf(t, parse(2, `"admitted","endorsed"`))
		_, res, err := serve.Verify(ctx, deps, out.ReceiptHash)
		if err != nil || !res.OK() {
			t.Fatalf("Verify from the filed body: %v\n%s", err, res.Summary())
		}
		if !slices.ContainsFunc(res.Notes, func(n string) bool {
			return strings.Contains(n, "from the body filed with the receipt") && strings.Contains(n, "the catalog no longer holds that version")
		}) {
			t.Errorf("the verdict does not say it replayed the filed body: %q", res.Notes)
		}

		deps.Published = noTemplateBodies{deps.Published}
		_, _, err = serve.Verify(ctx, deps, out.ReceiptHash)
		if !errors.Is(err, serve.ErrTemplateMoved) || !strings.Contains(err.Error(), "unable to verify") ||
			!strings.Contains(err.Error(), "no body was filed with the receipt") {
			t.Errorf("Verify with neither the file nor a filed body: err = %v, want ErrTemplateMoved saying why", err)
		}
	})

	t.Run("the version holds another body", func(t *testing.T) {
		deps := h.deps
		deps.Templates = registryOf(t, parse(1, `"admitted"`))
		_, _, err := serve.Verify(ctx, deps, out.ReceiptHash)
		if !errors.Is(err, serve.ErrTemplateMoved) || !strings.Contains(err.Error(), "unable to verify") {
			t.Errorf("Verify against another body at the same version: err = %v, want ErrTemplateMoved, filed body or not", err)
		}
	})
}

// noTemplateBodies is a store that never filed a template body, as one written before
// receipts filed them.
type noTemplateBodies struct{ serve.Store }

func (s noTemplateBodies) Get(ctx context.Context, kind, digest string) ([]byte, error) {
	if kind == receipt.TemplateBodyKind {
		return nil, ops.ErrNotFound
	}
	return s.Store.Get(ctx, kind, digest)
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

	absent := ops.Digest([]byte("never served"))
	if _, _, err := serve.Verify(ctx, h.deps, absent); !errors.Is(err, ops.ErrNotFound) {
		t.Errorf("Verify(absent): err = %v, want ErrNotFound", err)
	}
}
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

// Rewording a bundled Definition's description in place changes the manifest's bytes,
// and so the definition-set version a new receipt records (TestSetVersion_MovesWhenA
// DescriptionIsReworded holds that half in internal/def). A receipt cut before the edit
// still verifies, with a note that the set has moved: a description governs what a
// person reads, not what was admitted or how the fold reads it.
func TestVerify_RewordedDescriptionKeepsAnOlderReceiptVerifying(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.seed(t)
	ctx := context.Background()

	out, err := serve.Run(ctx, h.deps, "bp", false)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	digest := func(manifest string) string {
		sum := sha256.Sum256([]byte(manifest))
		return "sha256:" + hex.EncodeToString(sum[:])
	}
	older := digest(`{"schema":{"description":"made by a card"}}`)
	newer := digest(`{"schema":{"description":"a reading of blood pressure"}}`)
	if older == newer {
		t.Fatal("the reworded manifest hashes the same as the original")
	}

	deps := h.deps
	deps.DefinitionSetVersion = newer
	_, res, err := serve.Verify(ctx, deps, out.ReceiptHash)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !res.OK() {
		t.Errorf("a receipt cut before a description was reworded no longer verifies:\n%s", res.Summary())
	}
	if len(res.Notes) != 1 || !strings.Contains(res.Notes[0], "definition set has moved") {
		t.Errorf("the move was not reported: %v", res.Notes)
	}
}
