---
type: gap
title: A weekly bucket width disables composed reads and incremental mints
description: The docs say weekly is also implemented; span.New accepts it, but a weekly id does not subdivide a year, so LayoutOf refuses it and every read and mint falls back to the full fold.
timestamp: 2026-10-11T00:00:00Z
looked_at:
  - internal/span/layout.go
  - internal/mint/read.go
  - internal/mint/incremental.go
  - docs/ARCHITECTURE.md
  - docs/GLOSSARY.md
---

`Layout` subdivides years into months into days; `LayoutOf` returns an error for a week id. `startingRun` and `fromLastRun` then report ok=false, and `partition` with a budget fails with a message to clear the log and mint again. A weekly configuration therefore works only with `checkpoint_budget: 0`, and then every timeline read folds the whole ledger. ARCHITECTURE §7 says `checkpoint_budget: 0` keeps the fixed width but not that weekly loses the composed path. Whether weekly is meant to survive the subdivision design is not recorded. See [buckets](/components/span.md).
