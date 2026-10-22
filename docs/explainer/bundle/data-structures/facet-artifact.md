---
type: data-structure
title: Facet artifact row
description: "One row per artifact version in the ledger's facet table: the governing metadata as columns, the payload as an opaque BLOB, and no status column."
timestamp: 2026-10-11T00:00:00Z
resource: internal/store/ledger/ledger.sql#facet
anchor: "PRIMARY KEY (facet_id, artifact_version)"
basis: code
tier: supporting
group: ledger
references:
  - to: /data-structures/definition-manifest.md
    how: admitted under
    card: "n:1"
---

| Column | Meaning |
|---|---|
| `facet_id` | uuidv5 of the source ref; identity, never fact-time |
| `artifact_version` | INTEGER, orders numerically; version n needs n-1 beneath it (trigger) |
| `type`, `confidence`, `classification`, `definition_version` | attributes about itself |
| `fact_time` | fixed-width UTC text, promoted out of the payload at the gate |
| `series` | the Definition's series stamp, empty when the type has none |
| `binding_person_id`, `binding_assurance`, `binding_span_hint` | the association pillar's result |
| `prov_source`, `prov_injector`, `prov_source_ref`, `prov_method`, `prov_model`, `prov_watermark` | provenance; the derivation fields are empty for a captured fact |
| `source_schema`, `schema_version`, `content_hash`, `payload_kind`, `media_type` | the payload descriptor |
| `payload` | BLOB, the canonical bytes; never a JSON column |

Triggers refuse update, delete and an insert over an existing key. The Go value is `facet.Facet`, built only by `facet.New`, which re-derives the id and hash from the bytes so a facet cannot disagree with itself. There is no status column; a test pins that the struct has no status field. `ledger_meta` beside it pins `person_id`, `ledger_id`, `schema_version` and each `schema_migration:<n>`, write-once.
