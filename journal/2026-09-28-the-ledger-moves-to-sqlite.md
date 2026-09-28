---
type: journal
date: 2026-09-28
session: 8
title: "The ledger moves to a SQLite file of its own"
tags: [anam, journal, storage, sqlite, ledger, append-only]
---

# The ledger moves to a SQLite file of its own

Branch `sqlite-ledger`. The ledger package now writes to `ledger.db`. The gate, fold, inspector and
catch-up kept their interfaces. The decision and its reasons are in
[[ledger-is-a-sqlite-file-of-its-own]].

## How the choice was made

Matt's question was whether to hand-roll a key, value, attribute, time store, use SQLite, or use bbolt.
Three of his follow-ups narrowed it.

1. *"Ideally with primary and secondary keys."* Secondary keys are the expensive part of a hand-rolled
   store: every write has to update the record and each index atomically and survive a crash between
   them. That is the datom engine ARCHITECTURE §8 says not to build.
2. *"I think it's json and based on predefined schema."* Half right. The payload is canonical JSON. The
   Definition pins identity, fact-time and required fields, and nothing else: no types, no closed shape
   in lenient mode. So an attribute-grain table could be filled reliably from the envelope and the
   required fields, and not from the payload as a whole.
3. *"A file for the data and another for operations?"* Yes. The operational store's repair is to delete
   its file.

## The design review changed four things before any code

Run through Hickey and Armstrong, as AGENTS.md requires.

- **Batch size is policy, so it moved to the gate.** The ledger has no limit to enforce.
- **The ledger's schema-mismatch error must not advise deletion.** The operational store's does, and
  the two errors would otherwise read alike.
- **Lock contention gets its own sentinel**, `ErrBusy`.
- **A reader must be tested against a ledger whose writer died.** The test copies the file and its
  write-ahead log out from under a live writer and opens the copy read-only.

## A correction to something I said

I told Matt a separate ledger file made backup a plain copy. That is only true with nothing holding the
file open. In WAL mode recent commits can sit in `ledger.db-wal`, and a long-running inspector keeps the
log alive. The example config and ARCHITECTURE §8 now say to use `.backup`.

## Measured

Same three sources, same machine, scratch directory.

| | DynamoDB Local, July | SQLite, this branch |
|---|---|---|
| Ingest, first sight | 5092 envelopes in 49.1s, batched | 6570 envelopes in 4.0s |
| Re-ingest, all no-op | not recorded | 6570 envelopes in 0.8s |
| Full suite, `-race` | budgeted 25 minutes | 17.6s wall |
| Tests skipped without the endpoint | every ledger package | none |

The two ingest rows are not the same input. The Withings source grew from 5092 to 5481 rows, and the
SQLite figure includes the other two sources.

## The equivalence check

`anam serve bp` on the SQLite ledger:

```
served     234 facets  sha256:12af8b15bd032da3ba4477bcb119a1483a5fa8df7dc7c8d149f2b0cbd01a0f8e
```

The July journal recorded `sha256:12af8b15…` for the same 234 facets on DynamoDB. The blood pressure file
has not changed since 11 July, so this is the same source through a different ledger producing the same
served bytes. `receipt verify` returned reproduced and intact.

Per-type counts match July's for the five health types. `health.measurement` is 5481.

A `sqlite3` shell issuing `UPDATE facet` and `DELETE FROM anam_event` against the scratch ledger was
refused both times with the trigger's message.

The inspector ran as one process while ingest ran as another. Eight page loads during and after the
ingest all returned 200, with the watermark climbing from 0 to 6570.

## Not done, on purpose

- **The DynamoDB ledger was not migrated**, and the table on unraid was not touched.
- **No secondary indexes in the ledger file, and no attribute-grain table.** Nothing reads either yet.
  The fact-time, type and anam-time lookups still live in tracking and still catch up by replay.
- **Matt's live `tracking.db` was not touched.** It was derived from the DynamoDB ledger, and its schema
  version no longer matches this build. The first command that opens it will refuse it and say to
  delete it.

## Open, from the Definition review

Found while answering whether Definitions are fit for use. None is fixed on this branch.

- **Identity encoding can collide.** `source_ref` joins identity values with a colon and the values
  contain colons. Length-prefixing fixes it and changes every facet id.
- **Fact-time has no zone.** Zoneless source times are read as UTC, so blood pressure sits about ten
  hours early against Garmin. A named zone is needed; a fixed offset is wrong under daylight saving.
- **Required means present, not filled.** A required field holding null or an empty string passes.
- **Nothing guards a Definition's second version.** A changed identity key would re-key every facet.

The first two change facet ids or fact-times, so they need an empty ledger to take effect. A new
ledger is the cheap moment for them.

## State

- `scripts/check.sh` green: vet, `golangci-lint` 0 issues, gosec clean, every package passing.
- The AWS SDK is out of `go.mod`.
