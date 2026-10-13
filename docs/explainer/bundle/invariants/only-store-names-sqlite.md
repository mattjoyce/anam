---
type: invariant
title: Only internal/store may name SQLite or database/sql
description: The substrate is accidental; scripts/check.sh fails the build if database/sql or sqlite is imported outside internal/store, with internal/capture allowed to read a source in its native format.
timestamp: 2026-10-11T00:00:00Z
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

AGENTS.md, design lens, and ARCHITECTURE §8. The carve-out and the rule are two greps in the pre-merge gate: nothing but `internal/store` and `internal/capture` may import the SQL driver, and nothing in `internal/capture` may import `internal/store`. See [the substrate is accidental](/decisions/the-substrate-is-accidental.md).
