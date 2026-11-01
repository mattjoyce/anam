package daemon

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// hold occupies the write queue until the returned release is called. It returns once
// the holding write has begun, so the queue is known to be held.
func hold(t *testing.T, d *Daemon) (release func()) {
	t.Helper()
	entered, free, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		done <- d.Write(context.Background(), time.Minute, func(context.Context, Stores) error {
			close(entered)
			<-free
			return nil
		})
	}()
	<-entered
	released := false
	release = func() {
		if !released {
			released = true
			close(free)
			if err := <-done; err != nil {
				t.Errorf("holding write: %v", err)
			}
		}
	}
	t.Cleanup(release)
	return release
}

// drain returns once every write the queue accepted before it has run: the queue has
// one worker, so a write accepted now runs after them.
func drain(t *testing.T, d *Daemon) {
	t.Helper()
	if err := d.Write(context.Background(), time.Minute, func(context.Context, Stores) error { return nil }); err != nil {
		t.Fatalf("drain: %v", err)
	}
}

// A batch whose budget is spent waiting for an occupied queue is refused as busy within
// that budget, not after importer_wait, and nothing of it is written once the queue
// frees (card 110; the blue team's TestPressureImporterQueueHonorsCallerBudget).
func TestQueueWaitSpendsTheCallersBudget(t *testing.T) {
	t.Parallel()
	cfg := importerConfig(t)
	cfg.ImporterWait = time.Minute // far beyond the budget, so only the budget can end the wait
	d := open(t, cfg)
	start(t, d)
	before := head(t, d)
	release := hold(t, d)

	weight, _ := d.defs.current.Load().Lookup("health.weight")
	req := httptest.NewRequest(http.MethodPost, "/v1/envelopes", strings.NewReader(batch(weighIn("budget", 80, weight.Version))))
	req.Header.Set("Authorization", "Bearer "+importerToken)
	req.Header.Set(TimeoutHeader, "50ms")
	rec := httptest.NewRecorder()
	began := time.Now()
	d.Handler().ServeHTTP(rec, req)
	if elapsed := time.Since(began); elapsed > cfg.ImporterWait/2 {
		t.Errorf("a 50ms budget waited %v for the queue", elapsed)
	}
	if rec.Code != http.StatusServiceUnavailable || rec.Header().Get("Retry-After") == "" {
		t.Errorf("status %d, Retry-After %q; want 503 with Retry-After: %s", rec.Code, rec.Header().Get("Retry-After"), rec.Body)
	}

	release()
	drain(t, d)
	if got := head(t, d); got != before {
		t.Errorf("W = %d after the refused batch, want %d: it was written after all", got, before)
	}
}

func TestQueueBeforeAcceptance(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		budget time.Duration
		cancel bool
		want   error
	}{
		{"the budget runs out", 20 * time.Millisecond, false, ErrBusy},
		{"the caller goes away", time.Minute, true, context.Canceled},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := open(t, newConfig(t))
			start(t, d)
			release := hold(t, d)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tt.cancel {
				time.AfterFunc(20*time.Millisecond, cancel)
			}
			var ran atomic.Bool
			err := d.tryWriteWithin(ctx, time.Minute, tt.budget, func(context.Context, Stores) error {
				ran.Store(true)
				return nil
			})
			if !errors.Is(err, tt.want) {
				t.Errorf("tryWriteWithin = %v, want %v", err, tt.want)
			}

			release()
			drain(t, d)
			if ran.Load() {
				t.Error("the write ran after its caller was answered")
			}
		})
	}
}

// A caller already gone when the queue is free offers the select two ready cases. It
// may take either; the write must not run on either.
func TestQueueCancelRacesAFreeQueue(t *testing.T) {
	t.Parallel()
	d := open(t, newConfig(t))
	start(t, d)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var ran atomic.Int32
	for range 200 {
		err := d.tryWriteWithin(ctx, time.Minute, time.Minute, func(context.Context, Stores) error {
			ran.Add(1)
			return nil
		})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("tryWriteWithin = %v, want context.Canceled", err)
		}
	}
	drain(t, d)
	if n := ran.Load(); n != 0 {
		t.Errorf("%d writes ran for a caller already gone", n)
	}
}

// A write the worker takes after its budget has ended is not started, and is busy, as
// it would have been an instant earlier.
func TestQueueRunRefusesASpentBudget(t *testing.T) {
	t.Parallel()
	d := open(t, newConfig(t))
	ran := false
	spent := time.Now().Add(-time.Millisecond)
	err := d.run(context.Background(), job{
		fn:     func(context.Context, Stores) error { ran = true; return nil },
		caller: context.Background(),
		until:  func(time.Time) time.Time { return spent },
	})
	if !errors.Is(err, ErrBusy) || ran {
		t.Errorf("run = %v, ran %v; want ErrBusy and not run", err, ran)
	}
}

// Once accepted, a write runs to its end whatever its caller does, and its outcome is
// the one reported: an answer given before it ended would misreport a write that went
// on to happen.
func TestQueueDisconnectAfterAcceptance(t *testing.T) {
	t.Parallel()
	d := open(t, newConfig(t))
	start(t, d)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	entered, proceed := make(chan struct{}), make(chan struct{})
	var ran atomic.Bool
	result := make(chan error, 1)
	go func() {
		result <- d.tryWriteWithin(ctx, time.Minute, time.Minute, func(wctx context.Context, _ Stores) error {
			close(entered)
			<-proceed
			if err := wctx.Err(); err != nil {
				return err // the write's own context must outlive its caller's
			}
			ran.Store(true)
			return nil
		})
	}()
	<-entered
	cancel()
	close(proceed)
	if err := <-result; err != nil {
		t.Errorf("tryWriteWithin = %v, want the write's own outcome, nil", err)
	}
	if !ran.Load() {
		t.Error("the accepted write did not finish")
	}
}
