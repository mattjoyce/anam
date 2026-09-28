---
id: 202609282250
type: decision
title: "The ledger is a SQLite file of its own"
created: 2026-09-28
status: evergreen
tags: [anam, decision, storage, sqlite, ledger, append-only, triggers]
project: anam
related: ["[[substrate-is-accidental]]", "[[invariants-enforced-by-the-substrate]]", "[[identity-first-keys-and-no-indexes]]", "[[sqlite-is-the-operational-store]]", "[[state-is-checkpoint-plus-replay]]"]
---

# The ledger is a SQLite file of its own

**Decision.** The ledger moves from DynamoDB Local to `ledger.db`, a SQLite file separate from
`tracking.db`. Three tables: `facet`, `anam_event`, `ledger_meta`. Every table is append-only by trigger.
The watermark is `MAX(seq)`, not a stored counter.

## What was weighed

Three candidates: a hand-rolled key, value, attribute, time store; SQLite; bbolt.

| | Hand-rolled | SQLite | bbolt |
|---|---|---|---|
| Refuses an overwrite or delete | only what our code refuses | yes, by trigger and key | no |
| Inspector reads while ingest writes | we build it | yes, WAL | no, the writer holds an exclusive file lock |
| Atomic artifact and event | we build it, plus crash recovery | one transaction | one transaction |
| Secondary keys | we build index maintenance and recovery | declared | hand-maintained buckets |
| New dependency | none | none, ops already uses it | one |

SQLite won on enforcement. [[invariants-enforced-by-the-substrate]] names the loss of database-held
append-only as the largest price of DynamoDB. SQLite is the only candidate that gets it back. bbolt
failed on process layout: `anam inspect` is a long-running reader and `anam ingest` is a separate writer.

DynamoDB had been kept for its conditional transaction and its counter. Neither survives as a reason
once the store is in-process: a SQLite transaction gives the atomic write, and a gapless `seq` makes the
counter redundant.

## Why a separate file and not more tables in tracking.db

- **The operational store's repair is deletion.** A schema or bucket-width mismatch tells the operator
  to delete `tracking.db` and re-run init. In a shared file that repair would delete the ledger.
- **Read-only is per file.** Commands that only read open `ledger.db` with `mode=ro`, and SQLite refuses
  their writes. They still need to write to tracking, so in a shared file they would hold a connection
  able to insert into ledger tables.
- **The rebuild test stays physical.** Deleting the tracking file and replaying proves the derived tier
  is derived.

The cost: no transaction spans two files, so catch-up by replay stays. One file would let fold state
update inside the write transaction and delete catch-up. That is the main argument the other way.
Merging two files later is easy; splitting one is not.

## What the database now holds

| Invariant | Held by |
|---|---|
| no row updated or deleted | `BEFORE UPDATE` / `BEFORE DELETE` triggers on all three tables |
| no row overwritten by `INSERT OR REPLACE` | `BEFORE INSERT` trigger refusing an existing key |
| `seq` gapless and monotonic | `BEFORE INSERT` trigger: `seq` must be `MAX(seq) + 1` |
| version n sits on version n-1 | `BEFORE INSERT` trigger on `facet` |
| one ledger, one person | `person_id` pinned in `ledger_meta`, checked on every open |

The replace trigger is not redundant with the delete trigger. `INSERT OR REPLACE` removes the old row
without firing delete triggers unless `recursive_triggers` is on, and that pragma is per connection. A
mutation check confirmed it: with the insert trigger removed, the database accepted the replace and the
payload changed.

The version trigger exists because `KnownFacets` reads a missing v1 as "never admitted".

## What disappeared

The 49-item batch cap as a substrate limit, the probe's retry and backoff, `ErrThrottled`, the positional
cancellation mapper and its layout functions, zero-padded sort keys, the seeded counter, the AST test
that forbade mutating API calls, the 25-minute test timeout, and test packages that skipped when the
endpoint was away.

`MaxBatch` stays at 49 and moves to the gate. It is policy now: a batch is one transaction, so its events
share one anam-time. Keeping the number keeps batch boundaries where they were.

## What was added that the move made necessary

**The ledger has an id, and the operational store pins it.** The replay cursor is only a number. A
rebuilt ledger holding the same facts reaches the same head, so the cursor reads an old index as
current, with every anam-time in it belonging to the previous ledger. A test written to show this passed
`Sync` with no error until the pin was added.

## Costs, named

- **Backup is not a plain copy while the file is open.** Recent commits can sit in `ledger.db-wal`. Use
  `sqlite3 ledger.db ".backup <dest>"`.
- **A trigger is not a security boundary.** Whoever holds the file can drop it.
- **A ledger schema change needs a written migration.** The schema-mismatch error says so, and says not
  to delete the file.
- **The existing DynamoDB ledger was not migrated.** A re-ingest restores payloads and stamps new
  anam-times. Moving the old anam-times would mean copying rows out of DynamoDB.
