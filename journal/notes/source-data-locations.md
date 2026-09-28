---
id: 202607100013
type: note
title: "Source data — where the real health DB lives, and the missing BP CSV"
created: 2026-07-10
status: evergreen
tags: [anam, note, sources, health-db, bp-csv, mvp]
project: anam
related: ["[[reference-implementation-mint-cli]]", "[[facet-definition-format]]"]
---

# Source data — where the real health DB lives, and the missing BP CSV

Established by inspection at the start of the second session, correcting `MVP.md` §8.

## `health-db` — use `/home/matt/admin/data/healthdata.db`

Three copies of `healthdata.db` exist. They are **not** interchangeable:

| Path | Contents |
|---|---|
| `/home/matt/admin/data/healthdata.db` | **the real one** — `day` (2026-03-07 → 2026-06-06), `week`, `month` and `year` back to 2024-01 |
| `/mnt/Projects/healthdata/healthdata.db` | degenerate — exactly *one* `month` period and *one* `year` period |
| `/home/matt/Projects/ductile-healthdata/test/healthdata.db` | a one-week `day` fixture |

`MVP.md` §8 says "the `/mnt` copy holds only `month`/`year` periods," which undersells it: that copy holds
a single period of each, and cannot demonstrate a timeline at all. Since a longitudinal record is the
entire point of [[the-inversion]], the demo must run against the `admin/data` copy.

The mapping stays period-agnostic regardless, so this changes which rows appear — not the code.

## Schema, and the facet grain it implies

```
metric_groups(group_id, name)
metrics(metric_id, group_id, name, unit)
summaries(period_type, period_start, period_end, metric_id, avg_value, min_value, max_value, sample_count)
```

`summaries` is keyed `(period_type, period_start, metric_id)`, which is exactly the coarse grain the MVP
chose: **one facet per group per period**, `event_time = period_start`, payload
`{period_start, metrics:{<metric_id>:{avg,min,max}}}` — nested, hence the JSON Pointer addressing in
[[facet-definition-format]].

Seven groups: `activity`, `body`, `breath`, `calories`, `heart`, `sleep`, `stress`. Sources are `garmin`
and `withings`.

## `bp-csv` — missing

The blood-pressure CSV sent over LocalSend in the first session **was never persisted**; it is nowhere
under `/home/matt` or `/mnt`. It must be re-sent, and this time committed to `testdata/` so it survives.

It blocks the `health.blood-pressure` capture worker and the demo's 2026-01-24 "No filodipine" reading.

**It has no substitute.** `bp_systolic` and `bp_diastolic` do exist inside `healthdata.db`'s `heart`
group (88 rows each, back to 2024-01-01) — but only as *period summaries*, not per-reading rows, and
without the comment field. The MVP wants one facet per reading, placed at its reading time. Summarised BP
is a different fact at a different grain, which is precisely why `bp_*` is excluded from
`health.heart-rate` and given its own source.

## 2026-09-29: the table above is reversed

The health ETL now runs on unraid, and the copy it writes is the real one.

| Path | Contents |
|---|---|
| `/mnt/Projects/healthdata/healthdata.db` | **the real one**, written by the unraid ductile gateway after every Garmin sync and Withings weigh-in. Daily rows back to 2024 |
| `/home/matt/admin/data/healthdata.db` | stale. Nothing has written it since a hand refresh on 2026-09-28 |

`~/.config/anam/imports.yaml` points at the unraid copy. The blood pressure export did
arrive, on 11 July, and is declared there too. How the whole chain fits together is in
`docs/pipeline.html`.
