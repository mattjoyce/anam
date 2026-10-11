# Importers — sources outside Anam

Design proposal · 2026-10-08. The service's side is built (cards 51, 52, 53); §8 is the contract
every importer complies with. Importers now exist for Withings (card 54), blood pressure (card 80)
and the DreamStation (card 70), in `anam-importers`; the built-in `withings` worker is gone.

Anam's capture workers for vendor sources (`healthdb`, `withings`, `bpcsv`) were compiled into the
binary that owns the ledger. `withings` (card 54) and `bpcsv` (card 80) have left; `healthdb`
remains until card 55 finishes. This proposal moves them out: an importer is a ductile plugin that
shapes one source's data to a Facet Definition and submits it to the Anam service. Anam keeps the
Definitions, the gate and the record, and knows nothing about any vendor.

## 1. Where this comes from

Senechal came first: one system that fetched on a schedule, stored, summarised and answered. It was
cleaved twice. **Ductile** took "when to do things": a fancy cron, which grew webhooks and file and
folder watchers. **healthdata.db** and its ETL took "what was measured", meant to be the source of
record itself, which is why it joins Garmin and Withings and stores daily, weekly and monthly
summaries. **Anam** succeeds healthdata.db, as the record of what was learned and when.

Each cleave pulled one concern out, and each left a braid behind. The ETL still fetches, normalises,
joins and summarises in one place. Anam inherited reading the ETL's output, so its binary carries
vendor table layouts. As of 2026-10-08 Anam is healthdata.db's only consumer.

The idea before Anam was external importers that receive, pull or watch folders, and use Facet
Definitions to shape payloads. The `records` worker already works that way: it asks the Definition
how to compose identity and knows no source. The vendor workers drifted from it because sharing a
binary made the drift free. [ARCHITECTURE.md](ARCHITECTURE.md) §2 already calls capture workers
"a pluggable client, not part of the kernel"; this makes the code agree.

## 2. The shape

```text
 source ──trigger──▶ ductile pipeline ────────────────────────────▶ Anam service
                     ┌──────────────┐   ┌──────────────┐            ┌──────────────────┐
  webhook            │ source step  │   │ anam_submit  │  envelopes │ gate (3 pillars) │
  file/folder watch ─▶ reads native ├──▶│ one plugin,  ├───────────▶│ Definitions      │
  schedule / poll    │ shapes to a  │   │ every source │  token per │ ledger           │
                     │ Definition   │   └──────────────┘  source    └──────────────────┘
                     └──────────────┘
```

- **Ductile runs importers.** Its webhooks, watchers and schedules are the three ways the original
  importers took data in. It decides when, and where the result goes. It holds no observation as its
  own record; its `plugin_facts` stay operational state about runs (cursors, last seen).
- **A source step knows one vendor.** It reads the native format, shapes rows to the Definition it
  targets, and does not join or summarise. Garmin's quirks live in the Garmin importer. Write it in
  whatever the source's ecosystem favours; for Garmin and Withings that is Python.
- **`anam_submit` is one generic plugin.** It posts envelopes under the importer's token. It knows
  Anam's endpoint and nothing about sources.
- **Anam owns the Definitions, the gate and the record.** Joining is the timeline. Weekly and monthly
  views are projections or derived facets (ARCHITECTURE.md §11), not admitted facts: a weekly
  average is a computation, not something that happened.

Two drift checks fall out of the lineage. Ductile holding schemas or derived data is Senechal growing
back on that side. Anam scheduling, polling or knowing a vendor's tables is the same on this side.

## 3. Prerequisites in Anam

These make an importer correct in any language, without importing Anam's code.

1. **The gate composes identity.** Done (card 51). The gate composes `source_ref` from the
   Definition's `identity_key`; an envelope may leave it out, and one that carries it is refused on a
   mismatch. The identity rule lives once, in the Definition, and importers only shape payload fields.
   The built-in workers no longer compose it.
2. **A Definition's schema is standard JSON Schema.** Done (card 52). The `schema` block is JSON
   Schema draft 2020-12, so an importer validates locally with a stock library (Python's
   `jsonschema`: `jsonschema.validate(payload, manifest["schema"])`) before sending, and the gate
   runs the same check. The Anam-specific
   contract (`identity_key`, `time_semantics`, classification, validation mode) stays in `meta`.
   Health measures conform to a pinned [Open mHealth](https://www.openmhealth.org/) (IEEE 1752)
   schema where one exists, decided on 2026-10-08
   ([journal note](../journal/notes/open-mhealth-is-the-payload-vocabulary-for-health-measures.md)). The first is `exercise.session` (card 56): an OMH `physical-activity`
   payload, one type for Technogym, Strong and later Garmin, each shaping to the same Definition.
3. **Validation becomes real.** Done (card 52). Every schema is closed: it declares each field with
   its type and refuses anything else, and `lenient` is gone. Once outside code writes payloads,
   facet-compliance is the guard, and each Definition states exactly what a valid payload is.
4. **Two endpoints on the service.** Done (card 53); §8 is the contract.
   - `GET /v1/definitions[/{type}[/{version}]]`: the published contract an importer shapes to.
   - `POST /v1/envelopes`: a batch, each envelope naming its type and Definition version, admitted
     through the gate. Per-source tokens scoped to the types a source may submit replace the static
     `mvpScopes` map: the authenticated pillar as credentials, as Splunk's HTTP Event Collector
     pins a token to its sourcetypes.
5. **Registering a Definition** (`POST /v1/definitions`). Done (card 53). It checks a new version against the
   compatibility rule `def.Guard` already enforces (no re-keying, no re-placing in time), and a
   registered version is immutable, as in a schema registry. Installing an importer registers the
   Definitions it emits, so adding a source is one act.

## 4. What is borrowed, and from where

| From | What Anam takes |
|---|---|
| Confluent Schema Registry | Subjects with immutable numbered versions; compatibility checked at registration; each message names its schema version |
| CloudEvents | The envelope's wire shape (`id`, `source`, `type`, `time`, `subject`, `dataschema`), so any CloudEvents producer can speak to Anam |
| OpenTelemetry Collector, Fluent Bit, Vector | Receiver → processor → exporter: ductile pipelines are this |
| OpenTelemetry `schema_url` | A batch says which Definition version it was shaped to |
| Splunk HTTP Event Collector | Ingress tokens pinned to what they may write |
| Elastic integrations | An importer ships with, and registers, the Definitions it emits |
| Open mHealth, HealthKit, Health Connect | Existing vocabularies for personal health data |
| Kafka compacted topics | The fold is a keyed latest-wins view; Anam computes it over a log it never compacts |

Where log management stops applying: logs tolerate loss and sampling and have no identity. Anam has
`identity_key` and supersession, and loses nothing. Borrow the edges, keep the middle.

## 5. What it costs

- **New Definitions for health data.** `health.sleep` today is one row per `(group, period_type,
  period_start)` in the ETL's schema. Per-night sleep from Garmin is a different shape and identity:
  new types or versions, not the old ones reworded.
- **The ledger retires the summary facets.** About 5,800 `health-db` facets, every weekly, monthly
  and yearly one among them, are retracted as their source goes, with daily observations admitted
  alongside. The fold handles it; it is a visible event in the record, and it comes after the move to
  unraid (card 46), never during it.
- **The ETL's cleaning moves into importers.** Whatever ductile-healthdata learned about Garmin (FIT
  events, seven-day week blocks) goes to the Garmin importer.
- **Rebuild.** Today one `anam ingest` rescans every source. With importers, each needs a "send
  everything again" mode, idempotent because commit is, and Anam needs a way to ask for it.
- **Run atomicity.** Today a dead share stops the whole run before anything is written. With push,
  each source succeeds or fails on its own.

## 6. Open decisions

- **Definitions in the ledger.** Datomic transacts its schema as data, so the schema at any moment
  is an as-of read. Admitting each Definition as a fact would let every facet's admission point at a
  Definition in the record. It changes doctrine (Definitions live in the operational store, under
  what Anam authors), so it is decided deliberately, not drifted into.
- **The rebuild protocol:** how Anam asks an importer to resend, and how it knows one has finished.
- **CloudEvents verbatim or Anam's `Envelope` with CloudEvents field names.**

## 7. Order

1. The service owns the record (cards 45, 46).
2. Prerequisites 1 to 3: identity composed by the gate, JSON Schema, real validation.
3. The endpoints (prerequisites 4 and 5).
4. **Withings importer.** Done (card 54). A webhook trigger, fetch the new measure group, shape it,
   `anam_submit`. A weigh-in reaches the ledger in seconds, with no ETL and no hourly scan. It matched
   the built-in `withings` worker facet for facet, and that worker has been removed.
5. **Garmin importer**, daily observations from `garmin.db`; then retire `healthdb`, the ETL and
   healthdata.db.
6. One-off exports (Technogym, Strong, Fitbit) get no importer. Each is a document in the library,
   turned into a records file once by a script kept in `scripts/`, and reshaped by regenerating that
   file with the same ids (card 58). The blood pressure app turned out to be live, exported
   periodically, so it has an importer (card 80), and the built-in `bpcsv` worker is gone. `records`
   and `documents` stay inside Anam: they are formats, not sources.

## 8. The importer contract

What an importer must do to put facts in the record, and what the service promises back. Built in
card 53; the Python base every importer starts from is card 54's.

### 8.1 The repo

Importers live in one repo, `anam-importers`, one folder each. A folder holds the importer's code
and the Definitions it emits, so installing an importer brings its contract with it:

```text
anam-importers/
  withings/
    manifest.yaml              ductile plugin manifest
    importer.py                reads the source, shapes payloads
    definitions/
      health.weight.json       one manifest per type it emits
    tests/
  anam_submit/                 the generic plugin that posts envelopes
```

A Definition is a JSON manifest in exactly the format of Anam's own (`internal/def/manifests`):
`meta` carries the type, version, `identity_key`, `time_semantics`, classification and
`validation_mode`; `schema` is closed JSON Schema 2020-12 (§3, prerequisite 3). A type lives in one
place. While Anam's binary bundles a type, no importer may register it; an importer takes a type over
when a release drops it from the binary, registering the same body at the same version. The other
direction works too: a release that bundles a type an importer registered admits it against the
bundled Definition from then on, keeps the registered rows for the facets admitted under them, and
says so in its log. No two importers may name the same type unless the binary bundles it: a bundled
type's Definitions come from Anam, so no importer can register a version that refuses another's
envelopes, and any number may submit it, each under its own source (`exercise.session` from Garmin
and Concept2). A release that drops a type two importers share refuses to start until one of them
gives it up.

### 8.2 Tokens

Each importer has its own token, and the daemon knows only its SHA-256. `importers_file`
(`ANAM_IMPORTERS_FILE`) names a YAML file, read when the daemon starts:

```yaml
importers:
  - source: withings
    token_sha256: 9f86d081...      # printf %s "$token" | sha256sum
    types: [health.weight]
```

- `source` is the source every envelope it submits is admitted as. A facet's identity includes its
  source, so an importer that takes over a built-in worker's facets uses that worker's source name.
- `types` are the types it may submit, and the only types it may register Definitions for.
- An importer's token opens `POST /v1/envelopes`, `POST /v1/definitions` and
  `GET /v1/definitions...`, and nothing else: it cannot read the record. The operator's token
  (`token_file`) reads the Definitions but submits nothing (403).
- Make a token with `openssl rand -hex 32`. Present it as `Authorization: Bearer <token>`.

### 8.3 Definitions

| Call | Answers |
|---|---|
| `GET /v1/definitions` | Every type the gate admits: `type`, `origin` (`bundled` or `registered`), `current`, `versions`. |
| `GET /v1/definitions/{type}` | The current manifest, as published. |
| `GET /v1/definitions/{type}/{version}` | That version's manifest. 404 if not held. |
| `POST /v1/definitions` | Body: one manifest. 201 registered; 200 already registered, same body; 409 a different body at that version, an older version than one held, a changed `identity_key` or `fact_time`, or a type the binary bundles; 422 not a valid manifest; 403 a type outside the token's `types`. |

A registered version is immutable, and "same body" compares canonical JSON, so reformatting the file
is harmless. A change is a new version, which may change the schema, classification or series freely
but not what keys or places a facet. An importer registers its Definitions every time it starts:
that is a no-op until a manifest changes, and it is how a lost operational store comes back.

### 8.4 Envelopes

`POST /v1/envelopes`, body:

```json
{"envelopes": [
  {"type": "health.weight", "definition_version": 2,
   "payload": {"id": "withings-123", "date": "2026-10-09 07:30", "kg": 80.5}}
]}
```

- `type`, `definition_version` and `payload` are required. `definition_version` must be the type's
  current version: the gate refuses a payload shaped to any other.
- `source_ref` is optional. The gate composes it from the Definition's `identity_key`; sent, it must
  match. `asserted_person` and `span_hint` are optional.
- There is no `source` field, and an unknown field is a 400: the token names the source.
- A batch holds 1 to 49 envelopes, at most one per facet. Send more as more batches.

The batch is admitted whole or not at all:

| Status | Body | Meaning |
|---|---|---|
| 200 | `w`, `changed`, `results[]` (`index`, `outcome`, `facet_id`, `artifact_version`) | Admitted. `outcome` is `admitted`, `superseded` or `no-op`. |
| 422 | `error`, `refused[]` (`index`, `type`, `reason`) | Nothing written. Every envelope that failed is named. Fix or drop them and send the batch again. |
| 400 | `error` | The body is not one submission: bad JSON, an unknown field, no version, 0 or more than 49 envelopes, or anything but whitespace after the submission. Nothing written. |
| 401 / 403 | `error` | No importer's token, or the operator's. |
| 413 | `error` | The body is over 16 MB, counting whatever follows the submission. Nothing written. The whole body is read before the batch is queued, and the daemon cuts off a body that takes more than 30 s to arrive. |
| 503 | `error`, `Retry-After` | Another write held the ledger for more than 5 s, usually an ingest. Wait and send the same batch again. |

Sending a batch again is always safe. The same payload under the same identity is a `no-op`; a
changed payload supersedes, keeping the old version. That is the whole retry and resend protocol: an
importer that is unsure whether a batch landed, because the connection dropped or timed out, sends it
again.

The daemon mints once batches stop arriving for 2 s, in the background, so neither the answer nor the
next batch waits on it.

### 8.5 What an importer must do

1. Register every Definition in its folder at start, and stop if one is refused.
2. Validate each payload locally against its Definition's `schema` with a stock JSON Schema library,
   so a refusal at the gate means the importer and Anam disagree, not that a row was malformed.
3. Shape payloads only. Compose no identity, join nothing, summarise nothing (§2).
4. Submit in batches of at most 49, resubmit on 503, and report a 422's reasons as a failed run.
5. Offer a resend-everything mode. It is a rebuild's first step, and idempotent by construction.
   Label its batches as the parts of one snapshot (§8.6), so the operator can see what the source
   no longer holds.

### 8.6 Retraction by complete resend

An importer never tombstones. It says what its source still holds, and the operator retracts what
it no longer does (card 78; Matt, 2026-10-10).

- **A snapshot.** A resend-everything run labels each batch as one part of a snapshot:
  `{"snapshot": {"id": "<run id>", "part": 3, "parts": 27, "types": ["health.measurement"]}, "envelopes": [...]}`.
  The id is the importer's and fresh for each run (1 to 128 of letters, digits and `. _ : -`). Every
  type must be the importer's, and every envelope in the part one of them. A part admits exactly as a
  batch does; the answer carries `snapshot: {parts_admitted, complete}`. Parts may come in any order
  and be resent. A part is its envelopes, order aside: sent again with the same envelopes it adds
  nothing, before or after the snapshot completes; sent again under the same number with other
  envelopes it is a 409, and nothing of it is admitted (card 107). A run that reads its source again
  is a new run, with a new id. A part naming an id begun with other types or another part count is
  likewise a 409. An incremental run carries no snapshot.
- **What Anam keeps.** The admitted parts and the facet ids they carried, `no-op` included, in the
  operational store. It is disposable: an incomplete snapshot is dropped after a day, a source keeps
  its two newest complete ones, and a lost table comes back with the next resend.
- **The report.** `GET /v1/snapshots/{id}`, under the importer's token (its own snapshots only) or
  the operator's: `complete`, `parts_admitted`, `carried`, and once complete `live` (the source's live
  facets of the types now), `absent` and up to 1,000 `absent_ids`. Absent means live, not carried,
  and untouched since the run began (`started_w`, the watermark after its first part): a facet an
  incremental run sent beside or after the run is never called absent (card 94). The one gap Anam
  cannot see is the importer's own: a facet admitted between the importer reading its source and
  sending its first part is judged as if the run had read it.
- **Retracting.** `anam retract <source>` on an importer's source plans from its newest complete
  snapshot, with the same guards as a scan: an empty run is refused, `--max-fraction` bounds a run,
  and nothing is written without `--yes`. There is no automatic retraction: a tombstone cannot be
  undone (a resend of the same row is a no-op), and an importer is outside the error kernel.
- **Retiring a source.** When an importer replaces a source under another name (card 55: `garmin`
  for `health-db`), the old source is neither empty nor unreadable; it is gone by decision, and
  nothing scans it again. `anam retract <old-source> --retired` plans every live facet of the old
  source, reports the count by type, and with `--yes --max-fraction 1` tombstones them all, one
  anam-event each. It refuses a source that an import or an importer still declares, so the
  declaration is removed first and a live source cannot be retired by mistake. The artifacts stay,
  and a read as of before the retirement still shows every facet (card 120). The order of a
  cut-over is therefore: the new importer's complete snapshot admits and reports 0 absent, the old
  source's declaration comes out, then `--retired`.
- **The alert.** An importer whose complete snapshot reports `absent` above zero raises a ductile
  event so the gateway tells the operator; Anam sends nothing itself.

### 8.7 Not yet

- **Receipts.** A receipt pins the bundled Definition set's version (`def.SetVersion`); registered
  Definitions are not in it.

