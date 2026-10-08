package daemon

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/mattjoyce/anam/internal/api"
	"github.com/mattjoyce/anam/internal/catchup"
	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/store/ledger"
	"github.com/mattjoyce/anam/internal/store/ops"
)

// IngestFunc runs one ingest of every declared import on the daemon's write handles.
// It reports each source it reached, including the one it failed on, alongside the
// error: batches that committed are in the ledger whatever the error says.
type IngestFunc func(ctx context.Context, s Stores) ([]api.SourceResult, error)

// operation is an api.Operation the daemon holds, with the channel that closes when
// its run finishes. Callers are given copies of the value, never the channel.
type operation struct {
	api.Operation
	done chan struct{}
}

// keptOperations bounds how many finished operations the daemon remembers. An hourly
// schedule fills it in two days; a caller asking after that reads the ledger.
const keptOperations = 50

// ingests is the ingest coordinator: at most one run in progress and at most one
// pending. A request while a run is in progress joins the pending run, because the
// running one has already read its sources and would miss whatever the request was
// sent about.
type ingests struct {
	mu      sync.Mutex
	running *operation
	pending *operation
	byID    map[string]*operation
	order   []string
	sources map[string]*api.SourceStatus
}

func newIngests() *ingests {
	return &ingests{byID: map[string]*operation{}, sources: map[string]*api.SourceStatus{}}
}

// RequestIngest returns the id of the run that will take in what is in the sources
// now: a new one that starts at once, or the pending one.
func (d *Daemon) RequestIngest() (string, error) {
	select {
	case <-d.quit:
		return "", ErrStopped
	default:
	}
	if d.opts.Ingest == nil {
		return "", errors.New("daemon: no ingest configured")
	}

	in := d.ingests
	in.mu.Lock()
	defer in.mu.Unlock()
	if in.pending != nil {
		return in.pending.ID, nil
	}
	op := &operation{
		Operation: api.Operation{ID: newID(), Kind: "ingest", State: api.StatePending, RequestedAt: d.now()},
		done:      make(chan struct{}),
	}
	in.remember(op)
	if in.running == nil {
		in.running = op
		go d.runIngests(op)
	} else {
		in.pending = op
	}
	return op.ID, nil
}

// remember keeps op, dropping the oldest finished operation past the bound.
// Callers hold mu.
func (in *ingests) remember(op *operation) {
	in.byID[op.ID] = op
	in.order = append(in.order, op.ID)
	for len(in.order) > keptOperations {
		oldest := in.byID[in.order[0]]
		if oldest.State == api.StatePending || oldest.State == api.StateRunning {
			break
		}
		delete(in.byID, in.order[0])
		in.order = in.order[1:]
	}
}

// Operation returns a copy of the operation with id, if the daemon still holds it.
func (d *Daemon) Operation(id string) (api.Operation, bool) {
	in := d.ingests
	in.mu.Lock()
	defer in.mu.Unlock()
	op, ok := in.byID[id]
	if !ok {
		return api.Operation{}, false
	}
	c := op.Operation
	c.Sources = append([]api.SourceResult(nil), op.Sources...)
	return c, true
}

// WaitOperation returns the operation with id once it has finished, or as it stands
// when ctx ends or shutdown begins, whichever is first.
func (d *Daemon) WaitOperation(ctx context.Context, id string) (api.Operation, bool) {
	d.ingests.mu.Lock()
	held, ok := d.ingests.byID[id]
	d.ingests.mu.Unlock()
	if !ok {
		return api.Operation{}, false
	}
	select {
	case <-held.done:
	case <-ctx.Done():
	case <-d.quit: // shutdown answers a waiting caller rather than holding it up
	}
	return d.Operation(id)
}

// runIngests runs op, then each run that became pending meanwhile, until none is.
func (d *Daemon) runIngests(op *operation) {
	for op != nil {
		d.runIngest(op)

		in := d.ingests
		in.mu.Lock()
		op, in.pending = in.pending, nil
		in.running = op
		in.mu.Unlock()
	}
}

func (d *Daemon) runIngest(op *operation) {
	in := d.ingests
	in.mu.Lock()
	op.State, op.StartedAt = api.StateRunning, d.now()
	in.mu.Unlock()

	var before, after *int64
	var results []api.SourceResult
	// The wait is bounded by shutdown (ErrStopped) and the run by its own deadline;
	// no caller's context is involved.
	err := d.Write(context.Background(), d.cfg.IngestTimeout, func(ctx context.Context, s Stores) error {
		before = d.head(ctx, s.Ledger)
		var err error
		results, err = d.opts.Ingest(ctx, s)
		after = d.head(ctx, s.Ledger)
		d.catchUp(ctx, s)
		return err
	})

	in.mu.Lock()
	defer in.mu.Unlock()
	defer close(op.done)
	op.FinishedAt, op.WBefore, op.WAfter, op.Sources = d.now(), before, after, results
	for _, r := range results {
		op.Changed += r.Admitted + r.Superseded
	}
	op.State, op.Summary = api.StateSucceeded, api.Verdict(op.Changed)
	if err != nil {
		op.State, op.Summary, op.Error = api.StateFailed, "", err.Error()
		slog.Error("daemon: ingest failed", "operation", op.ID, "err", err)
	} else {
		slog.Info("daemon: ingest done", "operation", op.ID, "w_before", deref(before), "w_after", deref(after))
	}
	for _, r := range results {
		st := in.sources[r.Source]
		if st == nil {
			st = &api.SourceStatus{Source: r.Source}
			in.sources[r.Source] = st
		}
		st.LastScanAt = op.StartedAt
		if r.Admitted+r.Superseded > 0 {
			st.LastAdmissionAt = op.StartedAt
		}
	}
}

// catchUp replays the ledger into the operational store's index, so reads of it are
// as fresh as the last write. It runs inside a write, on the write handles, with the
// ingest budget rather than the per-call timeout: a replay of the whole ledger after
// the index was rebuilt must be able to finish. A failure is not the write's. An
// index left behind is reported in every answer read from it; an index found wrong,
// built from another ledger or ahead of this one, is recorded, and the routes that
// read it refuse until the daemon is restarted on a sound one.
func (d *Daemon) catchUp(ctx context.Context, s Stores) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), d.cfg.IngestTimeout)
	defer cancel()
	buckets, _ := d.cfg.Buckets() // Open has already accepted the width.
	res, err := catchup.Sync(ctx, s.Ledger, s.Ops, buckets)
	switch {
	case errors.Is(err, ops.ErrLedgerChanged), errors.Is(err, catchup.ErrIndexAhead):
		d.index.set(err)
		slog.Error("daemon: operational index is wrong", "err", err)
	case err != nil:
		slog.Warn("daemon: index catch-up, left behind", "err", err)
	case res.To != res.From:
		slog.Info("daemon: index caught up", "from", res.From, "to", res.To)
	}
}

// head reads W for an operation's record. It has its own deadline, so a run that
// spent its own still reports where the ledger stands; nil means it could not.
func (d *Daemon) head(ctx context.Context, l *ledger.Ledger) *int64 {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), d.cfg.Timeout)
	defer cancel()
	w, err := l.Head(ctx)
	if err != nil {
		slog.Warn("daemon: head unreadable", "err", err)
		return nil
	}
	v := int64(w)
	return &v
}

// sourceStatuses is a copy of what has been seen of each source, in name order.
func (in *ingests) sourceStatuses() []api.SourceStatus {
	in.mu.Lock()
	defer in.mu.Unlock()
	out := make([]api.SourceStatus, 0, len(in.sources))
	for _, st := range in.sources {
		out = append(out, *st)
	}
	sortSources(out)
	return out
}

func (in *ingests) ids() (running, pending string) {
	in.mu.Lock()
	defer in.mu.Unlock()
	if in.running != nil {
		running = in.running.ID
	}
	if in.pending != nil {
		pending = in.pending.ID
	}
	return running, pending
}

func (d *Daemon) now() string { return clock.Format(time.Now()) }

func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b) // crypto/rand.Read never returns an error.
	return "op-" + hex.EncodeToString(b)
}

func deref(p *int64) string {
	if p == nil {
		return "unknown"
	}
	return fmt.Sprint(*p)
}
