---
type: data-structure
title: Snapshot tables
description: "The operational store's record of an importer's resend-everything run: the run with its started and completed watermarks, each part's digest, and the facet ids the parts carried."
timestamp: 2026-10-11T00:00:00Z
resource: internal/store/ops/ops.sql#snapshot
anchor: "CREATE TABLE IF NOT EXISTS snapshot ("
basis: code
tier: supporting
group: index
references:
  - to: /data-structures/facet-artifact.md
    how: carried
    card: "n:n"
---

| Table | Columns | Meaning |
|---|---|---|
| `snapshot` | `id, source, types, parts, started_at, started_w, completed_at, completed_w` | one run; `started_w` is the head after its first part, `completed_at` set by the part that makes `parts` |
| `snapshot_part` | `id, part, digest` | the sha256 over the part's sorted envelope lines, so a part resent with other envelopes is refused (409) |
| `snapshot_facet` | `id, facet_id` | every id a part carried, no-ops included |

Tier disposable (a fourth word beside derived, provisioned and authored): incomplete runs older than `snapshot_ttl` (a day) are dropped on the next part, a source keeps its `snapshot_keep` (two) newest complete runs, and a lost table comes back with the next resend. `GET /v1/snapshots/{id}` reports complete, parts_admitted, carried and, once complete, live, absent and up to 1,000 absent ids.
