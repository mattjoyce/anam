---
type: journal
date: 2026-10-18
tags: [anam, journal, garmin, sleep, time, importers, unraid, churn, deploy]
---

# 18 October: a facet with eleven versions

A Saturday that started with one question from the inspector: why does last night's sleep have
ten versions? The answer was two bugs, neither in Anam, in two different layers below it, and the
record had kept the evidence for both.

## What the versions said

Facet `garmin:16:sleep:2026-10-17` had eleven versions in fourteen hours, and every diff was the
same field flipping: `end_date_time` 08:03, then 07:30, then 08:03 again. The events came in pairs
twelve minutes apart. Recent days were the same, ten and twelve versions each. The ledger had done
exactly what it should: two different payloads were presented and it kept both. The question was
who presented them.

Reading the versions also showed a second thing. Every night started around midnight and ended
around eight, as UTC, which is eleven in the morning Sydney time. The durations proved the stamps
were local: 00:33 to 08:03 is 450 minutes, and Garmin's 353 asleep plus 97 awake is 450. Garmin's
own JSON put the start at 13:33Z. Two cards, 174 for the zone and 175 for the churn.

## The zone

The importer's `instant()` said "GarminDB's UTC DATETIME text" and stamped the text with `Z`.
GarminDB converts Garmin's GMT epoch to a naive datetime in its process's zone, and on 11 October
the `garmindb` container was given `TZ=Australia/Sydney` so that card 128's cron would run on
Sydney time. The assumption was true in a UTC container and stopped being true the day the zone
was set, and nothing failed: a wall clock stamped `Z` is a valid instant, just the wrong one. The
fix is one line, read the text in `ZONE`, the zone the days are already local days in, and emit
UTC. The fixture night moved from 12:44Z to 01:44Z and a new test holds the night of 16/17 October
to Garmin's own stamps. Merged as d4fad25 in anam-importers. The ids do not change, so a `poll`
resends every night and each facet supersedes; every monthly bucket with sleep in it re-mints
once. The deploy and the resend are ThinkPad.unraid's.

## The churn

The gateway's `garmin` plugin polls `garmin.db`'s mtime every ten minutes. Its job records for the
17th showed two mtimes in one hour, three minutes apart. GarminDB's hourly run writes the sleep row
twice: the import writes Garmin's JSON (end 08:03), then analyze "ensures a Sleep row exists" by
summarising the sleep events, and writes `end` as the start of the last event, 07:30. A poll that
lands between the two writes reads 08:03, the next reads 07:30: two versions. Most hours the poll
lands after both and sees only the end state, so five hours in twelve showed the pair. That is a
lesson in itself: a poller on a file's mtime sees a run's end state unless it falls between writes,
so paired versions minutes apart are the signature of two writers, not one.

The analyze step is guarded, `if existing == 0`, so it should leave a day with a row alone. My
first reading blamed an `overwrite` flag; it was wrong, and I said so. The real cause took a
reproduction. On a copy of the databases inside the container, with SQL echo on, the guard binds
the day's window as datetime strings, `'2026-10-17 00:00:00.000000'` to `'2026-10-18 ...'`, and
the primary key is stored as `'2026-10-17'`. In text order a prefix sorts before the string it
prefixes, so the day's own key sits before its window while tomorrow's key sits inside it. The
guard counts tomorrow's row. During the 17th there was no 18th row, so the count was zero all day
and analyze rewrote the 17th every hour; at midnight the run imported the empty "today" file,
created the 18th's row, and from then on the 17th's guard was satisfied and the 08:03 stood, which
is why the day always settled on the right value by morning. With the 18th's row deleted on the
copy, the upstream guard returned zero and an exact key lookup returned one.

Upstream knew: GarminDB issue 300, fixed in 3.7.0 by making the day columns DateTime, which is the
schema change card 32 already flags as breaking the importer's ids. 4.0.0 drops the writer
entirely but still needs card 128's `--latest` patch. So the fix is interim: one more `RUN` in the
image's Dockerfile beside the existing patch, a `sed` to an exact key lookup, with a `grep` that
fails the build if upstream changes the line and a `py_compile` after. The rebuild was refused in
this session as a production deploy and went to ThinkPad.unraid with the commands, along with the
daemon deploy for card 172.

## Live

ThinkPad.unraid rebuilt the image and recreated the container, both patches present, and deployed
the daemon from 265c26c as v0.336, W unchanged. The June read now shows ramipril, felodipine and the
multivitamin. The 17th's facet held at v11 through the day's fourteen runs, which is consistent but
not proof: the 18th's empty row would have satisfied the old guard too. The proof is a day whose
sleep has landed while the next day's row does not exist, and at five in the afternoon Garmin's
file for the 18th was still empty, so the watch had not synced. Card 175 stays open until it does.

## Hickey × Armstrong

**Hickey.** Both bugs are a value read as a place. The `sleep` row is one slot that two writers
overwrite in turn; the ledger, which keeps every value, is what made the overwriting visible. The
zone bug is a type without its unit: a naive datetime whose zone lives in an environment variable
on another machine.

**Armstrong.** Nothing in Anam's kernel was touched or needed to be. The disposable layers failed
in the way disposable layers should: the importer will resend and supersede, the image was
rebuilt, and the record is behind on the zone until the poll, never wrong about what it was told.
The one thing worth guarding against is the permission layer refusing a deploy mid-task; the work
was finished up to the deploy and handed over with its exact commands, which is the right shape.

## Left open

- Card 175's proof, after the watch syncs.
- Card 174's deploy and `poll` resend, briefed to ThinkPad.unraid.
- Card 166's guidance text, then its build; card 173 after it.
- Card 32, the GarminDB upgrade, which retires the 175 patch.
