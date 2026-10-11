---
type: data-structure
title: Fold entry
description: "One live facet in a canonical value: its governing metadata, derived status and payload, with no artifact version and no watermark so the hash is stable across rebuilds."
timestamp: 2026-10-11T00:00:00Z
resource: internal/fold/value.go#Entry
anchor: "type Entry struct {"
basis: code
tier: supporting
group: ledger
references:
  - to: /data-structures/facet-artifact.md
    how: is the live version of
    card: "1:1"
---

| JSON | From |
|---|---|
| `facet_id`, `type`, `fact_time` | the artifact |
| `series` | the artifact; omitted when empty so old hashes do not move |
| `source`, `source_ref` | provenance |
| `status` | the folded state: admitted or endorsed in a value |
| `classification`, `content_hash` | the artifact |
| `payload` | the raw canonical bytes |

Deliberately absent: the artifact version (a superseded chain collapses) and seq or W. A `Value` is entries sorted by fact-time then facet id, serialised once, hashed once; checkpoint values, served values and the timeline all carry these.
