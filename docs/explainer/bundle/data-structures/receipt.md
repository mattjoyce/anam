---
type: data-structure
title: Receipt
description: "Proof of what a projection served: the served hash, the facet ids, the watermark and ledger id, the template and definition-set versions, served_at, the span, and the query a read narrowed by."
timestamp: 2026-10-11T00:00:00Z
resource: internal/receipt/receipt.go#Receipt
anchor: "type Receipt struct {"
basis: code
tier: supporting
group: published
references:
  - to: /data-structures/fold-entry.md
    how: pins the ids of
    card: "1:n"
  - to: /data-structures/projection-template.md
    how: pruned by
    card: "n:1"
---

| JSON | Type | Meaning |
|---|---|---|
| `served_hash` | string | hash of the served value's bytes |
| `facet_ids` | []string | in served order; an empty list for an empty serve |
| `watermark` | int64 | the W the derivation ran at |
| `ledger_id` | string | the ledger that W counts in; empty on receipts from before 2026-10-07, which verify with a note |
| `template`, `template_version` | string, int | the exact pruning; a moved version refuses verification |
| `definition_set_version` | string | hash of the bundled manifests, provenance only |
| `served_at` | string | the clock layout |
| `span` | string | `whole-person` or `fact-time[from,to)` |
| `query` | object, optional | `{from, before, at, type}` a read narrowed by after the template; nil for a whole serve |

Written as indented JSON to `receipt/sha256-<hex>.json`; parsed with unknown fields refused. Verification reports two independent proofs: reproduced (re-derived hash and id set match) and intact (the served file still hashes to `served_hash`).
