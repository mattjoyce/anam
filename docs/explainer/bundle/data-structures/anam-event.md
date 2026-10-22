---
type: data-structure
title: Anam-event row
description: "One row per change in Anam's knowledge: a gapless seq, one of five kinds, the facet and artifact version it concerns, and anam-time."
timestamp: 2026-10-11T00:00:00Z
resource: internal/store/ledger/ledger.sql#anam_event
anchor: "seq                    INTEGER PRIMARY KEY CHECK (seq >= 1),"
basis: code
tier: supporting
group: ledger
references:
  - to: /data-structures/facet-artifact.md
    how: concerns
    card: "n:1"
---

| Column | Meaning |
|---|---|
| `seq` | INTEGER primary key; a trigger allows only `MAX(seq)+1`, so it is gapless and `W = MAX(seq)` |
| `kind` | `admitted`, `superseded`, `tombstoned`, `endorsed`, `rejected`; nothing else |
| `facet_id` | the facet it concerns |
| `anam_time` | when Anam's knowledge changed; one instant per batch, so non-unique, tie-broken by seq |
| `artifact_version` | the version in force after the event |
| `prior_artifact_version` | set for superseded only |

The gate writes admitted and superseded through `AdmitBatch` and tombstoned through `Mark`; endorsed and rejected have no writer yet. The index of these in the operational store, `anam_event_index`, is what `anam changes` reads, ordered `(anam_time, seq)`.
