---
type: invariant
title: The derived tier must stay provably rebuildable from the ledger
description: The ledger is the system of record; the library's files hold documents; the operational store's derived tables come back by replay and must never acquire a second home for a payload, an event or W.
timestamp: 2026-10-11T00:00:00Z
resource: internal/store/ops/derived.go#ApplyReplay
anchor: "return fmt.Errorf(\"ops: replay cursor moved since it was read at %d\", fromSeq)"
basis: docs
tier: supporting
enforced_at: internal/catchup/catchup.go#Sync
enforced_anchor: "case int64(head) < last:"
applies_to:
  - /components/store-ops.md
  - /components/catchup.md
  - /components/store-ledger.md
  - /terms/ledger.md
tested_by:
  - internal/fold/rebuild_test.go#TestRebuildFromSource_IdenticalCanonical
  - internal/catchup/catchup_test.go#TestSync_IncrementalAndIdempotent
---

ARCHITECTURE §12, fourth bullet, and §8. The rebuild test drops the derived tier and re-derives it from the ledger; if a facet payload, an anam-event or W ever acquired a second home there, the test would pass on data it never rebuilt and "Anam owns no source fact" would stop being falsifiable.

What enforces it in code: the replay cursor is a position, not a watermark, and moves only in the same transaction as the rows it accounts for; an index ahead of the ledger is refused at daemon start; the index holds a facet's folded state and hash, never its payload.
