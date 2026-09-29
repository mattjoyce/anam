---
type: journal
date: 2026-09-30
session: 10
title: "Ingest runs on a clock, and the gateway it runs in had two silent faults"
tags: [anam, journal, ingest, ductile, schedule, failure]
---

# Ingest runs on a clock, and the gateway it runs in had two silent faults

`anam ingest` now runs every hour in the ThinkPad ductile gateway. The first
scheduled run took 1.6 seconds. The `sqlite-ledger` branch was merged to `master`
the same morning.

## The plan changed twice, both times because of something measured

The card suggested a trigger from the ETL's completion event. I first planned a
watcher on `healthdata.db` that would raise an event, and a pipeline step that ran the
ingest. Two findings ended that.

| Finding | What it ruled out |
|---|---|
| Lifecycle hooks fire only for root jobs, and only for plugins that set `notify_on_complete` | An ingest that runs as a pipeline step can fail without anyone being told |
| The existing watcher on the same share had been stalled for three days | Building on a second watcher of the same kind |

So the ingest is a scheduled root job. It needs no watcher and no trigger. An hourly
run costs two seconds and writes nothing when there is no news.

## What changed in Anam

- **Ingest reads every import before it admits any.** It used to scan and admit one
  import at a time. With the share unmounted, the blood pressure export was admitted
  and then `health-db` failed. The test wrote three facets before the fix and none
  after. The ledger is now opened for writing only after the last scan succeeds.
- **A complete run ends with one line.** Either `ledger changed: N facets admitted or
  superseded` or `ledger unchanged: nothing new in any source`. The gateway matches
  the second to stay quiet.
- **The imports origin names its own variable.** A file named by `$ANAM_IMPORTS` was
  labelled `env:ANAM_CONFIG`.

## What was wrong in the gateway

Neither fault is Anam's. Both would have made the scheduled ingest look healthy while
it was not.

**No failure had ever been announced.** The `job-failure-notify` pipeline had zero
runs against ten failed jobs. No plugin had opted in to hooks. When one did, the
message arrived as "Job failed:" and nothing else, because the template asked for
`{message}` and the payload carries `plugin` and `error`. The ductile documentation
describes a `message` field that the code does not send. The template now reads
`Job failed [{plugin}]: {error}`.

**A job can stall without failing.** On 27 September a poll of the Garmin watcher
finished in 0.2 seconds, and the gateway could not record it: `database is locked`.
The job stayed marked as running. The scheduler skipped the next 6,840 polls with
`already_running`. The folder watcher stalled the same way on 30 September. A restart
cleared both.

## Two things the gateway's own checks did not catch

- `config check` passed a file holding both `plugins` and `pipelines`. The reload
  refused it. A restart at that moment would have failed to boot.
- After a successful reload, the first run of the new plugin failed with `not found
  in registry`. A restart fixed it.

## Verified

| Case | How | Result |
|---|---|---|
| Normal run, nothing new | Scheduled run in the gateway | Succeeded, no message sent |
| Normal run, new data | By hand, same binary and environment | 20 admitted, 15 superseded |
| Share not mounted | Temporary job, empty scratch ledger, a path that does not exist | Failed, 0 facets written, Discord message sent |

Not verified: a scheduled run that finds new data and announces it. The pipeline is
registered and its condition is the inverse of the quiet case, which did hold. The
next Garmin sync will show it.

## Open

- **The stall is not a failure, so nothing announces it.** The staleness alert is
  the guard, and it is not built.
- **The scheduled job runs `~/go/bin/anam`.** A change to Anam reaches it only after
  `make install`.
- **The failure text starts with two lines of provenance.** The cause comes third.
