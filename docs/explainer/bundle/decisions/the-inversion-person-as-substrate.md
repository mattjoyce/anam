---
type: decision
title: The person is the substrate
description: "Anam inverts Arch's keystone: the person, not the episode, is the unit of storage, and the whole-life read is the default; minting stays per bucket."
timestamp: 2026-10-11T00:00:00Z
resource: docs/ARCHITECTURE.md#The one inversion
anchor: "### The one inversion (read this before anything else)"
basis: docs
tier: supporting
alternatives:
  - Keep Arch's grain, where every read is scoped to a bounded episode and the person is a thin identity index (refused, because a personal record wants to be longitudinal)
  - Make the whole timeline the mint unit as well as the read default (tried, retracted in ARCHITECTURE §1 as a conflation of read default and mint unit)
affects:
  - /components/read.md
  - /components/mint.md
  - /terms/inversion.md
  - /terms/derived-span.md
---

**Context.** Arch deliberately demoted the person from substrate to index, on the grounds that a person is unbounded and "longitudinal is system-of-record gravity". A personal soul-record wants exactly that gravity.

**Choice.** Re-promote the person. A ledger file is pinned to one `person_id`, every facet is on one timeline in fact-time order, and a read is "my facts, in time order", optionally narrowed. No identity graph, no merge.

**Why.** At the scale of one life the cost Arch could not pay across a jurisdiction is affordable, and the longitudinal account is the whole product.

**What it costs.** Anam owns the continuity: the composed, ordered account exists nowhere upstream. It is re-derivable by re-ingesting every source, but not held anywhere else while Anam is off, so the ledger's order, watermarks and checkpoints are backed up rather than assumed rebuildable. The inversion does not license whole-life minting: the read default is unbounded, the mint unit is a bucket.
