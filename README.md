# Anam

> *anam*: Irish for *soul*.

A personal, longitudinal record of one person's life, kept as immutable facts. Health readings,
workouts, medicines, lab results, trips, documents and notes land on one timeline, each with where
it came from and when it was true. The current state is always worked out from the facts, never
stored, and nothing is ever overwritten.

This is a personal project, published so others can read it and borrow from it. The sources, units
and time zone in the examples are the author's.

![The Anam inspector showing a month of made-up facts on one timeline, grouped by heart, body, activity, travel and documents](docs/assets/inspector-timeline.png)

*The inspector, served by the daemon, over a month of made-up facts. Each mark is a facet at its
fact time; the green strip is the checkpoints.*

## Ideas

- **The whole-person timeline is the default read.** A trip, an illness or a training block is an
  optional lens over it, never a requirement.
- **Facts accrete.** A correction is a new version beside the old one. `--as-of` shows what Anam
  knew at any earlier moment.
- **Two clocks.** Fact time is when something was true. Anam time is when Anam learned it.
- **One way in.** Every fact arrives through an admission gate that checks it against a
  Definition, so a bad row never enters the record.
- **Proof of what was shown.** A projection prunes the timeline for a reader, and a receipt proves
  exactly what it served.
- **Anam reads no source itself.** Importers do that, and Anam can be rebuilt from them.

## Facet definitions

Every fact is a **facet** of a named type, and each type has a versioned **Definition**: a JSON
Schema, which field names the facet, and which field says when it was true. Definitions are
immutable once published, so a change is a new version. Anam bundles the 27 below, and importers
register more.

### Bundled with Anam

| Type | Version | What it records | Keyed by | Fact time |
|---|---|---|---|---|
| `health.blood-pressure` | 5 | One blood pressure reading, as an Open mHealth body | `date_excel`, `reading` | `date_excel` |
| `health.weight` | 3 | One body weight | `id` | `date` |
| `health.body-composition` | 3 | A period's body metrics (fat, muscle and the rest) | `group`, `period_type`, `period_start` | `period_start` |
| `health.heart-rate` | 3 | A period's heart rate metrics | `group`, `period_type`, `period_start` | `period_start` |
| `health.heart-rate-alert` | 3 | One abnormal high heart rate alert from a watch | `source`, `kind`, `fact_time` | `fact_time` |
| `health.sleep` | 3 | A period's sleep metrics | `group`, `period_type`, `period_start` | `period_start` |
| `health.metric-group` | 3 | A period's metrics for the health groups with no type of their own | `group`, `period_type`, `period_start` | `period_start` |
| `exercise.session` | 5 | One session of exercise, as an IEEE 1752 physical activity | `id` | session start |
| `gym.visit` | 4 | One gym visit | `id` | `date` |
| `med.started` | 4 | A medicine course starting | `id` | `date` |
| `med.dose-changed` | 4 | A change of dose within a course | `id` | `date` |
| `med.stopped` | 3 | A course ending | `id` | `date` |
| `med.missed-dose` | 4 | A dose the person says they missed | `id` | `date` |
| `med.compliance-estimate` | 4 | The person's own estimate of how well they kept to a course | `id` | `date` |
| `condition.noted` | 4 | A condition named in a document | `id` | `date` |
| `illness.started` | 3 | The day an illness began | `id` | `date` |
| `illness.ended` | 3 | The day an illness ended | `id` | `date` |
| `lab.panel` | 4 | One pathology request, the header of a draw's report | `id` | `collected` |
| `lab.result` | 4 | One analyte's result from one draw | `id` | `collected` |
| `donation.blood` | 4 | One visit to give blood | `id` | `date` |
| `visit.clinic` | 4 | One appointment with a clinic or practitioner | `id` | `date` |
| `travel.departed` | 4 | Leaving home on a trip | `id` | `date` |
| `travel.flight` | 4 | One flight leg | `id` | `date` |
| `travel.arrived` | 4 | Arriving at one place during a trip | `id` | `date` |
| `travel.returned` | 4 | Getting home from a trip | `id` | `date` |
| `note.annotation` | 3 | A note in the person's own words, about some facets or none | `id` | `at` |
| `doc.reference` | 3 | A pointer to one file in the documents library; the bytes never enter the ledger | `path` | `date` |

### Registered by importers

These come with [`anam-importers`](https://github.com/mattjoyce/anam-importers), which publishes
them to the daemon on first use.

| Type | Version | What it records | Importer |
|---|---|---|---|
| `health.measurement` | 3 | One Withings measurement | withings |
| `health.step-count` | 1 | One day's steps, as an Open mHealth body | garmin |
| `health.heart-rate-daily` | 1 | One figure for a day's heart rate, as an Open mHealth body | garmin |
| `sleep.episode` | 1 | One sleep episode, as an IEEE 1752 body | garmin |
| `garmin.day` | 1 | One day of a Garmin watch's daily summary | garmin |
| `sleep.cpap-session` | 1 | One therapy session from a DreamStation card | dreamstation |
| `location.track` | 1 | One UTC hour of a phone's positions | owntracks |
| `location.stay` | 1 | One place a phone stayed | owntracks |

The bundled files are in [`internal/def/manifests/`](internal/def/manifests/), and a running
daemon lists what it holds at `GET /v1/definitions` and in the inspector's definitions page.
Adding a type is a Definition file, not code.

## Getting facts in

Every source reaches Anam through an importer, in the sibling
[`anam-importers`](https://github.com/mattjoyce/anam-importers) repository. An importer reads one
source (Garmin, Withings, a blood pressure export, a DreamStation card, OwnTracks, Concept2),
shapes its rows to a Definition, and posts them to the daemon under its own token. Two formats
are Anam's own:

- **`records`**: hand-written YAML records for labs, medicines, travel and notes.
- **`documents`**: a folder of files with sidecars, each a `doc.reference`. The importer posts the
  bytes and Anam keeps them in its library.

The importers are plugins for the [ductile](https://github.com/mattjoyce/ductile) gateway, a small
scheduler and event router, but that is only how the author runs them. Each is a script that can be
run by hand, and anything that can post an envelope can feed Anam (`docs/IMPORTERS.md`).

## Letting an AI read it

An AI reaches the record through the daemon's API, and there are three ways to do that.

- **MCP.** [`anam-mcp`](https://github.com/mattjoyce/anam-mcp) is an adapter for any MCP client. It
  offers a few tools (describe the record, describe a type, read the timeline, verify a receipt)
  that cite their evidence, and one to save the person's own words as a note.
- **The CLI.** The `anam` command talks to the daemon when `ANAM_SERVER` is set, so a local model
  with a shell tool can run `anam timeline` and `anam changes` and read the output.
- **`curl`.** The API is plain HTTP and JSON, documented in [`docs/API.md`](docs/API.md). A skill for
  Claude Code, [`.claude/skills/anam-service`](.claude/skills/anam-service/SKILL.md), teaches the
  routes and the checks.

```sh
export ANAM_SERVER=http://127.0.0.1:8787 ANAM_TOKEN_FILE=~/.config/anam/token ANAM_ZONE=Australia/Sydney

anam timeline --from 2026-09-17 --to 2026-10-17
anam changes --since 2026-10-01

curl -H "Authorization: Bearer $(cat $ANAM_TOKEN_FILE)" \
  "$ANAM_SERVER/v1/timeline?zone=$ANAM_ZONE&type=health.blood-pressure"
```

Reads are narrowed by role. A token sees only what its role allows, and an importer's token sees
only its own source's snapshots.

## Quick start

You need Go (see `go.mod` for the version). Nothing here needs a server, a token or any other
machine; everything runs on loopback against two SQLite files.

```sh
make build

# person_id has no default on purpose: it is pinned into the ledger at init, so a
# defaulted one would name a person nobody chose for the life of the file.
export ANAM_PERSON_ID=me

# The ledger and the operational store are two SQLite files. Both default to
# ~/.local/share/anam/ (ledger.db and tracking.db); override with
# ANAM_LEDGER_PATH and ANAM_DB_PATH.
./anam init

# Run the daemon. On loopback it needs no token. Leave it running and use another terminal.
./anam daemon
curl http://127.0.0.1:8787/healthz                 # ready
ANAM_SERVER=http://127.0.0.1:8787 ./anam status   # the ledger id, W, and each importer's last batch
```

Open `http://127.0.0.1:8787/` for the inspector. If `ANAM_SERVER` is set in your shell, `init` and
`daemon` refuse to run, because a configured server is treated as the owner of the files. Unset it
for those two commands.

**Read.** The whole-person timeline is the default, and everything else is a lens over it.

```sh
./anam timeline                                    # the canonical, whole
./anam timeline --from 2025-05-01 --to 2026-07-01  # narrowed by fact time
./anam changes --since 2026-07-10                  # what Anam learned since, by Anam time
```

**Publish and prove.** A projection prunes the canonical, and a receipt proves what was served.

```sh
./anam serve                        # the menu: you select a projection, you cannot define one
./anam serve bp                     # publish: a served value and a receipt, content-addressed
./anam receipt list
./anam receipt show <hash>          # what was served, from the artifact alone
./anam receipt verify <hash>        # re-derive it from the ledger, and re-hash the file
```

**Check a projection before publishing it**, and ask why a facet is in or out.

```sh
./anam timeline --projection bp                                # the served view, with its receipt hash
./anam timeline --projection bp --explain                      # every facet's verdict, and the reason
./anam timeline --projection bp --as-of 2026-07-10T20:00:00Z   # what it stood on when Anam knew less
```

**Mint.** A checkpoint caches the fold, one value per monthly bucket, to bound replay.

```sh
./anam checkpoint --dry-run         # which buckets would mint, and to what hash
./anam checkpoint                   # mint every bucket whose value moved
./anam checkpoint list              # every checkpoint ever minted
```

## How it fits together

```text
importers (envelopes, one token per source) → admission gate → append-only ledger (SQLite)
    → checkpoints → the fold → projections and the CLI → receipts
```

The **ledger** is the record. It is a SQLite file whose triggers refuse any update or delete. A
second SQLite file holds what Anam itself authors, and it can be rebuilt by replaying the ledger.
Facts reach the ledger only through the gate, which checks each one is compliant with its
Definition, authenticated, and associated with the person.

## Docs

- [`docs/PRFAQ.md`](docs/PRFAQ.md): what Anam is and why. Start here.
- [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md): the model, contracts and mechanics.
- [`docs/explainer/explainer.html`](docs/explainer/explainer.html): a checked, one-concept-per-file
  tour of the code, rendered as a page.
- [`docs/API.md`](docs/API.md): the HTTP API.
- [`docs/GLOSSARY.md`](docs/GLOSSARY.md): the shared language.
- [`docs/IMPORTERS.md`](docs/IMPORTERS.md): how sources outside Anam feed it, and the contract each
  importer follows.
- [`docs/MVP.md`](docs/MVP.md): the first slice, its scope and success criteria.
- [`docs/SELF-LOG.md`](docs/SELF-LOG.md): the self-log, documents as references, and the snapshot read.
- [`docs/INBOX.md`](docs/INBOX.md): from a dropped document to facts in the ledger.
- [`docs/HISTORY.md`](docs/HISTORY.md): where Anam came from. Optional reading.
- [`AGENTS.md`](AGENTS.md): engineering standards and the design lens the code is reviewed through.

## Status

A working prototype that holds one person's record and is in daily use. Expect the code to change
and old data to be rebuilt rather than migrated.

## Licence

Apache-2.0, see [`LICENSE`](LICENSE). Third-party material, the IEEE 1752 schemas bundled for
conformance tests, is listed in [`NOTICE`](NOTICE).
