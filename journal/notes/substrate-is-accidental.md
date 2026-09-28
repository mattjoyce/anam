---
id: 202607100018
type: decision
title: "The substrate is accidental complexity — so let each part be held by whatever holds it best"
created: 2026-07-10
modified: 2026-07-10
status: evergreen
tags: [anam, decision, storage, sqlite, dynamodb, architecture, brooks]
project: anam
related: ["[[brooks-below-the-gate-beck-above-it]]", "[[invariants-enforced-by-the-substrate]]", "[[state-is-checkpoint-plus-replay]]", "[[reference-implementation-mint-cli]]"]
---

# The substrate is accidental complexity — so let each part be held by whatever holds it best

**Decision.** Anam stores in **three** places, split by role, under one rule:

> **Truth is immutable and lives in exactly one place.** The **ledger** (DynamoDB) is the system of
> record: facets, record-events, and the watermark `W`. The **filesystem** holds published artifacts:
> content-addressed checkpoint values and served receipts. **SQLite** holds nothing that cannot be
> rebuilt by replaying the ledger.

Only `internal/store` may name any of them; `scripts/check.sh` enforces it.

## The note's own history is the argument

This note has been rewritten twice in one day.

1. **"DynamoDB stays, keep the boundary honest."** Brooks's triage listed the accidental costs — GSIs, the
   marshalling envelope, `TransactWriteItems`, a synthesised `seq`, fixed-width keys — and then flinched
   from the conclusion.
2. **"SQLite, everything."** Once the costs were listed the case collapsed: DynamoDB gave Anam nothing
   Anam used, and SQLite could *enforce* two invariants that DynamoDB can only hope for.
3. **Three substrates, split by role** (this version). The ledger wants a datom-shaped indexed store; the
   published artifacts want to be immutable files; the tracking state wants a small mutable database. One
   store was being asked to be three things.

Each rewrite cost one document and no code, because the boundary check went in **before** the store did.
That is the entire practical content of "accidental complexity must stay reversible" — and it was tested
three times in a day.

> Accidental complexity that cannot be reversed has been promoted to essential by neglect.

## What each substrate is actually for

- **DynamoDB — the ledger.** Datom-*shaped*, not a datom *engine* (Arch ADR 0031): facts as
  `[entity attribute value tx]`, the store supplies sorted indexes, Anam supplies the shape and five
  fixed query forms. Grain is **hybrid at the verbatim line**: coarse for the verbatim substrate (one
  item per facet; datoms cover only the envelope; the payload is a single opaque Binary), fine for
  derived facets and consent, where field-level history actually pays. Shredding a verbatim payload into
  per-field triples would impose a schema at ingest — crossing the line the Facet exists to draw.
- **Files — published artifacts.** Checkpoint values (`VALUE#<hash>`) and served receipts. Immutable,
  content-addressed, GC-able when unserved and subsumed. Backed up, not rebuilt.
- **SQLite — tracking.** Per-span `V` and `Wᵥ`, the span registry, the checkpoint and receipt index, sync
  cursors, GC state. All derived. The sync cursor was *already* declared "a cache, not truth"; this
  generalises that status to everything SQLite holds.

## The cost, named rather than discovered

The all-SQLite design had append-only enforced by a `BEFORE UPDATE`/`BEFORE DELETE` trigger — the
database refusing to overwrite a fact, whoever asked. **DynamoDB cannot do this.** Nothing stops an
`UpdateItem`, and DynamoDB Local has no IAM to deny one.

So Anam's deepest invariant drops a rung on the [[invariants-enforced-by-the-substrate|enforcement
ladder]]: from *the substrate refuses* back to *our code must not, and a test checks*. That is the single
largest price of the split, it lands on the error kernel, and it is paid **knowingly**. Writing it down
here is the difference between a trade and an accident.

## Why there is no distributed transaction

`commit` must write the facet artifact, its `admitted` record-event, and the bump of `W` atomically — a
facet with no event is invisible to the fold; an event with no facet is a dangling reference. **There is
no transaction across two databases.** So all three live in DynamoDB, in one `TransactWriteItems`.

SQLite is never written in the same breath as the ledger. It catches up afterwards by replaying
record-events since its last-seen `seq`, which is safe precisely because the ledger is append-only and
`commit` is idempotent:

> A crash leaves SQLite **behind**, never **wrong** — and behind is fixable by replay.

## What made all of this cheap

Because Anam owns no source fact, **a substrate swap is a re-ingest, not a migration.** The cost of being
wrong is a rebuild, in any direction. When reversal is that cheap, the argument for choosing the simplest
arrangement — and for changing your mind the moment the facts change — is overwhelming.

## 2026-09-28: a fourth arrangement

The ledger left DynamoDB for a SQLite file of its own ([[ledger-is-a-sqlite-file-of-its-own]]). The
three roles above still hold. Two of them are now SQLite files, kept apart, and the cost named above is
gone: the database refuses an overwrite again. "A substrate swap is a re-ingest" needs one qualifier the
move exposed. Payloads come back from their sources. Anam-times do not.
