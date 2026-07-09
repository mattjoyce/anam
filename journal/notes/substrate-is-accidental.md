---
id: 202607100018
type: decision
title: "The substrate is accidental complexity, so it must stay reversible"
created: 2026-07-10
status: evergreen
tags: [anam, decision, storage, dynamodb, architecture, brooks]
project: anam
related: ["[[brooks-below-the-gate-beck-above-it]]", "[[fixed-width-timestamps-lexical-order]]", "[[reference-implementation-mint-cli]]"]
---

# The substrate is accidental complexity, so it must stay reversible

**Decision.** DynamoDB stays. But **only `internal/store` may import the AWS SDK**, no SDK type or
sort-key string escapes that package, and `scripts/check.sh` enforces the boundary mechanically.

## Brooks's triage, applied honestly

Anam's **essential** complexity — the difficulty inherent in the problem — is: two clocks (event-time
against record-time), an append-only fold deriving current status, idempotent admission of re-read source
rows, verbatim payloads, and the whole-person read as the default ([[the-inversion]]).

Almost everything else built so far is **accidental** — difficulty attending our chosen approach, not the
problem:

| Mechanism | Exists because of |
|---|---|
| [[fixed-width-timestamps-lexical-order\|fixed-width timestamps]] | DynamoDB sorts sort keys as strings |
| single-table modelling, the `kind` discriminator | DynamoDB has no joins |
| GSI1 / GSI2 | DynamoDB cannot query a non-key attribute |
| the marshalling envelope | DynamoDB's attribute-value encoding |
| `TransactWriteItems` | DynamoDB's atomicity unit |

Anam is a **single-user, single-partition, local** store. `AGENTS.md`'s own database default is
`database/sql` with real SQLite testdata, which would supply ordering, transactions, and range queries
for nothing, and would delete every row of that table.

## Why DynamoDB stays anyway

Because the accidental/essential distinction is a claim about *cost*, not a command to rewrite. Three
real values sit on the DynamoDB side: Anam re-instantiates **Arch** on Arch's substrate;
[[reference-implementation-mint-cli|`mint-cli`]] is a working precursor that materially de-risks the
store; and a spike against the live endpoint confirmed transactions, conditional writes, GSI queries,
key ordering, and opaque Binary payloads all behave.

## The actual requirement

Brooks's instruction is not *switch substrate*. It is: **do not pay for accidental complexity twice.**
Pay once, in one package, and keep the receipt.

> Accidental complexity that cannot be reversed has been promoted to essential by neglect.

So the domain — `facet`, the gate, the fold, the projections — must not know DynamoDB exists. Swapping
the substrate should mean writing one new store, not touching the domain. That is cheap to hold now and
ruinous to retrofit, which is exactly why the boundary check went into the pre-merge gate *before* the
store was written rather than after.

## Where it has already leaked

`clock`'s fixed-width layout exists purely because DynamoDB sorts strings — accidental complexity that
has already crossed into a domain-adjacent package. It is harmless (the format is correct on its own
merits, and a fixed-width UTC timestamp is defensible anywhere) and it is not worth undoing. But it is
worth *naming*, because this is precisely how a boundary erodes: one reasonable exception at a time.
