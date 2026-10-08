---
id: 202610082300
type: decision
title: "Open mHealth is the payload vocabulary for health measures"
created: 2026-10-08
status: evergreen
tags: [anam, decision, definitions, open-mhealth, payload, importers]
project: anam
related: ["[[guiding-lens-hickey-armstrong]]"]
---

# Open mHealth is the payload vocabulary for health measures

**Decision (Matt, 2026-10-08).** Where an [Open mHealth](https://www.openmhealth.org/) (IEEE 1752)
schema exists for a health measure, the payload of the Anam facet type for it conforms to a pinned
version of that schema. The Facet Definition wraps it with what OMH does not have. This adopts OMH as a
payload vocabulary, decided per Definition; it does not adopt OMH's data model.

## The tension

Three levels were on the table. Borrowing only OMH's building blocks keeps every payload Anam's own,
which means designing every health shape here, and nothing outside Anam can read one without a mapping.
Adopting OMH as the model (its data points and headers as the envelope, IEEE 1752 conformance as a
system rule) does not fit: OMH has no identity across restatements, no supersession and no anam-time,
which are what Anam is. The middle level takes OMH where it is strong, the shape of a measurement, and
leaves Anam's contract where it is.

## The rule

- **Scope.** Health measures with an OMH schema: blood pressure, body weight, body fat, BMI, height,
  heart rate, sleep, physical activity, step count, calories, medication schedules and doses,
  adherence, and later blood glucose, oxygen saturation, temperature, respiratory rate. Types with no
  OMH schema (labs, conditions, travel, illness, visits, documents, device alerts) keep Anam's own
  schemas, built from OMH's parts: `time_frame` and `{value, unit}`.
- **The payload is the OMH body.** Fields the source records and OMH has no place for sit beside the
  body in snake_case. The OMH header is not in the payload: `schema_id` is the Definition reference,
  and `acquisition_provenance` (source, modality, source creation time) belongs to the envelope's
  provenance. A re-scan that changes only provenance must not supersede a facet.
- **The Definition wraps it.** `meta` names the conformance (`conforms_to: omh:blood-pressure:4.0`)
  and keeps Anam's contract: `identity_key`, `time_semantics` pointing into `effective_time_frame`,
  classification, validation mode. OMH identifies a data point by a random uuid, so the payload carries
  an `id` field holding the source's own key (a Withings `grpid`, a Technogym `on`, bp-csv's time and
  ordinal), and `identity_key` points at it.
- **Type names stay Anam's.** `health.blood-pressure` stays; conformance is a field. A different
  upstream vocabulary later is a Definition change, not a rename.
- **Pinned, never fetched.** OMH versions are immutable once published. The Definition carries its
  copy of the schema file, so validation needs no network and an admitted facet's schema cannot change
  under it.
- **Versioning.** A new OMH version is a new Definition version when identity and time placement are
  unchanged, under `def.Guard`'s existing rule; otherwise a new type.

## What this costs, and what it buys

It costs OMH's awkward parts (blood pressure 4.0's nesting, closed unit lists) and a reshaping of
today's health payloads. Reshaping is cheap: an unchanged identity supersedes in place and keeps the
history; a changed identity moves by retraction and new admissions, or through an importer that reads
the old ledger as a source and carries its anam-times as provenance; and every source is kept
(`healthdata/garmin`, `healthdata/withings`, `healthdata/bp-csv`, `private/records`, the library), so a
rebuild is always possible.

It buys payloads that outlive Anam's code: someone else has argued about units, posture and time
intervals, anything that speaks OMH or Health Connect can read the record without Anam, and importers
(docs/IMPORTERS.md) have a schema to shape to instead of one to invent.

## Applications

- Card 56, `exercise.session` on `physical-activity-1.2`: the first, decided before this note and now
  its first instance.
- Card 52: the JSON Schema form of a Definition is how conformance is checked.
- Cards 54 and 55: the Withings and Garmin importers shape to OMH-conforming Definitions.
