package serve_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/serve"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

// A receipt names the ledger its watermark counts in. Verified against another
// ledger, it says so, and does not report a failed reproduction (card 18).
func TestVerify_AgainstAnotherLedgerSaysSo(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.seed(t)
	ctx := context.Background()

	out, err := serve.Run(ctx, h.deps, "bp", false)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	want, err := h.l.ID(ctx)
	if err != nil {
		t.Fatalf("ID: %v", err)
	}
	if out.Receipt.LedgerID != want {
		t.Errorf("receipt names ledger %q, want %q", out.Receipt.LedgerID, want)
	}

	// A different ledger, same person, same artifacts on disk: a rebuild, in effect.
	other, err := ledger.Create(ctx, filepath.Join(t.TempDir(), "ledger.db"), person)
	if err != nil {
		t.Fatalf("ledger.Create: %v", err)
	}
	defer func() { _ = other.Close() }()
	deps := h.deps
	deps.Ledger = other

	_, _, err = serve.Verify(ctx, deps, out.ReceiptHash)
	if !errors.Is(err, serve.ErrOtherLedger) {
		t.Fatalf("Verify against another ledger = %v, want ErrOtherLedger", err)
	}
	if !strings.Contains(err.Error(), want) {
		t.Errorf("the refusal does not name the receipt's ledger: %v", err)
	}
}

// A receipt from before ledger ids were recorded still verifies, with a note.
func TestVerify_UnnamedLedgerIsANote(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.seed(t)
	ctx := context.Background()

	out, err := serve.Run(ctx, h.deps, "bp", false)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	r := out.Receipt
	r.LedgerID = ""
	raw, err := r.Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}
	if strings.Contains(string(raw), "ledger_id") {
		t.Fatal("an unnamed ledger still serialises a ledger_id")
	}
	old, err := h.store.Put("receipt", raw)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	_, res, err := serve.Verify(ctx, h.deps, old)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !res.OK() {
		t.Errorf("an unnamed receipt did not verify:\n%s", res.Summary())
	}
	if !strings.Contains(res.Summary(), "names no ledger") {
		t.Errorf("no note about the unnamed ledger:\n%s", res.Summary())
	}
}
