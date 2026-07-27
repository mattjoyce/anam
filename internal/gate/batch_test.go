package gate_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/fold"
	"github.com/mattjoyce/anam/internal/gate"
	"github.com/mattjoyce/anam/internal/store/ledger"
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

// The claim batching has to earn: it is a throughput change, not a semantic one.
// The same envelopes through Commit one at a time and through CommitBatch must
// produce the same canonical — same entries, same order, same hash.
//
// The hash is the oracle because it covers everything a consumer can see. What it
// deliberately excludes is the bookkeeping batching *does* change: seq assignment
// and anam-time. Those differ, and must.
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
		if _, err := gSingle.Commit(ctx, env, b); err != nil {
			t.Fatalf("Commit %s: %v", env.SourceRef, err)
		}
	}

	gBatch, lBatch := newGate(t)
	if _, err := gBatch.CommitBatch(ctx, envs, resolveAll(t, gBatch, envs)); err != nil {
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

	if _, err := g.CommitBatch(ctx, envs, resolveAll(t, g, envs)); err != nil {
		t.Fatalf("first CommitBatch: %v", err)
	}
	head, err := l.Head(ctx)
	if err != nil {
		t.Fatalf("Head: %v", err)
	}

	results, err := g.CommitBatch(ctx, envs, resolveAll(t, g, envs))
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

	if _, err := g.CommitBatch(ctx, envs, resolveAll(t, g, envs)); err != nil {
		t.Fatalf("first CommitBatch: %v", err)
	}

	corrected := envelopes(4)
	corrected[2] = bpEnvelopeAt(2, 999)

	results, err := g.CommitBatch(ctx, corrected, resolveAll(t, g, corrected))
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

// One transaction leaves the ledger in one place, so every result of one call must
// report that place — the no-ops included.
//
// The shape that caught this: change the *first* envelope and leave the rest, so the
// last result is a no-op. A no-op is decided before the transaction, and when it took
// its W from the head read there, it reported a watermark the write had already moved
// past. An ingest tally reads the last result, so the run printed a stale W.
func TestCommitBatch_EveryResultReportsTheWatermarkTheCallEndedAt(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	g, l := newGate(t)
	envs := envelopes(3)

	if _, err := g.CommitBatch(ctx, envs, resolveAll(t, g, envs)); err != nil {
		t.Fatalf("first CommitBatch: %v", err)
	}

	corrected := envelopes(3)
	corrected[0] = bpEnvelopeAt(0, 999)

	results, err := g.CommitBatch(ctx, corrected, resolveAll(t, g, corrected))
	if err != nil {
		t.Fatalf("second CommitBatch: %v", err)
	}
	head, err := l.Head(ctx)
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	if head != 4 {
		t.Fatalf("head = %d, want 4 (three admissions, one supersession)", head)
	}
	for i, res := range results {
		if res.W != head {
			t.Errorf("result %d (%s) reports W=%d, the ledger is at %d", i, res.Outcome, res.W, head)
		}
	}
}

// A batch that writes nothing must report the watermark it found, not one it moved:
// the all-no-op re-ingest, where the caller's W must stay put.
func TestCommitBatch_AllNoOpsReportTheUnmovedWatermark(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	g, l := newGate(t)
	envs := envelopes(3)

	if _, err := g.CommitBatch(ctx, envs, resolveAll(t, g, envs)); err != nil {
		t.Fatalf("first CommitBatch: %v", err)
	}
	head, err := l.Head(ctx)
	if err != nil {
		t.Fatalf("Head: %v", err)
	}

	results, err := g.CommitBatch(ctx, envs, resolveAll(t, g, envs))
	if err != nil {
		t.Fatalf("second CommitBatch: %v", err)
	}
	for i, res := range results {
		if res.Outcome != gate.OutcomeNoOp {
			t.Errorf("result %d = %s, want no-op", i, res.Outcome)
		}
		if res.W != head {
			t.Errorf("result %d reports W=%d after writing nothing, want %d", i, res.W, head)
		}
	}
}

// One transaction cannot write a key twice, and two envelopes for one facet in a
// single batch is nearly always a duplicated source row. Refuse it by name rather
// than letting DynamoDB reject the transaction with an error that identifies
// nothing.
func TestCommitBatch_RefusesTwoEnvelopesForOneFacet(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	g, _ := newGate(t)

	envs := []facet.Envelope{bpEnvelopeAt(1, 118), bpEnvelopeAt(2, 120), bpEnvelopeAt(1, 130)}
	_, err := g.CommitBatch(ctx, envs, resolveAll(t, g, envs))
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

	if _, err := g.CommitBatch(ctx, envs, resolveAll(t, g, envs)); err == nil {
		t.Fatal("a batch with a mis-keyed envelope was accepted")
	}
	if head, err := l.Head(ctx); err != nil || head != 0 {
		t.Errorf("watermark = %d (err %v) after a rejected batch, want 0", head, err)
	}
}

// The size bound is DynamoDB's transaction limit, and exceeding it must be a clear
// refusal rather than a cancelled transaction deep in the kernel.
func TestCommitBatch_RefusesOversizedBatches(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	g, _ := newGate(t)

	envs := envelopes(ledger.MaxBatch + 1)
	if _, err := g.CommitBatch(ctx, envs, resolveAll(t, g, envs)); err == nil {
		t.Errorf("a batch of %d was accepted (MaxBatch is %d)", len(envs), ledger.MaxBatch)
	}
}
