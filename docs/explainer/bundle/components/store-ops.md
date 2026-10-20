---
type: component
title: Operational store (internal/store/ops)
description: "The tracking.db SQLite file: Definitions, the facet and event indexes, the checkpoint log, mint runs and seals, snapshot parts, and the pins that refuse a mismatched ledger or width."
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

Each table is annotated with its tier in the schema.

| Tier | Tables | Comes back from |
|---|---|---|
| derived | `facet_index`, `facet_edge`, `anam_event_index`, `checkpoint` (+ `span_head` view), `checkpoint_size`, `mint_run`, `mint_seal`, `sync` | replaying the ledger, then minting |
| provisioned | `definition` (bundled, every version), `registered_definition` (immutable by trigger) | the binary, and each importer registering again |
| authored | `meta` (schema_version, bucket_width, ledger_id) | the file itself |
| disposable | `snapshot`, `snapshot_part`, `snapshot_facet` | the importer's next resend |

`Open` applies the schema, pins the bucket width and refuses a file built at another, and `PinLedger` refuses an index derived from another ledger. `OpenReadOnly` is what the daemon's read routes use. The repair is to delete the file and replay, which cannot reach a fact.

Data structures here: [checkpoint row](/data-structures/checkpoint-row.md), [facet index row](/data-structures/facet-index-row.md), [edge index row](/data-structures/facet-edge-row.md), [snapshot part](/data-structures/snapshot-part.md).
