---
type: component
title: Operational store (internal/store/ops)
description: "The tracking.db SQLite file: Definitions, the facet and event indexes, the checkpoint log, mint runs and seals, snapshot parts, the operator's admission holds, and the pins that refuse a mismatched ledger or width."
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
| provisioned | `definition` (bundled, every version), `registered_definition` (immutable by trigger) | the binary, and each importer registering again; `anam migrate` copies the registered rows to a new store with their registration moments (card 200) |
| authored | `meta` (schema_version, bucket_width, ledger_id), `registered_projection` (immutable by trigger), `admission_hold` (operator admission holds; immutable by trigger) | the file itself, the operator's own registrations and holds |
| disposable | `snapshot`, `snapshot_part`, `snapshot_facet` | the importer's next resend |

`Open` applies the schema, pins the bucket width and refuses a file built at another, and `PinLedger` refuses an index derived from another ledger. `OpenReadOnly` is what the daemon's read routes use. The repair is to delete the file and replay, which cannot reach a fact.

`admission_hold` holds a source's admission at the daemon's door (`POST /v1/hold`): a held source's envelopes and Definition registrations are refused, and the workbench is never held. A held row is not rewritten; releasing it deletes it. It is authored state, so `anam migrate` carries it into the new store like the registered rows.

Data structures here: [checkpoint row](/data-structures/checkpoint-row.md), [facet index row](/data-structures/facet-index-row.md), [edge index row](/data-structures/facet-edge-row.md), [snapshot part](/data-structures/snapshot-part.md).
