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

**Writing.** `Run` tries `fromLastRun`: with a budget, a sealed log and a last run, it replays the tail in (R, W], finds the buckets those events dirtied (where each touched facet's version sat at R and sits at W), rebuilds them from their checkpoint plus the tail, splits any over budget along the calendar, and settles each by comparing its hash with the head row. Unchanged buckets stand without being decoded. Then it records the run with a digest of its checkpoints and clears the checkpoints the newest run does not read. `RunFull` folds the whole ledger and settles every bucket; it is the oracle the incremental mint is tested against and the repair when the log is unsealed, and it writes back any value the log names that the store lost.

**Reading.** `ReadAt` loads the leaf checkpoints of the last run at or below W, applies the tail, and gives a composed value with no fold of the whole ledger. `ReadWindow` decodes only the buckets a fact-time window overlaps and those the tail dirtied, streaming every other bucket's bytes into the whole-value hash. Both return ok=false, and the caller folds in full, when the log is not pinned to this ledger, has no run, holds weekly ids, or can no longer yield the checkpoints the run took (a row or a value is gone). A checkpoint is a cache, and a missing one is a miss.

Traced in [mint checkpoints](/workflows/mint-checkpoints.md) and [read the timeline](/workflows/read-the-timeline.md).
