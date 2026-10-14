---
type: journal
date: 2026-10-09
cards: [59, 62, 46, 69, 51, 52, 53, 56, 70]
---

# 9 October: the record moves to unraid, and reads start from checkpoints

The day began with a number that blocked the move. Every API read folded the whole ledger, the NAS
took 2.7 s to fold a 14,000-facet scratch ledger, and the daemon's default timeout was 5 s. By
breakfast reads composed from checkpoints, and by mid-afternoon the record lived on unraid. The
evening closed Definitions to standard JSON Schema, gave importers a door of their own, and started
the first one. Overnight the first scheduled backup ran, and a decoder for the CPAP machine's SD
card existed in its own repository.

## Reads that start from checkpoints

Card 59 began with a bench, not a design. `anam-bench gen` builds a synthetic record through the
real gate, with corrections, retractions, a mint and a tail after it, in three shapes up to 600,000
facets.

The first profile overturned the assumption. A full read of the real record took 975 ms, and
replaying its 19,587 events was 45 ms of that. The rest was fetching 14,000 live facets one query
each: 44% of the CPU was SQLite's parser preparing the same statement 14,000 times, 25% the
connection pool's mutex. Fetching 500 versions per query, a `VALUES` list joined to the primary key,
took the read to 341 ms. The architecture work that followed was still needed, because the
remaining cost still grew with the ledger, but the profile changed the order. Fix the fetch first,
then know which gain came from which.

Then the reads changed shape. A timeline read starts from the checkpoints of the latest mint run at
or below its watermark and applies only the events after it ([[state-is-checkpoint-plus-replay]],
[[checkpoint-is-a-replay-floor]]). A read bounded by fact-time decodes only the buckets it overlaps.
Every other bucket is checked against its hash and streamed into the whole value's hash undecoded,
so the header's whole-person hash stays exactly what the full fold gives.

The numbers, one-month reads, medians. On the ThinkPad the real record went from 980 ms to 62 ms
minted at head, and the 600k synthetic from 46.8 s to 1.5 s. On the NAS the real record went from
2.7 s to 62 ms. Narrow reads ran faster on the NAS than the laptop, because its Ryzen has SHA-256
instructions and hashing was by then most of a narrow read. At 600k facets 54% of a one-month read was SHA-256, the price of promising a whole-value hash on every read. A Merkle root would remove it, at the cost of a hash that depends on the bucket layout (card 60).

## Buckets split on cost, along the calendar

A fixed monthly bucket is wrong at both ends of a personal record: dozens of near-empty files for
the sparse early years, and one late correction reopening a dense month whole. Splitting by count
would fix the sizes and break the property composition relies on, that a fact-time sits in exactly
one bucket. Matt's resolution was that performance decides when to split and calendar time decides
where. A bucket starts as a year and splits into months, then days, once its value passes
`checkpoint_budget`, 1 MiB by default. Bytes, not a timing taken at mint, so the NAS and the
ThinkPad cut the same layout. The budget came from the bench: about 46 ms/MB to re-derive on the
NAS, so 1 MiB keeps one dirtied bucket near 50 ms.

The layout needed no new table. A bucket is split exactly when a checkpoint of one of its children
exists, so the layout at any watermark is read off the checkpoint log. The real record re-minted
into 47 checkpoints, 13 whole years and 34 months, where monthly had 146.

Card 62 then made the mint itself cheap. A mint rebuilds only the buckets the tail since the last
run touched, and seals the log at its row count, so a mint that crashed after writing sends the next
mint and every read to the full fold. At 600k facets, 128 ms against 19.5 s. `--full` keeps the full
fold as oracle and repair.

## The move

At Matt's "time to migrate" the record left the ThinkPad (card 46). The ThinkPad's ingest
stopped, both stores were copied with `sqlite3 .backup`, and the database hashes and 223 files
matched by sha256 on each side. The container came up at W=19631 with 14,014 live facets, as on
the ThinkPad, and its first gateway ingest changed nothing.

Two things went wrong, and both were caught. The first mint hit the 5 s default timeout partway,
which is exactly the crashed mint card 62 had handled that morning, so the next run went full. The
container got `ANAM_TIMEOUT: 30s`, the full mint took 9.5 s, and the next one from the last run took
0.06 s. A local `anam ingest` against the renamed ThinkPad directory then created an empty
operational store before refusing, which was fixed to refuse first and create nothing.

A third fault surfaced in the evening. The compose file tags its image `anam:${ANAM_VERSION:-dev}`,
so a maintenance run without the variable took a stale image, and `up -d` restarted the daemon on
it. Card 56's dry run refused on the old code, nothing was written, and the daemon was back on the
right version a minute later. The runbook now says to export the version first.

## Backups as the daemon's own operation

Card 69 made backup a write, not a file copy beside the daemon. `POST /v1/backups` runs on the write
queue: each store copies itself with `VACUUM INTO` inside one write, so nothing lands between them,
then artifacts and documents copy as files under a `.partial` name, renamed when whole. Fourteen are kept, on a different disk from the record. The first copy opened as the same ledger at the same W, its append-only triggers intact. The gateway asks for one at
02:30 Sydney. The first scheduled run took 4.6 s for 82 MB.

## The contract tightens, and importers get a door

Card 51 let the gate compose `source_ref` from the Definition's identity key, so an envelope may
leave it out ([[identity-is-definition-owned]], [[source-ref-is-identity-not-content]]). A re-ingest
of all five sources was 13,996 no-ops. Card 52 made every Definition closed JSON Schema, draft
2020-12. Every field a payload may hold is declared, and anything else is refused. The 27 manifests
were generated from every artifact version in a copy of the live ledger, and a stock Python
validator admitted all 19,564 versions. A re-ingest through the new gate refused nothing. The
schema validator was the day's one new direct dependency, accepted by Matt.

Card 56 rebuilt `exercise.session` in the closed format, conforming to IEEE 1752.1 and generated
from the 907 sessions the Technogym and Strong extractors write. Technogym's `Mets` turned
out to be `MetsMin` times the minutes in all three sessions that carried it, so the names described
the wrong numbers. Matt judged the field bespoke, and it was dropped. Ten minutes of arithmetic
settled what the carry-everything principle could not. The live move retracted exactly the 410
`gym.visit` facets the new type replaced, and an `--as-of` read before it still shows all 410.

Card 53 closed the night. `POST /v1/envelopes` takes a batch from an importer under its own token,
admitted whole or refused whole with every failing envelope named, and `POST /v1/definitions`
registers the types it emits. A registered version is immutable. Within minutes `anam-importers`
had its first commit, a shared Python base, `anam_submit`, and the Withings importer (card 54).

## The CPAP machine

Matt uploaded the DreamStation's SD card and asked for a clean-room importer, "something we can
share with the world" (card 70). The decisions came first. One facet per device session, because
that is what the machine reports, since it starts about 40 sessions a month. LOINC's PAP panel is
defined per 24 hours, so it names the nightly read, not the fact. A statistic the device does not
report is left out of the payload rather than written as zero, which OSCAR learned at its schema
v19. The "summary" file turned out to hold no statistics, and the counts live in the events file.

By midnight `~/Projects/dreamstation` held an Apache-2.0, stdlib-only decoder with 48 tests and a
clean-room format note. Across all 1,317 sessions on the card it decoded the statistics record that
OSCAR leaves as guesses, exact in every session.

## Left open

- Card 60: a Merkle root for the whole-value hash, which still grows with the record.
- Card 67: prune the ThinkPad's pre-move copy after 2026-11-09.
- Off-NAS backups. Every daily copy sits on the NAS.
- Card 54: the Withings importer is built locally, with a value-rendering question for Matt.
- Card 70: the DreamStation decoder posts nothing yet. Its transport waits on the importer path.
