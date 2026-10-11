---
type: invariant
title: Projections prune, never reshape or generate; a consumer selects, never defines
description: A template includes or excludes whole facets by metadata filters only, and the only templates are the versioned ones bundled in the binary.
timestamp: 2026-10-11T00:00:00Z
resource: internal/projection/projection.go#Apply
anchor: "verdicts, err := Explain(v, t, g, s)"
basis: code
tier: supporting
enforced_at: internal/projection/projection.go#Apply
enforced_anchor: "verdicts, err := Explain(v, t, g, s)"
applies_to:
  - /components/projection.md
  - /components/serve.md
  - /terms/projection.md
  - /data-structures/projection-template.md
tested_by:
  - internal/projection/projection_test.go#TestApply_PropertyPruneOnly
  - internal/projection/projection_test.go#TestApply_TypeAllowListDeniesByDefault
---

ARCHITECTURE §12, twelfth bullet, and §9. The knobs are a type allow-list (deny by default), a status filter, a maximum classification, a fact-time window and a shape; the shape is the one knob whose verdict depends on other entries, which is why it is a template knob and not a read-time flag. A served value is a subset of the canonical's entries with the same bytes per entry, which the property test checks over random templates. A wide-open template reproduces the canonical hash.
