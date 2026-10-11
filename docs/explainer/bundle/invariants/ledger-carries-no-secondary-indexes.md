---
type: invariant
title: The ledger carries no secondary indexes
description: An index is derived, and derived things live in the operational store; the ledger serves two access paths, both on a primary key.
timestamp: 2026-10-11T00:00:00Z
resource: docs/ARCHITECTURE.md#Invariants
anchor: "The ledger carries **no secondary indexes**"
basis: docs
tier: supporting
enforced_at: not enforced
applies_to:
  - /components/store-ledger.md
  - /components/store-ops.md
  - /terms/ledger.md
---

ARCHITECTURE §12, eighth bullet, last sentence, and §8. The two paths: one facet's versions highest first (the current version commit compares against), and the anam-events in a seq range (replay since a watermark). The timeline in fact-time order, a type filter and changes_since are indexes in `tracking.db`.

The code disagrees in one place: every writable open creates `anam_event_by_facet` on `(facet_id, seq)` for the retraction check (card 65), a secondary index in the ledger file. See [the ledger has one secondary index](/gaps/ledger-has-one-secondary-index.md).
