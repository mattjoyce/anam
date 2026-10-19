---
type: invariant
title: Metadata and payload are separated; the payload is opaque
description: Anam governs and projections filter on the facet's attributes and descriptor; the payload is verbatim, immutable, stored as bytes and never parsed into a canonical schema.
timestamp: 2026-10-11T00:00:00Z
resource: internal/store/ledger/schema.go#schema
anchor: "payload            BLOB    NOT NULL,"
basis: code
tier: supporting
enforced_at: internal/store/ledger/schema.go#schema
enforced_anchor: "payload            BLOB    NOT NULL,"
applies_to:
  - /components/store-ledger.md
  - /components/projection.md
  - /components/gate.md
  - /terms/facet.md
---

ARCHITECTURE §12, tenth bullet, and §3. The payload column is a BLOB, never a JSON column. The gate reads a payload to validate it, to extract fact-time, series and identity, and to canonicalise it for the hash, and writes it unaltered: a number the source wrote as text stays text. A projection's verdict reads `fold.Entry` fields only, and the consent request has no payload field by design. The ledger is datom-shaped with a fat opaque value, not EAVT: there is no `systolic` attribute in the store to ask for, and nothing is indexed by field. This is a storage rule, not a ceiling on reads: fields, and the [query](/terms/query.md) card 173 decided, read payload fields from the served copy after a projection has decided what is served ([a read may select by payload; the store never does](/decisions/a-read-may-select-by-payload-the-store-never-does.md)).
