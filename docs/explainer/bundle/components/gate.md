---
type: component
title: Admission gate
description: "The only ingress: resolve binds an envelope to the person, commit validates it against its Definition, composes identity, and appends through the ledger idempotently."
timestamp: 2026-10-11T00:00:00Z
resource: internal/gate/gate.go#New
anchor: "func New(l Ledger, defs Definitions, scopes TokenScopes, clock Clock, personID string) *Gate {"
basis: code
tier: core
group: kernel
depends_on:
  - /components/store-ledger.md
  - /components/def.md
  - /components/facet.md
  - /components/fold.md
---

Two calls, as the doctrine says: `Resolve` enforces association (binding and its assurance), `CommitBatch` enforces facet-compliance and token scope and writes.

| Pillar | Where |
|---|---|
| Facet-compliant | the Definition's closed JSON Schema, `validation_mode`, fact-time pointer and zone, series pointers |
| Authenticated | the token scopes: a source may inject only the types it is scoped for |
| Associated | binding to the one person, with assurance high / medium / review |

What commit computes, per envelope: the [source ref](/terms/source-ref.md) from the Definition's `identity_key` (refusing one the envelope carried that disagrees), the `facet_id` as uuidv5 of it, the fact-time promoted out of the payload, the series stamp, the content hash of the canonical payload. Then the outcome: no prior version is `admitted`; a prior version with the same hash is `no-op`; a different hash is `superseded` with a new artifact version. A batch of up to `MaxBatch` (49) envelopes is one transaction, so a refused envelope writes nothing for the whole batch.

The retry on a stale watermark lives here, not in the store, because a worker may already crash mid-batch and be re-run to known-good. `Retract` writes a tombstone event against the current version and no artifact.

The gate is half of the [error kernel](/terms/error-kernel.md); the other half is the [ledger store](/components/store-ledger.md).
