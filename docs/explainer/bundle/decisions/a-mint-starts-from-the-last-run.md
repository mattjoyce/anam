---
type: decision
title: A mint starts from the last run
description: The checkpoints at the last run are the value at that W, so only the buckets the events since then touched can differ; the full mint remains as oracle and repair.
timestamp: 2026-10-11T00:00:00Z
resource: internal/mint/incremental.go#fromLastRun
anchor: "if d.Budget <= 0 {"
basis: docs
tier: supporting
tested_by:
  - internal/mint/incremental_test.go#TestRun_FromTheLastRunIsTheFullMint
alternatives:
  - Fold the whole ledger on every mint and compare each bucket (kept as --full; 19.8 s against 132 ms at 600k facets on the ThinkPad)
  - Decide re-minting by W > covers_w (refused; W is global, so every ingest would re-mint every bucket)
affects:
  - /components/mint.md
  - /data-structures/checkpoint-row.md
  - /terms/mint.md
---

**Context.** Card 62. Whether a bucket needs re-minting is decided by its value, not by bookkeeping about it, and the tail is the pre-filter.

**Choice.** A run records its W after its checkpoints, and the run records a digest of the checkpoints a read from it takes. The next mint replays (R, W], rebuilds the buckets those events touched from their checkpoint plus the tail, splits over budget, and compares by hash. A log that no longer yields those checkpoints, or holds rows past the run, is one a mint crashed in or a cache lost entries from, so until a mint records a run again mints fold in full and a read that needs a missing checkpoint folds in full.

**Why.** A mint costs its tail, the buckets it touched and a stat per checkpoint, which is what lets the daemon mint after every change.

**What it costs.** The digest says the checkpoints are the ones the run took, not that their values are right; the full mint settles every bucket against the fold whatever the log holds, and is the test oracle the incremental path must equal.
