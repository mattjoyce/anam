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

	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

// IngestFunc runs one ingest of every declared import on the daemon's write handles.
// It reports each source it reached, including the one it failed on, alongside the
// error: batches that committed are in the ledger whatever the error says.
type IngestFunc func(ctx context.Context, s Stores) ([]SourceResult, error)

// SourceResult is one declared import's part of a run.
type SourceResult struct {
	Source     string `json:"source"`
	Offered    int    `json:"offered"`
	Admitted   int    `json:"admitted"`
	Superseded int    `json:"superseded"`
	Unchanged  int    `json:"unchanged"`
	// Committed is how many of Offered have an outcome. Less than Offered means the
	// run stopped inside this source.
	Committed int `json:"committed"`
}

// NothingNew is the summary of a complete run that wrote nothing. It is a fixed
// string because a scheduler matches on it to stay quiet about such a run.
const NothingNew = "ledger unchanged: nothing new in any source"

// Verdict is the one-line summary of a complete run: whether the ledger moved, in a
// line that reads the same to a person and to a string match.
func Verdict(changed int) string {
	if changed == 0 {
		return NothingNew
	}
	return fmt.Sprintf("ledger changed: %d facets admitted or superseded", changed)
}

// Operation states.
const (
	StatePending   = "pending"
	StateRunning   = "running"
	StateSucceeded = "succeeded"
	StateFailed    = "failed"
)

// Operation is one ingest run as the API reports it. It lives in memory and is lost
// when the daemon restarts. The durable answer to "what landed" is the range
// WBefore..WAfter, which the ledger can always be asked about again.
type Operation struct {
	ID          string         `json:"operation_id"`
	Kind        string         `json:"kind"`
	State       string         `json:"state"`
	RequestedAt string         `json:"requested_at"`
	StartedAt   string         `json:"started_at,omitempty"`
	FinishedAt  string         `json:"finished_at,omitempty"`
	WBefore     *int64         `json:"w_before,omitempty"`
	WAfter      *int64         `json:"w_after,omitempty"`
	Sources     []SourceResult `json:"sources,omitempty"`
	// Changed is how many facets the run admitted or superseded, failed or not.
	Changed int `json:"changed"`
	// Summary is Verdict's line for a run that succeeded.
	Summary string `json:"summary,omitempty"`
	Error   string `json:"error,omitempty"`

	// done is closed when the run finishes.
	done chan struct{}
}

// finished reports whether the operation has reached a final state.
func (op Operation) finished() bool { return op.State == StateSucceeded || op.State == StateFailed }

// SourceStatus is what the daemon has seen of one source since it started.
type SourceStatus struct {
	Source string `json:"source"`
	// LastScanAt is when the last run that read this source began.
	LastScanAt string `json:"last_scan_at"`
	// LastAdmissionAt is when the last run that admitted or superseded anything
	// from this source began. Empty if none has since the daemon started.
	LastAdmissionAt string `json:"last_admission_at,omitempty"`
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
	running *Operation
	pending *Operation
	byID    map[string]*Operation
	order   []string
	sources map[string]*SourceStatus
}

func newIngests() *ingests {
	return &ingests{byID: map[string]*Operation{}, sources: map[string]*SourceStatus{}}
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
	op := &Operation{ID: newID(), Kind: "ingest", State: StatePending, RequestedAt: d.now(), done: make(chan struct{})}
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
func (in *ingests) remember(op *Operation) {
	in.byID[op.ID] = op
	in.order = append(in.order, op.ID)
	for len(in.order) > keptOperations {
		oldest := in.byID[in.order[0]]
		if oldest.State == StatePending || oldest.State == StateRunning {
			break
		}
		delete(in.byID, in.order[0])
		in.order = in.order[1:]
	}
}

// Operation returns a copy of the operation with id, if the daemon still holds it.
func (d *Daemon) Operation(id string) (Operation, bool) {
	in := d.ingests
	in.mu.Lock()
	defer in.mu.Unlock()
	op, ok := in.byID[id]
	if !ok {
		return Operation{}, false
	}
	c := *op
	c.Sources = append([]SourceResult(nil), op.Sources...)
	return c, true
}

// WaitOperation returns the operation with id once it has finished, or as it stands
// when ctx ends or shutdown begins, whichever is first.
func (d *Daemon) WaitOperation(ctx context.Context, id string) (Operation, bool) {
	op, ok := d.Operation(id)
	if !ok || op.finished() {
		return op, ok
	}
	select {
	case <-op.done:
	case <-ctx.Done():
	case <-d.quit: // shutdown answers a waiting caller rather than holding it up
	}
	return d.Operation(id)
}

// runIngests runs op, then each run that became pending meanwhile, until none is.
func (d *Daemon) runIngests(op *Operation) {
	for op != nil {
		d.runIngest(op)

		in := d.ingests
		in.mu.Lock()
		op, in.pending = in.pending, nil
		in.running = op
		in.mu.Unlock()
	}
}

func (d *Daemon) runIngest(op *Operation) {
	in := d.ingests
	in.mu.Lock()
	op.State, op.StartedAt = StateRunning, d.now()
	in.mu.Unlock()

	var before, after *int64
	var results []SourceResult
	// The wait is bounded by shutdown (ErrStopped) and the run by its own deadline;
	// no caller's context is involved.
	err := d.Write(context.Background(), d.cfg.IngestTimeout, func(ctx context.Context, s Stores) error {
		before = d.head(ctx, s.Ledger)
		var err error
		results, err = d.opts.Ingest(ctx, s)
		after = d.head(ctx, s.Ledger)
		return err
	})

	in.mu.Lock()
	defer in.mu.Unlock()
	defer close(op.done)
	op.FinishedAt, op.WBefore, op.WAfter, op.Sources = d.now(), before, after, results
	for _, r := range results {
		op.Changed += r.Admitted + r.Superseded
	}
	op.State, op.Summary = StateSucceeded, Verdict(op.Changed)
	if err != nil {
		op.State, op.Summary, op.Error = StateFailed, "", err.Error()
		slog.Error("daemon: ingest failed", "operation", op.ID, "err", err)
	} else {
		slog.Info("daemon: ingest done", "operation", op.ID, "w_before", deref(before), "w_after", deref(after))
	}
	for _, r := range results {
		st := in.sources[r.Source]
		if st == nil {
			st = &SourceStatus{Source: r.Source}
			in.sources[r.Source] = st
		}
		st.LastScanAt = op.StartedAt
		if r.Admitted+r.Superseded > 0 {
			st.LastAdmissionAt = op.StartedAt
		}
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
func (in *ingests) sourceStatuses() []SourceStatus {
	in.mu.Lock()
	defer in.mu.Unlock()
	out := make([]SourceStatus, 0, len(in.sources))
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
