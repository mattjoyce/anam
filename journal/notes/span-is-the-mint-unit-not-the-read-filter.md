---
id: 202607100020
type: decision
title: "Minting is per span; the read scope and the mint scope are different concerns"
created: 2026-07-10
status: evergreen
tags: [anam, decision, mint, span, checkpoint, brooks]
project: anam
related: ["[[mint-unit-whole-timeline]]", "[[the-inversion]]", "[[state-is-checkpoint-plus-replay]]", "[[source-ref-is-identity-not-content]]"]
---

# Minting is per span; the read scope and the mint scope are different concerns

**Decision.** Anam mints **per span, never whole-life** (Arch ADR 0033). Two tiers, deliberately given
different names so they cannot be confused again:

| Tier | What it is | Minted? |
|---|---|---|
| **Bucket span** | a calendar window over **event-time** (default: one month). Every facet is a member of exactly one, by arithmetic on its `event_time`. **Invisible to the reader** — nobody asks for it. | **yes** — the unit of checkpointing |
| **Derived span** | an interval the person names: `[from, to]`, a saved bookmark, or "everything" | **never.** Composed on demand from bucket values; materialised only when a receipt is served |

The whole-person read stays Anam's default ([[the-inversion]]). It is a **derived span**, composed from
bucket checkpoints plus the open bucket's replay. It never re-folds the person.

## The distinction

> The scope you **read** and the scope you **materialise** are different questions. A reader wants a
> life; a fold wants a bound.

Nothing forces them to agree, and a system that lets the read scope dictate the mint scope will fold an
unbounded set to answer a bounded question. Anam did exactly that for a day — see
[[mint-unit-whole-timeline]], retracted.

A bucket span is not a lens, a filter, or a concept the person ever meets. It is the *unit of
materialisation*, chosen by arithmetic, invisible at the surface. Which is why calling both things
"span" was the whole problem.

## Anam buckets on event-time, where Arch buckets on record-time

Arch's bucket is a *storage-time* window (*"arithmetic on its record-time, no lookup"*), which guarantees
a sealed bucket never receives a new member — new facets always land in the open one.

Anam can do better, and the reason is [[source-ref-is-identity-not-content|the identity rule]]:
`facet_id = uuidv5(source_ref)`, and a superseding artifact is **the same source row re-read**. Same
`source_ref` ⇒ same `event_time` ⇒ **the same bucket**. So *every version of a logical facet lives in one
bucket*, and:

- the lifelong view is a **disjoint union** of bucket values — no cross-bucket version resolution;
- a checkpoint aligns with the **time-bounded question a person actually asks** ("that training block",
  "the month I was ill"), which record-time buckets never would.

The cost: a **backfilled** facet — an old `event_time` ingested today — re-opens its sealed bucket and
re-mints it. One bucket, a few hundred facets. Garmin backfills routinely. This is cheap, and it is
correct, and it is the trade worth making.

## Why "everything" is a poor span

It is expressible, and nearly useless. Enquiry is almost always time-bounded — a trip, an illness, a
training block — and a whole-life checkpoint bounds no replay at all: it *is* the replay. A life is what
you **compose**, not what you **mint**.
