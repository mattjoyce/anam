---
type: invariant
title: Sort keys are identity-first, and integers are typed
description: An artifact's key carries facet_id then artifact_version, never fact_time; version and seq are INTEGER columns so they order numerically; times are fixed-width UTC text.
timestamp: 2026-10-11T00:00:00Z
resource: internal/store/ledger/ledger.sql#facet
anchor: "PRIMARY KEY (facet_id, artifact_version)"
basis: code
tier: supporting
enforced_at: internal/store/ledger/ledger.sql#facet
enforced_anchor: "PRIMARY KEY (facet_id, artifact_version)"
applies_to:
  - /components/store-ledger.md
  - /data-structures/facet-artifact.md
  - /data-structures/anam-event.md
---

ARCHITECTURE §12, eighth bullet, and §8. Key an artifact on its fact-time and a corrected row whose fact-time moved lands at a fresh key: the prior-version lookup misses it, commit concludes "never seen", and writes v1 again, two live artifacts for one id. Identity-first makes it unrepresentable. A version key built as text orders `v10 < v9`; typed columns remove that.

§12 still says "every integer in a sort key is zero-padded", a holdover from the DynamoDB substrate; the schema comment says typed columns need no padding (see [architecture doc has stale sentences](/gaps/architecture-doc-stale-sentences.md)).
