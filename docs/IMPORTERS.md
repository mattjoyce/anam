# Importers — sources outside Anam

Design proposal · 2026-10-08 · documentation only; nothing here is implemented yet.

Anam's capture workers for vendor sources (`healthdb`, `withings`, `bpcsv`) are compiled into the
binary that owns the ledger. This proposal moves them out: an importer is a ductile plugin that
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

1. **The gate composes identity.** It already recomputes `source_ref` from the Definition's
   `identity_key` and refuses a mismatch. When an envelope leaves `source_ref` out, the gate composes
   it. The identity rule then lives once, in the Definition, and importers only shape payload fields.
2. **A Definition's schema is standard JSON Schema.** Today the `schema` block is Anam's own
   pointer-keyed format, checkable only by Go inside Anam. As JSON Schema, an importer validates
   locally with a stock library before sending, and the gate runs the same check. The Anam-specific
   contract (`identity_key`, `time_semantics`, classification, validation mode) stays in `meta`.
   Health Definitions adopt or cite an [Open mHealth](https://www.openmhealth.org/) (IEEE 1752)
   schema where one fits.
3. **Validation becomes real.** `lenient` with a near-empty schema is harmless while Anam writes its
   own payloads. Once outside code writes them, facet-compliance is the guard, so each Definition an
   importer targets states what a valid payload is.
4. **Two endpoints on the service.**
   - `GET /v1/definitions[/{type}[/{version}]]`: the published contract an importer shapes to.
   - `POST /v1/envelopes`: a batch, each envelope naming its type and Definition version, admitted
     through the gate. Per-source tokens scoped to the types a source may submit replace the static
     `mvpScopes` map: the authenticated pillar as credentials, as Splunk's HTTP Event Collector
     pins a token to its sourcetypes.
5. **Registering a Definition** (`POST /v1/definitions`) checks a new version against the
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
- **Huma** for the new endpoints (card 49), since adding routes is when it is cheapest.

## 7. Order

1. The service owns the record (cards 45, 46).
2. Prerequisites 1 to 3: identity composed by the gate, JSON Schema, real validation.
3. The endpoints (prerequisites 4 and 5).
4. **Withings importer.** A webhook trigger, fetch the new measure group, shape it, `anam_submit`. A
   weigh-in reaches the ledger in seconds, with no ETL and no hourly scan. It replaces the built-in
   `withings` worker once it matches it facet for facet.
5. **Garmin importer**, daily observations from `garmin.db`; then retire `healthdb`, the ETL and
   healthdata.db.
6. `bpcsv` as a folder-watch importer. `records` and `documents` stay inside Anam: they are formats,
   not sources.
