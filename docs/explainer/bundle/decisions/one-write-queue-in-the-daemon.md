---
type: decision
title: One write worker in the daemon
description: Every ledger and operational-store write inside the daemon is a job on one channel run by one goroutine under the job's deadline; reads use read-only handles and take no turn.
timestamp: 2026-10-11T00:00:00Z
resource: internal/daemon/queue.go#work
anchor: "j.done <- j.fn(ctx, d.writes)"
basis: code
tier: supporting
tested_by:
  - internal/daemon/daemon_test.go#TestWritesRunOneAtATime
  - internal/daemon/daemon_test.go#TestCallerGivingUpDoesNotCutTheWrite
alternatives:
  - Rely on SQLite's file lock and the gate's stale-watermark retry alone (would work for correctness, but a batch and a mint interleaving by lock would make the 503-with-Retry-After contract impossible to state)
affects:
  - /components/daemon.md
  - /components/catchup.md
  - /components/mint.md
---

**Context.** Cards 42 to 45 made the daemon the one process that owns the files.

**Choice.** Operator writes wait 250 ms to be accepted; an importer's batch waits 5 s, long enough to wait out a mint and short of a long write such as a backup. An accepted write runs on the daemon's context, so a client that disconnects does not cut it short and an answer never misreports a write that happened. The mint after importer batches is its own job, so its failure fails no batch.

**Why.** Serialising writes makes catch-up and the mint simple: each sees the ledger it was queued behind.

**What it costs.** A ledger lock held by another process (a CLI run against the same files) surfaces as a 500, not a 503 (see [ledger busy is not a 503](/gaps/architecture-doc-stale-sentences.md)); the reason for the exact wait constants is in comments, not a card.
