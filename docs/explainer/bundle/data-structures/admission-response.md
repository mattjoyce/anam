---
type: data-structure
title: Admission response
description: "What POST /v1/envelopes answers: w, changed, one positional result per envelope, and snapshot progress; or a 422 naming every refused envelope."
timestamp: 2026-10-11T00:00:00Z
resource: internal/api/importers.go#Admission
anchor: "type Admission struct {"
basis: code
tier: supporting
group: wire
references:
  - to: /data-structures/batch-submission.md
    how: answers
    card: "1:1"
---

| Status | Field | Meaning |
|---|---|---|
| 200 | `w` | the watermark after the batch, or the head read when every envelope was a no-op |
| 200 | `changed` | admitted plus superseded |
| 200 | `results[]` | `{index, outcome, facet_id, artifact_version}`, outcome `admitted`, `superseded` or `no-op` |
| 200 | `snapshot` | `{id, parts, parts_admitted, complete}` when the batch was a part |
| 422 | `error`, `refused[]` | `{index, type, reason}` for every failure; nothing written |
| 400 | `error` | not one submission |
| 409 | `error` | snapshot part mismatch or changed |
| 413 | `error` | body over the bound |
| 503 | `error`, `Retry-After: 15` | another write held the queue past 5 s |

The importer's client accepts a 200 only when `w` is a positive integer, `changed` equals the count it can see, and there is exactly one result per envelope with a known outcome; anything less is retried as a transport fault.
