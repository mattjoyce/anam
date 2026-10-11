---
type: decision
title: "The substrate is accidental: only internal/store names SQLite"
description: The problem is a personal record, not a store; scripts/check.sh fails the build if database/sql or sqlite is named outside internal/store, or a capture worker imports the store.
timestamp: 2026-10-11T00:00:00Z
resource: scripts/check.sh#boundary
anchor: "grep -vE '^github.com/mattjoyce/anam/internal/(store|capture)'"
basis: docs
tier: supporting
alternatives:
  - Let the domain packages query SQL directly (refused; the domain must not know what it is stored in, and the ledger asks only for ordered keys and one atomic multi-row write)
  - Forbid database/sql in capture too (refused; a source that is a SQLite file needs it to be read at all, and that is the source's substrate, not Anam's)
affects:
  - /components/store-ledger.md
  - /components/store-ops.md
  - /components/capture-healthdb.md
  - /components/facet.md
---

**Context.** AGENTS.md's design lens: Hickey on what is complected. The ledger moved from DynamoDB to SQLite once already (journal, 2026-09-28) behind the same interface.

**Choice.** The row mapping is quarantined to the store package; a row is not a domain type. The one carve-out is a capture worker reading a source's own SQLite file, which may never import `internal/store`.

**Why.** Because Anam owns no source fact, a change of substrate can be a re-ingest; the anam-times are what would not come back, so a ledger whose history matters is moved by copying rows.

**What it costs.** A mechanical grep rather than a type-level guarantee; and the words "store" and "database" are easy to say in a package that should not.
