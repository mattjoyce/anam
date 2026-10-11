---
type: component
title: Health-db worker (internal/capture/healthdb)
description: The built-in reader of the retired health ETL's SQLite file; still compiled in and declared in the scope map after its source was retired.
timestamp: 2026-10-11T00:00:00Z
resource: internal/capture/healthdb/healthdb.go#Scan
anchor: "func Scan(ctx context.Context, dbPath, assertedPerson string) ([]facet.Envelope, error) {"
basis: code
tier: detail
group: sources
depends_on:
  - /components/facet.md
---

The one capture worker that needs `database/sql`, which is the carve-out `scripts/check.sh` allows: a source that is a SQLite file needs it to be read at all. It emits `health.sleep`, `health.body-composition`, `health.heart-rate`, `health.metric-group` and `health.heart-rate-alert`. Card 55 retired the source in favour of the Garmin importer and tombstoned its facets; the worker, its ingest case and its example import remain. See [health-db worker still built in](/gaps/healthdb-worker-still-built-in.md).
