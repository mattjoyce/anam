package daemon

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/mattjoyce/anam/internal/api"
	"github.com/mattjoyce/anam/internal/catchup"
	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/mint"
	"github.com/mattjoyce/anam/internal/store/ops"
)

// mintAtHead mints at the head, as its own write: a mint is not part of what a batch
// admitted, so its failure is reported and fails nothing (ARCHITECTURE.md §7). It
// starts from the last run, so it costs the buckets the batches touched.
func (d *Daemon) mintAtHead() *api.MintRun {
	var m api.Mint
	err := d.Write(context.Background(), d.cfg.Timeout, func(ctx context.Context, s Stores) error {
		var err error
		m, err = mint.Report(ctx, d.mintDeps(s), api.MintQuery{})
		return err
	})
	if err != nil {
		slog.Error("daemon: mint at head failed", "err", err)
		return &api.MintRun{Error: err.Error()}
	}
	slog.Info("daemon: minted", "w", m.W, "full", m.Full, "from", m.From,
		"minted", m.MintedCount(), "buckets", len(m.Outcomes),
		"collected_checkpoints", m.Collected.Checkpoints, "collected_bytes", m.Collected.Bytes)
	if m.CollectError != "" {
		slog.Warn("daemon: collecting old checkpoints failed; the next mint tries again", "err", m.CollectError)
	}
	return &api.MintRun{W: m.W, Full: m.Full, From: m.From, Minted: m.MintedCount(), Buckets: len(m.Outcomes)}
}

// requestMint asks the mint worker for a mint at the head, without waiting. A request
// already waiting covers this one.
func (d *Daemon) requestMint() {
	select {
	case d.mints <- struct{}{}:
	default:
	}
}

// mintOnRequest is the mint worker: once requests stop arriving for mint_quiet, it
// mints, until shutdown. An importer's batch is answered when it is admitted, not
// when it is minted, so its mint runs here rather than in the request.
func (d *Daemon) mintOnRequest() {
	for {
		select {
		case <-d.quit:
			return
		case <-d.mints:
		}
		quiet := time.NewTimer(d.cfg.MintQuiet)
		for waiting := true; waiting; {
			select {
			case <-d.quit:
				quiet.Stop()
				return
			case <-d.mints:
				quiet.Reset(d.cfg.MintQuiet)
			case <-quiet.C:
				waiting = false
			}
		}
		d.mintAtHead()
	}
}

// mintIfBehind asks for a mint when the last mint run is behind the head: a batch
// admitted just before a shutdown left its mint undone.
func (d *Daemon) mintIfBehind(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, d.cfg.Timeout)
	defer cancel()
	head, err := d.read.Head(ctx)
	if err != nil || head == 0 {
		return
	}
	run, ok, err := d.readOps.LastMintRun(ctx, int64(head))
	if err == nil && (!ok || run < int64(head)) {
		d.requestMint()
	}
}

// catchUp replays the ledger into the operational store's index, so reads of it are
// as fresh as the last write. It runs inside a write, on the write handles, with the
// long-write budget (long_write_timeout) rather than the per-call timeout: a replay
// of the whole ledger after the index was rebuilt must be able to finish. A failure is not the write's. An
// index left behind is reported in every answer read from it; an index found wrong,
// built from another ledger or ahead of this one, is recorded, and the routes that
// read it refuse until the daemon is restarted on a sound one.
func (d *Daemon) catchUp(ctx context.Context, s Stores) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), d.cfg.LongWriteTimeout)
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

// sources is what the daemon has seen of each importer since it started: when its
// last batch arrived and when one last changed the record. It is a view for
// /v1/status, held in memory and forgotten on restart; the ledger is the record.
type sources struct {
	mu   sync.Mutex
	seen map[string]*api.SourceStatus
}

func newSources() *sources { return &sources{seen: map[string]*api.SourceStatus{}} }

// saw records a batch from source at at.
func (in *sources) saw(source, at string, admitted bool) {
	in.mu.Lock()
	defer in.mu.Unlock()
	st := in.seen[source]
	if st == nil {
		st = &api.SourceStatus{Source: source}
		in.seen[source] = st
	}
	st.LastScanAt = at
	if admitted {
		st.LastAdmissionAt = at
	}
}

// statuses is a copy of what has been seen of each source, in name order.
func (in *sources) statuses() []api.SourceStatus {
	in.mu.Lock()
	defer in.mu.Unlock()
	out := make([]api.SourceStatus, 0, len(in.seen))
	for _, st := range in.seen {
		out = append(out, *st)
	}
	sortSources(out)
	return out
}

func (d *Daemon) now() string { return clock.Format(time.Now()) }
