---
type: decision
title: Health measures use Open mHealth bodies where one exists
description: health.blood-pressure v4 and later, and the leaves of activity.exercise-session, conform to a pinned Open mHealth (IEEE 1752) schema, with the source's own text kept beside it.
timestamp: 2026-10-11T00:00:00Z
resource: internal/def/manifests/health.blood-pressure.json#required
anchor: '"required": ["date_excel", "reading", "systolic_blood_pressure", "diastolic_blood_pressure", "effective_time_frame"],'
basis: docs
tier: detail
alternatives:
  - One Definition per source in the source's own shape (how health.sleep and the ETL types were; a second source for the same measure then needs a second type)
affects:
  - /components/importer-bp-csv.md
  - /data-structures/definition-manifest.md
---

**Context.** Decided 2026-10-08 (journal note, IMPORTERS §3). `activity.exercise-session` was one type for Technogym, Strong, Garmin and Concept2 because each shapes to the same OMH `physical-activity` body. Card 195 split it into a leaf per vendor, since each fills its own quarter of the fields beside that body: the body and a shared core stay on every leaf.

**Choice.** A pinned OMH schema inside Anam's closed JSON Schema, plus the export's own columns as text so nothing the source wrote is lost. Identity and fact-time still come from the source's fields (`date_excel`, `reading`).

**Why.** An existing vocabulary lets sessions and readings from different sources sit on one timeline as one type.

**What it costs.** Each new source re-shapes to the vocabulary rather than passing its rows through, and the extractors that wrote v4 rows still say v4 while the current version is v5.
