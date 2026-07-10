---
id: 202607100022
type: decision
title: "Ledger and canonical are two surfaces — one word must not name both"
created: 2026-07-10
status: evergreen
tags: [anam, decision, ledger, canonical, glossary, naming]
project: anam
related: ["[[state-is-checkpoint-plus-replay]]", "[[verbatim-payload-metadata-separation]]", "[[mint-unit-whole-timeline]]"]
---

# Ledger and canonical are two surfaces — one word must not name both

**Decision** (Arch ADR 0032's MC5, adopted). Anam has two read surfaces over the same facts, and exactly
one of them is called *canonical*.

| Surface | Contains | Answers |
|---|---|---|
| **Ledger** | every facet ever admitted, **marked at its current status** — superseded and tombstoned included | forensic reads, `as-of`, `changes_since`, rebuild |
| **Canonical** | the **live-only** fold at a watermark — superseded and tombstoned facets are *excluded from the value* | the normal serve path; what projections prune |

A tombstoned facet stays in the ledger forever, marked. It is absent from the canonical value. Neither
surface is more true than the other — **they answer different questions**, and Anam had been using one
name for both.

## How the collision happened

Two documents, each internally coherent, each right about its own object:

- *"Retraction is a tombstone; the canonical account stays append-only"* — true of the **ledger**.
- *"The canonical is a live-only current-state fold"* — true of the **canonical**.

Neither was wrong. They named different things. A single glossary entry then fused them, and the fused
definition propagated: Anam's projections grew a **tombstone visibility** knob, which only makes sense if
"canonical" retains tombstones.

> Two correct sentences about two different objects, sharing one noun, produce a third sentence that is
> about nothing.

## What the correction removes

`tombstone_visible` disappears as a projection filter. A projection prunes the **canonical**, which is
live-only by construction — there is nothing to hide. A reader who wants tombstones is not asking a
projection for a variant; they are asking the **ledger** a forensic question, and that is a different
call.

Notice that the knob was *implementable*, *testable*, and *wrong*: nothing about building it would have
revealed the confusion. Only naming the two surfaces did.

## The general shape

This is the same failure as [[mint-unit-whole-timeline|the mint-unit retraction]], one level up. There,
"span" named two concerns and carried a conclusion between them. Here, "canonical" named two surfaces and
carried a *feature* between them.

> A glossary is not documentation. It is a **constraint on what can be said**. When one term names two
> objects, the system will grow a feature that belongs to neither.
