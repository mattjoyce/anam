---
id: 202607100002
type: decision
title: "The inversion — Anam re-promotes the Person to substrate, and accepts the gravity"
created: 2026-07-10
modified: 2026-07-10
status: evergreen
tags: [anam, decision, core, arch]
project: anam
related: ["[[person-substrate-grain]]", "[[anam-owns-the-continuity]]", "[[span-is-the-mint-unit-not-the-read-filter]]", "[[2026-07-10-genesis]]"]
---

# The inversion — Anam re-promotes the Person to substrate, and accepts the gravity

**Decision.** Anam keeps all of Arch's machinery and inverts exactly one thing: **the Person is the
substrate, and the whole-person timeline read is the default and normal mode.**

## The tension, stated properly

An earlier draft of this note said Anam inverts a *read default* — that Arch requires an Episode-span
filter and Anam does not. That undersells it, and cites the wrong ancestor.

Arch's keystone is **ADR 0035**, and it goes much further:

> *"The **Episode is the unit** — of storage, cost, completeness, and the checkpoint span. The **Person
> is a thin identity index** — a core identity record holding linkage only, never content."*

Arch had *already tried* the person-as-substrate design (its own ADR 0025) and **reversed it**, because:

> *"A **Person** is unbounded — a life has no edges, is never complete, is retained indefinitely… and
> longitudinal is **system-of-record gravity**."*

So Anam does not merely relax a filter. **Anam re-promotes the Person to substrate — the exact move ADR
0035 was written to undo** — and takes on the gravity, knowingly, at the scale of one life where the cost
Arch could not pay across a jurisdiction is affordable.

## Why this side wins (for Anam)

- The consumer is one person reading themselves; there is no third party for whom a whole-life read is a
  hazard.
- Continuity **is** the product, not a leak. Composing episodes on demand would be ceremony that fights
  the goal.
- The costs 0035 itemises — cost, completeness, span problems — are the costs of *a population of
  unbounded lives*. Anam has one, and it is small.

## What it does *not* license

The inversion is a claim about **what a reader gets by default**. It says nothing about what a fold is
bounded by. Anam once read it as licensing a whole-timeline mint unit; that was a lexical accident, and
it is retracted ([[span-is-the-mint-unit-not-the-read-filter]]).

> The read default is unbounded. **The mint unit is not.**

## What keeps it honest — and what does not

Removing Arch's safety filter does not make Anam a rogue system of record. The line that holds is
[[verbatim-payload-metadata-separation|"Anam owns no source fact"]]: every payload is
verbatim-from-source and rebuildable.

But that line is **not the whole truth**, and ADR 0035 says so to our face. Under person-grain the
continuity exists nowhere else. See [[anam-owns-the-continuity]] — the honest version of Anam's founding
invariant, and the one that has to be stated aloud if Anam is not to become, quietly, the thing Arch
refused to be.

## Consequences

- [[person-substrate-grain]] — the person is the storage substrate and the default read.
- [[anam-owns-the-continuity]] — every fact rebuilds; the composed account does not.
- [[span-is-the-mint-unit-not-the-read-filter]] — minting stays bounded, per bucket span.
- Arch's identity graph and merge are dropped (single person).
- A derived Span survives as a *saved query* / bookmark, never an aggregate.
