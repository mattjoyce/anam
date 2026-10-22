---
type: term
title: Operational store
description: The second SQLite file, authoritative for what Anam authors, with each table annotated by where it comes back from.
timestamp: 2026-10-11T00:00:00Z
---

The operational store (`tracking.db`) holds Anam's own state: Definitions, projection templates, token scopes, the facet and anam-event indexes, the checkpoint log, mint runs, receipts, snapshot parts, and the operator's admission holds. It is a real store, not a cache. Each table has a tier: derived (replay the [ledger](/terms/ledger.md)), provisioned (embedded assets, config, importers registering again), authored (back it up).

Only the derived tier is disposable, and it must stay provably derived: the repair is to delete the file and replay, which cannot reach a fact. It pins the ledger's id and the bucket width it was built at and refuses a mismatch. The component is [ops store](/components/store-ops.md).
