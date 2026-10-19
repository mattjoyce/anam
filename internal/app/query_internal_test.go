package app

import (
	"context"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/store/ledger"
)

// anam query check runs the equality check over a real ledger file, read-only, and
// reports what it compared. Sampling and a projection narrow it; a snapshot-shaped
// projection is refused.
func TestQueryCheck_OverARealLedger(t *testing.T) {
	t.Parallel()
	f := newLedgerFixture(t)
	before := f.admit(t, bpReadings)

	out, _, err := cli(t, f.cfgPath, "query", "check")
	if err != nil {
		t.Fatalf("query check: %v\n%s", err, out)
	}
	for _, want := range []string{"W=3", "live=3", "compared    3 of 3 fact-times (every 1)", "OK   current/2 = fold.Snapshot"} {
		if !strings.Contains(out, want) {
			t.Errorf("output omits %q:\n%s", want, out)
		}
	}

	out, _, err = cli(t, f.cfgPath, "query", "check", "--every", "2", "--projection", "bp", "--ledger", f.ledgerPath, "--person", "matt")
	if err != nil {
		t.Fatalf("query check --every 2 --projection bp: %v\n%s", err, out)
	}
	for _, want := range []string{"projection  bp v1", "served=3", "compared    2 of 3 fact-times (every 2)", "OK"} {
		if !strings.Contains(out, want) {
			t.Errorf("output omits %q:\n%s", want, out)
		}
	}

	if _, _, err := cli(t, f.cfgPath, "query", "check", "--projection", "meds"); err == nil || !strings.Contains(err.Error(), "snapshot-shaped") {
		t.Errorf("a snapshot-shaped projection = %v, want a refusal naming the shape", err)
	}
	if _, _, err := cli(t, f.cfgPath, "query", "check", "--every", "0"); err == nil {
		t.Error("--every 0 was accepted")
	}

	// The check wrote nothing: the ledger's head is where the admit left it.
	l, err := ledger.OpenReadOnly(context.Background(), f.ledgerPath, "matt")
	if err != nil {
		t.Fatalf("open ledger: %v", err)
	}
	defer func() { _ = l.Close() }()
	if after, err := l.Head(context.Background()); err != nil || after != before {
		t.Errorf("watermark moved from %d to %d (%v)", before, after, err)
	}
}
