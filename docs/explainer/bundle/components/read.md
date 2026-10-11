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

`Source` is the four handles a read needs: ledger, checkpoints, artifacts, Definitions. `Timeline` chooses: a bare window with checkpoints to hand takes `mint.ReadWindow`; a projection or `--at` takes the whole value; `--explain` drops the checkpoints and folds in full so it can list displaced versions. `Absent` computes what a complete resend did not carry. `FitTimeline` measures the wire encoding and refuses a read over `max_read_bytes` with the parameters that would narrow it. `Publish` is the daemon's receipted read.

Traced in [read the timeline](/workflows/read-the-timeline.md).
