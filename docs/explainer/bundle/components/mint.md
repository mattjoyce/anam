---
type: component
title: Mint
description: Writes bucket checkpoints after each change, starting from the last run and touching only the buckets the tail dirtied; also the composed reads that start from those checkpoints.
timestamp: 2026-10-11T00:00:00Z
resource: internal/mint/mint.go#Run
anchor: "func Run(ctx context.Context, d Deps, dryRun bool) (Result, error) {"
basis: code
tier: supporting
group: derived
depends_on:
  - /components/fold.md
  - /components/span.md
  - /components/store-ledger.md
  - /components/store-ops.md
---

Two jobs in one package, because they share the layout and the tail logic.

**Writing.** `Run` tries `fromLastRun`: with a budget, a sealed log and a last run, it replays the tail in (R, W], finds the buckets those events dirtied (where each touched facet's version sat at R and sits at W), rebuilds them from their checkpoint plus the tail, splits any over budget along the calendar, and settles each by comparing its hash with the head row. Unchanged buckets stand without being decoded. Then it records the run and seals the log. `RunFull` folds the whole ledger and settles every bucket; it is the oracle the incremental mint is tested against and the repair when the log is unsealed.

**Reading.** `ReadAt` loads the leaf checkpoints of the last run at or below W, applies the tail, and gives a composed value with no fold of the whole ledger. `ReadWindow` decodes only the buckets a fact-time window overlaps and those the tail dirtied, streaming every other bucket's bytes into the whole-value hash. Both return ok=false, and the caller folds in full, when the log is not pinned to this ledger, not sealed, has no run, or holds weekly ids.

Traced in [mint checkpoints](/workflows/mint-checkpoints.md) and [read the timeline](/workflows/read-the-timeline.md).
