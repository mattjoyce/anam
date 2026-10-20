---
type: data-structure
title: Edge index row
description: The operational store's derived index of each live edge facet's ends, one row per facet id on each side, written by catch-up from the edge's own payload and rebuilt by replaying the ledger.
timestamp: 2026-10-21T00:00:00Z
resource: internal/store/ops/ops.go#facet_edge
anchor: "CREATE TABLE IF NOT EXISTS facet_edge ("
basis: code
tier: detail
group: index
references:
  - to: /data-structures/facet-artifact.md
    how: indexes the ends of
    card: "n:1"
  - to: /data-structures/facet-index-row.md
    how: joins for the edge's fact-time
    card: "n:1"
---

| Column | Meaning |
|---|---|
| `edge_id` | the edge facet's id, a `record.note` whose payload joins facets (card 104) |
| `kind` | what the join means, as the edge's `kind` says it; `about` when it says none |
| `side` | `from` or `to`: which list of the payload the facet id came from; `about`, the v1 name, is the `to` side |
| `facet_id` | one facet id the edge names on that side |

The primary key is `(edge_id, side, facet_id)`, and an index by `facet_id` answers "the live edges touching X" in one lookup. Catch-up writes the rows in the same transaction as the facet index row and the cursor: an edge's rows are replaced whole at each of its versions and gone when it is tombstoned, so a crash leaves the index behind, never wrong, and a store replayed from genesis holds the same rows as one caught up batch by batch. The query surface needs no table: the bundled `linked/2` derives the same relation from the served payloads.

Read by `GET /v1/receipts/{hash}/facets/{facet_id}`, which answers the entry as the receipt served it beside the edges touching it at the index's own watermark.
