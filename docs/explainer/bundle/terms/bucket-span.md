---
type: term
title: Bucket span
description: The minted span, a calendar window over fact-time that every facet belongs to by arithmetic; invisible to the reader.
timestamp: 2026-10-11T00:00:00Z
---

A bucket span is a calendar window over [fact-time](/terms/fact-time.md), one month by default (`bucket_width`, weekly also implemented), subdivided along the calendar when a bucket's value is over `checkpoint_budget`. Every facet is a member of exactly one by arithmetic on its fact-time, with no lookup. Bucket ids are self-describing (`bucket:2026-07`, `bucket:2026-W28`, a day for a split bucket).

Because `facet_id` is identity and not contents, every version of a facet carries the same fact-time and lives in one bucket, so the lifelong view is a disjoint union of bucket values. Contrast the [derived span](/terms/derived-span.md), which is never minted.
