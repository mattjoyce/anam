---
type: decision
title: A distinction goes in the cheapest place that lets its reader ask cleanly
description: A type is domain.concept[.specialisation…]; a distinction lives in the type name, the Definition's meta, a payload field, provenance, an edge or a projection, and is promoted up that list only when something forces it.
timestamp: 2026-10-21T00:00:00Z
resource: internal/facet/facet.go#typePattern
anchor: "var typePattern = regexp.MustCompile("
basis: docs
tier: supporting
alternatives:
  - Exactly two segments, domain.concept (card 147's sentence "Type stays domain.concept … two segments … as today"; it kept the old shape, nobody reasoned it, and the code never enforced it, so it is dropped)
  - Put every distinction in the type name (refused; each one costs a rename run and a new version of every facet, and a type per exercise or per test code multiplies without bound)
  - Put every distinction in the payload (refused; a different shape or a type that closes a series cannot be a field, because the fold reads that from the Definition)
affects:
  - /data-structures/definition-manifest.md
  - /decisions/a-hard-coded-value-is-a-default-or-an-invariant.md
---

**Context.** Card 147 closed the head of a type name at eight domains (card 222 made it nine, below) and wrote that the name stays `domain.concept`. The two-segment half was never argued, and `facet.typePattern` takes any number of dotted segments. Matt, 2026-10-21: "we control our destiny not AGENTS.md". A third or later segment is allowed, for a specialisation. That leaves the question of when a distinction earns a segment at all, and the answer was being found type by type.

**Choice.** A type is `domain.concept[.specialisation…]`. The domain is one of the closed nine, `health`, `activity`, `place`, `record` and `mind` in use, and `social`, `money`, `media` and `work` held back. A head is added only by a logged decision. A distinction goes in the cheapest of six places that still lets its reader ask the question cleanly, and moves up only when something forces it.

| Place | What changing it costs | Holds |
|---|---|---|
| Type name | A rename run, and a new version of every facet | A different shape, or different time semantics |
| Definition meta (`family`, `kind`) | A Definition version | A grouping that crosses concepts, or a note for readers |
| Payload field | Nothing per fact; the schema may loosen in a new version | What varies fact to fact inside one shape |
| Provenance (source, binding) | Fixed when the fact is admitted | Who said it |
| Edge | A fact of its own | A relation between facts |
| Projection | A template version | A view, a filter or an ordering |

The list runs from dearest to cheapest. Start at the bottom and promote a distinction only for a reason.

**What forces a type.** A different shape, meaning a different schema. Or different time semantics: a type that closes a series is its own type, because the fold reads the closing rule from the Definition and cannot read it from a payload.

**The segments.** The concept is the second segment. A third segment is a specialisation, and it means one of two things. It is a kind within the concept (`health.medication.started`, `health.medication.stopped`). Or it is the vendor whose own shape the payload is (`activity.exercise-session.garmin`). A vendor's own payload shape always takes the vendor leaf, even with one vendor. A standard shape (Open mHealth, IEEE 1752) takes none, so the name says whether to expect a portable payload. The Definition's `kind` is meant to say which of the two a leaf is. Today `kind` takes `measure`, `reading` or `event` (card 176), so it cannot yet, and the Definition's description has to.

**Other rules.**
- What varies fact to fact within one shape is a payload field.
- Who said it is provenance, unless it changes the shape.
- `family` is for a grouping that crosses concepts.
- A relation between facts is an edge, never a field on either.
- A domain head is not a distinction a type earns. `mind`, the domain of self-reported inner states, was added by Matt's decision of 2026-10-22 (card 222), so that mood or energy can join later as concepts without another head. Attention is its first concept: `mind.attention` is a reading, and `mind.attention.started` and `mind.attention.ended` are a pair that closes a series, each of its own type for the reason above.
- A type with no live facets and no writer is history. It is marked superseded (card 184), not renamed.

**Examples.**
- Exercise sessions split by vendor. A Garmin session and a Concept2 session carry different payload shapes, so each is its own type under `activity.exercise-session`, not one type with a `vendor` field. Card 195 split them into four leaves, and renamed `activity.day`, `health.measurement` and `health.cpap-session` to their vendor's leaf.
- Medication started and stopped as separate types. Stopped closes a series, and the fold reads that from the Definition, so it cannot be a `status` field on one type.
- Rowing against walking is a payload field, `activity_name`. Both are an exercise session of one shape, so a split would only cost a rename run per new sport.
- Coffee, wine and a rich meal are one type, `health.intake`, with `kind` a payload field (card 232). Each is an event at a minute with no series and one time rule, so a split would only cost a type per new kind; the per-kind fields a payload must carry are held by the schema's `if` and `then`, and a reader asks for the kind with `attr`.
- `family: meds` is meta. It groups the medication types for a reader and changes no shape, so it needs neither a segment nor a field.

**Why.** Each place up the list is dearer to change and harder to undo, so the cheap place that works is the right place. A rule that says so gives the same answer each time a new kind of fact arrives, where before it was whatever the last type did.

**What it costs.** The cheap places are the ones a reader cannot select by in the store, since the store never opens a payload. A distinction that readers will filter on every day has to be argued up, and that argument is a judgement the rule narrows and does not remove. The existing hyphenated names keep their shape until a card moves them; the medication, illness and trip names left theirs in card 196. `health.heart-rate-day` left its in card 205 by a new type, not a rename: `health.heart-rate.day` holds a day's three figures as one fact, a different shape, and the old type's facts are retired by the Garmin importer's snapshot.
