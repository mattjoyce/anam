---
type: component
title: Read (internal/read)
description: "The read API behind timeline, changes, publish and the snapshot report: parses the query, picks composed or full fold, narrows, and bounds the answer."
timestamp: 2026-10-11T00:00:00Z
resource: internal/read/timeline.go#Timeline
anchor: "func Timeline(ctx context.Context, src Source, q api.TimelineQuery) (api.Timeline, error) {"
basis: code
tier: supporting
group: derived
depends_on:
  - /components/mint.md
  - /components/fold.md
  - /components/projection.md
  - /components/serve.md
  - /components/def.md
  - /components/store-ledger.md
  - /components/store-ops.md
---

`Source` is the four handles a read needs: ledger, checkpoints, checkpoint values, Definitions. `Timeline` chooses: a bare window with checkpoints to hand takes `mint.ReadWindow`; a projection or `--at` takes the whole value; `--explain` drops the checkpoints and folds in full so it can list displaced versions. `Absent` computes what a complete resend did not carry. `FitTimeline` measures the wire encoding and refuses a read over `max_read_bytes` with the parameters that would narrow it. `Publish` is the daemon's receipted read; it files the whole value once and answers its first page, and `ReceiptPage` reads every page from the served value by a cursor it checks against the receipt (card 92). Every read narrows through one predicate, `receipt.Query.Admits`, the one a verify replays. `Query` runs a Datalog program through a projection with a receipt; its evaluation, `Evaluate`, is exported for the inspector's selection page (card 199), so the page and `POST /v1/query` answer one program alike within `max_read_bytes`.

Traced in [read the timeline](/workflows/read-the-timeline.md).
