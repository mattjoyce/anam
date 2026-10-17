# Anam

> *anam* — Irish for *soul*.

A personal, longitudinal **episodic-context store** about one person. It is one person's
record of their own life (health, documents, places, notes) held as immutable facts, with the
present state derived from them. This is a personal project, published so others can read it and
borrow from it. The sources, units and time zone in the examples are the author's.

The whole-person timeline is the default read. An Episode (a trip, an illness, a training block)
is an optional lens over it, never a requirement.

The parts: the **Facet** held verbatim, an append-only **ledger**, two clocks (fact-time and
anam-time), a resolve→commit **admission gate**, **mint**/checkpoint, **fold-on-read**, prune-only
**projections**, and **receipts**. Current state is always derived from the facts, never stored.

## Status

Prototype. First slice proves the vertical path end-to-end against real data:

```
importers (envelopes, one token per source) → ingest (gate) → append-only ledger (SQLite)
    → mint → canonical fold → projection + CLI viewer → receipt
```

## Sources

Every source reaches Anam through an importer in the sibling
[`anam-importers`](https://github.com/mattjoyce/anam-importers) repository, a plugin for the
[ductile](https://github.com/mattjoyce/ductile) gateway (a small scheduler and event router, the
author's own) that shapes one source to a Facet Definition and posts envelopes under its own
token (`docs/IMPORTERS.md`). Ductile is only how the author runs them; an importer is a script
that can be run by hand, and anything that can post an envelope can feed Anam: Garmin, Withings, the blood pressure export, the DreamStation card,
OwnTracks, and two formats of Anam's own:

- **`records`** — hand-written YAML records: labs, medicines, travel, notes (`anam_records`).
- **`documents`** — a folder of files with sidecars, each a `doc.reference`; the importer posts
  the bytes and Anam keeps them in its library (`anam_documents`).

Anam reads no source itself.

Extensible to "all sorts of facets" later — `calendar.event`, `mail.thread`, `note.zettel`, …

## Docs

- [`docs/PRFAQ.md`](docs/PRFAQ.md) — what Anam is and why (start here).
- [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) — the model, contracts, and mechanics.
- [`docs/GLOSSARY.md`](docs/GLOSSARY.md) — the shared language.
- [`docs/MVP.md`](docs/MVP.md) — the first slice: scope and success criteria.
- [`docs/SELF-LOG.md`](docs/SELF-LOG.md) — the next sources: the self-log, documents as references, and the snapshot read.
- [`docs/INBOX.md`](docs/INBOX.md) — the runbook: from a dropped document to facts in the ledger.
- [`docs/IMPORTERS.md`](docs/IMPORTERS.md) — proposal: sources outside Anam, as ductile plugins shaped by Definitions.
- [`AGENTS.md`](AGENTS.md) — engineering standards and the guiding design lens.

## Quick start

You need Go (see `go.mod` for the version). Nothing here needs a server, a token or any other
machine; everything below runs on loopback against two SQLite files.

```sh
make build

# person_id has no default on purpose: it is pinned into the ledger at init, so a
# defaulted one would name a person nobody chose for the life of the file.
export ANAM_PERSON_ID=matt

# The ledger and the operational store are two SQLite files. Both default to
# ~/.local/share/anam/ (ledger.db and tracking.db); override with
# ANAM_LEDGER_PATH and ANAM_DB_PATH.
./anam init

# Run the daemon. On loopback it needs no token. Leave it running and use another terminal.
./anam daemon
curl http://127.0.0.1:8787/healthz                 # ready
ANAM_SERVER=http://127.0.0.1:8787 ./anam status   # the ledger id, W, and each importer's last batch
```

If `ANAM_SERVER` is set in your shell, `init` and `daemon` refuse to run, because the commands
treat a configured server as the owner of the files. Unset it for the commands above.

**Admit.** Facts arrive through `anam daemon`, from importers posting to `POST /v1/envelopes`,
each under its own token (`importers_file`, `docs/IMPORTERS.md` §8).

**Read.** The whole-person timeline is the default; everything else is a lens over it.

```sh
./anam timeline                                    # the canonical, whole
./anam timeline --from 2025-05-01 --to 2026-07-01  # narrowed by fact-time
./anam changes --since 2026-07-10                  # what Anam learned since, by anam-time
```

**Publish and prove.** A projection prunes the canonical; a receipt proves what was served.

```sh
./anam serve                        # the menu — you select a projection, you cannot define one
./anam serve bp                     # publish: a served value and a receipt, content-addressed
./anam receipt list
./anam receipt show <hash>          # what was served, from the artifact alone
./anam receipt verify <hash>       # re-derive it from the ledger, and re-hash the file
```

**Mint.** A checkpoint is a cache of the fold, one value per monthly bucket, there to bound replay.

```sh
./anam checkpoint --dry-run         # which buckets would mint, and to what hash
./anam checkpoint                   # mint every bucket whose value moved; unchanged ones are left alone
./anam checkpoint list              # every checkpoint ever minted, by bucket and version
```

**Look at a projection before publishing it**, and ask why a facet is in or out:

```sh
./anam timeline --projection bp                          # the served view, with its receipt hash
./anam timeline --projection bp --explain                # every facet's verdict, and the reason
./anam timeline --projection bp --as-of 2026-07-10T20:00:00Z   # what it stood on when Anam knew less
./anam inspect                                           # the web harness; /projection switches templates
```

## Licence

Apache-2.0, see [`LICENSE`](LICENSE). Third-party material, the IEEE 1752 schemas bundled for
conformance tests, is listed in [`NOTICE`](NOTICE).
