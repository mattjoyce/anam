---
type: component
title: Buckets (internal/span)
description: "Bucket arithmetic over fact-time: the configurable width, self-describing ids, and the layout that subdivides a year into months and days."
timestamp: 2026-10-11T00:00:00Z
resource: internal/span/span.go#New
anchor: "func New(w Width) (Buckets, error) {"
basis: code
tier: supporting
group: derived
depends_on: []
---

`New` is the only constructor of `Buckets` and the only place an unknown width is refused; config calls it at load so a typo dies at startup. `Buckets.Of` is arithmetic with nothing to fall back to. Ids say their own width: `bucket:2026-07`, `bucket:2026-W28`, `bucket:2026`, `bucket:2026-07-03`.

`Layout` is derived from the checkpoint log, never stored: a year is split once a checkpoint of one of its months exists, a month once a day exists. Splits only add. A weekly id does not subdivide a year, so `LayoutOf` refuses it; see [weekly width disables composed reads](/gaps/weekly-width-disables-composed-reads.md).
