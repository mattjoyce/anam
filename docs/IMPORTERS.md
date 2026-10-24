# Importers — sources outside Anam

Design proposal · 2026-10-08, built by 2026-10-13. The service's side is built; §8 is the contract every importer complies with. Importers exist for Withings,
blood pressure, the DreamStation, Garmin and OwnTracks, in
`anam-importers`.

Anam's capture workers (`healthdb`, `withings`, `bpcsv`, `records`, `documents`) were compiled into
the binary that owns the ledger. All five have left: `withings`, `bpcsv`,
`healthdb` (its facets retired on 2026-10-11, the package deleted on 2026-10-13), and
`records` and `documents` (to `anam_records` and `anam_documents`). `anam ingest` and
`POST /v1/ingest` went with them. An
importer is a ductile plugin that shapes one source's data to a Facet Definition and submits it to the
Anam service. Anam keeps the Definitions, the gate and the record, and knows nothing about any vendor.

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

1. **The gate composes identity.** Done. The gate composes `source_ref` from the
   Definition's `identity_key`; an envelope may leave it out, and one that carries it is refused on a
   mismatch. The identity rule lives once, in the Definition, and importers only shape payload fields.
2. **A Definition's schema is standard JSON Schema.** Done. The `schema` block is JSON
   Schema draft 2020-12, so an importer validates locally with a stock library (Python's
   `jsonschema`: `jsonschema.validate(payload, manifest["schema"])`) before sending, and the gate
   runs the same check. The Anam-specific
   contract (`identity_key`, `time_semantics`, classification, validation mode) stays in `meta`.
   Health measures conform to a pinned [Open mHealth](https://www.openmhealth.org/) (IEEE 1752)
   schema where one exists, decided on 2026-10-08
   (the journal's decision note "Open mHealth is the payload vocabulary for health measures"). The first is `activity.exercise-session`: an OMH `physical-activity`
   payload, one type for Technogym, Strong and later Garmin, each shaping to the same Definition.
   It is split into a leaf per vendor (`activity.exercise-session.garmin`, `.concept2`,
   `.technogym` and `.strong`): the IEEE body and a shared core on each, with only that vendor's
   own fields beside it.
3. **Validation becomes real.** Done. Every schema is closed: it declares each field with
   its type and refuses anything else, and `lenient` is gone. Once outside code writes payloads,
   facet-compliance is the guard, and each Definition states exactly what a valid payload is.
4. **Two endpoints on the service.** Done; §8 is the contract.
   - `GET /v1/definitions[/{type}[/{version}]]`: the published contract an importer shapes to.
   - `POST /v1/envelopes`: a batch, each envelope naming its type and Definition version, admitted
     through the gate. Per-source tokens scoped to the types a source may submit replace the static
     `mvpScopes` map: the authenticated pillar as credentials, as Splunk's HTTP Event Collector
     pins a token to its sourcetypes.
5. **Registering a Definition** (`POST /v1/definitions`). Done. It checks a new version against the
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
- **The ledger retires the summary facets.** Done 2026-10-11: 5,818 `health-db` facets,
  every weekly, monthly and yearly one among them, tombstoned by `anam retract health-db --retired`
  at W 32988, with the Garmin importer's daily observations admitted first. The fold handled it; it
  is a visible event in the record.
- **The ETL's cleaning moves into importers.** Whatever ductile-healthdata learned about Garmin (FIT
  events, seven-day week blocks) goes to the Garmin importer.
- **Rebuild.** Each importer has a "send everything again" mode (`poll`), idempotent because commit
  is, and labels it a snapshot (§8.6) so Anam knows when one has finished. What is not yet written or
  drilled is the whole: a fresh ledger rebuilt from every source in turn.
- **Run atomicity.** Today a dead share stops the whole run before anything is written. With push,
  each source succeeds or fails on its own.

## 6. Open decisions

- **Definitions in the ledger.** Datomic transacts its schema as data, so the schema at any moment
  is an as-of read. Admitting each Definition as a fact would let every facet's admission point at a
  Definition in the record. It changes doctrine (Definitions live in the operational store, under
  what Anam authors), so it is decided deliberately, not drifted into.
- **What a rebuild keeps.** The rebuild protocol is §8.8: each importer's `poll` resends its source
  whole, and its snapshot says when it has finished. Still open: whether a rebuild should keep the
  old ledger's anam-times and superseded versions (a migration importer would), and
  whether an operator's retraction of a fact its source still holds should replay.
- **CloudEvents verbatim or Anam's `Envelope` with CloudEvents field names.**

## 7. Order

1. The service owns the record.
2. Prerequisites 1 to 3: identity composed by the gate, JSON Schema, real validation.
3. The endpoints (prerequisites 4 and 5).
4. **Withings importer.** Done. A webhook trigger, fetch the new measure group, shape it,
   `anam_submit`. A weigh-in reaches the ledger in seconds, with no ETL and no hourly scan. It matched
   the built-in `withings` worker facet for facet, and that worker has been removed.
5. **Garmin importer.** Done (2026-10-11): sleep, daily heart rate, steps, the day and the
   sessions from `garmin.db`, the heart rate alerts from the FIT files. `health-db` left the imports,
   its 5,818 summary facets were retired, the ETL pipelines were removed and healthdata.db stays as an
   archive. The built-in `healthdb` worker was deleted on 2026-10-13.
6. One-off exports (Technogym, Strong, Fitbit) get no importer. Each is a document in the library,
   turned into a records file once by a script in `anam-importers` `records/extract`, and reshaped by regenerating that
   file with the same ids. The blood pressure app turned out to be live, exported
   periodically, so it has an importer, and the built-in `bpcsv` worker is gone. `records`
   and `documents` were kept inside as formats until Matt's word of 2026-10-11 that they leave too:
   records by `anam_records`, documents by `anam_documents` posting bytes to
   `POST /v1/documents`, and the in-process workers went.

## 8. The importer contract

What an importer must do to put facts in the record, and what the service promises back. The Python base every importer starts from lives in `anam-importers`.

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
`validation_mode`, and may declare `kind` (`measure`, `reading` or `event`) and `family` (a
lower-case word such as `meds`), which say what the type is so a reader can draw and filter it
without an edit to Anam; the gate ignores both. It may also declare `identity_template`,
the type's id rule for the one identity field the payload carries: literal text with
`{source}`, `{/pointer}`, `{/pointer|slug}` (lower case, runs of other characters turned to one
hyphen) and `{/fact_time_pointer|<Go time layout>}` placeholders, as
`{source}:med/{/drug|slug}/dose/{/taken_at|2006-01-02T15:04}` for `health.medication.dose`. The gate
fills the identity field from it when an envelope leaves it out, and refuses an envelope whose field
disagrees, showing both. A writer that wants the id before it sends builds it with `def.ComposeID`;
one that is not Go leaves the field out. A change of template is a new version. `schema` is closed JSON Schema 2020-12
(§3, prerequisite 3). A type lives in one
place. While Anam's binary bundles a type, no importer may register a version of it; an importer
takes a type over when a release drops it from the binary, registering the same body at the same
version. The other direction works too: a release that bundles a type an importer registered admits
it against the bundled Definition from then on, keeps the registered rows for the facets admitted
under them, and says so in its log. An importer that still sends the body the binary now carries,
at its version, is answered unchanged (200), not refused, so the release and the importer's
redeploy need no order between them; any other body of a bundled type is a 409. No two importers may name the same type unless the binary bundles it: a bundled
type's Definitions come from Anam, so no importer can register a version that refuses another's
envelopes, and any number may submit it, each under its own source (Garmin and Concept2 both sent
`activity.exercise-session` until each had its own leaf). A release that drops a type two importers share refuses to start until one of them
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
  source, so an importer that takes over another's facets uses that source's name, as `anam_records` and
  `anam_documents` kept `records` and `documents`.
- `types` are the types it may submit, and the only types it may register Definitions for: the
  types it owns.
- `any_type: true`, in place of `types`, lets an importer submit every type the daemon knows and
  own none: it registers no Definition (403), and a type with no Definition is refused to it as to
  anyone. It is for a source that is the person speaking, `conversation`; the source in
  every facet's identity keeps what the person asserts apart from what a device observed of the
  same kind. `types` and `any_type` together are refused at start. Two edges to know: `any_type`
  reaches facets and not the library, so `POST /v1/documents` stays with a source that lists
  `record.document`; and a series is not source-stamped as an identity is, so in a snapshot read the
  latest entry of a series wins whatever its source, and the person saying `health.medication.stopped` closes
  the course the records file started.
- An importer's token opens `POST /v1/envelopes`, `POST /v1/documents` and `POST /v1/definitions`,
  and every read a `reader` has: the record, receipts, Definitions, and its own
  source's snapshots. It does not open minting, backups, projection templates or the inspector
  (403). The operator's token (`token_file`) reads but submits no envelopes (403).
- The `workbench` entry is the one principal with no token. It lets the operator enter
  facts by hand at `POST /v1/entry`, which the daemon admits as source `workbench`, so no importer
  token ever reaches a browser. It is listed with the importers, takes `types` as the allow-list of
  what may be entered by hand, and a `token_sha256` on it is refused at start, as is `any_type`:

  ```yaml
  importers:
    - source: workbench
      types: [health.medication.dose]
  ```

  The `workbench` source is reserved. It owns none of its types, so another importer may own one
  (`types` for a type no binary bundles is that importer's alone), and the workbench registers no
  Definition. Leave the entry out and `POST /v1/entry` answers 404, saying the file declares no
  workbench principal. `anam retract workbench --facet <id> --yes` undoes one entry. The attention
  types (`mind.attention`, `mind.attention.started`, `mind.attention.ended`), `health.intake` and
  `money.purchase` are entered by listing them in `types`; the form is drawn from their Definitions, so nothing else
  changes. The form draws single values only, so a purchase entered by hand has its totals and no
  `items`; the lines come from a records file. The intake form shows every field of every kind, and the gate refuses a drink with a
  meal's fields or a meal with a drink's.
- The same file names principals that are not importers, under `principals`, each with a `name` and
  a `role` (`reader` or `operator`) and no source or types. An importer may be listed there too,
  with `role: importer`, a name, a source and types or `any_type`. Names, sources and token hashes
  are each used once. API.md §2 has the roles.
- `receipts: always` on a principal files every read its token makes as a receipt that names the
  principal, so `GET /v1/receipts?principal=<name>` answers who read what. It is off unless set (`off` says
  so), it is for a `reader` or an `importer`, and a value other than `always` or `off` is refused at start.
  The operator and the `workbench` entry cannot have it. A token with it must read through a projection:
  a timeline read without one, or with `explain`, is a 400, and it opens only the receipts that name
  it. A `dry_run` query still files nothing. API.md has the whole contract:

  ```yaml
  principals:
    - name: assistant
      role: reader
      token_sha256: 9f86d081...
      receipts: always
  ```
- Make a token with `openssl rand -hex 32`. Present it as `Authorization: Bearer <token>`.

### 8.3 Definitions

| Call | Answers |
|---|---|
| `GET /v1/definitions` | Every type the gate admits: `type`, `origin` (`bundled` or `registered`), `current`, `versions`. |
| `GET /v1/definitions/{type}` | The current manifest, as published. |
| `GET /v1/definitions/{type}/{version}` | That version's manifest. 404 if not held. |
| `POST /v1/definitions` | Body: one manifest. 201 registered; 200 already registered, same body; 409 a different body at that version, an older version than one held, a changed `identity_key` or `fact_time`, or a type the binary bundles; 422 not a valid manifest; 403 a type outside the token's `types`. |

Every version is held, and a rebuilt operational store holds them again: the versions Anam's own
bundle ever carried are provisioned from the binary (`internal/def/manifests/history`),
and an importer's versions come back because the importer keeps every version it has ever emitted
of the types it emits now, the current one in `definitions/<type>.json` and each earlier one in
`definitions/history/<type>.v<n>.json`, and registers them all, each type's oldest first, since a
version older than one held is refused (§8.5 rule 1). A folder that drops an old version
loses the words its facets were admitted under. History kept under a type's old name, which a
rename carried away, is the binary's to provision and is not registered.

A registered version is immutable, and "same body" compares canonical JSON, so reformatting the file
is harmless. A change is a new version, which may change the schema, classification or series freely
but not what keys or places a facet. An importer registers its Definitions every time it starts:
that is a no-op until a manifest changes, and it is how a lost operational store comes back.

A payload says how its source's clock was read in `source_time`: `value`, the time as
the source supplied it; `zone`, the IANA zone, `UTC` or fixed offset it is read in; `basis`, who
decided that zone, `source` (the source supplied it, or the value is UTC by its definition),
`configuration` (a setting a person made for this source) or `assumption` (the importer chose it,
and logs that it did; a Definition's zone with nothing behind it is an assumption). A Definition
carries it by copying one of the shapes in `internal/def/sourcetime/` as a top-level property, and
requires it as any other field; Parse refuses any other shape, and the gate refuses a zone that
does not exist. A daily aggregate's interval is the Open mHealth `effective_time_frame`, the interval
the source counted, each end at its own offset. A Definition whose fact time is a zoneless wall clock
gets that clock refused when its zone skips or repeats it (a daylight-saving gap or overlap): the
importer reads the source's offset where there is one, and `anam_importer.clock.local_instant`
refuses the same cases in Python before it sends, leaving the record out with a log line, so the
run is no snapshot. Every importer sends it: Garmin, Withings, DreamStation, OwnTracks
and stays with a UTC instant or epoch (`source`); Concept2 with `date_utc` (`source`), or a web
entry's date in the Logbook's own zone (`source`) or Sydney (`assumption`); bp_csv with its `zone`
key (`configuration`) or the Definition's zone (`assumption`); links by where its date came from;
documents and the pathology extractor in Sydney (`assumption`); the Fitbit sleep and Strong
extractors in the zone they are given (`configuration`). A source that gives the end of a session,
as Concept2 and Technogym do, keeps the end as `value` and its Definition says so. A writer may
read a wall clock only in the zone its Definition's fact time names, unless the fact time names its
own zone. The types still read from a Sydney wall clock without requiring it are listed in
`internal/def/sourcetime_test.go`, and that list only shrinks.

A fact may name its own zone. `time_semantics.fact_time` may carry a `zone_pointer`, a
field of `source_time`; a payload that holds a zone there is read in it, and one that holds none in
the Definition's zone, as before. The self-report types point it at `source_time.offset`, which the
second shape, `internal/def/sourcetime/v2.json`, adds and requires: the offset from UTC the wall
clock had where it was said, which names one instant even in an hour the zone repeats, beside
`zone`, the IANA name where the writer knows it (else the offset again). The gate refuses an offset
the named zone does not keep at that wall clock: London never showed 01:30 on 29 March 2026, and
never sat at +05:00. `record.edge` v2, `health.medication.dose` v2, `health.intake` and the three
`mind.attention` types require it: the conversation
writer and the Workbench send it with `basis: source` when the person's client or browser names the
zone, else anam-mcp's `ANAM_ZONE` with `basis: configuration`, and a writer that cannot tell the
offset refuses rather than guess. `health.weight` v5 declares it without requiring it, since
records sends 1,600 weigh-ins without it, and a date-only type stays a local date. The dose's
`identity_template` ends `{/source_time/offset?}`, an optional placeholder that renders nothing
when the field is absent, so a dose recorded before keeps its id and one local minute in two zones
is two doses. The guard lets a version add a `zone_pointer`, and append optional placeholders to an
id template, because no stored version of the type let a payload carry the field they read
([ARCHITECTURE.md](ARCHITECTURE.md) §3a).

A vendor's own payload shape is named for its vendor, at the leaf after the concept:
Garmin's sessions and days are `activity.exercise-session.garmin` and `activity.day.garmin`,
Withings' rows `health.measurement.withings`, a DreamStation's sessions
`health.cpap-session.dreamstation`. A new source of the same concept is a new leaf with
the shared core and its own fields, never a wider type. A reader asks across the leaves by the
prefix, `activity.exercise-session.`. The importer that registers a type renames it by a
rename table (`renames/`), registering the leaf before the run; a leaf's first version may place its
facets by a new fact-time rule, as `activity.day.garmin` reads the start of the day Garmin counted
where `activity.day` read the date at Sydney midnight.

A manifest may carry a third block, `handling`, beside `meta` and `schema`. It holds what
every writer and reader of the type must agree on for its facts to mean the same thing and the schema
cannot say: how a drug is named when the name is half the series key, the shape of a self-log id, what
a file's disappearance means. Anam makes three promises about it and no more. It is part of the
version, so the 200/409 comparison includes it and an envelope's `definition_version` names the
handling it was shaped under. It is served verbatim, on `GET /v1/definitions/{type}` and the
inspector's Definition page, to whoever is about to write or read facets of the type; `def.Guard`
ignores it. And Anam checks only that it is an object: no vocabulary, no verbs it knows, nothing
read at the gate, with one exception, `label`. By convention a `guidance` key holds prose, so a person or an agent reading the
Definition before writing a records file or an importer has somewhere to start; the rest is the
type's own. The test for what belongs there is advice to the author, not a gate: a thing goes in
`handling` if every writer of the type must agree on it. Retry policy, poll cadence and a person's
tolerance for drift fail that test and stay in the importer's config.

The one key Anam reads is `label`, a short template a page draws a facet of the type as, so a row
of the selection page reads "melatonin 5 mg" and not a type and a time. It is literal text with
`{/pointer}` fields read from the payload, as `identity_template` reads them, and nothing else: no
`{source}`, no transform, no optional mark, because a label may never fail. A pointer the payload lacks
renders as nothing, a text field as its first line, and a label whose fields are all empty is no label,
so the page shows the type and the time instead. Each pointer must name a field the schema declares,
which Parse checks, so a typo fails at load. Nothing builds an id or a check from a label. It is part of
the version like the rest of `handling`, so adding or changing one is a new Definition version, and
a type whose importer shapes envelopes to a fixed Definition version (blood pressure, the Garmin and
Withings leaves) can take a label only in step with that importer, since the gate refuses an envelope
shaped to another version. A Definition with no `label` needs none: it is optional.

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
| 413 | `error` | The body is over 49 payloads at `max_payload_bytes` plus 1 MiB (about 16 MB at the 300 KiB default), counting whatever follows the submission. Nothing written. The whole body is read before the batch is queued, and the daemon cuts off a body that takes longer than `http_read_timeout` (30 s) to arrive. |
| 503 | `error`, `Retry-After` | Nothing written. Either another write held the ledger for longer than this batch could wait (`importer_wait`, 5 s, or less when the request's budget is shorter, below), usually a backup or a full mint, or the operator holds this source's admission, which the body names. Wait as long as `Retry-After` says and send the same batch again: a batch the queue did not take in time is dropped, not started later, so only the resend can admit it. |

A request's budget is the shorter of the daemon's `timeout` and the request's own `Anam-Timeout`
header (a Go duration, `25s`), and it counts from arrival: the wait for the write queue spends it,
and the write has what is left. The caller's say ends when the queue takes the batch.
Before that, a request that runs out of budget is 503 and one whose caller hangs up is dropped,
and neither is ever written. After it, the batch is admitted or refused whole, to its own end,
even if the caller has gone; resending it then is a `no-op`.

Sending a batch again is always safe. The same payload under the same identity is a `no-op`; a
changed payload supersedes, keeping the old version. That is the whole retry and resend protocol: an
importer that is unsure whether a batch landed, because the connection dropped or timed out, sends it
again.

The daemon mints once batches stop arriving for 2 s, in the background, so neither the answer nor the
next batch waits on it.

### 8.4a Documents

A document's bytes reach the library through Anam, never by an importer writing into Anam's
directory. `POST /v1/documents?ext=<extension>`, body the raw bytes (at most `max_document_bytes`, 64 MiB by default),
under an importer token whose `types` include `record.document`. The answer is `sha256` (hex),
`bytes`, `media_type` (from the extension) and `new`. 201 stored; 200 the library already held
these bytes; 400 an extension that is not letters and digits, at most 16; 403 a token that may not
submit `record.document`. There is no conflict: Anam names the stored file by its sha256,
so other bytes are another file and nothing is overwritten. The extension only labels that file for
a person browsing the library.

Then the importer submits the `record.document` envelope as any other. Its `path` is the document's
name and identity, whatever the importer files it under; its `sha256` is how Anam finds the bytes.
The same `path` with another `sha256`, a rescan, supersedes the facet, and the first scan's bytes
stay for the facet version that names them. An envelope naming a `sha256` the library holds no
bytes for is refused with the batch (422), before the gate, and nothing of the batch is admitted.

Where in the library a file sits is the library's own business and never enters a fact. A file
goes in the library root until that holds `library_folder_limit` files (default 1000), then into
a subfolder named by the first character of its sha256, then the first two inside that, and so on.
A stored file never moves; a lookup finds it by its name.

### 8.5 What an importer must do

1. Register every Definition in its folder, every version of each type it emits, oldest first, and
   stop if one is refused.
2. Validate each payload locally against its Definition's `schema` with a stock JSON Schema library,
   so a refusal at the gate means the importer and Anam disagree, not that a row was malformed.
3. Shape payloads only. Compose no identity, join nothing, summarise nothing (§2).
   A facet's identity is its source and its identity values, **not its type**, so an importer that
   emits several types keeps their identities apart: a `health.sleep-episode` and an `activity.steps`
   from `garmin` for the same day must not both be keyed on the bare date (the Garmin ids are
   `sleep:<day>`, `steps:<day>`, `day:<day>`). The gate refuses an envelope whose facet id is held
   by a facet of another type, naming both types and the source_ref, and writes nothing.
4. Submit in batches of at most 49, resubmit on 503, and report a 422's reasons as a failed run.
5. Offer a resend-everything mode. It is a rebuild's first step, and idempotent by construction.
   Label its batches as the parts of one snapshot (§8.6), so the operator can see what the source
   no longer holds.

### 8.6 Retraction by complete resend

An importer never tombstones. It says what its source still holds, and the operator retracts what
it no longer does (Matt, 2026-10-10).

- **A snapshot.** A resend-everything run labels each batch as one part of a snapshot:
  `{"snapshot": {"id": "<run id>", "part": 3, "parts": 27, "types": ["health.measurement.withings"]}, "envelopes": [...]}`.
  The id is the importer's and fresh for each run (1 to 128 of letters, digits and `. _ : -`). Every
  type must be the importer's, and every envelope in the part one of them. A part admits exactly as a
  batch does; the answer carries `snapshot: {parts_admitted, complete}`. Parts may come in any order
  and be resent. A part is its envelopes, order aside: sent again with the same envelopes it adds
  nothing, before or after the snapshot completes; sent again under the same number with other
  envelopes it is a 409, and nothing of it is admitted. A run that reads its source again
  is a new run, with a new id. A part naming an id begun with other types or another part count is
  likewise a 409. An incremental run carries no snapshot.
- **What Anam keeps.** The admitted parts and the facet ids they carried, `no-op` included, in the
  operational store. It is disposable: an incomplete snapshot is dropped after `snapshot_ttl` (a day), a
  source keeps its `snapshot_keep` (two) newest complete ones, and a lost table comes back with the next resend.
- **The report.** `GET /v1/snapshots/{id}`, under the importer's token (its own snapshots only) or
  the operator's: `complete`, `parts_admitted`, `carried`, and once complete `live` (the source's live
  facets of the types now), `absent` and up to `max_absent_ids` (1,000) `absent_ids`. Absent means live, not carried,
  and untouched since the run began (`started_w`, the watermark after its first part): a facet an
  incremental run sent beside or after the run is never called absent. The one gap Anam
  cannot see is the importer's own: a facet admitted between the importer reading its source and
  sending its first part is judged as if the run had read it.
- **Retracting.** `anam retract <source>` on an importer's source plans from its newest complete
  snapshot, with the same guards as a scan: an empty run is refused, `--max-fraction` bounds a run,
  and nothing is written without `--yes`. There is no automatic retraction: a tombstone cannot be
  undone (a resend of the same row is a no-op), and an importer is outside the error kernel.
- **One facet by hand.** A source no importer resends (`workbench`, where a person enters a record;
  `conversation`) has no snapshot to compare with. `anam retract <source> --facet <id>` shows the facet and
  what the tombstone would do, and `--yes` appends it; `POST /v1/facets/{id}/retract` is the same for the
  operator (API.md). It refuses a facet of another source, a facet that is not live, and a source an
  importer declares and has a complete snapshot for, which the importer's resend reconciles instead.
- **Retiring a source.** When an importer replaces a source under another name (`garmin`
  for `health-db`), the old source is neither empty nor unreadable; it is gone by decision, and
  nothing scans it again. `anam retract <old-source> --retired` plans every live facet of the old
  source, reports the count by type, and with `--yes --max-fraction 1` tombstones them all, one
  anam-event each. It refuses a source that an import or an importer still declares, so the
  declaration is removed first and a live source cannot be retired by mistake. The artifacts stay,
  and a read as of before the retirement still shows every facet. The order of a
  cut-over is therefore: the new importer's complete snapshot admits and reports 0 absent, the old
  source's declaration comes out, then `--retired`.
- **Retiring a type.** When an importer stops sending a type it still owns, because a new type
  holds the same facts in another shape (Garmin's three `health.heart-rate-day` facts a
  day became one `health.heart-rate.day`), its complete resend keeps naming the old type in the
  snapshot's `types` and carries none of it. Every live facet of the old type is then absent, and
  `anam retract <source>` tombstones them by the plan above; `--retired` does not apply, since the
  source is still declared. The importer's principal keeps the old type in its `types` for as
  long as the importer names it, or the snapshot is refused.
- **The alert.** An importer whose complete snapshot reports `absent` above zero raises a ductile
  event so the gateway tells the operator; Anam sends nothing itself.

### 8.7 Not yet

- **Receipts.** A receipt pins the bundled Definition set's version (`def.SetVersion`); registered
  Definitions are not in it.

### 8.8 Rebuilding the record from its sources

Anam owns no facts, so a ledger can be started again from nothing (a re-keyed Definition, a new
schema, a corrupted file) by having every importer resend its source whole. The drill
proves it without touching the record: `scripts/rebuild-drill.py` in anam-importers runs each
importer's `poll` and `anam_submit` into a scratch daemon on a new ledger and compares the result
with the live one, type by type, by facet id, content hash and status.

**The drill.** Run it first, every time, with a plan naming each live importer and its instance
config (anam-importers `docs/rebuild-drill.md`). Clean is zero `only-live`, `only-scratch` and
`differ` for every type, or a difference named and explained. The scratch ledger it leaves is a
rebuilt record, built by the same path a live rebuild takes.

**A rebuild of the live record.**

1. Back up (`POST /v1/backups`) and note W.
2. Stop the daemon. Move `ledger.db` and `tracking.db` aside, never delete them: they are the
   record of what Anam knew until now, and the receipts published from it verify only against
   that file.
3. `anam init` at the same paths, start the daemon. Bundled Definitions are there at once; an
   importer's registered Definition comes back with its first batch.
4. Run each importer's `poll` through the gateway (`POST /plugin/<instance>/poll`): `anam_records`,
   `anam_documents`, `anam_withings`, `anam_bp_csv`, `anam_dreamstation`, `anam_garmin`,
   `anam_concept2`, `anam_owntracks`. Each is a complete snapshot.
5. Check each: its last submit reports `snapshot complete, 0 of N live absent`, and
   `GET /v1/snapshots/{id}` says `complete`. Mint (`POST /v1/checkpoints`), then compare counts by
   type with the drill's report.

**What a rebuild cannot bring back.** What only the ledger held:

- anam-times. Every fact is admitted at the time of the rebuild, so an as-of read before it, and
  every receipt naming the old ledger's W, has nothing to read in the new file. The old file keeps
  them.
- Superseded versions. A source resends what it holds now, so the new ledger has each fact's
  current payload and none of its history.
- Retractions. A fact retracted because its source dropped it stays gone, since the source no
  longer sends it. A fact an operator retracted while its source still holds it comes back live:
  fix it at the source first. A retired source (`--retired`) sends nothing, so its facets do not
  return at all, as neither live nor tombstoned.
- Facts from a source that no longer exists. Every source the record holds today has an importer,
  so there are none; the drill's `only-live` lists any that appear.

**The drill's record.** 2026-10-14, from the ThinkPad, records and documents from the Library share
against v0.308-775ffb9: 2,939 facets in 18 types, every facet id, content hash and status equal to
the live record's, in about 17 seconds of polls and submits. The six importers on unraid are the morning's run.

### 8.9 Migrating to a fresh ledger

A rebuild (8.8) asks every importer for its source again. When the churn is in the ledger and the
sources are fine (superseded versions, retired types, a night of resends), `anam migrate`
carries what is live now from the old ledger into a new one, so nothing is re-imported. It reads the
old pair, `ledger.db` and `tracking.db`, read-only, and the new pair must not exist.

```
anam migrate --from <old ledger.db> --from-store <old tracking.db> \
             --to <new ledger.db> --to-store <new tracking.db> \
             [--program <file defining pick/1>] [--yes]
```

It needs `person_id` like any command (`ANAM_PERSON_ID`), and touches no daemon.

**What it carries.** Every live facet of the old ledger at its head, as it folds: the current version
of each, under its own type, source, source ref, binding and payload, through the gate. Superseded
versions and tombstoned facets are not carried. Each facet is held to the type's current Definition
version first, and is carried under it only if its payload passes and its id, content hash, fact time,
series, classification and descriptor all come out as they were, so only the definition version
number differs (Matt, 2026-10-22). Otherwise it is carried under the version it was admitted under,
as before. The old store's
registered Definitions and projections are written to the new store first, so every such version is
held. With `--program`, only the facets that a Datalog program's `pick/1` selects over the whole
live value (no projection narrowing) are carried; the selection page's programs (docs/QUERY.md) work
as they are. A `record.edge` that names a facet the selection does not carry is still carried, and
the dry run lists each such end as "link to a facet not carried". Carrying that facet later
reconnects it, since a facet's id derives from its identity.

**The dry run** is the default. It prints, per type and source, how many facets it would carry, how many of
those move to the current version (MOVED) and how many stay under their own (OWN), and how many the
gate would refuse. A line follows for each group that moves, "moved from v1 to the current v5", and
for each group kept under an older version, with why the current one did not take it. It writes
nothing, not even the new files. A facet is refused when its status is not a first sight, or when
neither the current version nor the one it was admitted under carries it: no Definition is held at
that version as a contract that parses today, or the gate would make another facet of it (a different
id, hash, fact time, series, classification or descriptor). If anything is refused, `--yes` writes
nothing either.

**`--yes`** creates the new ledger as `anam init` does and the new store, writes the registered rows,
admits the facets in blocks of `gate.MaxBatch`, then prints the proof: the new ledger's live facets
equal the selection, facet for facet by id, content hash, fact time and status; W old and W new; the
old and new file sizes; and a second dry run with the new pair as `--from`, which must select the
same set (the carry is idempotent). The proof holds identity to id, content hash, fact time and
status, and reports the definition version moves apart, with the same lines the dry run printed. A
run that fails partway removes the files it made.

**What it cannot carry.** A facet that neither version carries. A Definition version that no longer
parses as a contract cannot be held to: the binary keeps older bundled versions as history words, and
a body from before validation was closed (`validation_mode: "lenient"`) is not a Definition today. The 1,600 live
`health.weight` facets of the live record are admitted under such a v1, but their payloads pass the
current version with the same id, hash and fact time, so they move to it and are flagged "moved from
v1". A facet whose own version does not parse and whose payload the current version does not pass is
named in the dry run and blocks a run that selects it.

**What it cannot bring back.** Anam-time. Every facet is admitted at the cut, one event each, so the
new ledger's W is the number of facets carried, no as-of read before the cut can be answered from it,
and the receipts naming the old W verify only against the old file.

**The procedure for the live record.**

1. Pause every importer, or hold admission, so the ledger does not move while the files are copied.
2. Back up (`POST /v1/backups`) and note W.
3. Stop the daemon.
4. `anam migrate` from the live pair to new file names, first without `--yes`, and read the report:
   counts per type and source, no refusals, the links listed. Then run it with `--yes`.
5. Check the proof the run prints, and that W new is the count carried.
6. Swap the files: keep the old pair, made read-only, beside the new one as the archive, and put the
   new pair at the live paths.
7. Start the daemon. Catch-up replays the new ledger into the new operational store, and the
   daemon provisions the bundled Definitions; the registered ones are already there.
8. Resume the importers. An importer that resends what is live finds it a no-op; one that resends
   something the migration did not carry admits it as new.

Rehearse on a copy first: run `anam migrate` on a backup, start a daemon on the new pair on a spare
port, and compare `GET /v1/coverage` and a projection's timeline with a daemon on a copy of the old
pair. The 2026-10-22 rehearsal on the backup at W 61829 (14,633 live facets) carried 13,033 facets
in about 7 seconds, W 13,033, a ledger of 14.4 MB from 57.9 MB, equal by id, hash and status, with
coverage per type and the `meds` and `heart` timelines identical on both.
