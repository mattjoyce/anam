---
id: 202607100017
type: decision
title: "Brooks below the gate, Beck above it — the error kernel sets the step size"
created: 2026-07-10
status: evergreen
tags: [anam, decision, planning, incremental-delivery, error-kernel, brooks, beck]
project: anam
related: ["[[guiding-lens-hickey-armstrong]]", "[[substrate-is-accidental]]", "[[invariants-as-structural-tests]]"]
---

# Brooks below the gate, Beck above it — the error kernel sets the step size

**Decision.** The build ladder is sized by which side of the **error kernel** a rung sits on.

- **Below the gate** — the append-only ledger and the admission gate — work is **depth-first and
  complete**. No stubs, no provisional shapes, no "we'll harden it later."
- **Above the gate** — fold, timeline, projections, receipts, mint, insights — work is in the
  **smallest step that yields information**. Ship it, look at it, learn, continue.

## The tension this resolves

Brooks and Beck genuinely disagree about how to approach work, and averaging them produces the worst of
both: a half-built gate *and* a slow feedback loop.

- **Brooks** wants conceptual integrity and depth: a half-finished gate corrupts the core, and a
  coherent design is worth features left out. Applied everywhere, this delays all feedback until the
  whole concept stands.
- **Beck** wants the step shrunk until the feedback loop is tight: the smallest verifiable change is the
  safest, because learning lives in the loop. Applied everywhere, this leaves the trusted core
  provisional.

Anam already contains the line that decides between them. Armstrong's **error kernel**
([[guiding-lens-hickey-armstrong]]) is *"the append-only ledger plus the admission-gate invariants —
everything else is disposable and rebuildable from source."*

That is not just a runtime property. It is a **planning instruction**:

> Where corruption is unrecoverable, be Brooks. Where the artifact is disposable, be Beck.

Nothing above the gate can repair a corrupt ledger, so the gate does not get to be provisional. But
nothing above the gate is *precious* either — Anam owns no facts and rebuilds from source — so a wrong
projection, a wrong timeline view, a wrong checkpoint costs a re-run and nothing more. Those are exactly
the places where small steps and fast feedback are free.

## Two corollaries the pair produced

**Mint is a cache of the fold.** It exists to bound replay; at this scale there is no replay cost to
bound. Building it before the fold — as the original `MVP.md` §2 chain did — solves an unmeasured
performance problem. Keep the seam (a record-time watermark), defer the machinery. The reordered ladder
puts `timeline` at rung four and mint at rung eight.

**The first rung was not code.** Brooks says *plan to throw one away*; Beck says *take the smallest step
that yields information*. These turn out to be the same instruction. The riskiest assumption in the
project — that DynamoDB Local supports atomic conditional `TransactWriteItems`, on which the whole
crash-clean property rests — was retired in five minutes of `curl` against a scratch table that was then
deleted. Zero production code, maximum information.

> Before building on an assumption, price the experiment that would falsify it. It is almost always
> cheaper than the design that assumes it.

## The habit this indicts

Beck's third tenet: structural and behavioural change must not share a commit, or a bisect cannot tell
you which one broke. Two commits in this project's own history violate it — one carried a config
fail-fast *and* a new type; another carried config discovery, a removed default, *and* a `gosec` fix.
One decision per commit, from here.
