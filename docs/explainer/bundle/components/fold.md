---
type: component
title: Fold
description: Derives each facet's current status from its anam-events in seq order and builds the canonical value, sorted and hashed.
timestamp: 2026-10-11T00:00:00Z
resource: internal/fold/fold.go#Fold
anchor: "func Fold(events []facet.AnamEvent) (map[string]State, error) {"
basis: code
tier: core
group: derived
depends_on:
  - /components/store-ledger.md
  - /components/facet.md
---

| Event kind | Precondition | State after |
|---|---|---|
| admitted | unknown facet, version 1 | v1, admitted |
| superseded | prior version is the current one | next version, admitted |
| tombstoned | concerns the current version | tombstoned |
| endorsed | concerns the current version | endorsed |
| rejected | concerns the current version | rejected |

Live means admitted or endorsed. `StatusSuperseded` never appears in a state: only an artifact version is superseded, the facet is re-admitted at the new one. A stream that breaks a precondition is `ErrCorruptStream`.

`ReadAt` replays the ledger to a watermark, folds, fetches exactly the version the fold derived for each live facet, and builds a `Value`: [fold entries](/data-structures/fold-entry.md) sorted by fact-time then id, serialised once, hashed. The watermark is not in the bytes, so a rebuilt ledger reaches the same hash at a different W.

`Snapshot` and `Fates` give the last-known-value read over a [series](/terms/series.md). `WatermarkAt` translates an anam-time into a watermark for `--as-of`. `Displaced` lists every version no longer in force, for `--explain`.
