---
type: component
title: anam_bp_csv
description: Shapes a blood-pressure app export to health.blood-pressure v5, an Open mHealth body beside the export's own text, one reading sent once from the newest export.
timestamp: 2026-10-11T00:00:00Z
resource: ../anam-importers/bp_csv/run.py#handle
anchor: "def handle(request: Dict[str, Any]) -> Dict[str, Any]:"
basis: code
tier: supporting
group: importers
depends_on:
  - /components/importer-base.md
---

`handle` takes one export from a folder watch; `poll` resends every export under `export_dir` as one snapshot. The CSV reader mirrors Go's `encoding/csv`, and an export whose record boundaries cannot be read for certain is refused whole. Readings sharing a timestamp are told apart by an ordinal, which with the Excel date is the Definition's identity. Exports order by the date in their name, and a reading two exports hold goes once, from the newer, so a corrected comment stands and a resend is no-op. Traced in [admit a batch](/workflows/admit-a-batch.md).
