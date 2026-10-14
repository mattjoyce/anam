---
type: journal
date: 2026-10-11
---
# 11 October: health-db retires, and the importers take over

The day began with two blue-team reports on the table, one against the daemon's API and one against
the importers, and eleven cards cut from them before breakfast. It ended with the health ETL
stopped, `health-db` retired from the ledger, Garmin and Concept2 arriving through importers of
their own, and the first version of the MCP adapter reading the record. Several agents worked at
once all day, and by evening the board had an `assigned:` field and the rules for sharing a tree
were written down.

## The blue team's findings

The API review (Anam 6bc1cf8) gave five findings, cut as cards 106 to 110. The importers review gave
six, 112 to 117. The importer ones were small and closed the same morning in anam-importers 58e692d
and 13be06c. The client no longer follows a redirect off its origin, and reports a batch admitted
only on a whole admission receipt. `anam_bp_csv` refuses an export whose records cannot be told
apart, because the old worker's lenient quoting let a comment missing its closing quote swallow the
next reading. The real export of 11 July still reads 234 readings unchanged.

Two of the core findings were built the same day. Card 107 bound a snapshot part to its contents. A
part had been deduplicated by its number alone, so a resend of part 3 carrying other envelopes added
their ids to the run and moved what a complete snapshot called absent. A part now carries a digest
of its envelopes' canonical forms as a sorted set, and a changed part is a 409 before the ledger is
touched. Card 109 found that `decodeSubmission` read one JSON value and stopped, so a valid batch
followed by garbage was admitted and moved W. The handler now asks the same bounded reader for EOF.
The remaining three, 106, 108 and 110, waited for later days.

## A read gets a size

Card 88 bounded what a read answers with. `max_read_bytes` defaults to 512 KiB, measured by the same
encoder that writes the response, and over it the daemon answers 413 with a code and the narrowings
the query has not used yet. The fold still runs in full before the measure, so the bound protects
the answer and the client's context, not the daemon's work. It was live on v0.244 by the afternoon,
the unnarrowed read refused and the narrowed ones answered.

## Retiring a source is a decision

Card 55's cut-over needed every live `health-db` facet tombstoned, about 5,800 of them, because the
Garmin importer replaces that source under another name. `anam retract` could not do it. It
reconciles a rescan against the ledger and refuses a scan that comes back empty, since an empty
source and an unreadable one look the same. That guard is right for reconciling and it makes
retirement impossible.

Card 120 added `anam retract <source> --retired`. It plans every live facet of the source, reports
the count by type, and only tombstones with `--yes --max-fraction 1`. It refuses a source any import
or importer still declares, so the declaration has to come out first. The empty-scan guard was left
alone, because weakening it to fit a case it was never meant for would have cost the reconciling
path its safety ([[retiring-a-source-is-a-separate-verb-from-reconciling-it]]). Rehearsed on a copy of the pre-move ledger, it tombstoned 5,808 facets across five
types in 20 seconds, and a rerun had nothing to plan. The artifacts stay, so a read as of before
still shows them ([[tombstone-is-a-record-event]]).

## Garmin and Concept2 through importers

`anam_garmin` was built over the previous evening and finished this morning. It sends five types,
the heart rate alerts from the watch's FIT files, `sleep.episode` in the IEEE 1752 shape,
`health.heart-rate-daily` as three Open mHealth figures a day, `health.step-count` and `garmin.day`.
One shape question went to Matt. A facet's id is the source plus its identity values, without the
type, so `garmin.day` for a date superseded that date's step count. Matt chose to have each type's
id name what it is (`sleep:`, `steps:`, `day:`), so no two types from one source share an identity
([[identity-is-definition-owned]]).

Card 121 added the watch's sessions as `exercise.session` v4, one facet per FIT session at its true
UTC start, 200 sessions from 199 files. Card 58's work had found that Technogym's 93 "outdoor
activities" were Garmin's, synced through Garmin Connect and matching to the second and the calorie,
so they leave Technogym's extract and come from the watch.

The cut-over ran in the afternoon. The Garmin importer is live with 4,378 facets, 5,818 `health-db`
facets were tombstoned, and the health ETL and `anam_fit_events` stopped. The staleness check moved
with it, comparing GarminDB's newest day with Anam's newest `garmin.day`.

Card 122 followed. `anam_concept2` reads the Logbook API, since Google's health APIs do not reach
ErgData's data, and caches each result as a file so handle and poll never touch the network. Matt
decided strokes go inside the session, since a stroke has no identity of its own, and that
hand-entered results are admitted. `exercise.session` v5 carries splits and strokes. Then the daemon
refused to start, because the importers file let only one importer name a type. A bundled type
already has one author, the binary, so the rule was narrowed to registered types. Garmin and
Concept2 now both submit `exercise.session`, each under its own source. Concept2 went live with 21
workouts.

## The records worker leaves, and its fixtures

Card 118 moved the built-in records worker into `anam_records`. The proof of a port is not matching
outputs on samples, which only tests the quirks you thought of. It is the system's own idempotency.
The importer reproduces Go's yaml and number rules, and refuses a number it cannot write the same
way. Ingest through the old path, resend through the new, and require every envelope back as no-op.
Card 119, the removal of ingest from core, was cut the same morning, blocked on the three importers.

Card 58 moved the one-off extractors out of Anam into `records/extract`, each writing the same
records as before on the same exports (1,597 weights, 409 workouts, 53 pathology records). A sweep
found real readings and a real lab number in test fixtures, copied across with the code. Provenance
does not survive a copy, so fixtures are now checked against the real exports.

## The timeline catches up

The Garmin types had no style, so the live record drew in the grey "other" row under the empty rows
of the types they replaced. Card 124 put them in the sleep, heart and activity rows. The retired
types keep their styles, because a read as of an earlier W still draws them.

With 6,251 tombstones against 14,443 admitted, a third of the band was ghosts. Card 131 added a
status filter that rides every link, and a full-width toggle. Card 132 lets a shift-drag or
ctrl-click select marks and drafts a `note.annotation` for them as records YAML to install. The
inspector stays read-only on the ledger. Submitting from it is card 93's decision.

## Hickey × Armstrong

**Hickey.** Two verbs had been braided into one. Reconciling is an observation of the source,
retiring is an operator's decision, and `--retired` names the decision instead of faking an
observation. The shared-type rule had braided "one author per type" with "one importer per type".
For a bundled type the author is the binary, and two importers can share it without either owning
it.

**Armstrong.** Every cut-over step was rehearsed on a copy before it ran live, and the rehearsal is
what found that retraction could not retire.

## How the work is shared

AGENTS.md gained sections on dependencies (a new one is Matt's call), Python (stdlib plus PyYAML),
keeping cards true as work happens, and working beside other agents. A worktree is for when a
collision is likely, not for every change. Thirty-eight open cards got `assigned: None`, and the
dispatcher began assigning by that field.

Jev was tried as a card-priority classifier ([[jev-card-priority-2026-10-11]]). It agreed with the
board on 20 of 33 cards, disagreed upward, and was confidently wrong where a card argued well for
itself. Given the whole board as state it reached 21. Priority is a function of the whole board,
which one card at a time does not carry.

In the afternoon Matt wrote down what Anam is for. Ten seconds of context today, recovered six
months later, through a conversational assistant over MCP. Cards 72, 89, 93 and 104 were ordered
against it. The same evening anam-mcp got its read-only adapter, four tools and a resource, each one
call on the daemon, tested against a real daemon on a scratch ledger.

## Left open

- Cards 106, 108 and 110 from the API review.
- Card 118's real cut-over, waiting on ops to wire `anam_records`. Card 119 waits on it, 55 and 103.
- Card 111. A schema rebuild on 10 October lost the old Definition versions, which had survived only
  by accumulating.
- Card 123. The pipeline page still draws the health ETL.
- Cards 126 and 127, a Garmin pull on demand and an assistant that can ask for one.
