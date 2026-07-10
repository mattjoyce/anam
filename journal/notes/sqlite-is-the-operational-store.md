---
id: 202607102351
type: decision
title: "SQLite is the operational store — three recovery tiers, not a cache"
created: 2026-07-10
status: evergreen
tags: [anam, decision, sqlite, substrate, tiers, rebuild]
project: anam
related: ["[[substrate-is-accidental]]", "[[invariants-enforced-by-the-substrate]]", "[[doctrine-aphorisms-outlive-their-reasoning]]", "[[identity-first-keys-and-no-indexes]]"]
---

# SQLite is the operational store — three recovery tiers, not a cache

**Decision.** SQLite holds whatever operational state Anam needs — definitions, projection templates,
token scopes, the derived indexes, span registry, cursors, bookmarks, logs. It is a real database,
authoritative for what Anam authors. What varies per table is not authority but **the way back**:

| Tier | Recovery |
|---|---|
| **derived** | replay the ledger |
| **provisioned** | embedded assets and config — i.e. git |
| **authored** | a backup, or nowhere |

The tier is annotated on each table in the schema, because it is the only thing that tells a later
reader whether `DROP` is survivable.

## The tension

The doctrine said *"SQLite holds nothing that cannot be rebuilt by replaying the ledger"* — one recovery
source for a store with three, and a demotion of a real database to a cache. Matt rejected it flat:
*"of course sqlite will have data not anywhere else, it's entirely operational."* My first fix was a
sharper universal rule; he rejected that too — *"don't force the rules so hard."* The correct fix was no
rule at all: a per-table fact.

## What survives, and why

One obligation remains, and it is a test's precondition rather than a principle: **the derived tier must
stay provably derived.** The rebuild test (MVP §7) drops it and re-derives from the ledger; if a facet
payload, a record-event, or `W` ever acquired a second home there, the test would pass on data it never
rebuilt, and "Anam owns no source fact" would stop being falsifiable. And because the fold reads the
derived indexes, catch-up must assert `last_seq == W` before folding — behind is fixable, wrong is not
representable.

## What it moved

`def#<type>#v<n>` left the ledger. A Facet Definition is not something Anam *knew*; it is how Anam
*interprets* — governance, not record. Definitions are provisioned rows (every version, so an old
artifact's `definition_version` still resolves). The error kernel got smaller, which is the direction it
should only ever move.
