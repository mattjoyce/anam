package serve_test

import (
	"context"
	"testing"

	"github.com/mattjoyce/anam/internal/mint"
	"github.com/mattjoyce/anam/internal/serve"
)

// mintRun cuts checkpoints at the head and returns the run's watermark.
func (h *harness) mintRun(t *testing.T) int64 {
	t.Helper()
	res, err := mint.Run(context.Background(), h.mint, false)
	if err != nil {
		t.Fatalf("mint.Run: %v", err)
	}
	return int64(res.W)
}

// composed is h.deps with the checkpoint log to hand.
func (h *harness) composed() serve.Deps {
	d := h.deps
	d.Checkpoints = h.db
	return d
}

// A serve composed from the last mint run and its tail publishes the same value, with
// the same receipt, as one folded from the ledger's first event; and a receipt served
// either way verifies either way. Both template shapes are held to it: bp is a log,
// meds a snapshot over the composed whole value (card 64 is about reading a prefix of
// it, not this).
func TestRun_ComposedFromTheLastRunIsTheFullFold(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, template string
		seed           func(h *harness)
		tail           func(h *harness)
	}{
		{
			name: "log shape", template: "bp",
			seed: func(h *harness) { h.seed(t) },
			tail: func(h *harness) {
				h.admit(t, "bp-csv", "health.blood-pressure", "2026-01-24T06:30:00Z", `"systolic": 116, "diastolic": 74`) // corrected
				h.admit(t, "bp-csv", "health.blood-pressure", "2026-02-01T06:30:00Z", `"systolic": 120, "diastolic": 78`) // a new month
				h.admit(t, "health-db", "health.sleep", "2026-02-01T22:00:00Z", `"deep_minutes": 55`)
			},
		},
		{
			name: "snapshot shape", template: "meds",
			seed: func(h *harness) {
				h.med(t, "health.medication.started", "m1", "2026-03-01", "ramipril", 1)
				h.med(t, "health.medication.dose-changed", "m2", "2026-05-10", "ramipril", 1)
				h.med(t, "health.medication.started", "m5", "2026-04-01", "atorvastatin", 1)
			},
			tail: func(h *harness) {
				h.med(t, "health.medication.stopped", "m3", "2026-08-01", "ramipril", 1) // closes a course started before the run
				h.med(t, "health.medication.started", "m4", "2026-10-01", "ramipril", 2)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			var h *harness
			if tc.template == "meds" {
				h = medHarness(t)
			} else {
				h = newHarness(t)
			}
			tc.seed(h)

			// Before any mint run there is nothing to compose from: the serve folds in
			// full and says so.
			before, err := serve.Run(ctx, h.composed(), tc.template, false)
			if err != nil {
				t.Fatalf("Run before any mint run: %v", err)
			}
			if before.Run != 0 {
				t.Errorf("a serve before any mint run reports run %d", before.Run)
			}

			run := h.mintRun(t)
			tc.tail(h)

			full, err := serve.Run(ctx, h.deps, tc.template, true)
			if err != nil {
				t.Fatalf("Run (full fold): %v", err)
			}
			composed, err := serve.Run(ctx, h.composed(), tc.template, false)
			if err != nil {
				t.Fatalf("Run (composed): %v", err)
			}
			if int64(composed.Run) != run || composed.Tail == 0 {
				t.Errorf("composed serve started from run %d with tail %d, want run %d and a tail", composed.Run, composed.Tail, run)
			}
			if full.Run != 0 {
				t.Errorf("a serve without the checkpoint log reports run %d", full.Run)
			}
			if composed.Receipt.ServedHash != full.Receipt.ServedHash {
				t.Errorf("composed served %s, the full fold %s", composed.Receipt.ServedHash, full.Receipt.ServedHash)
			}
			if composed.CanonicalHash != full.CanonicalHash || composed.CanonicalLen != full.CanonicalLen {
				t.Errorf("composed canonical %s (%d), the full fold %s (%d)",
					composed.CanonicalHash, composed.CanonicalLen, full.CanonicalHash, full.CanonicalLen)
			}
			if composed.ReceiptHash != full.ReceiptHash {
				t.Errorf("the receipts differ: composed %s, full %s", composed.ReceiptHash, full.ReceiptHash)
			}

			// Every receipt verifies both ways: the one served before the run (its
			// watermark is below the run, so the composed verify folds in full), and the
			// one served after it.
			for _, hash := range []string{before.ReceiptHash, composed.ReceiptHash} {
				for name, d := range map[string]serve.Deps{"full fold": h.deps, "composed": h.composed()} {
					_, res, err := serve.Verify(ctx, d, hash)
					if err != nil {
						t.Fatalf("Verify %s via %s: %v", hash, name, err)
					}
					if !res.OK() {
						t.Errorf("receipt %s did not verify via %s:\n%s", hash, name, res.Summary())
					}
				}
			}
		})
	}
}

// A composed verify is still exact after the ledger moves past the receipt: the
// watermark names the run to start from and the tail to apply, not the head.
func TestVerify_ComposedAfterTheLedgerAdvances(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.seed(t)
	ctx := context.Background()
	h.mintRun(t)
	h.admit(t, "bp-csv", "health.blood-pressure", "2026-02-01T06:30:00Z", `"systolic": 120, "diastolic": 78`)

	out, err := serve.Run(ctx, h.composed(), "bp", false)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// The world moves on both sides of the checkpoint: a correction to a served
	// reading from before the run, a new reading, and a second mint run after them.
	h.admit(t, "bp-csv", "health.blood-pressure", "2026-01-24T06:30:00Z", `"systolic": 116, "diastolic": 74`)
	h.admit(t, "bp-csv", "health.blood-pressure", "2026-02-02T06:30:00Z", `"systolic": 119, "diastolic": 77`)
	h.mintRun(t)

	_, res, err := serve.Verify(ctx, h.composed(), out.ReceiptHash)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !res.OK() {
		t.Fatalf("a sound receipt did not verify from checkpoints after later ingest and a later mint:\n%s", res.Summary())
	}
	fresh, err := serve.Run(ctx, h.composed(), "bp", true)
	if err != nil {
		t.Fatalf("Run (after ingest): %v", err)
	}
	if fresh.Receipt.ServedHash == out.Receipt.ServedHash {
		t.Error("the served hash did not move across a correction and a new reading; the reproduction above is vacuous")
	}
}
