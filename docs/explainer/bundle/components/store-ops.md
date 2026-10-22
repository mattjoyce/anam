---
type: component
title: Operational store (internal/store/ops)
description: "The tracking.db SQLite file: Definitions, the facet and event indexes, the checkpoint log and its values, receipts and saved selections, mint runs, snapshot parts, and the pins that refuse a mismatched ledger or width."
timestamp: 2026-10-11T00:00:00Z
resource: internal/store/ops/ops.go#Open
anchor: "func Open(ctx context.Context, path string, buckets span.Buckets) (*DB, error) {"
basis: code
tier: supporting
group: derived
depends_on:
  - /components/fold.md
  - /components/span.md
  - /components/facet.md
---

Each table is annotated with its tier in the schema, `internal/store/ops/ops.sql`, which is embedded.

| Tier | Tables | Comes back from |
|---|---|---|
| derived | `facet_index`, `facet_edge`, `anam_event_index`, `checkpoint` (+ `span_head` view), `checkpoint_value`, `checkpoint_size`, `mint_run` (with its digest of the checkpoints a read takes), `sync` | replaying the ledger, then minting |
| provisioned | `definition` (bundled, every version), `registered_definition` (immutable by trigger) | the binary, and each importer registering again; `anam migrate` copies the registered rows to a new store with their registration moments (card 200) |
| authored | `registered_projection`, `receipt`, `served_value`, `saved_selection`, `saved_selection_code`, `inline_projection` (immutable by trigger), `meta` (schema_version, bucket_width, ledger_id) | a backup, or nowhere |
| disposable | `snapshot`, `snapshot_part`, `snapshot_facet` | the importer's next resend |

`Open` applies the schema, pins the bucket width and refuses a file built at another, and `PinLedger` refuses an index derived from another ledger. `OpenReadOnly` is what the daemon's read routes use. The content-addressed tables are filed and read through `Put`, `Get`, `Raw`, `Size` and `List` by kind (see [stored content is a row keyed by its hash](/decisions/content-is-a-row-keyed-by-its-hash.md)), `RunCheckpoints` takes a run's checkpoints and says when the log no longer yields the ones the run saw, and `CollectCheckpoints` ends a mint by removing the checkpoints the newest run does not read. Checkpoints are a cache. The repair is to delete the file and replay, which cannot reach a fact.

Data structures here: [checkpoint row](/data-structures/checkpoint-row.md), [facet index row](/data-structures/facet-index-row.md), [edge index row](/data-structures/facet-edge-row.md), [snapshot part](/data-structures/snapshot-part.md).
