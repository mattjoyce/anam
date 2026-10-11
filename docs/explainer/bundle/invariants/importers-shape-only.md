---
type: invariant
title: An importer shapes payloads and nothing else
description: No identity, no joins, no summaries, no state of the record, no token; an envelope names no source and no source_ref.
timestamp: 2026-10-11T00:00:00Z
resource: ../anam-importers/anam_importer/envelopes.py#envelope
anchor: "It names no source and no"
basis: docs
tier: supporting
enforced_at: ../anam-importers/anam_importer/envelopes.py#envelope
enforced_anchor: "It names no source and no"
applies_to:
  - /components/importer-base.md
  - /components/importer-bp-csv.md
  - /components/importer-dreamstation.md
  - /components/importer-records.md
---

docs/IMPORTERS.md §2 and §8.5 item 3; anam-importers/AGENTS.md. The drift checks fall out of the lineage: ductile holding schemas or derived data is Senechal growing back on one side, an importer joining or summarising is it growing back on the other. The base enforces the envelope's shape; the rest is review. Importers do write the payload fields the identity pointers name (an ordinal, an id), which the contract's wording does not settle (see [does writing an id field compose identity](/gaps/importers-write-identity-fields.md)).
