---
type: gap
title: The ledger has one secondary index
description: §12 says the ledger carries no secondary indexes; every writable open creates anam_event_by_facet on (facet_id, seq) for the per-facet history the retraction check needs.
timestamp: 2026-10-11T00:00:00Z
looked_at:
  - internal/store/ledger/ledger.go
  - internal/store/ledger/read.go
  - internal/store/ledger/facetevents_test.go
  - docs/ARCHITECTURE.md
---

Card 65 added `FacetEvents` and the index so a retraction check costs that facet's events, not a replay. It is not schema-versioned and a read-only open does not create it. The invariant's reason (an index is derived and belongs in the operational store) still holds for the timeline and type filter; this one index serves the kernel's own write path. The doc has not been updated to say so. See [the ledger carries no secondary indexes](/invariants/ledger-carries-no-secondary-indexes.md).
