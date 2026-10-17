---
type: invariant
title: Queue wait spends the caller's budget up to acceptance
description: A request's whole time is the shorter of the daemon's timeout and the caller's Anam-Timeout; waiting for the write queue spends it, a write whose caller has gone or whose budget is spent never starts, and an accepted write is reported as it happened.
timestamp: 2026-10-17T00:00:00Z
resource: internal/daemon/queue.go#tryWriteWithin
anchor: "busy := time.NewTimer(min(wait, budget))"
basis: code
tier: supporting
enforced_at: internal/daemon/queue.go#run
enforced_anchor: "if !started.Before(until) {"
applies_to:
  - /components/daemon.md
  - /decisions/one-write-queue-in-the-daemon.md
  - /workflows/admit-a-batch.md
tested_by:
  - internal/daemon/queue_test.go#TestQueueWaitSpendsTheCallersBudget
  - internal/daemon/queue_test.go#TestQueueRunRefusesASpentBudget
  - internal/daemon/queue_test.go#TestQueueDisconnectAfterAcceptance
---

Card 110. Before it, a write waited up to the queue's wait and then ran under a deadline counted from when it began, so the time in the queue was not part of the request's budget. Now `tryWriteWithin` takes the request's budget as an end time counted from arrival: the wait to be accepted is `min(wait, budget)`, so a 10 ms batch facing an occupied queue is a 503 in 10 ms, not after `importer_wait`; `accept_wait` for an operator's write and `importer_wait` for an importer's, and the write gets what is left. A request that runs out of budget before the queue takes it is `ErrBusy`, a 503, and its write never started. A caller that goes away first gets its context's error, and its write never starts either.

`run` checks both again at the moment the worker takes a job, because a select that finds the queue free and the caller's deadline expired at once may pick either. After acceptance the write runs on the daemon's context to its own deadline and its outcome is waited for whatever the caller does, since an answer given before it ended would misreport a write that went on to happen. `Write`, for the daemon's own long writes (a backup, a replay), starts its deadline when the worker takes the job, so time in the queue does not spend a long write's time. See [one write worker in the daemon](/decisions/one-write-queue-in-the-daemon.md) and [every external call carries a deadline](/invariants/external-calls-fail-fast.md).
