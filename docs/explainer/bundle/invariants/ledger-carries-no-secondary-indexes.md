---
type: invariant
title: The ledger carries no derived indexes
description: Anything derived lives in the operational store; the ledger serves three access paths, two on a primary key and one on its own lookup index, anam_event_by_facet, for the gate's fold of one facet.
timestamp: 2026-10-22T00:00:00Z
resource: docs/ARCHITECTURE.md#Invariants
anchor: "The ledger carries **no derived indexes**"
basis: docs
tier: supporting
enforced_at: not enforced
applies_to:
  - /components/store-ledger.md
  - /components/store-ops.md
  - /terms/ledger.md
---

ARCHITECTURE §12, eighth bullet, last sentences, and §8. The three paths: one facet's versions highest first (the current version commit compares against), the anam-events in a seq range (replay since a watermark), and one facet's events by `anam_event_by_facet` on `(facet_id, seq)`, the lookup the gate's retraction check folds from (card 65). That index is on the ledger's own rows and serves the kernel's write path, so it is a lookup, not a derivation. The timeline in fact-time order, a type filter and changes_since are derived, and live in `tracking.db`. The index is installed as its own step from `internal/store/ledger/indexes.sql` (card 221) and a read-only open does not create it.

Until 2026-10-22 the doc said "no secondary indexes" while the code held this one; see [the ledger has one secondary index](/gaps/ledger-has-one-secondary-index.md), now closed.
