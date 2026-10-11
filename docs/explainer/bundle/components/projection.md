---
type: component
title: Projection
description: Applies a bundled template to a canonical value, metadata filters only, and explains each facet's verdict by the knob that decided it.
timestamp: 2026-10-11T00:00:00Z
resource: internal/projection/projection.go#Explain
anchor: "func Explain(v fold.Value, t Template, g Gate, s Series) ([]Verdict, error) {"
basis: code
tier: supporting
group: published
depends_on:
  - /components/fold.md
  - /components/facet.md
---

Knobs, applied in this order so the first failing one names the reason: type allow-list, status filter, classification ceiling, fact-time window, then the consent gate (`consent.Open` returns allow today; a nil gate is refused), then the shape. Shape `snapshot` runs over what the other knobs admitted and demotes entries to displaced-by or closed-by using `fold.Fates`. The payload is never read. Templates are the three embedded [projection templates](/data-structures/projection-template.md): `bp`, `sleep`, `meds`.
