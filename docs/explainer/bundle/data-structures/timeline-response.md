---
type: data-structure
title: Timeline response
description: "What GET /v1/timeline answers: the watermark, counts, the whole-person hash, the shown entries with their versions, and verdicts when a projection or explain was asked for."
timestamp: 2026-10-11T00:00:00Z
resource: internal/api/timeline.go#Timeline
anchor: "type Timeline struct {"
basis: code
tier: detail
group: wire
references:
  - to: /data-structures/fold-entry.md
    how: lists
    card: "1:n"
---

| Field | Meaning |
|---|---|
| `w` | the watermark the read ran at |
| `live`, `shown` | live facets in the whole value, and how many the window and type kept |
| `canonical` | the whole-person hash at W, not the window's |
| `as_of`, `at`, `at_hash` | the knowledge and fact-time points named, and the snapshot's hash |
| `projection` | the template served through, if any |
| `entries` | fold entries, in fact-time order |
| `versions` | artifact version per shown facet |
| `verdicts`, `counts` | with `--explain` or a projection |

Over the daemon the encoded size is measured first; a read over `max_read_bytes` is a 413 carrying `too_large: {matched, bytes, max_bytes, narrow[]}`.
