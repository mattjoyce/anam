package daemon

import (
	"context"
	"time"
)

// Write runs fn alone, under deadline, and returns its error. ctx bounds only the wait
// to be accepted and the wait for the result: once fn starts it runs on the daemon's
// context, so a caller that gives up does not cut a write short. Write returns
// ErrStopped once shutdown has begun.
func (d *Daemon) Write(ctx context.Context, deadline time.Duration, fn WriteFunc) error {
	j := job{fn: fn, deadline: deadline, done: make(chan error, 1)}
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

// TryWrite runs fn alone, under deadline, if the queue takes it within accept_wait,
// and returns ErrBusy if another write holds it longer. Once fn is accepted it is
// waited for: its own deadline bounds it, and an answer given before it ended would
// misreport a write that went on to happen.
func (d *Daemon) TryWrite(deadline time.Duration, fn WriteFunc) error {
	return d.tryWriteWithin(d.cfg.AcceptWait, deadline, fn)
}

// tryWriteWithin is TryWrite waiting up to wait for the queue. An importer's batch
// waits out a mint or a short write rather than be refused for it.
func (d *Daemon) tryWriteWithin(wait, deadline time.Duration, fn WriteFunc) error {
	j := job{fn: fn, deadline: deadline, done: make(chan error, 1)}
	busy := time.NewTimer(wait)
	defer busy.Stop()
	select {
	case d.jobs <- j:
	case <-d.quit:
		return ErrStopped
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
			ctx, cancel := context.WithTimeout(base, j.deadline)
			j.done <- j.fn(ctx, d.writes)
			cancel()
		}
	}
}
