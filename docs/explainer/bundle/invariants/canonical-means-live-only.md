---
type: invariant
title: Canonical means the live-only value; the ledger means append-everything-marked
description: "One word, one surface: the canonical is the fold's live facets at a watermark, superseded and tombstoned ones excluded from the value and retained in the ledger, marked."
timestamp: 2026-10-11T00:00:00Z
resource: internal/fold/fold.go#Live
anchor: "return s.Status == facet.StatusAdmitted || s.Status == facet.StatusEndorsed"
basis: code
tier: supporting
enforced_at: internal/fold/fold.go#Live
enforced_anchor: "return s.Status == facet.StatusAdmitted || s.Status == facet.StatusEndorsed"
applies_to:
  - /components/fold.md
  - /terms/canonical.md
  - /terms/ledger.md
  - /components/projection.md
tested_by:
  - internal/fold/fold_test.go#TestFold_Liveness
---

ARCHITECTURE §12, ninth bullet. Anam once used "canonical" for the append-everything surface; that is corrected. A projection over the canonical sees live facets by construction, which is why tombstone visibility is no longer a projection knob: a reader who wants tombstones is asking the ledger a forensic question.
