---
type: component
title: Ledger store (internal/store/ledger)
description: "The append-only SQLite file: facet artifacts, anam-events and ledger_meta, with triggers that refuse update and delete for every writer."
timestamp: 2026-10-11T00:00:00Z
resource: internal/store/ledger/ledger.go#Open
anchor: "func Open(ctx context.Context, path, personID string) (*Ledger, error) {"
basis: code
tier: core
group: kernel
depends_on:
  - /components/facet.md
---

Three tables, three access paths, no secondary indexes.

| Table | Key | Holds |
|---|---|---|
| `facet` | (facet_id, artifact_version) | one [facet artifact](/data-structures/facet-artifact.md) per version, payload as a BLOB |
| `anam_event` | seq | one [anam-event](/data-structures/anam-event.md) per change in knowledge |
| `ledger_meta` | key | person_id, ledger_id, schema_version and each schema_migration, write-once |

The primitive is two calls, not one with an optional artifact: `Append` writes an artifact with its admitted or superseded event, `Mark` writes a status event alone. Both run in one transaction that checks, in order, that the head is the watermark the caller read (else `ErrStaleWatermark`), that this version is not already written (same bytes is `ErrAlreadyWritten`, different bytes is `ErrDivergentArtifact`), and that the insert is accepted (else `ErrCorrupt`). There is no watermark row: W is `MAX(seq)`, and a trigger lets an event take only the next seq.

`Create` is the one place the schema moves. `Open` refuses a file pinned to another person or at another schema version. `OpenReadOnly` opens with `mode=ro`, so SQLite refuses every write on that handle; every command that only reads uses it. The file runs in WAL mode.
