---
type: component
title: Catch-up (internal/catchup)
description: Replays anam-events since the index's last seen seq into the operational store, so the derived tier is behind after a crash, never wrong.
timestamp: 2026-10-11T00:00:00Z
resource: internal/catchup/catchup.go#Sync
anchor: "func Sync(ctx context.Context, l Ledger, o Ops, buckets span.Buckets) (Result, error) {"
basis: code
tier: supporting
group: derived
depends_on:
  - /components/store-ledger.md
  - /components/store-ops.md
  - /components/fold.md
  - /components/span.md
---

There is no transaction across the two files, so the index is never written in the same breath as the ledger. `Sync` reads the index's `last_seq`, replays the ledger's events after it, folds the state each leaves, stamps each facet's bucket from its fact-time, and writes [facet index rows](/data-structures/facet-index-row.md) and event index rows. An index ahead of the ledger is `ErrIndexAhead`, which the daemon refuses at start rather than serving from.

The daemon runs it as its first write and after every admission. `anam retract` does not run it, so after a local retraction the index is behind until the daemon's next start.
