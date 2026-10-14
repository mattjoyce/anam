package daemon

import (
	"context"
	"time"
)

// The write queue runs one write at a time. Where the caller's say ends is
// acceptance, the moment the worker takes a write: before it, the caller's context
// and budget govern, and a write whose caller has gone or whose budget has run out
// is never started; after it, the write runs to its own deadline on the daemon's
// context, and its outcome is reported as it happened, never cut short because the
// caller stopped listening (card 110).

// Write runs fn alone and returns its error. deadline bounds fn and starts when the
// worker takes it, so a long write (a backup, a replay) does not spend its time in
// the queue. ctx bounds the wait to be accepted and the wait for the result. Write
// returns ErrStopped once shutdown has begun.
func (d *Daemon) Write(ctx context.Context, deadline time.Duration, fn WriteFunc) error {
	j := job{fn: fn, caller: ctx, until: func(started time.Time) time.Time { return started.Add(deadline) },
		done: make(chan error, 1)}
	select {
	case d.jobs <- j:
	case <-d.quit:
		return ErrStopped
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case err := <-j.done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// TryWrite runs fn alone within budget if the queue takes it within accept_wait, and
// returns ErrBusy if another write holds it longer.
func (d *Daemon) TryWrite(ctx context.Context, budget time.Duration, fn WriteFunc) error {
	return d.tryWriteWithin(ctx, d.cfg.AcceptWait, budget, fn)
}

// tryWriteWithin runs fn alone if the queue takes it within wait, as TryWrite. An
// importer's batch waits importer_wait, to wait out a mint or a short write rather
// than be refused for it.
//
// budget is the request's whole time, the shorter of the daemon's timeout and the
// caller's Anam-Timeout: the wait for the queue spends it, and fn has what is left.
// So the wait is never longer than the budget, and a request that runs out of it
// before the queue takes it is ErrBusy, its write never started. A caller that goes
// away before then gets ctx's error, and its write never starts either. Once fn is
// accepted it is waited for whatever ctx does: an answer given before it ended would
// misreport a write that went on to happen.
func (d *Daemon) tryWriteWithin(ctx context.Context, wait, budget time.Duration, fn WriteFunc) error {
	end := time.Now().Add(budget)
	j := job{fn: fn, caller: ctx, until: func(time.Time) time.Time { return end }, done: make(chan error, 1)}
	busy := time.NewTimer(min(wait, budget))
	defer busy.Stop()
	select {
	case d.jobs <- j:
	case <-d.quit:
		return ErrStopped
	case <-ctx.Done():
		return ctx.Err()
	case <-busy.C:
		return ErrBusy
	}
	return <-j.done
}

// work is the single write worker. It returns when quit closes, but never while a
// write is running: the select is only reached between writes.
func (d *Daemon) work(base context.Context) {
	for {
		// Shutdown wins over a write offered at the same moment.
		select {
		case <-d.quit:
			return
		default:
		}
		select {
		case <-d.quit:
			return
		case j := <-d.jobs:
			j.done <- d.run(base, j)
		}
	}
}

// run starts one accepted write, unless its caller is already gone or its time already
// spent. A select that finds the queue free and the caller's deadline or cancellation
// at once may take either, so the check is made again here, where the write begins.
func (d *Daemon) run(base context.Context, j job) error {
	started := time.Now()
	if err := j.caller.Err(); err != nil {
		return err
	}
	until := j.until(started)
	if !started.Before(until) {
		return ErrBusy
	}
	ctx, cancel := context.WithDeadline(base, until)
	defer cancel()
	return j.fn(ctx, d.writes)
}
