---
type: gap
title: anam_submit's manifest does not declare current_versions
description: The records importer emits payload.current_versions and anam_submit reads it, but the submit manifest's input_schema and consume list name only envelopes, definitions, batch and snapshot.
timestamp: 2026-10-11T00:00:00Z
looked_at:
  - ../anam-importers/anam_submit/manifest.yaml
  - ../anam-importers/anam_submit/run.py
  - ../anam-importers/records/manifest.yaml
---

A ductile values or fact_outputs check would not see the link. Harmless at runtime; the field reaches `handle` as part of the event payload. See [batch event](/data-structures/batch-event.md).
