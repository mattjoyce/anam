---
type: term
title: Mint
description: Checkpointing per bucket after each ingest that moved W, rebuilding only the buckets the tail touched.
timestamp: 2026-10-11T00:00:00Z
---

The mint writes [checkpoints](/terms/checkpoint.md). It starts from the last recorded run, reads the tail of [anam-events](/terms/anam-event.md) since it, rebuilds only the buckets those events touched, splits any over budget, compares each by hash with its head checkpoint, and writes only what changed. A run records its `W` and seals the log; until a mint seals it again, reads fold in full.

The daemon mints after every ingest that moved `W`, as its own write on the write queue. A full mint (`anam checkpoint --full`) settles every bucket against the fold and is both the oracle the incremental mint is tested against and the repair. The component is [mint](/components/mint.md).
