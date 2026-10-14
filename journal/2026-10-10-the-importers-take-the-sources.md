---
type: journal
date: 2026-10-10
---

# 10 October: the importers take the sources, and the contract learns to talk

The day began with two built-in workers still reading sources inside the binary, and Definitions
that said what shape a field had but never what it meant. It ended with Withings and the blood
pressure exports reaching Anam only through importers, a CPAP importer live with 1,317 nights, every
bundled Definition described field by field, and a way for an importer to say what its source no
longer holds. Some 25 merges to core, most of them before dinner, then two audits that turned into
eight cards, three of them built before midnight.

## The importers take the sources

Withings went first (card 54). The importer, `anam_withings`, had been ready for a day, held on one
question from card 76. Moving the Withings reader from Go to Python changed the bytes of 612 values
without changing a single number. A SQLite REAL has no text form of its own, so `CAST(value AS
TEXT)` gives whatever the SQLite build renders. The Go driver wrote `29.371000000000002`, Python's
SQLite writes `29.371`. Same double, different content hash. Matt chose the shortest decimal that
reads back as the same double, which is portable, and accepted a one-time cost. The cut-over took
the entry out of `imports.yaml`, turned the importer on, and polled. The first poll gave 5,309 no-op
and 222 superseded, the second all no-op. Then the worker came out of the binary (16f5350), so an
`imports.yaml` that still names withings now fails as an unknown source. Nothing should read one source twice.

Blood pressure followed the same path (card 80). `anam_bp_csv` had been live since v0.204 at 234 of
234 no-op, so the worker went at 18:41. The records importer still admits the 25 diary readings.

Then the question underneath both. Documenting `health.blood-pressure` bumped it to v3, and the gate
admits only at the current version, so the importer's own copy at v2 was refused. The words had
changed and the contract had not, and still both sides had to ship together. The fix chosen was not
to make the gate cleverer but to give each Definition one owner (Matt, card 86, [[one-owner-per-definition]]). `health.measurement`
left the bundle for the Withings importer. The deploy proved the takeover as predicted. The type
read 404 until one Withings poll, the importer registered its byte-identical copy, adopted at v3,
and 5,531 facets resent as no-op.

The DreamStation importer went live through the importers stream around noon (card 70). Its first
poll admitted 1,317 `sleep.cpap-session` facets, W 21195 to 22512. Core added a diamond in the sleep row.

## Words on the contract

Card 71 started as an inspector page and became a documentation pass. The design panel (Tufte ×
Majors, Naur × Procida, and a survey of JSON viewers) agreed on one unit, a row per field keyed by
its JSON Pointer. The meta block's pointers (identity, fact time, series) became badges on the rows
they point at, so a reader sees at once that `date_excel` is both the clock and half the identity.

Writing the words found two bugs. The closed-schema lint walked every key, so an example payload
holding `"type": "object"` would have been read as an open subschema. It now skips the value keywords
and walks `properties` and `$defs` by field name, so a field called `default` is still checked.
And `lab.result` refused `time_unknown`, which its own extractor writes on an untimed draw. Neither
had shown before, because no Definition carried examples.

By 12:09 all 27 bundled Definitions were described, each a version bump with keying and fact time
unchanged. Card 84 then re-read fifteen of them against the documents behind them. `condition.noted`
dates the request, not the draw. Technogym's `mets` and `mets_min` are reversed. Ten questions only Matt can answer are on the card.

Blood pressure then took its last two steps of the day. v4 (card 83) is the Open mHealth
blood-pressure-4.0 body, systolic and diastolic as unit-values, the time frame as RFC 3339 with the
Sydney offset, and `Seated` mapped to `sitting` ([[open-mhealth-is-the-payload-vocabulary-for-health-measures]]).
The source's own fields stay beside it as text, so identity and fact time hold and every reading
supersedes in place. It was breaking in one window. The deploy, the importer and a reshaped diary
had to go together. The window ran on v0.220 after a backup, with 234 export and 25 diary readings
superseded, nothing admitted, W 22512 to 22771. The only outage was 30 seconds on the gateway,
because the importers' entrypoints had been committed without the execute bit. The evening audit
then found the export carried six columns v4 did not name, and v5 (card 97) took them as text, byte
for byte the importer's file.

## Forgetting, carefully

An importer that resends everything can say what is gone. Card 78 lets it label a run's batches as
parts of one snapshot. Anam records the facet ids each part carried, and once the snapshot is
complete it reports the source's live facets that were not carried. The design question was whether
the importer should then retract them itself. It should not. The gate decides a no-op by content
hash alone, so when a wrongly tombstoned row is sent again, the resend is a no-op and the facet stays
dead ([[tombstone-is-a-record-event]]). No event reverses a tombstone. A bug that labels a partial
resend complete would delete everything it forgot to send, while a fact that stays live a few days
until someone confirms costs almost nothing. So it is report-only, and `anam retract` plans from the
newest complete snapshot ([[retraction-by-resend-reports-and-never-tombstones-itself]]).

The audit found the hole the same evening (card 94). Absence was judged from the snapshot's end, so
an incremental run admitting beside a multi-part resend could put a held facet on the retract plan.
It is judged from the snapshot's start now. That bumped the operational store to schema 7, which is
rebuilt on deploy because the tier is derived.

Retraction itself got cheaper in the morning (card 65). The gate replayed the whole ledger from seq 0
to learn whether one facet was already tombstoned. A facet's state depends only on its own events, so
it now folds just those, through an index created idempotently on any writable open. There was no
schema bump. An index holds no rows and fires no trigger, while a bump would have made the deployed
daemon refuse the live ledger.

## Reads that compose and cite

Card 63 made `serve` and `verify` compose from the last mint run instead of folding the ledger.
Card 64 did the same for a snapshot read at a date. Nothing after D can change a snapshot at D, so
the prefix of buckets up to D's holds every series' history, and no series index is needed. Card 85
bounded the inspector's facet and event lists, 7.9 MB and 6.5 MB pages at the live record, to 300 rows a page.

The API grew what the MCP adapter would need. Step 0 of card 72 split the tool mapping out of
`docs/API.md` into the new `anam-mcp` repository, and its review of the live daemon became cards 87
to 93. Card 87 was built that night. `POST /v1/receipts` publishes a timeline read through a
projection, the receipt records the narrowing, and verify replays it through `serve.Narrow`, the one
function both sides call, so an answer and its citation cannot drift
([[published-is-a-receipt-not-a-record-event]]). Reads also take `w`, a watermark, so a follow-up
stands on exactly what a cited read knew. 

Late in the evening `note.annotation` arrived (card 104), a note the person writes about one or more
facets. It names them by id and never touches them. Which version of each was current is the ledger's
to say at the note's watermark, so nothing of that is stored.

## Hickey × Armstrong

**Hickey.** Two complections came apart. The Definition held a contract and its documentation, and
because both were versioned as one value, a word edit forced a joint deploy. One owner per type left
them braided but put the braid in one place. Retraction had braided "what is gone" with "make it
gone". Card 78 separated the report from the act, and a person sits between them.

**Armstrong.** The two audits (`reviews/luminary-day-audit-2026-10-10.md` and the importers' twin)
found real faults in work merged hours before, so the review stays. Two small ones came from process,
not code. `check.sh | tail -1` reports `tail`'s exit, so a commit ran on a failed gate. And the merge
of card 104 hid two failing tests behind a pipe, fixed in e7d9dd7. Capture the exit code before
filtering.

## Left open

- Card 84: ten field questions for Matt.
- Cards 89 to 93: the API gaps from the MCP review, among them a reader token, paging, and whether
  anything but an importer may write a source (drafted for Matt).
- Cards 96 to 101 came from the audits. 96 was done that night, 97 waited on the importer's deploy,
  98 and 99 went to the importers stream, and 100 and 101 hold the low findings.
- Card 47: the restore drill passed from a backup alone. Only the off-NAS copy remains, Matt's choice.
- Card 55: Garmin's daily types, held on Matt's choices and a cross-type identity question for core.
