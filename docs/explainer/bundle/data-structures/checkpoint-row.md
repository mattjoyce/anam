---
type: data-structure
title: Checkpoint log
description: "The operational store's checkpoint, mint_run and checkpoint_size tables: one row per mint of a bucket, the runs that checked every bucket with a digest of the checkpoints they took, and the sizes."
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
| `mint_run` | `w, ran_at, picks` | a run that checked every bucket at W; a read starts from the last one at or below its W. `picks` is a digest of the checkpoints the run took (span, version, value hash): a read that cannot take the same ones declines and folds in full |
| `checkpoint_size` | `span_id, v, facets` | the count, so an undecoded bucket can still be counted |

All derived, and a cache that may go missing: a mint removes the rows the newest run does not read, and a read that finds one gone folds past it. Replay the ledger and mint again. The layout (which years are split into months, which months into days) is read from the ids, never stored. `meta` beside them pins `schema_version`, `bucket_width` and `ledger_id`.
