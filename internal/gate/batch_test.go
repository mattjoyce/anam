package gate_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/fold"
	"github.com/mattjoyce/anam/internal/gate"
)

// bpEnvelopeAt builds a distinct reading — identity_key is /measured_at, so the
// minute is the facet.
func bpEnvelopeAt(minute, systolic int) facet.Envelope {
	at := fmt.Sprintf("2026-01-24T06:%02d:00Z", minute)
	return facet.Envelope{
		Type: "health.blood-pressure", Source: "bp-csv", Injector: "bp-csv",
		SourceRef: "bp-csv:" + at,
		Payload: []byte(fmt.Sprintf(
			`{"measured_at": %q, "systolic": %d, "diastolic": 76}`, at, systolic)),
	}
}

func resolveAll(t *testing.T, g *gate.Gate, envs []facet.Envelope) []facet.Binding {
	t.Helper()
	bs := make([]facet.Binding, len(envs))
	for i, env := range envs {
		b, err := g.Resolve(env)
		if err != nil {
			t.Fatalf("Resolve %s: %v", env.SourceRef, err)
		}
		bs[i] = b
	}
	return bs
}

func envelopes(n int) []facet.Envelope {
	envs := make([]facet.Envelope, n)
	for i := range envs {
		envs[i] = bpEnvelopeAt(i, 110+i)
	}
	return envs
}

// The claim batching has to earn: it is a throughput change, not a semantic one. The
// same envelopes twelve times as a block of one, and once as a block of twelve, must
// produce the same canonical — same entries, same order, same hash.
//
// Commit is now CommitBatch of one, so this no longer guards two implementations
// against each other; it guards one implementation against its own block size, which
// is the property that was actually at stake. The hash is the oracle because it covers
// everything a consumer can see. What it deliberately excludes is the bookkeeping
// block size *does* change: seq assignment and anam-time. Those differ, and must.
func TestCommitBatch_ProducesTheSameCanonicalAsCommit(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	envs := envelopes(12)

	gSingle, lSingle := newGate(t)
	for _, env := range envs {
		b, err := gSingle.Resolve(env)
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		if _, _, err := gSingle.Commit(ctx, env, b); err != nil {
			t.Fatalf("Commit %s: %v", env.SourceRef, err)
		}
	}

	gBatch, lBatch := newGate(t)
	if _, _, err := gBatch.CommitBatch(ctx, envs, resolveAll(t, gBatch, envs)); err != nil {
		t.Fatalf("CommitBatch: %v", err)
	}

	single, err := fold.Read(ctx, lSingle)
	if err != nil {
		t.Fatalf("fold single: %v", err)
	}
	batched, err := fold.Read(ctx, lBatch)
	if err != nil {
		t.Fatalf("fold batched: %v", err)
	}

	if single.W != batched.W {
		t.Errorf("watermark: single %d, batched %d — the seq block must be the same length",
			single.W, batched.W)
	}
	if single.Value.Hash() != batched.Value.Hash() {
		t.Errorf("canonical differs:\n  single  %s\n  batched %s",
			single.Value.Hash(), batched.Value.Hash())
	}
	if single.Value.Len() != len(envs) {
		t.Errorf("single admitted %d of %d envelopes", single.Value.Len(), len(envs))
	}
}

// Re-running an import must write nothing and report every facet as a no-op — the
// batch path's half of the idempotency promise.
func TestCommitBatch_IsIdempotent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	g, l := newGate(t)
	envs := envelopes(8)

	if _, _, err := g.CommitBatch(ctx, envs, resolveAll(t, g, envs)); err != nil {
		t.Fatalf("first CommitBatch: %v", err)
	}
	head, err := l.Head(ctx)
	if err != nil {
		t.Fatalf("Head: %v", err)
	}

	results, _, err := g.CommitBatch(ctx, envs, resolveAll(t, g, envs))
	if err != nil {
		t.Fatalf("second CommitBatch: %v", err)
	}
	for i, res := range results {
		if res.Outcome != gate.OutcomeNoOp {
			t.Errorf("envelope %d re-ingested as %s, want no-op", i, res.Outcome)
		}
	}
	if head2, err := l.Head(ctx); err != nil || head2 != head {
		t.Errorf("watermark moved %d → %d on a re-ingest", head, head2)
	}
}

// A changed payload under a known identity supersedes, in a batch exactly as
// singly — and the unchanged siblings around it stay no-ops rather than being
// dragged into a new version.
func TestCommitBatch_SupersedesAlongsideNoOps(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	g, _ := newGate(t)
	envs := envelopes(4)

	if _, _, err := g.CommitBatch(ctx, envs, resolveAll(t, g, envs)); err != nil {
		t.Fatalf("first CommitBatch: %v", err)
	}

	corrected := envelopes(4)
	corrected[2] = bpEnvelopeAt(2, 999)

	results, _, err := g.CommitBatch(ctx, corrected, resolveAll(t, g, corrected))
	if err != nil {
		t.Fatalf("second CommitBatch: %v", err)
	}
	for i, res := range results {
		want := gate.OutcomeNoOp
		if i == 2 {
			want = gate.OutcomeSuperseded
		}
		if res.Outcome != want {
			t.Errorf("envelope %d = %s, want %s", i, res.Outcome, want)
		}
	}
	if results[2].ArtifactVersion != 2 {
		t.Errorf("corrected facet is at v%d, want v2", results[2].ArtifactVersion)
	}
}

// The watermark a call reports must be where the ledger actually ended.
//
// This is the shape that caught the bug W used to have: change the *first* envelope
// and leave the rest, so the last result is a no-op. When each result carried its own
// W, the no-ops held the head read before the transaction while their written siblings
// held the one after — and an ingest tally, which reads the last result, printed a
// watermark the write had already moved past. There is one W now, so the disagreement
// has nowhere to live; this pins the value that replaced it.
func TestCommitBatch_ReportsTheWatermarkTheLedgerEndedAt(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	g, l := newGate(t)
	envs := envelopes(3)

	if _, _, err := g.CommitBatch(ctx, envs, resolveAll(t, g, envs)); err != nil {
		t.Fatalf("first CommitBatch: %v", err)
	}

	corrected := envelopes(3)
	corrected[0] = bpEnvelopeAt(0, 999)

	results, w, err := g.CommitBatch(ctx, corrected, resolveAll(t, g, corrected))
	if err != nil {
		t.Fatalf("second CommitBatch: %v", err)
	}
	if results[len(results)-1].Outcome != gate.OutcomeNoOp {
		t.Fatalf("the last result is %s; this test needs it to be a no-op", results[len(results)-1].Outcome)
	}

	head, err := l.Head(ctx)
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	if head != 4 {
		t.Fatalf("head = %d, want 4 (three admissions, one supersession)", head)
	}
	if w != head {
		t.Errorf("CommitBatch reported W=%d, the ledger is at %d", w, head)
	}
}

// A batch that writes nothing reports the watermark it found, not one it moved:
// the all-no-op re-ingest, where the caller's W must stay put.
func TestCommitBatch_AllNoOpsReportTheUnmovedWatermark(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	g, l := newGate(t)
	envs := envelopes(3)

	if _, _, err := g.CommitBatch(ctx, envs, resolveAll(t, g, envs)); err != nil {
		t.Fatalf("first CommitBatch: %v", err)
	}
	head, err := l.Head(ctx)
	if err != nil {
		t.Fatalf("Head: %v", err)
	}

	results, w, err := g.CommitBatch(ctx, envs, resolveAll(t, g, envs))
	if err != nil {
		t.Fatalf("second CommitBatch: %v", err)
	}
	for i, res := range results {
		if res.Outcome != gate.OutcomeNoOp {
			t.Errorf("result %d = %s, want no-op", i, res.Outcome)
		}
	}
	if w != head {
		t.Errorf("a batch that wrote nothing reported W=%d, want %d", w, head)
	}
}

// One transaction cannot write a key twice, and two envelopes for one facet in a
// single batch is nearly always a duplicated source row. Refuse it by name, before
// the ledger is asked to write anything.
func TestCommitBatch_RefusesTwoEnvelopesForOneFacet(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	g, _ := newGate(t)

	envs := []facet.Envelope{bpEnvelopeAt(1, 118), bpEnvelopeAt(2, 120), bpEnvelopeAt(1, 130)}
	_, _, err := g.CommitBatch(ctx, envs, resolveAll(t, g, envs))
	if err == nil {
		t.Fatal("a batch holding one facet twice was accepted")
	}
	if !strings.Contains(err.Error(), "same facet") {
		t.Errorf("error does not name the problem: %v", err)
	}
}

// A batch that fails validation must write nothing at all. Partial application
// would make "the import ran" mean less than it does on the single path.
func TestCommitBatch_WritesNothingWhenAnEnvelopeIsBad(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	g, l := newGate(t)

	envs := envelopes(3)
	bad := envs[1]
	bad.SourceRef = "bp-csv:not-what-identity-composes"
	envs[1] = bad

	if _, _, err := g.CommitBatch(ctx, envs, resolveAll(t, g, envs)); err == nil {
		t.Fatal("a batch with a mis-keyed envelope was accepted")
	}
	if head, err := l.Head(ctx); err != nil || head != 0 {
		t.Errorf("watermark = %d (err %v) after a rejected batch, want 0", head, err)
	}
}

// The size bound is the gate's policy, and exceeding it must be a clear refusal
// before anything is written.
func TestCommitBatch_RefusesOversizedBatches(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	g, _ := newGate(t)

	envs := envelopes(gate.MaxBatch + 1)
	if _, _, err := g.CommitBatch(ctx, envs, resolveAll(t, g, envs)); err == nil {
		t.Errorf("a batch of %d was accepted (MaxBatch is %d)", len(envs), gate.MaxBatch)
	}
}
