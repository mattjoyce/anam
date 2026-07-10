---
type: journal
date: 2026-07-10
session: 3
title: "R2 — the store, and five corrections on the way to it"
tags: [anam, journal, r2, store, ledger, ops]
---

# R2 — the store, and five corrections on the way to it

The session that was "just build `internal/store`" spent its first half rewriting the doctrine the store
was to be built on, and the second half building it. Thirteen commits: five doc corrections, three
packages, and this journal. The interesting part is not what was built — it's *how each correction was
found*, because the pattern repeated five times and it wasn't the same finder twice.

## How the corrections were found

**1. The design review found what the docs couldn't see.** `AGENTS.md` mandates a `luminaries` pass
before non-trivial work, and the handoff warned that skipping it once had already cost a live bug. The
*Before* pass on the R2 design surfaced the biggest find of the session: §6a's tombstone — *"a
status-bearing artifact version"* — had been **contradicted by our own type system since R0**
([[tombstone-is-a-record-event]]). No `Status` field, a reflection test forbidding one, nowhere to put
`tombstoned`. Arch's sentence, inherited verbatim, true of Arch, false of Anam. Nothing had noticed for
the project's whole life because nothing had yet needed to *write* a tombstone. The review also caught
the sort-key braiding ([[identity-first-keys-and-no-indexes]]) and the three-document disagreement over
`published` ([[published-is-a-receipt-not-a-record-event]]).

**2. The captain rejected a rule — twice.** I presented the review's SQLite conclusion wrapped in the
doctrine's own voice: *"SQLite holds nothing that cannot be rebuilt by replaying the ledger."* Matt:
*"this is wrong thinking… it's entirely operational."* My fix was a sharper universal — *"everything has
exactly one home and a named way back."* Matt again: *"don't force the rules so hard."* The correct fix
was no rule at all: a per-table recovery tier ([[sqlite-is-the-operational-store]]). The meta-lesson got
its own note — [[doctrine-aphorisms-outlive-their-reasoning]] — and the second half of it is the part
worth keeping: **the fix for an over-strong rule is not a better-worded rule.**

**3. One-line questions collapsed over-designs.** Twice I built castles and Matt knocked them down with
a sentence. I invented birth-bucket pinning to fix a "permanent double-count" when a corrected facet
changes buckets; *"it will need to have issued a supersede event on the old facet — how else can you
avoid double counting?"* dissolved it, because I'd been composing checkpoint *values* while the read
path is *floor + replay* ([[checkpoint-is-a-replay-floor]]). Then I designed per-span watermark
machinery for the weekly mint; *"this is a prototype"* deleted it in favour of always-fold,
dedup-by-hash. Both errors had the same root: reasoning about artifacts apart from the path that
consumes them.

**4. The spikes found what documentation couldn't promise.** Three claims underwrote 🔴 findings, so
they were tested against the live endpoint before being written into doctrine: an unseeded counter
rejects every first commit forever; `TransactionCanceledException` is positional and carries the
existing item back; unpadded version keys return v9 as the max of twelve. All three held. The third is
live today in `mint-cli`'s `AllSnapshots`.

**5. The tests found the last one.** The ledger's error map decided cancellation arms by *position
order*; the live stale-writer test mis-typed an ordinary race as `ErrCorrupt`, because a stale writer
fails **two conditions at once** (its seq is taken *and* the counter moved). The fix is precedence.
This one no review caught — only contact with a real concurrent write.

Five corrections, five different instruments: the mandated lens, the captain's pushback, the captain's
one-liners, the spike, the failing test. No single practice would have caught them all. That is the
reflection worth keeping.

## What R2 shipped

- **`internal/span`** — the bucket is one ISO week of event-time, UTC, half-open, tiling. The ISO
  week-year trap (2027-01-01 ∈ `2026-W53`) is pinned by a mutation-checked test. 100% coverage.
- **`internal/store/ledger`** — `Init` (table + seeded counter + schema-mismatch refusal for stale
  `-sharedDb` tables), `Admit`/`Mark` (one `TransactWriteItems`; two calls so `{no artifact, admitted}`
  is unrepresentable), the precedence error map, `CurrentVersion`, `Replay`, `Head`, `Drop`. Append-only
  is rung 2, so an AST-walk test forbids the mutating APIs. Real DynamoDB Local, unique table per run.
  82.6%.
- **`internal/store/ops`** — the operational schema with per-table recovery tiers; `span_head` as a
  view over the checkpoint log; the lost-file test as the executable meaning of the derived tier. 81.5%.
- **`internal/facet`** — `EventPublished` removed; five kinds, one per status.

Also: the module floor moved to go 1.24.0 (the AWS SDK wrote a patchless `go 1.24` that resolves to no
downloadable toolchain), and scope decisions got names: definitions out of the ledger, no secondary
indexes at all, roles-grant-projections at the serve boundary and nothing lower — Anam is a **neutral
data layer**; it stores and retrieves what it is asked, and health is a namespace, not the domain.

## Where we stand

R0 ✅ R1 ✅ **R2 ✅**. Next is R3 — the gate: `resolve` + `commit` as pure logic over primitives that now
exist (the retry loop, hash comparison, supersede-vs-no-op, three pillars). The `facet.New` constructor
gate and the explicit kind→status fold map are queued behind it (R3/R4 in `docs/MVP.md` §2). The BP CSV
is still missing — re-send and commit to `testdata/` before R5.
