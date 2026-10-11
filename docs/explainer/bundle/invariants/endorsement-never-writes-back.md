---
type: invariant
title: Agent findings are derived facets admitted as proposed; endorsement never writes back
description: An insight re-enters through the same gate as a proposed derived facet carrying its method and the watermark it was computed from; endorsed and rejected are anam-events that never reach a source.
timestamp: 2026-10-11T00:00:00Z
resource: docs/ARCHITECTURE.md#Agents as consumers
anchor: "**Admission is not endorsement.**"
basis: docs
tier: detail
enforced_at: not enforced
applies_to:
  - /components/gate.md
  - /terms/anam-event.md
  - /terms/status.md
---

ARCHITECTURE §12, thirteenth bullet, and §11. The shape exists: the artifact carries a birth status, the provenance has a derivation with method, model and watermark, the fold derives endorsed and rejected, and the facet struct validates a derived facet's fields. No code writes an endorsed or rejected event yet, and no worker proposes a derived facet; MVP rung R9 is unbuilt. The invariant is doctrine with a seam waiting for it.
