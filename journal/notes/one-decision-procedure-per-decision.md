---
id: 202607251500
type: decision
title: "A projection explains itself from the filter, not from a second account of it"
created: 2026-07-25
status: evergreen
tags: [anam, decision, projection, governance, explainability]
project: anam
related: ["[[ledger-and-canonical-two-surfaces]]", "[[as-of-reads-fetch-by-version]]", "[[invariants-as-structural-tests]]", "[[guiding-lens-hickey-armstrong]]"]
---

# A projection explains itself from the filter, not from a second account of it

**Decision.** `projection.Apply` is *built from* `projection.Explain`. One `verdict` function decides
whether an entry is served and, if not, which knob excluded it; `Apply` keeps the served ones and
`Explain` reports all of them. Likewise the derivation of which artifact versions are no longer in force
lives once, in `fold.Displaced`, and every viewer maps its result rather than recomputing it.

## The tension

A governed system has to answer "why is this reading not in the bp view?" The obvious shape is a filter
that returns a bool and, beside it, an explainer that re-checks the same conditions to produce a reason.
It reads fine and it is cheap to write.

It is also a system whose explanation can disagree with its behaviour. The moment the two are separate
code, "excluded: type not in the allow-list" is a *claim about* the filter rather than a *report from*
it, and nothing makes the claim true. A projection that could describe a filter it does not apply is
governance by coincidence.

## What this costs, and what it buys

The filter now returns a `Reason` rather than a bool, which is marginally more machinery for the
common path. In exchange:

- The exclusion reason cannot drift from the exclusion.
- Attribution is stable: an entry failing two knobs is reported against the first in documented order,
  not against whichever the evaluator reached first.
- A test can assert `Apply` and `Explain` agree across the *whole* closed knob space — four small
  enumerable sets, so the cross-product is the universal quantifier rather than a sample.

## The half I nearly missed

Having made this argument about the filter, I then wrote the *displaced-versions* derivation twice —
once for the CLI's `--explain`, once for the inspector's projection page. Same drift, other half, in the
same afternoon.

> The argument for sharing a decision procedure applies to **every** part of the decision. Having just
> made it about one part is no protection at all.

## Where the boundary actually is

Not everything belongs in one place. `Explain` reports on entries *in the canonical*; displaced versions
are not in the canonical at all — the canonical is live-only by construction — so they never reach a
template and can carry no exclusion reason. They are a separate standing, derived from the fold, and
that separation is real rather than an artefact of who wrote what first.
