---
id: 202607100010
type: decision
title: "Timestamps are fixed-width, or the timeline silently misorders"
created: 2026-07-10
status: evergreen
tags: [anam, decision, time, storage, dynamodb, go]
project: anam
related: ["[[person-substrate-grain]]", "[[mint-unit-whole-timeline]]", "[[invariants-as-structural-tests]]"]
---

# Timestamps are fixed-width, or the timeline silently misorders

**Decision.** Every timestamp Anam stores is formatted with the layout
`2006-01-02T15:04:05.000000000Z07:00` in UTC — always 30 bytes, always nine fractional digits.
**`time.RFC3339Nano` is banned from the codebase.**

## The tension

`ARCHITECTURE.md` §8 rests the whole storage design on one sentence: *"Times are formatted UTC,
fixed-width RFC3339Nano, so lexical order equals chronological order for range queries."* The sort keys
`facet#<event_time>#<facet_id>` and `rec#<record_time>#<seq>` are queried with `BETWEEN`. DynamoDB
compares them as **strings**. If string order ever diverges from time order, a timeline read returns the
wrong window — and returns it *quietly*, as a plausible-looking answer.

The obvious way to write that in Go is `t.Format(time.RFC3339Nano)`. The obvious way is wrong.

## Why the stdlib constant betrays you

Go's `RFC3339Nano` is defined as `...05.999999999Z07:00`, and in a Go layout **the digit `9` means
"elide trailing zeros."** So the format is *variable width*:

| Instant | `RFC3339Nano` | Width |
|---|---|---|
| `06:30:00.0` | `2026-01-24T06:30:00Z` | 20 |
| `06:30:00.5` | `2026-01-24T06:30:00.5Z` | 22 |
| `06:30:00.123456789` | `2026-01-24T06:30:00.123456789Z` | 30 |

Now compare the first two lexically. At index 19 sits `'Z'` (0x5A) against `'.'` (0x2E). So
`"…06:30:00Z" > "…06:30:00.5Z"` — the *earlier* instant sorts **after** the later one. Any two facets
whose event-times differ only in sub-second precision are transposed on the timeline.

Using `0` instead of `9` in the layout forces the zeros to be printed. Fixed width is what makes the
comparison total: a shorter string can never sort against a longer one on a shared prefix.

## The general shape

This is a **complect** in Hickey's sense, hiding inside a stdlib constant: *how a time is displayed* is
braided with *how a time is ordered*. `RFC3339Nano` is designed for the first job — it produces the
shortest faithful rendering, which is right for a log line and wrong for a sort key. The moment a
timestamp becomes a **key**, prettiness must yield to totality.

> A format designed for humans is not a format designed for a comparator. When a serialisation becomes
> an index, ask what its ordering does — not just what it reads like.

Parsing is strict for the same reason: `clock.Parse` rejects `2026-01-24T06:30:00Z` outright. A
timestamp without its nine digits is not one Anam wrote, and accepting it would let a foreign width back
into the keyspace.

## It outlived the substrate

This note was written while Anam stored on DynamoDB, whose sort keys are strings. Anam then moved to
SQLite ([[substrate-is-accidental]]) — and the rule survived unchanged, because event-time is still a
**text column that gets ordered**. The lesson generalises past any one store: the hazard belongs to
*ordered serialised timestamps*, not to DynamoDB.

## Enforcement

`internal/clock` owns `Layout` and `Width`; nothing else formats a time. A test asserts, over every pair
of a set of instants, that `Format(a) < Format(b)` exactly when `a.Before(b)`. Reverting `Layout` to the
stdlib constant fails it — this was checked by
[[invariants-as-structural-tests|actually making the mutation]], not assumed.
