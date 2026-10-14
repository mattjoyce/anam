---
id: 202610090700
type: decision
title: "Checkpoint buckets split on cost and cleave on the calendar"
created: 2026-10-09
status: evergreen
tags: [anam, decision, checkpoint, mint, read-path, performance]
project: anam
related: ["[[state-is-checkpoint-plus-replay]]", "[[hard-and-soft-checkpoints]]", "[[span-is-the-mint-unit-not-the-read-filter]]", "[[checkpoint-is-a-replay-floor]]"]
---

# Checkpoint buckets split on cost and cleave on the calendar

**Decision (Matt, with the agent, 2026-10-09, card 59).** A checkpoint bucket starts as a year. When
its value passes `checkpoint_budget` bytes (1 MiB by default) it splits along the next calendar
boundary, a year into months, a month into days. Performance decides *when* a bucket splits. The
calendar decides *where*. Reads compose from the newest checkpoints and replay only the tail since
them, so a read no longer folds the whole ledger.

## The tension

Matt asked why every fold read the whole ledger when hard checkpoints existed. Answering it meant
choosing a bucket shape, and a fixed calendar width is wrong at both ends of a personal record.
Monthly buckets turn the sparse early years into dozens of near-empty files, and make a dense month
of Garmin data one bucket that a single late correction reopens whole.

Splitting by count ("every N facets") fixes the sizes and breaks the property composition depends
on. A facet's bucket must be a function of its fact-time alone, so that every version of a facet
lands in one bucket and stitching buckets is a plain union with no version resolution. A backfill
would shift every count boundary after it. The calendar keeps that property and lets the size vary.

The trigger had its own tension. A time measured at mint is the real cost, but it depends on the
machine and its load, so the NAS and the ThinkPad would cut the same record differently. The size of
the bucket's value in bytes is a deterministic proxy for that cost, and tracks density better than a
count, since a lab panel and a heart-rate sample differ in size by orders of magnitude. The bench set
the budget, about 46 ms per MB to re-derive on the NAS, so 1 MiB keeps one dirtied bucket near 50 ms.

## The rule

- A fact-time sits in exactly one bucket. Ids stay self-describing (`bucket:2013`,
  `bucket:2026-07`, `bucket:2026-07-14`), found by walking down from the year.
- The layout is derived from the checkpoint log, not stored. A bucket is split exactly when a
  checkpoint of one of its children exists. Nothing new can disagree with the log, and an old
  monthly log reads as "years split into months" with no migration.
- Splits only go down. Thin buckets are not merged back.
- `checkpoint_budget: 0` keeps the fixed `bucket_width`.
- The layout changes what a read costs, never what it returns. The oracle test folds the whole
  ledger at every watermark and holds the composed read to it byte for byte, across corrections that
  move a facet between buckets, backfills, retractions that empty a bucket, and splits between mint
  runs.

Composing from checkpoints took a one-month read of the real record from 980 ms to 62 ms on the
ThinkPad, minted at head. Subdivision then re-minted the real record as 47 checkpoints, 13 whole
years and 34 months, where monthly had 146.
