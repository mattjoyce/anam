---
type: data-structure
title: Checkpoint log
description: "The operational store's checkpoint, mint_run, mint_seal and checkpoint_size tables: one row per mint of a bucket, the runs that checked every bucket, and the seal that proves no crashed run left rows behind."
timestamp: 2026-10-11T00:00:00Z
resource: internal/store/ops/checkpoint.go#CheckpointRow
anchor: "type CheckpointRow struct {"
basis: code
tier: supporting
group: index
references:
  - to: /data-structures/checkpoint-value.md
    how: names by hash
    card: "1:1"
---

| Table | Columns | Meaning |
|---|---|---|
| `checkpoint` | `span_id, v, covers_w, value_hash, minted_at`; PK (span_id, v) | one row per mint of a bucket; `span_head` is the view of the latest v per span |
| `mint_run` | `w, ran_at` | a run that checked every bucket at W; a read starts from the last one at or below its W |
| `mint_seal` | `checkpoints, w` | the row count after a run; the log is sealed when the max seal equals `COUNT(*)` of checkpoint |
| `checkpoint_size` | `span_id, v, facets` | the count, so an undecoded bucket can still be counted |

All derived: replay the ledger and mint again. The layout (which years are split into months, which months into days) is read from the ids, never stored. `meta` beside them pins `schema_version`, `bucket_width` and `ledger_id`.
