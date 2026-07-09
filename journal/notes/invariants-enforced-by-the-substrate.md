---
id: 202607100019
type: decision
title: "The enforcement ladder — push each invariant down until something else holds it"
created: 2026-07-10
status: evergreen
tags: [anam, decision, invariants, sqlite, types, testing, armstrong]
project: anam
related: ["[[invariants-as-structural-tests]]", "[[substrate-is-accidental]]", "[[canonicalisation-fixes-serialisation-not-values]]", "[[guiding-lens-hickey-armstrong]]"]
---

# The enforcement ladder — push each invariant down until something else holds it

An invariant can be *stated* in several places, and they are not equally strong. Ranked by how little
they depend on future discipline:

| Rung | Enforced by | Catches a violation | Binds |
|---|---|---|---|
| 1. A doc comment | nobody | never | nobody |
| 2. A test | CI | when the test runs | code paths a test exercises |
| 3. A type | the compiler | at build | all our code |
| 4. A database constraint | the substrate | at write, always | *every writer, including a `sqlite3` shell* |

**Push each invariant as far down that table as it will go.** Each rung removes a class of future
mistake that no amount of care removes.

## The same invariant at four rungs

Anam's own history is a worked example.

- *"The payload is verbatim and canonical"* began as a comment on `ContentHash`, which happily accepted
  raw bytes and returned a plausible, wrong digest. It became a **type** —
  [[canonicalisation-fixes-serialisation-not-values|`Canonical`]], a struct wrapping unexported bytes —
  and `ContentHash(rawBytes)` stopped compiling. Rung 1 → 3.
- *"A Facet has no current status"* is a claim about a struct's **shape**, so it became a reflection test
  that fails if a `Status` field ever appears ([[invariants-as-structural-tests]]). Rung 1 → 2, which is
  as far as it goes: Go cannot forbid a field.
- *"Facts are never modified, never deleted"* was doctrine, then a hope, and is now a `BEFORE UPDATE` /
  `BEFORE DELETE` trigger that calls `RAISE(ABORT)`. Rung 1 → **4**.
- *"Re-ingesting an unchanged row is a no-op"* was an atomic two-item write with a conditional check on a
  separate marker row. It is now `UNIQUE(source_ref, content_hash)`. Rung 3-ish → **4**, and an entire
  mechanism disappeared with it.

## Why rung 4 is different in kind

Rungs 2 and 3 bind *our code*. Rung 4 binds *the data*. A trigger refuses the write no matter who issued
it — a future refactor, a debugging session at the `sqlite3` prompt, a script written in a hurry at
midnight. For Anam's append-only ledger, which is Armstrong's **error kernel** and must not corrupt, that
distinction is the whole game.

This reframes a choice that looks like plumbing. When picking a substrate, do not only ask *what can it
store* and *how fast*. Ask:

> **Which of my invariants can this thing enforce for me?**

That question, not the deleted GSIs, is what settled [[substrate-is-accidental|Anam's substrate]]. An
embedded database that can hold two of your deepest promises is worth more than a distributed one that
holds none of them, when you have no distributed problem.

## The honest caveat

A trigger is not a security boundary. Whoever holds the file can `DROP TRIGGER`. The threat model here is
not an adversary — it is **us, six months from now, mid-refactor, reaching for the convenient thing.**
Against that threat, a constraint the database enforces is strictly better than a rule we remember, and
that is all it needs to be.

Rung 4 also has a cost worth naming: the invariant now lives in two languages, and the schema must be
read to understand the domain. That is a real tax on comprehension, paid once, in exchange for a promise
that cannot quietly lapse.
