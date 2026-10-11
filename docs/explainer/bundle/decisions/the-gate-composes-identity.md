---
type: decision
title: The gate composes identity from the Definition
description: An envelope may leave source_ref out; the gate composes it from identity_key through one length-prefixed function and refuses one that disagrees, so importers shape payloads and nothing else.
timestamp: 2026-10-11T00:00:00Z
resource: internal/def/check.go#SourceRef
anchor: 'return compose("identity", source, d.IdentityKey, doc)'
basis: docs
tier: supporting
tested_by:
  - internal/gate/gate_test.go#TestCommit_ComposesASourceRefLeftOut
  - internal/facet/id_test.go#TestSourceRef_DistinctTuplesNeverCollide
alternatives:
  - Each capture worker composes its own source_ref (how the built-in vendor workers drifted; they are gone or no longer compose it)
  - Join the identity values with a plain separator (refused; a value carrying the separator, a timestamp's colons, could make two tuples one string)
affects:
  - /components/gate.md
  - /components/def.md
  - /components/importer-base.md
  - /terms/source-ref.md
---

**Context.** Card 51, the first prerequisite for importers in any language without importing Anam's code. The identity rule has to live once.

**Choice.** `facet.SourceRef` writes the source name then each identity value as `<bytes>:<value>`. The gate resolves the Definition's `identity_key` pointers against the decoded payload, in declared order, and composes it. An envelope that carries a different one is `ErrIdentityMismatch`. The facet id is uuidv5 of the result under a fixed namespace.

**Why.** Identity decides which fact; the content hash decides whether it changed. Braiding values into the key would give a corrected reading a new facet id, orphaning the original instead of superseding it, and silently disabling idempotency for that source.

**What it costs.** An importer still writes the payload fields the pointers name (bp_csv's ordinal `reading`, garmin's `id`), which is where "compose no identity" has an edge the contract does not settle (see [does writing an id field compose identity](/gaps/importers-write-identity-fields.md)).
