package ledger

import (
	"context"
	"errors"
	"testing"
	"time"
)

// The re-ask under throttling is a retry, so it is bounded and it pauses. The bound is
// what matters: without it, sustained throttling is a hot loop against the thing
// already asking to be left alone, ended only by the caller's deadline firing
// somewhere else as a timeout that names nothing.
func TestProbeBackoff_IsBoundedAndRises(t *testing.T) {
	t.Parallel()

	var total time.Duration
	last := time.Duration(0)
	for attempt := 1; attempt < maxProbeAttempts; attempt++ {
		d := probeBackoff(attempt)
		if d < last {
			t.Errorf("backoff fell at attempt %d: %s after %s", attempt, d, last)
		}
		if d > 800*time.Millisecond {
			t.Errorf("backoff at attempt %d is %s, above the 800ms ceiling", attempt, d)
		}
		last = d
		total += d
	}
	if total > 3*time.Second {
		t.Errorf("a fully throttled probe waits %s before giving up — too long to surface as a failure", total)
	}
}

// The pause belongs to the request it is made in. A sleep that ignored the context
// would keep a cancelled ingest waiting on a table nobody is listening for any more.
func TestPause_ReturnsOnContextCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	err := pause(ctx, time.Hour)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("pause on a cancelled context = %v, want context.Canceled", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("pause took %s to notice cancellation", elapsed)
	}
}
