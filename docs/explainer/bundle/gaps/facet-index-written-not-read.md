---
type: gap
title: facet_index is written by catch-up but read by no read path
description: The schema comment says the timeline, type filter and bucket membership live here; the timeline in this tree composes from checkpoints or folds the ledger, and only anam changes reads the derived indexes.
timestamp: 2026-10-11T00:00:00Z
looked_at:
  - internal/store/ops/ops.go
  - internal/read/timeline.go
  - internal/read/changes.go
  - internal/mint/window.go
---

The read path moved to composed checkpoints (cards 59, 63, 64) and the comment describes the earlier intent. `WatermarkAt` for `--as-of` also replays the whole ledger rather than using `anam_event_index`, which the schema says exists for exactly that translation. The index still serves `anam changes` and the retraction's bucket lookups via the fold state it persists. See [facet index row](/data-structures/facet-index-row.md).
