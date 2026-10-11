---
type: decision
title: Fact-time is promoted out of the payload at the gate
description: The Definition's time_semantics pointer, Go layout and IANA zone turn a payload field into a first-class UTC column, so the timeline orders and filters without opening a payload.
timestamp: 2026-10-11T00:00:00Z
resource: internal/def/check.go#FactTimeOf
anchor: "t, err := time.ParseInLocation(tf.Layout, s, d.loc)"
basis: docs
tier: supporting
alternatives:
  - Leave fact-time inside the blob and parse every payload on every read (refused; it would make the metadata/payload separation a fiction)
  - Require sources to send UTC (refused; a zoneless source clock is the wall clock where the person stood, so the zone is the Definition's to declare)
affects:
  - /components/gate.md
  - /components/def.md
  - /terms/fact-time.md
  - /data-structures/facet-artifact.md
---

**Context.** The timeline is ordered by fact-time and a window is filtered by it; a bucket is arithmetic on it.

**Choice.** Every Definition declares `time_semantics.fact_time = {pointer, layout, zone}`. The gate resolves the pointer, parses in the zone, stores UTC in a fixed-width text column on the artifact and every canonical entry. The binary embeds `time/tzdata` so a container image needs no zone database.

**Why.** The promotion is what lets every read stay on the governed envelope.

**What it costs.** A Definition that changes its pointer or layout re-places facets in time, which is why `def.Guard` refuses a new version that changes it; a new placing rule needs a new ledger from the sources.
