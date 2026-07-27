package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/gate"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

// What is under test here is admitAll's bookkeeping across blocks — specifically that
// a failure part-way through still reports what landed, because those writes are in
// the ledger whatever the exit code says.
//
// The gate is faked rather than real, against this package's usual preference, because
// the scenario is "block 3 of 5 fails" and a real gate cannot be asked for that
// without breaking a real ledger mid-run. The fake commits nothing and proves nothing
// about the gate; the gate's own package tests that.

// fakeAdmitter fails at a chosen block, or refuses a chosen envelope at resolve.
type fakeAdmitter struct {
	failBlock   int    // block index whose CommitBatch errors; -1 for none
	failResolve string // source_ref whose Resolve errors; "" for none

	blocks int
	w      ledger.Watermark
}

func (f *fakeAdmitter) Resolve(env facet.Envelope) (facet.Binding, error) {
	if f.failResolve != "" && env.SourceRef == f.failResolve {
		return facet.Binding{}, fmt.Errorf("resolve refused %s", env.SourceRef)
	}
	return facet.Binding{PersonID: "matt", Assurance: facet.AssuranceHigh}, nil
}

func (f *fakeAdmitter) CommitBatch(_ context.Context, envs []facet.Envelope, _ []facet.Binding) ([]gate.Result, ledger.Watermark, error) {
	block := f.blocks
	f.blocks++
	if block == f.failBlock {
		return nil, 0, errors.New("commit refused")
	}

	// One watermark for the call, as the real gate returns — not one per result.
	f.w += ledger.Watermark(len(envs))
	out := make([]gate.Result, len(envs))
	for i, env := range envs {
		out[i] = gate.Result{
			Outcome: gate.OutcomeAdmitted, FacetID: env.SourceRef, ArtifactVersion: 1,
		}
	}
	return out, f.w, nil
}

func testEnvelopes(n int) []facet.Envelope {
	envs := make([]facet.Envelope, n)
	for i := range envs {
		envs[i] = facet.Envelope{
			Type: "health.blood-pressure", Source: "bp-csv", Injector: "bp-csv",
			SourceRef: fmt.Sprintf("bp-csv:%02d", i),
			Payload:   []byte(`{"systolic": 118, "diastolic": 76}`),
		}
	}
	return envs
}

// The finding this pins: a 5,092-row import that dies at block 100 of 104 used to
// report nothing at all, though ~4,900 facets were durably in the ledger.
func TestAdmitAll_ReportsWhatLandedWhenABlockFails(t *testing.T) {
	t.Parallel()

	f := &fakeAdmitter{failBlock: 3, failResolve: ""}
	prog, err := admitAll(context.Background(), f, testEnvelopes(5), time.Second, 1)
	if err == nil {
		t.Fatal("admitAll returned nil error though block 3 refused")
	}

	if prog.done != 3 {
		t.Errorf("done = %d, want 3 (blocks 0..2 committed)", prog.done)
	}
	if prog.w != 3 {
		t.Errorf("W = %d, want 3", prog.w)
	}
	if got := prog.tally[gate.OutcomeAdmitted]; got != 3 {
		t.Errorf("admitted = %d, want 3", got)
	}

	report := prog.report(5)
	for _, want := range []string{"W=3", "admitted=3", "stopped after 3 of 5", "re-run"} {
		if !strings.Contains(report, want) {
			t.Errorf("report %q missing %q", report, want)
		}
	}
}

// The zero watermark is a real value meaning "empty ledger". When no block completed
// it is simply unobserved, and reporting it as W=0 would state something the run never
// learned.
func TestAdmitAll_SaysNothingCommittedRatherThanReportingWZero(t *testing.T) {
	t.Parallel()

	f := &fakeAdmitter{failBlock: 0, failResolve: ""}
	prog, err := admitAll(context.Background(), f, testEnvelopes(5), time.Second, 2)
	if err == nil {
		t.Fatal("admitAll returned nil error though the first block refused")
	}
	if prog.done != 0 {
		t.Errorf("done = %d, want 0", prog.done)
	}

	report := prog.report(5)
	if !strings.Contains(report, "nothing committed") {
		t.Errorf("report %q does not say nothing was committed", report)
	}
	if strings.Contains(report, "W=") {
		t.Errorf("report %q states a watermark the run never observed", report)
	}
}

// A resolve failure aborts its block before any of it is committed, but the blocks
// behind it are already written and must still be accounted for.
func TestAdmitAll_ResolveFailureKeepsTheBlocksBehindIt(t *testing.T) {
	t.Parallel()

	f := &fakeAdmitter{failBlock: -1, failResolve: "bp-csv:04"}
	prog, err := admitAll(context.Background(), f, testEnvelopes(6), time.Second, 2)
	if err == nil {
		t.Fatal("admitAll returned nil error though an envelope would not resolve")
	}
	if !strings.Contains(err.Error(), "bp-csv:04") {
		t.Errorf("error does not name the envelope: %v", err)
	}

	// Blocks [0,1] and [2,3] committed; [4,5] died at resolve.
	if prog.done != 4 {
		t.Errorf("done = %d, want 4", prog.done)
	}
	if prog.w != 4 {
		t.Errorf("W = %d, want 4", prog.w)
	}
}

// The whole run: every envelope accounted for, and no partial-run note.
func TestAdmitAll_CompleteRunReportsNoRemainder(t *testing.T) {
	t.Parallel()

	f := &fakeAdmitter{failBlock: -1, failResolve: ""}
	prog, err := admitAll(context.Background(), f, testEnvelopes(5), time.Second, 2)
	if err != nil {
		t.Fatalf("admitAll: %v", err)
	}
	if prog.done != 5 || prog.w != 5 {
		t.Errorf("done = %d, W = %d, want 5 and 5", prog.done, prog.w)
	}

	report := prog.report(5)
	if strings.Contains(report, "stopped after") {
		t.Errorf("a complete run reported itself partial: %q", report)
	}
	if !strings.Contains(report, "admitted=5") {
		t.Errorf("report %q missing the tally", report)
	}
}
