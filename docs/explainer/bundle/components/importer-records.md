---
type: component
title: anam_records
description: Sends a records folder as one snapshot of the bundled types it carries, reading YAML with Go's yaml.v3 rules so every record the built-in worker admitted resends as no-op.
timestamp: 2026-10-11T00:00:00Z
resource: ../anam-importers/records/run.py#poll
anchor: "def poll(request: Dict[str, Any]) -> Dict[str, Any]:"
basis: code
tier: detail
group: importers
depends_on:
  - /components/importer-base.md
---

It carries no Definitions and does not validate locally: every type it sends is bundled in Anam, and each batch says `current_versions` so anam_submit stamps the version Anam publishes. Plain scalars resolve as Go resolves them (`yes` stays text, `0777` is 511, `1_000` is 1000), and a number Python would write differently from Go stops the run, because the gate keeps a number's text and a different one would supersede. The one-off extractors under `records/extract` write records files from exports that will never be sent again.
