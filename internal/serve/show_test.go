package serve_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/receipt"
	"github.com/mattjoyce/anam/internal/serve"
	"github.com/mattjoyce/anam/internal/store/artifact"
)

// The reads that go to the artifact tier rather than the ledger: Show renders what was
// published, and List enumerates it. Both must work with a ledger that is absent, which
// is why their Deps carry only a store.

// --- Show: the artifact, without the ledger --------------------------------------

// Show answers from the artifact tier alone. That restraint is the point: it is the
// question you must be able to answer when the ledger is gone, unreachable, or exactly
// the thing you doubt.
func TestShow_ReadsThePublishedArtifactOnly(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.seed(t)
	ctx := context.Background()

	out, err := serve.Run(ctx, h.deps, "bp", false)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Deps carrying nothing but the store — no ledger, no templates, no gate. If Show
	// reached for any of them this would panic rather than quietly pass.
	r, shown, err := serve.Show(serve.Deps{Artifacts: h.store}, out.ReceiptHash)
	entries := shown.Entries
	if err != nil {
		t.Fatalf("Show: %v", err)
	}
	if r.ServedHash != out.Receipt.ServedHash {
		t.Errorf("receipt served_hash = %s, want %s", r.ServedHash, out.Receipt.ServedHash)
	}
	if len(entries) != len(out.Receipt.FacetIDs) {
		t.Fatalf("Show returned %d entries, the receipt claims %d", len(entries), len(out.Receipt.FacetIDs))
	}
	for i, e := range entries {
		if e.FacetID != out.Receipt.FacetIDs[i] {
			t.Errorf("entry %d is %s, the receipt claims %s", i, e.FacetID, out.Receipt.FacetIDs[i])
		}
		if len(e.Payload) == 0 {
			t.Errorf("entry %s came back with no payload", e.FacetID)
		}
	}
}

// A tampered artifact must be refused rather than displayed as though it were what was
// served — showing altered bytes under a genuine receipt is the one failure a viewer
// of published artifacts must not have.
func TestShow_RefusesATamperedArtifact(t *testing.T) {
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
		t.Fatalf("read: %v", err)
	}
	tampered := strings.Replace(string(original), `"systolic":118`, `"systolic":119`, 1)
	if tampered == string(original) {
		t.Fatalf("the tamper changed nothing")
	}
	if err := os.WriteFile(path, []byte(tampered), 0o600); err != nil {
		t.Fatalf("tamper: %v", err)
	}

	if _, _, err := serve.Show(serve.Deps{Artifacts: h.store}, out.ReceiptHash); !errors.Is(err, artifact.ErrCorrupt) {
		t.Errorf("Show over a tampered artifact: err = %v, want ErrCorrupt", err)
	}
}

func TestShow_UnknownReceipt(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	absent := artifact.Digest([]byte("never served"))
	if _, _, err := serve.Show(serve.Deps{Artifacts: h.store}, absent); !errors.Is(err, artifact.ErrNotFound) {
		t.Errorf("Show(absent): err = %v, want ErrNotFound", err)
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
