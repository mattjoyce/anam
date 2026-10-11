---
type: gap
title: The health-db worker is still built in after its source was retired
description: Card 55 retired health-db in favour of the Garmin importer and tombstoned its facets, but internal/capture/healthdb, its ingest case, its scope entry and the example import remain in the binary.
timestamp: 2026-10-11T00:00:00Z
looked_at:
  - internal/capture/healthdb/healthdb.go
  - internal/app/ingest.go
  - imports.example.yaml
  - docs/IMPORTERS.md
---

IMPORTERS.md says `healthdb` remains until card 55 finishes; the board and the last commits say card 55 finished on 2026-10-11 with the staleness check moved to `garmin.day`. The worker, `scanImport`'s `case healthdb.Source`, `mvpScopes[healthdb.Source]` and the example import's `source: health-db` all remain. A declared `health-db` import that cannot be read would stop the whole local ingest run, since every import is scanned before any is admitted. Whether the worker is kept for a re-ingest of the retired ETL or is simply unpruned is not recorded. See [health-db worker](/components/capture-healthdb.md).
