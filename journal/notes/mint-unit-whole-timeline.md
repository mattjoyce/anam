---
id: 202607100004
type: decision
title: "Mint unit — the whole timeline (RETRACTED)"
created: 2026-07-10
modified: 2026-07-10
status: retracted
tags: [anam, decision, mint, retracted]
project: anam
related: ["[[span-is-the-mint-unit-not-the-read-filter]]", "[[the-inversion]]", "[[state-is-checkpoint-plus-replay]]"]
---

# Mint unit — the whole timeline (RETRACTED)

> **Retracted 2026-07-10, the same day it was written**, on reading Arch's ADRs 0032 and 0033 rather than
> the inherited summary of them. Superseded by [[span-is-the-mint-unit-not-the-read-filter]]. Kept, not
> deleted: the *shape* of the mistake is more instructive than the conclusion ever was.

## What it claimed

That because [[the-inversion]] removes the required span filter, Arch's *"inherited span = mint unit"*
has no counterpart in Anam — and therefore the mint unit becomes the **whole person timeline**.

## Why it was wrong

It conflated two different uses of one word.

> Removing the Span as a **required read filter** never required removing it as a **mint unit**.

The inversion is a claim about *what a reader gets by default*. The mint unit is a claim about *what a
fold is bounded by*. They are independent. Arch's ADR 0033 states it outright — *"Minting is per span;
whole-life is composed, never minted"* — and ADR 0032 names the exact cost of getting it wrong: *"under
Person-root a long span's live-set grows for life."*

The seduction was that the conclusion *felt* entailed. One decision (no required span at read) appeared
to license a second, unrelated one (no span at mint), because both sentences contain the word "span".

## The tell we ignored

This note's own "Caveats" section said:

> *"Cost. Folding the whole timeline to mint is fine at personal scale; it would not be at hospital
> scale — which is exactly why Arch refused it."*

So it **saw** the cost, correctly attributed it to Arch's refusal, and then filed it as *"noted, not a
problem here"* — never asking what mechanism Arch had built to avoid it. The answer was a bounded mint
unit that costs nothing at any scale. We reasoned about the objection instead of reading the solution.

> When you can name why the other design refused your choice, you are one step from the mechanism they
> refused it *with*. Go get it. Do not stop at "doesn't apply to me."

## The lesson worth keeping

> When one decision appears to license a second, check whether the word doing the work means the same
> thing in both. A term that names two concerns will carry a conclusion between them for free.

That is Hickey's *complect* in lexical disguise: two concerns braided by a shared name rather than by
shared code. The fix was to give them different names — **bucket span** (minted, invisible to the reader)
and **derived span** (a lens, composed on demand) — after which the false inference could no longer even
be stated.
