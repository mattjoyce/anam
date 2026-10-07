# 7 October, evening: the medicines, the rebuild, and the library

Three things in one sitting. The blood pressure medication got its name. The ledger was rebuilt
from the sources with a new identity encoding and a zone on every Definition. And the documents
the facts came from moved in beside the ledger as a library, each one a fact of its own.

## The medicines

Matt screenshotted the My Health Record medicines history, five phone screens across 2024, 2025
and 2026. Each dispensing date shows the prescription, the pharmacy's dispense record and the PBS
claim. Read together they answer the question every earlier note left open:

| Date | Change | Prescriber |
|---|---|---|
| 16 Aug 2024 | ramipril 2.5 mg daily started | Dr Van |
| 13 Sep 2024 | felodipine (Felodur ER) 2.5 mg daily added | Dr Van |
| 16 Oct 2024 | ramipril raised to 5 mg daily | Dr Van |
| 23 Dec 2025 | felodipine stopped, blood pressure in range, agreed with the GP | Matt's account |
| 21 Sep 2026 | ramipril 5 mg still dispensed | Dr Van |

Ramipril began the same day as the pathology request that noted hypertension. Felodipine was added
three days after the ECG. The ramipril dose went up the day after the home diary reached the GP. The
missed dose of 19 Sep 2024 was both drugs at 2.5 mg, and that record was corrected to say so. One
records file, three rows plus the stop, validated on a scratch ledger and then admitted live. The 2023
section of the history was not captured; if it holds a blood pressure drug the ramipril start moves
earlier.

## Why rebuild

Cards 11 and 12 could not be done in place, and Matt asked why, then asked why not just break the
rules since this is dev. Both answers are worth keeping.

Why not in place: a facet's id is a hash of its source ref, and the ledger decides by content hash.
Card 11 changes how the ref composes, so every facet gets a new id, and the ledger can only admit
the new one and tombstone the old: 13,613 tombstones and 13,613 admissions carrying no information.
Card 12 changes the fact-time of facets whose payload is unchanged; the gate sees the same hash and
says no-op, and fact-time lives on the immutable descriptor, so the correction can never land. There
is no event kind for "the time was wrong," because the payload was right.

Why the rebuild is the dev move, not the principled one: an in-place hack is not one UPDATE. The ids
are in the facet table, the event table, the checkpoint log and inside the content-addressed
artifacts; the fact-times are in every checkpoint's bucket assignment. By the time the hack is
consistent it has done everything a rebuild does, plus a trigger bypass, plus a script nobody runs
twice. The rebuild is four commands and no new code. The rule kept is "no code that bypasses the
ledger"; the file was disposable.

Before deciding, two checks. `anam retract --dry-run` on every source: 0 missing of 13,613. And every
one of the 19 records files traced to a document on the share with a matching hash, four of them
inside zips. The chain was intact, but half of it lived outside the ledger. Matt: the documents are
sources too; declare libraries, everything relative to a library, a default library beside the
ledger. That became card 29, and it went in before the rebuild so the new ledger carried the
documents from its first watermark.

## Hickey × Armstrong, before building

**Hickey.** The composition rule for a source ref lived in six places: the gate's `compose` and
five workers each joining with a colon by hand. One function now, `facet.SourceRef`, length-prefixed
so the values can carry anything. The records worker was the interesting case: it assumed
`records:<id>` and the docs carried a special case for blood pressure, whose id had to equal the
composition. Giving the worker the Definition and letting it ask, as the gate asks, removed the
assumption and the special case together; a diary record's id is now just another field.

The zone belongs on the Definition because it is part of how a source's clock is read, next to the
pointer and the layout. It is not a worker's concern (the worker reads bytes) and not the reader's
(the reader stands somewhere else). Withings proved the point: its ETL writes UTC, so a Withings row
at 19:42 UTC is a 05:42 reading; every other source writes the wall clock. The reader's zone is a
separate knob in config, used for bare dates on the command line and for showing fact-times. Two
zones, two concerns, not braided.

A library root is a place. The facet carries the library-relative path and the file's hash, and the
root is config on each host that reads it. The UNC path Matt first reached for would have put the
network topology into every document fact. The default library derives its place from the ledger's,
which is one knob fewer and makes the ledger's directory the whole record.

The guard (card 14) needed a place to hold what the store last saw. The place existed: the ops
store's `definition` table, created in an earlier slice and never written. Now every ingest writes it
and compares first.

**Armstrong.** Schema 3 is a refusal, not a migration. The old binary fails fast on the new ledger
and the new binary fails fast on the old, each with a message naming the remedy, and the refusal is
checked before any migration step runs so a refused file is exactly as it was. That is also what
kept the hourly gateway ingest from writing old-encoded ids into the new ledger during the rebuild:
the new binary was installed first, and the old ledger was refused by it, so the gateway's one run in
the gap wrote nothing. The guard runs before any source is read, so a Definition that would re-key
the ledger stops the run before the gate exists. A missing library is an empty library, not a
failure; a missing sidecar on a file that exists still refuses the whole scan. The inspector serves
document bytes under `os.Root`, so a path that climbs out or a symlink that points out gets the same
404 as a file that is not there.

## The rebuild

Installed the binary, moved 37 documents into the library with sidecars, repointed the records
headers, and ran the rebuild on a scratch ledger first: 13,650 admitted, 143 of 143 buckets minted,
retract dry run clean on all five sources. Then live: the old ledger, tracking store and artifacts
archived beside the data as `*.pre-rebuild-2026-10-07`, and the same numbers.

29 May 2025, read in the reader's zone: the Garmin day row at 00:00+10:00, the Withings weigh-in at
05:36+10:00, blood pressure at 05:43+10:00. Before tonight the blood pressure sat ten hours away from
the day it belonged to.

What the rebuild lost, named: the 157 superseded versions, anam-time before tonight, and the 26 July
receipt pinned at watermark 1089. All three are in the archived file.

## Left open

- The 2023 section of the medicines history, one scroll.
- A card for the glasses prescription values and CPAP therapy as types; both documents are in the
  library already.
- `overseas_travel_history.md` landed in the inbox tonight; the travel types exist.
- Card 21, checkpoints in the inspector, next.
