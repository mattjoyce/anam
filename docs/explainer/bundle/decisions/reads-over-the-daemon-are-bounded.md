---
type: decision
title: Reads over the daemon are bounded by bytes
description: A timeline over max_read_bytes is refused with 413 and the parameters that would narrow it, measured on the wire encoding; a published read pages instead (card 92); a local read and a verify are never bounded.
timestamp: 2026-10-11T00:00:00Z
resource: internal/read/bound.go#Fit
anchor: "if n > limit {"
basis: code
tier: detail
tested_by:
  - internal/app/bound_internal_test.go#TestTimeline_OverTheBoundIsRefusedWithWhatToNarrowBy
  - internal/app/bound_internal_test.go#TestPublish_OneFacetOverTheBoundFilesNothing
alternatives:
  - A row limit with a cursor (refused for changes_since, where a non-unique anam_time would step over a batch's instant; a byte bound names the cost the daemon actually pays)
  - No bound (how it was; a whole-person read over HTTP could take the daemon's memory and the client's)
affects:
  - /components/read.md
  - /components/daemon.md
  - /components/client.md
  - /data-structures/timeline-response.md
  - /decisions/published-reads-page-by-a-checked-cursor.md
---

**Context.** Card 88.

**Choice.** `Fit` encodes entries exactly as they will be sent and stops at the first one crossing `max_read_bytes` (512 KiB default, 0 unbounded). The refusal says how many matched, the bytes, the limit, and which of `type`, `from`, `to`, `at`, `projection` were unused. Since card 92 a publish or a receipt show over the bound is answered a page at a time instead (see [published reads page by a checked cursor](published-reads-page-by-a-checked-cursor.md)); only one facet over the bound alone is refused, and a publish holding one files nothing. The CLI client separately refuses a body over 64 MiB.

**Why.** A local read is the operator at their own files; over the daemon the answer travels and is held twice.

**What it costs.** The daemon computes the whole value before measuring it; a verify re-derives without a bound because it is the proof.
