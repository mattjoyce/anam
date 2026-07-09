---
id: 202607100018
type: decision
title: "The substrate is accidental complexity — so pick the simplest one that enforces the invariants"
created: 2026-07-10
modified: 2026-07-10
status: evergreen
tags: [anam, decision, storage, sqlite, dynamodb, architecture, brooks]
project: anam
related: ["[[brooks-below-the-gate-beck-above-it]]", "[[invariants-enforced-by-the-substrate]]", "[[fixed-width-timestamps-lexical-order]]", "[[reference-implementation-mint-cli]]"]
---

# The substrate is accidental complexity — so pick the simplest one that enforces the invariants

**Decision.** Anam stores in an embedded **SQLite** ledger. Only `internal/store` may name SQLite or
`database/sql`, and `scripts/check.sh` enforces that boundary.

> This note originally concluded *"DynamoDB stays, but keep the boundary honest."* Brooks's triage was
> right and the conclusion was not. Once the accidental costs were listed, the case for paying them
> collapsed. The reasoning is preserved because the reasoning is the point.

## Brooks's triage

Anam's **essential** complexity — inherent in the problem — is: two clocks (event-time against
record-time), an append-only fold deriving current status, idempotent admission of re-read source rows,
verbatim payloads, and the whole-person read as the default ([[the-inversion]]).

Everything below was **accidental** — difficulty attending the chosen approach, not the problem:

| Mechanism | Existed because of | Under SQLite |
|---|---|---|
| single-table modelling, the `kind` discriminator | DynamoDB has no joins | gone — seven tables |
| the marshalling envelope | DynamoDB's attribute-value encoding | gone — `database/sql` rows |
| GSI1 / GSI2 | DynamoDB cannot query a non-key attribute | `CREATE INDEX` |
| `TransactWriteItems` + a separate `sref#` marker item | DynamoDB's atomicity unit | one transaction; a `UNIQUE` constraint |
| a synthesised monotonic `seq` | DynamoDB has no counter | `INTEGER PRIMARY KEY AUTOINCREMENT` |
| timeouts, fail-fast, unreachable-endpoint handling *inside the error kernel* | DynamoDB Local is a network service | a library call |
| [[fixed-width-timestamps-lexical-order\|fixed-width timestamps]] | DynamoDB sorts sort keys as strings | **survives** — text columns still get ordered |

## What decided it

Not the list. Two things the list doesn't show.

**DynamoDB gave Anam nothing Anam used.** Not scale, not durability, not concurrency, not managed
operations — it was a JVM process on a NAS. Every mechanism above existed to satisfy the store, not the
problem. That is the definition of accidental complexity, and there was no compensating benefit to weigh
against it.

**SQLite can enforce two invariants that DynamoDB can only hope for.** Append-only becomes a `BEFORE
UPDATE` / `BEFORE DELETE` trigger; idempotency becomes `UNIQUE(source_ref, content_hash)`. Anam's two
deepest promises stop depending on our vigilance. See
[[invariants-enforced-by-the-substrate]] — this, not the deleted GSIs, is the real win.

## Why the decision was cheap

Because **Anam owns no source fact, a substrate swap is a re-ingest, not a migration.** The cost of being
wrong is a rebuild, in either direction. When the cost of reversal is that low, the argument for choosing
the *simplest* thing is overwhelming — and the cheapest moment to choose is before any store code exists,
which is exactly when the question got asked.

There were real values on the DynamoDB side: fidelity to **Arch**, whose substrate it is;
[[reference-implementation-mint-cli|`mint-cli`]] as a working precursor; and a spike proving it all
worked. They were not nothing. They were just not *reasons the domain needed it* — and one of them, a
stated interest in learning DynamoDB, is a legitimate personal goal that should be named as such rather
than dressed up as a design decision. As a design decision it does not survive Brooks.

## The rule that remains

> Accidental complexity that cannot be reversed has been promoted to essential by neglect.

Pay for the substrate once, in one package, and keep the receipt. The domain — `facet`, the gate, the
fold, the projections — must not know what it is stored in. That check went into the pre-merge gate
*before* the store was written, which is why changing substrate cost a rewrite of one document rather
than a rewrite of the system.
