---
id: 202607100023
type: decision
title: "A natural key must identify a row, never describe it"
created: 2026-07-10
status: evergreen
tags: [anam, decision, identity, idempotency, supersession, keys]
project: anam
related: ["[[canonicalisation-fixes-serialisation-not-values]]", "[[facet-definition-format]]", "[[span-is-the-mint-unit-not-the-read-filter]]"]
---

# A natural key must identify a row, never describe it

**Decision.** `source_ref` composes from the Facet Definition's declared `identity_key` — the fields that
say **which** row this is — and from nothing else.

```text
   bp-csv:2026-01-24T06:30:00                     ← identity
   bp-csv:2026-01-24T06:30:00:118/76/61           ← identity braided with content  ✗
```

`facet_id = uuidv5(namespace, source_ref)`. `content_hash` decides whether the row *changed*.

> **Identity says which fact. The hash says whether it changed.** Braid them and you lose the ability to
> say a fact changed at all.

## The bug this was found as

`GLOSSARY.md` shipped the second form, values and all, as its worked example. Follow it and the
supersession machinery **silently stops existing** for that source:

1. A blood-pressure reading is corrected: `118/76` → `121/76`.
2. The values are in the natural key, so `source_ref` changes.
3. So `facet_id` changes.
4. So the corrected reading enters as a **brand-new facet**, and the original is **orphaned** — still
   live, still on the timeline, never superseded.

The gate's rule *"same `source_ref`, different `content_hash` → supersede"* can never fire, because the
antecedent is unsatisfiable: any change to the content changes the key. Anam would show both readings,
forever, with no record that one replaced the other. Nothing errors. Every test passes.

## Why it is easy to write

A natural key made of *everything that identifies a row in practice* feels safer — more specific, less
likely to collide. That intuition is right for a **deduplication** key and exactly backwards for an
**identity** key. Deduplication asks *"have I seen these bytes?"*; identity asks *"is this the same
thing?"* Only the second admits the possibility that a thing has **changed**.

> A key built from the values can only ever tell you *this is new*. It can never tell you *this is the
> same thing, differently*.

This is the same distinction as [[canonicalisation-fixes-serialisation-not-values|serialisation versus
value]], moved up a level: there, the bytes had to be pinned so the *content* could be compared; here,
the identity must be pinned so the *change* can be seen.

## What it earns

Because a superseding artifact is the same source row re-read, it necessarily carries the same
`event_time`. So a logical facet **never changes bucket**, which is what lets Anam checkpoint on
event-time and compose the lifelong view as a disjoint union
([[span-is-the-mint-unit-not-the-read-filter]]). An identity rule paid for a storage property, two
sections away.

It also settles an open thread from genesis: **`identity_key` belongs in the Definition's `meta`**, and
`source_ref` composes from it. The definition owns identity. Content owns nothing.
