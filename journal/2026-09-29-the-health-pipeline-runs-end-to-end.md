---
type: journal
date: 2026-09-29
session: 9
title: "The health pipeline runs end to end, and most of the work was not in Anam"
tags: [anam, journal, health, etl, garmin, withings, ductile, unraid]
---

# The health pipeline runs end to end, and most of the work was not in Anam

Anam holds 11,134 facets, with daily Garmin and Withings summaries back to January 2024.
Yesterday it held 6,570 and its Garmin data stopped at 16 July. The chain from watch to
ledger is drawn in `docs/pipeline.html`.

## Matt's question was the right one, and I had answered a different one

Asked to pull over new data, I found Garmin ten weeks behind and refreshed the summary
file by hand. Then I started building a Garmin worker inside Anam, to read Garmin's
database directly. Matt stopped me twice.

First: *"there is a worker doing the etl to health data surely."* I had told him
nothing scheduled updated the file Anam reads. That was true of the file and wrong about
the system. There were three ETLs.

Second: *"wait, let's consider why it is the way it is now."* That is the question I
had skipped. I had gone from "the data is stale" to "build a reader", without asking
what was meant to keep it fresh and why it had stopped.

| Generation | State when found |
|---|---|
| Senechal, hourly cron on unraid | running, on Garmin files frozen since 22 March |
| ThinkPad ductile gateway | ETL step dropped in July's move to the privileged gateway |
| Unraid ductile gateway | registered in April, zero runs ever |

The unraid one was the intended owner, by Matt's own notes from March and April. It had
no trigger for either source and its image was not on the server. A second reader inside
Anam would have worked, and would have left all three of those in place.

## What the decision on grain settled

Matt: *"samples is too fine grain. daily is good enough, but we should take min and max
too."* GarminDB already computes a min and a max per day. The ETL put each value in the
average field and left the range empty, and it read 30 of the 47 columns. So the change
belonged in the ETL, and it was small.

## What was changed, and where

None of the code change is in this repository.

- **`ductile-healthdata` 0.3.0.** Min and max per period. All 47 columns. Seconds kept in
  durations. No sleep row for a period where the watch recorded none. Accepts the
  Withings event. Looks three days behind the earliest changed day.
- **Unraid gateway.** Image built. `garmin` plugin scheduled. A second pipeline routes
  the Withings event to the ETL.
- **Senechal.** Cron removed.
- **Anam.** The declared import now names the unraid file. The ledger was rebuilt from
  the sources.

## A defect the new test found in old code

The test for the Withings trigger inserted a measurement and asked for its day row.
There was none. The source writes `2026-09-27 18:42:09`, and the ETL compared that as
text against `2026-09-27T00:00:00`. A space sorts before a letter, so every measurement
sorted before the start of its own day. No Withings day period had ever matched, and
the first day of each week, month and year had been falling out of its aggregate.

Anam's own keys had the same family of defect in July, with unpadded integers. It is
the third time this project has met text that orders differently from what it means.

## Why the ledger was rebuilt

Re-ingesting would have superseded 792 facets and left 142 behind. Those 142 were
periods the new ETL no longer writes, mostly nights recorded as zero sleep. Anam has no
command to retract a facet whose source row has gone, so they would have stayed live and
wrong. Matt had already said the sources are the record, so the ledger was rebuilt:
11,134 facets in 4.8 seconds.

The blood pressure import now runs first. The receipt served on 26 July pins watermark
1089, and it verifies only if the 234 blood pressure facets are in by then. It still
returns reproduced and intact. That is an accommodation and it is written in the imports
file, because a receipt that names a watermark is only meaningful against the ledger
that issued it, and nothing in a receipt says which ledger that was.

## Measured

| | 28 Sept, morning | 29 Sept |
|---|---|---|
| Facets in Anam | 6,570 | 11,134 |
| Newest Garmin day | 16 July | 27 Sept |
| Oldest Garmin day | 7 March 2026 | 20 Sept 2024 |
| Withings daily summaries | none | 779 days |
| ETL runs recorded by the unraid gateway | 0 | 1, and counting |

## Open

- **`anam ingest` is by hand.** The pipeline keeps `healthdata.db` current. Anam catches
  up when someone runs the command.
- **No retraction by reconciliation.** A facet whose source row disappears stays live.
  The rebuild hid that. It will come back.
- **A receipt does not name its ledger.**
- **No staleness alert**, on either side.
- **Day boundaries.** Garmin's day is a Sydney calendar day and Anam reads its start as
  midnight UTC.
