---
id: 202607100007
type: decision
title: "Payloads are held verbatim; metadata and payload stay separate"
created: 2026-07-10
status: evergreen
tags: [anam, decision, facet, core, hickey]
project: anam
related: ["[[the-inversion]]", "[[facet-definition-format]]", "[[guiding-lens-hickey-armstrong]]"]
---

# Payloads are held verbatim; metadata and payload stay separate

**Decision.** A Facet holds its source fragment **verbatim** — stored as opaque bytes, never parsed into
a canonical schema. Governance and projections filter on the Facet's *metadata* (attributes +
descriptor); they never touch the *payload*. This is inherited from Arch §3 and is load-bearing across
Anam.

## Why it matters

- **It's what makes "Anam owns no fact" true.** If the body is held exactly as the source emitted it,
  Anam can always rebuild from source and never becomes a second, subtly-different original. This is the
  invariant that lets [[the-inversion|the whole-person read]] be safe without a span filter.
- **It avoids the "forever schema" trap.** No canonical clinical/health model to maintain as sources
  evolve; Anam registers source schemas as it meets them and stays out of their business.
- **Hickey alignment.** The payload is a *value* — immutable, comparable by content hash. Current status
  is never written onto it; status is a [[guiding-lens-hickey-armstrong|fold]] over record-events. One
  truth per fact; no place overwritten.

## Enforcement

Enforced at the storage layer: the payload is a DynamoDB **Binary** attribute, never a Map. The
single ingest-time inspection ([[facet-definition-format|the definition]]) reads only the few fields it
must (identity, event/recorded time) via JSON Pointer; after admission the body is opaque.

## Consequence for access control

Because payloads are whole and opaque, a projection includes or excludes **whole Facets** — it cannot
redact a field. Finer access control is achieved by *decomposing a finer Facet*, not by reaching into
the body.
