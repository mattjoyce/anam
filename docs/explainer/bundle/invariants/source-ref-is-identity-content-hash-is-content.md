---
type: invariant
title: source_ref is identity; content_hash is content
description: The natural key never contains a row's values, or supersession silently becomes duplication; the gate composes identity from identity_key and refuses an envelope that disagrees.
timestamp: 2026-10-11T00:00:00Z
resource: internal/gate/commit.go#prepare
anchor: "ErrIdentityMismatch, env.SourceRef, ref)"
basis: code
tier: supporting
enforced_at: internal/gate/commit.go#prepare
enforced_anchor: "ErrIdentityMismatch, env.SourceRef, ref)"
applies_to:
  - /components/gate.md
  - /components/def.md
  - /terms/source-ref.md
  - /terms/supersession.md
  - /terms/idempotency.md
tested_by:
  - internal/gate/gate_test.go#TestCommit_TypedRejections
  - internal/gate/batch_test.go#TestCommitBatch_SupersedesAlongsideNoOps
---

ARCHITECTURE §12, seventh bullet, and §6a. `facet_id = uuidv5(source_ref)`, and `source_ref` is the source name plus each identity value length-prefixed. The same source row always maps to the same facet; a changed row hashes differently and supersedes; an unchanged row is a no-op. Braiding a value into the key would give a corrected reading a different id, so it would enter as a new facet and orphan the original.

Enforced at the gate: an envelope may leave the ref out, and one that carries a ref other than the composed one is refused. Keyed by the Definition: a new version may not change `identity_key`.
