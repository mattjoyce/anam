---
type: workflow
title: Mint checkpoints
description: anam checkpoint over the daemon rebuilds only the buckets the events since the last run touched, writes a new version for each whose hash moved, and clears what the newest run does not read.
timestamp: 2026-10-11T00:00:00Z
resource: internal/mint/incremental.go#fromLastRun
anchor: "r, ok, err := d.Index.LastMintRun(ctx, int64(w))"
basis: code
tier: supporting
trigger: The operator runs `anam checkpoint` with `ANAM_SERVER` set; the daemon runs the same `mint.Report` on its own after importer batches and at start when behind.
steps:
  - actor: /components/cli.md
    caller: outside
    label: checkpointCmd reads --dry-run and --full
    action: "Builds an `api.MintQuery` and, with a server configured, hands it to the client; locally it opens the stores with `Budget: cfg.CheckpointBudget` (1 MiB by default)."
    resource: internal/app/checkpoint.go#checkpointCmd
    anchor: 'cmd.Flags().BoolVar(&q.Full, "full", false,'
  - actor: /components/client.md
    caller: /components/cli.md
    label: POST /v1/checkpoints
    action: The query goes as `dry_run` and `full` parameters.
    resource: internal/client/client.go#Mint
    anchor: 'q.Set("full", "true")'
  - actor: /components/daemon.md
    caller: /components/client.md
    label: take a turn, or 503 at once
    action: "`postCheckpoints` uses `TryWrite`, which waits `accept_wait` (250 ms) for the worker, or less when the request's budget is shorter; a write still running, such as a backup, answers 503 naming it, and a caller that has gone never starts the mint. The mint runs with the write handles and the daemon's budget."
    resource: internal/daemon/api.go#postCheckpoints
    anchor: "err := d.TryWrite(r.Context(), d.timeout(r), func(ctx context.Context, s Stores) error {"
  - actor: /components/mint.md
    caller: /components/daemon.md
    label: Report dispatches to Run or RunFull
    action: "`--full` takes `RunFull`; otherwise `Run` tries the incremental path and falls back to the full one when it says it cannot."
    resource: internal/mint/mint.go#Report
    anchor: "if q.Full {"
  - actor: /components/mint.md
    label: pin the ledger id in the index
    action: The first mint over a tracking file pins the ledger id; a different ledger is refused rather than indexed.
    resource: internal/mint/mint.go#pin
    anchor: "return d.Index.PinLedger(ctx, id)"
  - actor: /components/mint.md
    branch: no budget, unsealed log, no run, or --full
    label: full mint, every bucket settled
    action: "`full` folds the whole ledger, partitions the value by the layout the checkpoint log implies (or the fixed width when the budget is 0), subdivides over-budget buckets, settles every bucket that has live facets or was ever minted (an emptied bucket re-mints as the empty value), and records the run as in the final step. It is the oracle and the repair."
    ends: true
    reads: [/data-structures/anam-event.md, /data-structures/facet-artifact.md]
    resource: internal/mint/mint.go#full
    anchor: "res, err := fold.Read(ctx, d.Ledger)"
  - actor: /components/mint.md
    branch: a sealed log with a last run
    label: replay the tail since the last run
    action: "`fromLastRun` replays the events in (R, W] and finds the buckets they dirtied: where each touched facet's version sat at R and where its version sits at W."
    reads: [/data-structures/anam-event.md, /data-structures/checkpoint-row.md]
    resource: internal/mint/incremental.go#fromLastRun
    anchor: "tail, err := d.Ledger.Replay(ctx, run, w)"
  - actor: /components/mint.md
    label: rebuild touched and over-budget buckets
    action: Each dirtied bucket is loaded from its checkpoint value (hash-checked), touched facets dropped, the tail's live entries added. A bucket whose value is over budget is rebuilt too, because the budget was lowered or the run was at a fixed width.
    reads: [/data-structures/checkpoint-value.md]
    resource: internal/mint/incremental.go#rebuild
    anchor: "if n > int64(d.Budget) {"
  - actor: /components/mint.md
    label: split any bucket over budget
    action: A bucket whose serialised value exceeds the budget is split along the calendar, a year into months and a month into days, and its entries re-placed, until none is over or all are days. Bytes decide when, the calendar decides where.
    resource: internal/mint/mint.go#split
    anchor: "if len(val.Bytes()) <= budget {"
  - actor: /components/mint.md
    label: compare each bucket's hash with its head
    action: "`settle` builds the bucket's value and compares its hash with the head checkpoint row's. Whether a bucket needs re-minting is decided by its value, not by bookkeeping about it."
    reads: [/data-structures/checkpoint-row.md]
    resource: internal/mint/mint.go#settle
    anchor: "if minted && head.ValueHash == value.Hash() {"
  - actor: /components/store-ops.md
    caller: /components/mint.md
    branch: hash differs
    label: write the value and a new row
    action: The value's bytes go to `checkpoint_value` under their hash, then a `checkpoint` row at version head+1 covering W, then its facet count. A dry run returns before any write.
    writes: [/data-structures/checkpoint-value.md, /data-structures/checkpoint-row.md]
    resource: internal/mint/mint.go#settle
    anchor: "d.Values.Put(ctx, Kind, value.Bytes())"
  - actor: /components/mint.md
    branch: hash equal
    label: the head stands, undecoded
    action: Nothing is written; the facet count comes from `checkpoint_size`, recorded once if missing, and a checkpoint value that has gone missing is re-put.
    resource: internal/mint/incremental.go#unchanged
    anchor: "n, ok := sizes[ops.SpanVersion{SpanID: head.SpanID, V: head.V}]"
  - actor: /components/store-ops.md
    caller: /components/mint.md
    label: record the run and a digest of its picks
    action: "One transaction takes the checkpoints a read from W takes and upserts `mint_run(w, picks)` with their digest, after the checkpoints, so a run that crashed before this is invisible to readers and leaves rows the log's newest run does not account for until the next mint."
    writes: [/data-structures/checkpoint-row.md]
    resource: internal/store/ops/checkpoint.go#AppendMintRun
    anchor: "`INSERT INTO mint_run (w, ran_at, picks) VALUES (?, ?, ?)"
  - actor: /components/store-ops.md
    caller: /components/mint.md
    label: clear what the newest run does not read
    action: "`collect` ends a run that wrote: `CollectCheckpoints`, in one transaction, deletes the checkpoint rows, sizes and values that are not each span's latest checkpoint at or below the newest run. A checkpoint is a cache: a read that needs one that is gone declines and folds in full, and a later mint writes back a value the log names that the store lost."
    writes: [/data-structures/checkpoint-row.md]
    resource: internal/store/ops/collect.go#CollectCheckpoints
    anchor: "IS NOT, because a span with no row at or below the run has a NULL pick."
  - actor: /components/cli.md
    label: print each bucket's outcome
    action: One line per bucket, minted or unchanged or would-mint, with version, covered W, facet count and value hash, and a footer with the count at W, and what the collection removed.
    ends: true
    resource: internal/app/checkpoint.go#writeMint
    anchor: 'fmt.Fprintln(w, "BUCKET\tV\tCOVERS W\tFACETS\tVALUE\t")'
---

The mint's only job is to bound replay; freshness comes from the ledger. The incremental path costs its tail, the buckets the tail touched and a size per checkpoint value. The full mint remains as the oracle it is tested against and the repair for an unsealed log.

Related: [bucket on fact-time, subdivide by budget](/decisions/bucket-on-fact-time-subdivide-by-budget.md), [a mint starts from the last run](/decisions/a-mint-starts-from-the-last-run.md), [minting is per span, never whole-life](/invariants/minting-is-per-span-never-whole-life.md).
