---
type: decision
title: Bucket on fact-time, monthly by default, subdivided by a byte budget
description: Checkpoints partition the timeline by calendar window over fact-time; a bucket over checkpoint_budget splits a year into months and a month into days, so bytes decide when and the calendar decides where.
timestamp: 2026-10-11T00:00:00Z
resource: internal/span/layout.go#Layout
anchor: "type Layout struct{ split map[string]bool }"
basis: docs
tier: supporting
tested_by:
  - internal/mint/read_test.go#TestSubdivision_SplitsByBytesAndReadsAreStillTheFold
  - internal/mint/mint_test.go#TestRun_FactTimeMovesBetweenBuckets
alternatives:
  - Bucket on anam-time (not possible here; Anam can bucket on fact-time only because a superseding artifact is the same row re-read, so every version of a facet lands in one bucket)
  - A fixed width only, narrowed by hand as density rises (kept as checkpoint_budget 0; the measured worst bucket was 744 facets, and Garmin's monitoring tables would put 73,000 in one month)
affects:
  - /components/span.md
  - /components/mint.md
  - /terms/bucket-span.md
  - /data-structures/checkpoint-row.md
---

**Context.** A re-mint prices the worst bucket, never the mean, and density varies by orders of magnitude across sources.

**Choice.** Every facet is a member of exactly one bucket by arithmetic on its fact-time. The mint starts from one bucket per year and splits any bucket whose value is over budget (1 MiB default) along the calendar. The layout is derived from the checkpoint log, not stored: a bucket is split once a checkpoint of one of its children exists, and splits only add. Ids are self-describing so old ids keep their meaning when the default changes.

**Why.** The lifelong view is then a disjoint union of bucket values needing no cross-bucket version resolution, and a checkpoint aligns with the time-bounded question a person asks.

**What it costs.** A backfilled facet re-opens its sealed bucket. The width is checked once at config load and pinned into the index; changing it is a rebuild of the derived tier, not a migration. Weekly is accepted by config but does not subdivide a year (see [weekly width disables composed reads](/gaps/weekly-width-disables-composed-reads.md)).
