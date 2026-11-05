---
type: decision
title: The subject is the substrate
description: "The subject, not the episode, is the unit of storage, and the whole-record read is the default; minting stays per bucket."
timestamp: 2026-10-11T00:00:00Z
resource: docs/ARCHITECTURE.md#The whole-record read
anchor: "### The whole-record read (read this before anything else)"
basis: docs
tier: supporting
alternatives:
  - Scope every read to an episode, where every read is scoped to a bounded episode and the subject is a thin identity index (refused, because a record of one subject wants to be longitudinal)
  - Make the whole timeline the mint unit as well as the read default (tried, ruled out in ARCHITECTURE §1 as a conflation of read default and mint unit)
affects:
  - /components/read.md
  - /components/mint.md
  - /terms/inversion.md
  - /terms/derived-span.md
---

**Context.** A subject's history is unbounded and "longitudinal is system-of-record gravity". A soul-record of one subject, which might be a person, wants exactly that gravity.

**Choice.** Re-promote the subject. A ledger file is pinned to one subject id (`subject_id` in config), every facet is on one timeline in fact-time order, and a read is "the subject's facts, in time order", optionally narrowed. No identity graph, no merge.

**Why.** At the scale of one subject (for a person, one life) the cost of that gravity is affordable, and the longitudinal account is the whole product.

**What it costs.** Anam owns the continuity: the composed, ordered account exists nowhere upstream. It is re-derivable by re-ingesting every source, but not held anywhere else while Anam is off, so the ledger's order, watermarks and checkpoints are backed up rather than assumed rebuildable. This does not license whole-life minting: the read default is unbounded, the mint unit is a bucket.
