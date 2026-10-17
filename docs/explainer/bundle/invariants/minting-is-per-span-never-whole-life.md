---
type: invariant
title: Minting is per span, never whole-life
description: A checkpoint is the value of one bucket at a watermark; the lifelong view composes bucket values and never re-folds the person.
timestamp: 2026-10-11T00:00:00Z
resource: internal/mint/mint.go#settle
anchor: "if minted && head.ValueHash == value.Hash() {"
basis: code
tier: supporting
enforced_at: internal/mint/mint.go#settle
enforced_anchor: "if minted && head.ValueHash == value.Hash() {"
applies_to:
  - /components/mint.md
  - /components/span.md
  - /terms/checkpoint.md
  - /terms/bucket-span.md
tested_by:
  - internal/mint/mint_test.go#TestRun_FirstMintThenIdempotent
  - internal/mint/mint_test.go#TestCompose_MatchesTheFoldOverARange
---

ARCHITECTURE §12, third bullet, second half. The read default is unbounded and the mint unit is bounded, and the two must not be conflated: Anam once reasoned "no required span, therefore the mint unit is the whole timeline"; it does not hold. `settle` only ever hashes and writes one bucket's entries; there is no code path that writes a whole-person checkpoint.
