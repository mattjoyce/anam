---
type: component
title: Admission gate
description: "The only ingress: resolve binds an envelope to the person, commit validates it against its Definition, composes identity, and appends through the ledger idempotently."
timestamp: 2026-10-17T00:00:00Z
resource: internal/gate/gate.go#New
anchor: "func New(l Ledger, defs Definitions, scopes TokenScopes, clock Clock, personID string, limits Limits) *Gate {"
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

What commit computes, per envelope: the [source ref](/terms/source-ref.md) from the Definition's `identity_key` (refusing one the envelope carried that disagrees), the `facet_id` as uuidv5 of it, the fact-time promoted out of the payload, the series stamp, the content hash of the canonical payload. Then the outcome: no prior version is `admitted`; a prior version with the same hash is `no-op`; a different hash is `superseded` with a new artifact version. A batch of up to `MaxBatch` (49) envelopes is one transaction, so a refused envelope writes nothing for the whole batch. `MaxBatch` is a constant, not a config key: every importer cuts its batches to it, so a gate set lower would refuse their full batches, and it decides which events share an anam-time (card 151). The payload bound and the retry count are the two `gate.Limits` the daemon passes in from the config (`max_payload_bytes`, 300 KiB; `commit_retries`, 5); see [config limits](/data-structures/limits.md).

The retry on a stale watermark lives here, not in the store, because a worker may already crash mid-batch and be re-run to known-good. `Retract` writes a tombstone event against the current version and no artifact. `Rehearse` and `RenameBatch` (card 140) carry a facet into another type as its next version: the prepared envelope is marked with the type it leaves, nothing on the wire can set the mark, and the disposition supersedes on a type change only under it, with the payload unchanged or not; every other path keeps the refusal of a changed type (card 105).

The gate is half of the [error kernel](/terms/error-kernel.md); the other half is the [ledger store](/components/store-ledger.md).
