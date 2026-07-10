---
id: 202607102354
type: decision
title: "A checkpoint is a replay floor; buckets are ISO weeks; staleness is dedup'd by hash, not tracked"
created: 2026-07-10
status: evergreen
tags: [anam, decision, checkpoint, bucket, mint, iso-week, prototype]
project: anam
related: ["[[state-is-checkpoint-plus-replay]]", "[[span-is-the-mint-unit-not-the-read-filter]]", "[[doctrine-aphorisms-outlive-their-reasoning]]"]
---

# A checkpoint is a replay floor; buckets are ISO weeks; staleness is dedup'd by hash, not tracked

Three decisions from one conversation, each smaller than the design it replaced.

## A checkpoint is a floor, not a value

I nearly built birth-bucket pinning to prevent a "permanent double-count" when a corrected facet's
event-time moved it between buckets. The bug did not exist. Every read is *nearest checkpoint +
replay-since* ([[state-is-checkpoint-plus-replay]]); the `superseded` event in the tail drops the moved
facet from the old bucket's result. A sealed checkpoint that has gone stale is still **correct as a
floor** — staleness costs replay depth, never wrongness. Matt collapsed the whole error with one line:
*"it will need to have issued a supersede event on the old facet — how else can you avoid double
counting?"* The design error was reasoning about checkpoint *values* without writing down the read path
first.

## The bucket is one ISO week

Width and cadence are different knobs that were wearing one word: **width** bounds the cost of *making*
a checkpoint (facets folded), **cadence** bounds the cost of *using* one (events replayed). Chosen: ISO
week buckets, minted weekly. The trap pinned by test: the **ISO week-year is not the calendar year** —
2027-01-01 sits in `2026-W53`, so an id built from `t.Year()` splits one week across two ids and the
partition stops partitioning. Use both returns of `time.ISOWeek()`; zero-pad the week.

## No dirty-tracking — always fold, dedup by hash

The `W > Wᵥ` re-mint test needs a per-span watermark that a single global `W` cannot supply (after any
commit, *every* span looks busy). Rather than build per-span watermark machinery at R2, the prototype
always folds and lets the content address dedup: an unchanged bucket folds to an identical hash and
writes nothing. Correct without being clever. *"This is a prototype"* — the optimiser arrives when
replay depth is a measurement, not a fear. [[brooks-below-the-gate-beck-above-it]] reached the same
conclusion from the other side.
