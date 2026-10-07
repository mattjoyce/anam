# 7 October, night: the heart rate alarm, and a timeline that could read it

Matt asked about one event he had never seen in the data, Garmin's abnormal high heart rate alarm.
It was not in the ledger, not in `healthdata.db` and not in GarminDB's databases. It was in the raw
FIT files all along, 301 times since January 2025. By the end of the night it was in the ledger, and
the timeline had been rebuilt so the alarm looks like an alarm.

## Where the alarm was

Decoding all 7,702 wellness FIT files with Garmin's FIT SDK turned up a handful of undocumented event
numbers. Event 66 was the one. Its 16-bit data field packs two bytes. The high byte is the heart rate
that tripped the alarm, 100 to 130 bpm. The low byte is the threshold, 100 every time. Four of them
checked against GarminDB's per-minute heart rate each follow ten or more minutes at or above 100,
which is how the alarm is specified. None in the 2024 files, so it was switched on around January
2025. The busiest night was 25 September 2026, eight alarms between 20:32 and 00:14.

GarminDB downloads these files and then drops every event message on import. Its handler logs at
debug and stores nothing, in the 3.6.7 we run, in 4.0.0, and on `develop`. Upgrading would not have
found it. The upgrade is card 32 on its own merits: HRV, a login change, a database rebuild.

## The decisions

Matt chose to read the FIT files in a script in `ductile-healthdata`, not to patch GarminDB. Upstream
does not know what event 66 means, and that knowledge belongs in code he owns. The script,
`fit_events.py`, writes an `events` table into `healthdata.db`. `run.py` stays stdlib-only; the image
gains one pip line for the SDK. Hand-writing a FIT parser to keep the image dependency-free would
have been the wrong trade.

The script runs on unraid, never over the ThinkPad's mount. The share is CIFS with loose caching, and
SQLite written through that is a corruption waiting for a bad moment. Production runs inside the
image with the share bound in, the same way the ETL does.

## Hickey × Armstrong

**Hickey.** The alarm is a value with a time, not a cell in a summary grid. Squeezing it into
`summaries` as "alarms per day" would have kept the count and lost the evening of 25 September,
which is the whole story. So it got its own table, and in Anam its own type,
`health.heart-rate-alert`, identity (source, kind, fact_time). The payload carries what the watch
said and leaves out `source_file` and `first_seen`. They describe the ETL's reading, and a rebuilt
source could change them without the alarm changing, the same reason `last_updated` was kept out of
the summaries.

The timeline had braided two things into one slot number: which colour a type wears and which lane it
sits in. Both cycled, and past the eighth type nothing was defined, so eleven types drew as unstyled
black, the alarms among them. Now a type has a family (its row and hue), a shape, and a kind. The kind
was the late correction. Shape alone could not say "event": the first cut put blood pressure, weight
and 5,529 Withings measurements under "events only". A reading is not a happening. Measure, reading
and event are now separate, drawn as a filled circle, a hollow mark and a filled shape.

**Armstrong.** Each FIT file's scan record commits in one transaction with its events, so a killed
run leaves a file unscanned, never half-read, and the next run resumes. A file that will not decode
is reported and retried, and its neighbours still land. The backfill took 11m49s; a run with nothing
new takes 9 seconds. On the Anam side, a `healthdata.db` without the `events` table yields no alarms
and no error, so an old source file cannot fail an ingest. The ledger and tracking store were backed
up before the live ingest: 301 admitted, 5,492 no-op, W=13953, and a second run admitted nothing.

## The timeline

Eight family rows, one palette hue each, fixed in `internal/inspect/marks.go` so a filter never
repaints what is left. The alarm is a larger red triangle. The legend is the filter form, with a
checkbox per type and per family and presets for all types and events only. The filter rides in the
URL with the window, so a view is still a link. A type nobody has styled draws as a grey circle in an
"other" row, and a test holds every bundled Definition to a style. Checked by screenshot against the
live ledger, through headless Firefox; `browse --local` needs Chrome, which the ThinkPad lacks.

## Left open

- Card 33: nothing runs `fit_events.py` on a schedule yet. New alarms reach Anam only after someone
  runs it. The recommendation is a scheduled job on the unraid gateway.
- Card 32: the GarminDB upgrade.
- At whole-person zoom the dense rows smear, and the canonical strip still draws all 13,951 facets.
  A per-day density mark at wide zoom would read better. Noted on card 34.
- `ductile-healthdata`'s working branch is pushed but not merged into `main`.
