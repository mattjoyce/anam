---
id: 202610062200
type: decision
title: "A hard checkpoint has a place; a soft one is ephemeral; the mint promotes soft to hard"
created: 2026-10-06
status: evergreen
tags: [anam, decision, checkpoint, mint, inspect, vocabulary]
project: anam
related: ["[[checkpoint-is-a-replay-floor]]", "[[state-is-checkpoint-plus-replay]]", "[[span-is-the-mint-unit-not-the-read-filter]]", "[[published-is-a-receipt-not-a-record-event]]"]
---

# A hard checkpoint has a place; a soft one is ephemeral; the mint promotes soft to hard

**Decision.** A checkpoint comes in two provenances and one type. A **hard** checkpoint is a bucket's
live-only value at a watermark that has been given a place: a row in the checkpoint log and a file
under its hash. A **soft** checkpoint is the same value computed on demand, with no row and no file,
and it is **ephemeral**: shown, compared, dropped. The mint is the promotion from soft to hard. Only a
hard checkpoint is a replay floor.

**Where it came from.** The night of the first mint (R8), Matt asked how checkpoints should appear in
the inspector. The mint already computed a value for every bucket at W and compared its hash with
the stored one, writing only where they differed; the dry run printed thirty-four of them. The split
existed in the code with no name. Matt named it: *hard checkpoints exist in the db, a soft checkpoint
is synthetic, calculated on demand.* Then, to the worry that a computed value kept around becomes a
stale place: *soft is ephemeral.*

**What pulled the other way.** The doctrine says a checkpoint exists only to bound replay
([[state-is-checkpoint-plus-replay]]). A soft checkpoint bounds nothing, so the word is stretched.
The stretch is accepted because it buys the sentence that explains the mint in six words, and the
glossary says plainly that only a hard checkpoint is a floor. The alternative, calling the soft one
"the bucket's value at W", is exact and unusable in a UI legend.

**What it buys.**

- The three states a viewer needs fall out of one hash comparison: soft equals hard is *sealed*; soft
  differs is *behind*, which is a correct floor costing replay, never wrong
  ([[checkpoint-is-a-replay-floor]]); no hard is *never minted*. "Behind", not "broken".
- The cheap way to compute a soft checkpoint is the read path the architecture already names: the
  nearest hard checkpoint plus replay of the events since its `covers_w`. So the soft checkpoint is
  the general read and the hard one is its floor; wiring the timeline to compose from hard
  checkpoints is the same work as computing soft ones efficiently.
- As-of composes: a soft checkpoint at any past W, compared with the hard one whose `covers_w` is the
  greatest at or below it.
- Citing promotes. A receipt pins a hash that has an address, so a soft checkpoint that is to be
  cited is minted first ([[published-is-a-receipt-not-a-record-event]]). Nothing is cited that has no
  place.

**The one rule that keeps it honest.** Soft must not become a cache. The moment a soft checkpoint is
held between requests to save a fold, it is a place again, and a stale one with no row to say how
stale. Compute it, show it, drop it.

**Three words, kept apart.** Hard checkpoint, stored per bucket. Soft checkpoint, computed per bucket,
ephemeral. Derived span, a composition of either over a range the person names, never stored as one
([[span-is-the-mint-unit-not-the-read-filter]]).
