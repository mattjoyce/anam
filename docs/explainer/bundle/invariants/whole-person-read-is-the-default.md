---
type: invariant
title: The whole-person read is the default; a span is an optional lens
description: Storage is partitioned by the person onto one timeline; no span is required to read, and a derived span only narrows a read.
timestamp: 2026-10-11T00:00:00Z
resource: internal/read/timeline.go#Timeline
anchor: "case narrow(src, q, b):"
basis: code
tier: supporting
enforced_at: internal/read/parse.go#parseBounds
enforced_anchor: "b.from = clock.Format(t)"
applies_to:
  - /components/read.md
  - /components/cli.md
  - /terms/derived-span.md
  - /terms/inversion.md
---

ARCHITECTURE §12, third bullet, first half. `anam timeline` with no flags reads the whole canonical; `--from` and `--to` are optional and parsed into an inclusive-exclusive window applied after the value is composed. There is no fail-closed on a whole-person read, unlike Arch. The enforcement is the absence of a required parameter in `TimelineQuery` and the bounds parser treating an empty bound as open.
