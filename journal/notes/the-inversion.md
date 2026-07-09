---
id: 202607100002
type: decision
title: "The inversion — whole-person read is the default"
created: 2026-07-10
status: evergreen
tags: [anam, decision, core, arch]
project: anam
related: ["[[person-substrate-grain]]", "[[mint-unit-whole-timeline]]", "[[2026-07-10-genesis]]"]
---

# The inversion — whole-person read is the default

**Decision.** Anam keeps all of Arch's machinery and inverts exactly one thing: *the whole-person
timeline read is the default and normal mode.* An Episode/Span is an **optional** derived lens, never a
required filter, never minted, never sealed.

## The tension

Arch person-partitions storage **but** makes every read *filtered by an Episode span, default and
required*; a whole-person read is privileged, consent-gated, and denied by default. That is deliberate:
a hospital context layer that could return a patient's whole life would become a de-facto system of
record — an unbounded cost/completeness/clinical-safety trap. Arch's whole design bends to *avoid*
becoming longitudinal.

A personal **soul-record** wants the opposite. The value *is* the longitudinal whole — "me, over time,
with the evidence." The thing Arch refuses to be is precisely what Anam is for.

## Why this side wins (for Anam)

- The consumer is one person reading themselves; there is no third party for whom the whole-life read is
  a hazard.
- Continuity is the product, not a leak. Composing episodes on demand (Arch's model) would be
  ceremony that fights the goal.

## What keeps it honest without the span filter

Removing Arch's safety filter does **not** make Anam a rogue system of record. The line that holds is
[[verbatim-payload-metadata-separation|"Anam owns no source fact"]]: every payload is
verbatim-from-source and rebuildable. Anam is a *lens over your sources*, not a second original. That
invariant — not the span filter — is what makes the grain safe here.

## Consequences

- [[person-substrate-grain]] — the person is the storage substrate and the default read.
- [[mint-unit-whole-timeline]] — with no required span, the mint unit is the whole timeline.
- Arch's identity graph and merge are dropped (single person).
- A Span survives only as a *saved query* / bookmark, never an aggregate.
