---
type: invariant
title: Anam owns no source fact
description: No captured payload exists only inside Anam; every one is verbatim from its source and rebuildable, so deleting Anam loses no fact. Anam does own the continuity.
timestamp: 2026-10-11T00:00:00Z
resource: docs/ARCHITECTURE.md#Purpose and boundary
anchor: "> **Anam owns no source fact, and no captured payload exists only inside it"
basis: docs
tier: core
enforced_at: not enforced
applies_to:
  - /components/gate.md
  - /components/store-ledger.md
  - /terms/facet.md
tested_by:
  - internal/fold/rebuild_test.go#TestRebuildFromSource_IdenticalCanonical
---

The one invariant every other decision serves (ARCHITECTURE §1 and §12). If Anam were deleted tomorrow, every payload rebuilds from its source: the gate holds the payload opaque, the ledger stores it as the bytes it arrived as, and nothing in Anam ever authors a source fact.

What is not rebuildable is named just as plainly: the continuity. The composed, ordered, whole-life account exists nowhere upstream, so the ledger's order, its watermarks and its checkpoints are backed up rather than assumed rebuildable. The honest sentence is "Anam owns no fact, and Anam owns the continuity."

There is no mechanism that enforces this; it is a property of never parsing a payload and never writing back. The rebuild test is the check: re-ingesting the sources into a fresh ledger reaches the same canonical value.
