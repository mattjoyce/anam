---
id: 202607100004
type: decision
title: "Mint unit — checkpoint the whole timeline, not per-episode"
created: 2026-07-10
status: evergreen
tags: [anam, decision, mint, checkpoint]
project: anam
related: ["[[the-inversion]]", "[[person-substrate-grain]]"]
---

# Mint unit — checkpoint the whole timeline, not per-episode

**Decision.** The mint produces checkpoints over the **whole person timeline** at a record-time
watermark. There is no per-span/per-episode mint.

## The tension

Arch's doctrine is *"the inherited span (Episode) is the mint unit"* — checkpoints and seals are
per-episode, because the episode is the bounded thing with a completeness contract. That framing has no
home in Anam: after [[the-inversion]] there is **no required span**, so there is nothing per-episode to
mint.

## Resolution

The person timeline itself becomes the mint unit. A checkpoint is an immutable, content-addressed value
of the canonical fold: `hash = sha256(canonical bytes)` at a watermark. This is the clean reading of
Arch's rule under the inversion — "mint the bounded unit" where the bounded unit is now the whole
(bounded, for one person, by a life).

## Caveats / deferred

- **Cost.** Folding the whole timeline to mint is fine at personal scale; it would not be at hospital
  scale — which is exactly why Arch refused it. Noted, not a problem here.
- **Adaptive cadence** (Arch's "mint to the work, not the clock") is deferred; the MVP mints on a manual
  `anam checkpoint` trigger.
- Spans, being derived filters, are never sealed. If a "seal this episode" need ever appears, it would
  be a *new* inherited-span concept, not a change to this decision.
