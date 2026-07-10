---
type: journal
date: 2026-07-10
session: 4 (continued)
title: "R6 — changes_since, and the derived tier earns its first reader"
tags: [anam, journal, r6, catchup, ops, changes-since]
---

# R6 — changes_since, and the derived tier earns its first reader

Third rung this session. The operational store stops being an empty schema: catch-up replays the
ledger into the derived indexes, and `anam changes --since` answers the record-time question from
them. Over the real table: first call catches up 0→659 and answers in ~9s; once current, 25ms.

## The design that fell out

- **A facet_index row is a persisted `fold.State`.** `current_version + status` is exactly what the
  fold derives — so the schema gained a `status` column (v2), and catch-up became *incremental*:
  load the affected facets' persisted states, advance them through `fold.Apply` — the same exported
  kind→status map the canonical fold uses, not a second one that could drift — and upsert. No
  replay-from-genesis per sync.
- **One transaction per batch: rows + cursor together.** `ApplyReplay` moves the record-event rows,
  the facet rows, and `replay_cursor` atomically, with the cursor moved *conditionally from the seq
  it was read at* — the same shape as the ledger's watermark bump, one substrate down. A crash leaves
  the tier behind (fixable by the next sync); it cannot leave it half-applied.
- **Behind is fixable; ahead is wrong.** `Sync` treats `cursor > head` as `ErrIndexAhead` and stops:
  a derived tier claiming events the ledger doesn't hold is a mis-pointed database, not a lag.
- **Sync-on-read.** `anam changes` catches up, then queries — the §8 sentence "the fold must assert
  `last_seq == W` before folding" realised as "bring `last_seq` *to* `W`, transactionally, then read".
- gosec vetoed the classic `IN (?,?,…)` string assembly (G202). Rather than a `#nosec`, FacetStates
  does one prepared point-lookup per id — SQLite is in-process; N point reads are fine and the
  concatenation question evaporates.

## Where the rungs stand

R0–R6 done, all in-tree and green. The read-path cost that R5 measured now has its answer half-built:
the facet index exists and stays current; pointing `timeline` at it (instead of full replay + fetch)
can ride along with R8's checkpoints. Remaining: R7 (projection + receipt — the diagram's *use* tier),
R8 (mint), R9 (the proposed insight). `bp-csv` still waits on its export.
