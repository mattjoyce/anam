---
type: journal
date: 2026-07-10
session: 4 (continued)
title: "R5 — the big import"
tags: [anam, journal, r5, capture, health-db, ingest]
---

# R5 — the big import

Same session as R4, straight through: the `health-db` capture worker, the bundled definitions,
`anam init` and `anam ingest`, and then the thing itself — **659 facets, one command, one timeline
from 2024-01-01 to 2026-06-06**. Re-running the ingest: `admitted=0 no-op=659`, `W` unmoved. The §7
idempotency criterion, demonstrated on the real source the same hour the worker was written.

## The boundary earned its first carve-out

The substrate confinement (only `internal/store` names DynamoDB/SQLite/`database/sql`) met a source
that *is* a SQLite file. The resolution: `internal/capture` may read a **source** in its native format
— that is the source's substrate, not Anam's — and may never name the ledger's SDK. `check.sh`
enforces both halves. The alternative, hiding the reader inside `internal/store`, would have blurred
the one thing the boundary means: *Anam's* storage is swappable; a source speaks whatever it speaks.

## Decisions the data forced

- **`last_updated` stays out of the payload.** It is the sync pipeline's clock, not a health fact; a
  Garmin re-sync bumps it without changing a value, and carrying it would turn every re-scan into a
  spurious supersession. Content is the values; identity is `(group, period_type, period_start)`;
  `recorded_time` defaults to `event_time`.
- **The driver's lexeme is not the source's.** Scanning a `DATE`-declared column through
  `modernc.org/sqlite` hands back the driver's re-formatting (`…T00:00:00Z`) — a value the source
  never wrote, discovered when the worker's source_ref disagreed with the definition's. `CAST(… AS
  TEXT)` takes the stored bytes. The verbatim rule reaches all the way down into the driver.
- **`bp_*` excluded in the query, not the code** — one `WHERE NOT` clause, tested by asserting the
  aggregate never appears inside a `health.heart-rate` payload. The blood-pressure fact belongs to
  `bp-csv` at its own reading time; the health-db copy is a lossy aggregate with worse provenance.
- **Four manifests, one identity grain.** `/group` pinned by enum per type, so a mis-typed envelope
  fails compliance rather than convention. The strong test: the worker's composed `source_ref` is
  byte-identical to the one each bundled definition composes from the payload — identity-is-
  definition-owned, agreed in advance rather than discovered at the gate.

## Numbers, for the record

7 metric groups → 4 facet types; day periods 2026-03-07→06-06, week/month/year back to 2024-01.
Ingest of 659 envelopes took ~2m53s (sequential commits, ~260ms each against DynamoDB Local over the
LAN) — fine for a scan that will be a no-op almost every run. The whole-person `timeline` needed
`ANAM_TIMEOUT` raised past the 5s default: the R4 read path does a full replay plus one
`CurrentVersion` per live facet, priced honestly until the operational indexes (R6) and bucket
checkpoints (R8) bound it.

## Carried forward

- The read path's cost curve is now measured, which is exactly the evidence mint (R8) was waiting for.
- `sync_cursor` exists in the ops schema but nothing writes it — a full re-scan is honest and cheap at
  this scale.
- **R6 next**: `changes_since` over record-events — the first reader of the operational indexes, so
  catch-up and its `last_seq == W` assertion arrive with it.
- The BP CSV: still missing. `bp-csv`'s scope entry joins `mvpScopes` with its worker.
