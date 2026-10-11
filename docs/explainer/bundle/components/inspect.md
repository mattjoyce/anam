---
type: component
title: Inspector (internal/inspect)
description: "The read-only web harness over the full fold: timeline, facets, events, projections and definitions, plus the default library's bytes confined to its root."
timestamp: 2026-10-11T00:00:00Z
resource: internal/inspect/inspect.go#Handler
anchor: "func Handler(l Ledger, timeout time.Duration, buckets span.Buckets, library string, defs func() *def.Registry,"
basis: code
tier: detail
group: tools
depends_on:
  - /components/fold.md
  - /components/projection.md
  - /components/def.md
  - /components/span.md
  - /components/store-ledger.md
---

A diagnostic tool, not a shipped UI, and it keeps the full fold rather than reading from checkpoints. Routes: `/`, `/facets`, `/facet`, `/events`, `/projection`, `/document?path=`, `/definitions`, `/definition`. `/document` opens the library root with `os.OpenRoot`, so a path that climbs out or a symlink that points out gets the same 404 as a missing file. Mounted inside the daemon behind the operator token, or standalone with `anam inspect`.
