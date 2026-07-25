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

# person_id has no default on purpose: it is the ledger's partition key, so a
# defaulted one would quietly write facts into a partition nobody chose.
export ANAM_PERSON_ID=matt
export ANAM_DDB_ENDPOINT=http://192.168.20.4:8000   # the ledger (creds are static fakes, in-code)

./anam init
```

**Ingest.** Imports are *declared*, not passed on the command line — a path typed ad hoc
is provenance that evaporates with the shell history. Copy `imports.example.yaml` to
`~/.config/anam/imports.yaml`, name each source and its file, then:

```sh
./anam ingest                 # every declared source
./anam ingest bp-csv          # just one
```

**Read.** The whole-person timeline is the default; everything else is a lens over it.

```sh
./anam timeline                                    # the canonical, whole
./anam timeline --from 2025-05-01 --to 2026-07-01  # narrowed by event-time
./anam changes --since 2026-07-10                  # what Anam learned since, by record-time
```

**Publish and prove.** A projection prunes the canonical; a receipt proves what was served.

```sh
./anam serve                        # the menu — you select a projection, you cannot define one
./anam serve bp                     # publish: a served value and a receipt, content-addressed
./anam receipt list
./anam receipt show <hash>          # what was served, from the artifact alone
./anam receipt verify <hash>       # re-derive it from the ledger, and re-hash the file
```

**Look at a projection before publishing it**, and ask why a facet is in or out:

```sh
./anam timeline --projection bp                          # the served view, with its receipt hash
./anam timeline --projection bp --explain                # every facet's verdict, and the reason
./anam timeline --projection bp --as-of 2026-07-10T20:00:00Z   # what it stood on when Anam knew less
./anam inspect                                           # the web harness; /projection switches templates
```
