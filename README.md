# Anam

> *anam* — Irish for *soul*.

A personal, longitudinal **episodic-context store** about one person. Anam takes the
[Arch](../Arch) architecture doctrine — a person-centric, append-only, evidence-backed context layer —
and **inverts its one central decision** to serve a person rather than a hospital episode.

- **Arch** person-partitions storage but makes every read *filtered by an Episode span (default &
  required)*, so it can never become a longitudinal record.
- **Anam wants to be that longitudinal record.** The whole-person timeline read is the **default and
  normal** mode; an Episode/Span (a trip, an illness, a training block) is an *optional* lens.

Everything else in Arch carries over: the **Facet** held verbatim, an append-only **ledger**, two
clocks (event-time / record-time), a resolve→commit **admission gate**, **mint**/checkpoint,
**fold-on-read**, prune-only **projections**, and **receipts**.

## Status

Prototype. First slice proves the vertical path end-to-end against real data:

```
capture (health-db / bp-csv) → ingest (gate) → append-only ledger (DynamoDB)
    → mint → canonical fold → projection + CLI viewer → receipt
```

## Sources

- **`health-db`** — Garmin + Withings, unified into `healthdata.db` (an existing ETL). Facets:
  `health.sleep`, `health.body-composition`, `health.heart-rate`, `health.metric-group`.
- **`bp-csv`** — a blood-pressure export. Facet: `health.blood-pressure`.

Extensible to "all sorts of facets" later — `calendar.event`, `mail.thread`, `note.zettel`, …

## Docs

- [`docs/PRFAQ.md`](docs/PRFAQ.md) — what Anam is and why (start here).
- [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) — the model, contracts, and mechanics.
- [`docs/GLOSSARY.md`](docs/GLOSSARY.md) — the shared language.
- [`docs/MVP.md`](docs/MVP.md) — the first slice: scope and success criteria.
- [`AGENTS.md`](AGENTS.md) — engineering standards and the guiding design lens.

## Quick start

```sh
make build
export ANAM_DDB_ENDPOINT=http://192.168.20.4:8000   # the ledger (creds are static fakes, in-code)
./anam init
./anam ingest health --db /path/to/healthdata.db
./anam ingest bp --csv /path/to/blood-pressure.csv
./anam timeline --from 2025-05-01 --to 2026-07-01
```
