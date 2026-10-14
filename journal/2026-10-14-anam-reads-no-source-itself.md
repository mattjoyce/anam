---
type: journal
date: 2026-10-14
tags: [anam, journal, importers, library, config, queue, rebuild]
---

# 14 October: Anam reads no source itself

The day had two halves. In the morning the three repos were made ready to be read by strangers, and
Anam learned to take a document's bytes over the API. By evening the last two sources Anam still read
for itself, records and documents, had moved out to their importers, and the library had been
redesigned around a document's sha256. Overnight came card 151's sweep of every hard-coded value, card
110's queue deadline, and a drill that rebuilt part of the record from its sources alone. It ended
with 2,939 facets rebuilt facet for facet, and a clear list of what a rebuild cannot bring back.

## Ready for strangers

Matt chose Apache-2.0 for all three repos (cards 142 to 144), and LICENSE and NOTICE went into each
before seven. gitleaks over Anam's 298 commits found three lines that looked like keys and were not.
The anam-service skill names a token file beside a zone, and two test files hold the fixed, published
test token. `.gitleaks.toml` allowlists each by its line shape, the test token only in test files, so
a scan of the whole history is clean without blessing anything broader.

anam-importers had a quieter problem. Its Garmin fixtures had been copied from the watch: a real day's
steps and heart rate, a real night, three real session start times and a real alert. They moved to
2015 and 2014, years that share 2026's and 2025's calendars, so weekdays and Sydney's daylight-saving
changes still fall where the tests expect them and no value points at a real day. The README now says
whose importers these are and how to run one without ductile, and CI runs the tests on Python 3.10
and 3.12. anam-importers went public with its history on Matt's word. Anam's own flip still waits on
Matt's calls about LAN addresses, the journal and the README.

## The documents leave, and their bytes arrive

Card 102 gave Anam `POST /v1/documents`, so an importer can hand over a document's bytes and Anam
alone writes the library. The write goes through the write queue, so a backup never sees half a file,
and lands through a temporary file renamed into place. Card 103 built `anam_documents`, the documents
worker moved out. It reads each file once and sends the bytes before the envelopes that name them, so
no bytes ride in a ductile event. Everything the built-in worker had admitted resent as no-op.

That morning's design named a stored document by its path and answered 409 when other bytes arrived
at a held path. Matt redesigned it in the afternoon as card 150, and the redesign is simpler. The
library stores a file as `<sha256>.<ext>`, so other bytes are another file and there is no conflict to
report. A `doc.reference` keeps the path as the document's name and identity, and its sha256 finds the
bytes. A rescan under the same name supersedes, and the first bytes stay. A folder fills to
`library_folder_limit` files (1,000 by default) and then spills into `sha[:1]/`, then `sha[:1]/sha[:2]/`.
Nothing records where a file went, because lookup walks the library for the name.

## Card 119, the cut-over

With 102, 103 and 150 ready, card 119 removed the records and documents workers from the binary. Gone
with them were `anam ingest`, `POST /v1/ingest`, the operations route, the ingest coordinator, the
imports file, the library's auto-import, the carve-out in `check.sh`, and Anam's copies of the
extractors. Two things had only ever run inside an ingest, the bundled-Definition guard and the write
of the bundled Definitions. They now run when the daemon opens, each phase on its own deadline.

The cut-over was sequenced across three agents. ThinkPad.unraid moved the sources to the Library
share, mounted it in the gateway at `/app/anam-library`, and deployed v0.308-775ffb9. The records
importer resent all 2,901 records, and every one was a no-op. That one number closed card 118 as well,
since its port is proven by a no-op resend rather than by comparing outputs. One fix followed.
`documents/run.py` lacked its execute bit, which the gateway needs. anam-mcp dropped the ingest state
the daemon no longer reports. Card 70, the DreamStation importer, had been live since 10 October and
was closed too. Its leftovers became card 154 (a nightly CPAP read) and card 155 (Matt's check of the
decoder against DreamMapper).

## A config key overrides every opinionated default

Card 151 came from one sentence of Matt's: a config key overrides every opinionated default. Every
literal in core was sorted. A default is a value someone could reasonably want different, such as a
timeout, a byte cap, a retry count or a retention. Anam ended with 21, in `internal/config/limits.go`,
each with a `Default` constant equal to the old value, range-checked at load, and passed down as a
parameter. Introducing them changes nothing at deploy. `daemon.Open` checks them again, so a Config
built without loading cannot run with a zero wait. An invariant is a value whose change breaks a
contract or the stored record. Examples are the batch of 49, the schema versions, the file modes and
the token's 32-character minimum, and each now says beside it why it is fixed.

The sweep found a third kind, presentation and tuning. Inspector geometry, page sizes and store batch
sizes change no result anyone relies on, and the way to change them is to change the code. Without
that bucket every literal looks like either a missing knob or a sacred constant. `ingest_timeout`
kept its stale name, because unraid sets `ANAM_INGEST_TIMEOUT` and a rename would silently drop it
back to the default.

Cards 152 and 153 did the same for anam-importers and anam-mcp, each with its sorted list in its own
repo. The MCP sweep found one real gap. The adapter read the daemon's answer with an unbounded
`io.ReadAll`, where the CLI caps it at 64 MiB, so `ANAM_MCP_MAX_RESPONSE_BYTES` now does the same.

## Card 110, queue wait spends the deadline

A batch's deadline used to start when the write worker picked it up. A batch could wait its whole
budget in the queue and then get a fresh one, long after the importer had timed out and retried. Now
the budget is counted from arrival, as the shorter of the daemon's timeout and the caller's
`Anam-Timeout`. The wait for the queue spends it and the write gets what is left. A 10 ms batch facing
a busy queue is 503 in 10 ms. A batch whose budget runs out, or whose caller goes, before the worker
takes it is never written. The worker checks both again as it takes the job, since a select that finds
the queue free and the caller gone at once may take either. The blue team's probe became one of the
tests, with `importer_wait` set to a minute so that a pass proves the caller's clock decided.

## Hickey × Armstrong

**Armstrong.** Card 110 draws one line, acceptance, the moment the worker starts the job. Before it,
the job belongs to the caller and dies with the caller. After it, the job belongs to the ledger and
runs to the end, reporting its real outcome even if nobody is listening. Abandoning a write half way
buys nothing when the write is one transaction, and an idempotent commit makes the retry harmless. A
deadline that ignores the queue was a hang by another name.

**Hickey.** Card 141 tested [[anam-owns-the-continuity]] directly. `scripts/rebuild-drill.py` starts
a scratch daemon on an empty ledger, runs each importer's poll and submit against it, and compares the
result with live, type by type, on facet id, content hash and status. It only reads live. For records
and documents, 2,939 facets in 18 types matched exactly, in about 17 seconds. That proves the first
half of the note: identity and hash are values computed from the source, so Anam owns no fact.

The second half is what the drill could not rebuild. A rebuild loses each fact's anam-time, stamping
today instead. It loses superseded versions the source no longer holds. It loses operator retractions
of a fact the source still holds, because the importer sends it again and it comes back live. Those
are the continuity, and Anam authored them. Ledger backups are therefore not redundant with the
sources, and `docs/IMPORTERS.md` §8.8 now says so beside the rebuild procedure.

## Left open

- Matt's review of the default, invariant and presentation sorting on cards 151 to 153.
- Two questions in `docs/IMPORTERS.md` §6. Is the loss of anam-times and superseded versions
  acceptable in a rebuild, or should recovery go through card 140's migration importer? Should an
  operator's retraction of a fact its source still holds replay?
- Card 141's unraid half, the six importers that run there, briefed to ThinkPad.unraid.
- anam 5549ece and later is not deployed. Card 151 changes nothing at defaults, and card 110 changes
  behaviour. The importers now range-check their config, so the live instance configs need a look
  when they are next staged.
