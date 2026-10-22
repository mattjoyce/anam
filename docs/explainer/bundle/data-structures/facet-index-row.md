---
type: data-structure
title: Facet index row
description: The operational store's derived index of each facet's current version, status, fact-time and bucket, written by catch-up and provably rebuildable by replaying the ledger.
timestamp: 2026-10-11T00:00:00Z
resource: internal/store/ops/ops.sql#facet_index
anchor: "CREATE TABLE IF NOT EXISTS facet_index ("
basis: code
tier: detail
group: index
references:
  - to: /data-structures/facet-artifact.md
    how: indexes
    card: "1:1"
---

| Column | Meaning |
|---|---|
| `facet_id` | primary key |
| `type`, `fact_time` | for the timeline and type filter the schema comment promises |
| `bucket_id` | arithmetic on fact-time at the pinned width; persisted, which is why changing the width is a rebuild |
| `current_version`, `status` | exactly a persisted `fold.State` |
| `content_hash` | the current version's |

Beside it `anam_event_index` (`seq, anam_time, kind, facet_id, artifact_version`, indexed `(anam_time, seq)`) and `replay_cursor` (`last_seq`, a singleton moved conditionally in the same transaction as the rows). The timeline read in this tree composes from checkpoints and never queries this table; only `anam changes` reads the index (see [facet_index is written but not read](/gaps/facet-index-written-not-read.md)).
