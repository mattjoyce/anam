---
type: invariant
title: Any state is the nearest checkpoint plus replay since its watermark
description: A read at W equals the fold at W whether it is composed from the last run's checkpoints and the tail or folded from seq 0; the composed read is tested equal to the fold at every watermark.
timestamp: 2026-10-11T00:00:00Z
resource: internal/mint/read.go#ReadAt
anchor: "rows, _, ok, err := leavesAt(ctx, log, run)"
basis: code
tier: supporting
enforced_at: internal/mint/read.go#startingRun
enforced_anchor: "if sealed, err := log.LogSealed(ctx); err != nil || !sealed {"
applies_to:
  - /components/mint.md
  - /components/fold.md
  - /components/read.md
  - /terms/checkpoint.md
tested_by:
  - internal/mint/read_test.go#TestReadAt_IsTheFoldAtEveryWatermark
  - internal/mint/read_test.go#TestReadWindow_IsTheFoldInTheWindow
  - internal/serve/compose_test.go#TestRun_ComposedFromTheLastRunIsTheFullFold
---

ARCHITECTURE §7, Arch ADR 0032: a single indexed range scan. The current canonical is the head case; an as-of is the same scan with an upper bound. The guard that keeps it true is `startingRun`: a composed read refuses to start from a log that is unpinned, unsealed, has no run at or below W, or holds ids the layout cannot subdivide, and folds in full instead. So the composed path can be wrong only if the full fold is.
