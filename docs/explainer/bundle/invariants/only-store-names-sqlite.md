---
type: invariant
title: Only internal/store may name SQLite or database/sql
description: The substrate is accidental; scripts/check.sh fails the build if database/sql or sqlite is imported outside internal/store, with no carve-out since card 119.
timestamp: 2026-10-14T00:00:00Z
resource: scripts/check.sh#boundary
anchor: "FAILED: the store substrate is named outside internal/store:"
basis: code
tier: supporting
enforced_at: scripts/check.sh#boundary
enforced_anchor: "FAILED: the store substrate is named outside internal/store:"
applies_to:
  - /components/store-ledger.md
  - /components/store-ops.md
  - /components/facet.md
---

AGENTS.md, design lens, and ARCHITECTURE §8. The rule is one grep in the pre-merge gate over `go list`: nothing but `internal/store` may import the SQL driver, in code or tests. Until card 119 `internal/capture` was allowed it too, to read a source that is a SQLite file; that package is gone, and a source in its native format is now an importer's business. See [the substrate is accidental](/decisions/the-substrate-is-accidental.md).
