---
id: 202607100021
type: decision
title: "Any state = nearest checkpoint + replay-since; minting only bounds replay"
created: 2026-07-10
status: evergreen
tags: [anam, decision, mint, checkpoint, ledger, event-sourcing]
project: anam
related: ["[[ledger-and-canonical-two-surfaces]]", "[[span-is-the-mint-unit-not-the-read-filter]]", "[[brooks-below-the-gate-beck-above-it]]"]
---

# Any state = nearest checkpoint + replay-since; minting only bounds replay

**Decision** (Arch ADR 0032, adopted). The append-only ledger is the system of record.

> **Any state = nearest checkpoint + replay of the record-events since that checkpoint's watermark.**

One indexed range-scan. The current state is the head case; a historical `as-of` is the same scan with an
upper bound. State is a **query**, not a stored record.

## What this corrects

The intuitive reason to mint is **freshness**: publish a version periodically so consumers always find
something recent. That reason is wrong, and it produces a periodic tick that mints quiet spans for
nothing and under-mints bursty ones.

> The ledger is *always* current. A read replays to the head. **Freshness is a property of the read**,
> paid by a range-scan — not something a periodic write buys you.

So minting has exactly one remaining job: **bound read-time replay depth**. Without checkpoints, a
twenty-year span replays twenty years of events on every read. A checkpoint every *N* events caps replay
at *N*. That is all.

Two consequences fall straight out:

- **Cadence is adaptive, not periodic** — time-OR-size-OR-rate, whichever trips first. A quiet bucket
  mints nothing; a burst earns a checkpoint sooner. Cost tracks *change-pace*, not wall-clock.
- **Mint late.** If replay depth is not yet a measured problem, checkpoints buy nothing. Build the fold
  and the read first; mint when the range-scan gets long. This is the same conclusion
  [[brooks-below-the-gate-beck-above-it|Beck's lens]] reached independently — a checkpoint is a *cache of
  the fold*, and you don't cache what isn't slow.

## The watermark machinery

`W` is the **stream watermark**: a monotonic counter advanced by *every* record-event that can change the
live-set — including a pure status transition like a tombstone, which is born of no new facet.

> If a tombstone did not advance `W`, a quiet tick would reuse a checkpoint value that still lists the
> retracted facet.

Per minted span, keep `Wᵥ` — the watermark the last checkpoint actually folded. Then "does this span need
re-minting?" is one comparison: `W > Wᵥ`. That is the entire quiet-versus-busy decision, and `Wᵥ` is the
load-bearing half: without it, a tick cannot tell whether anything happened.

## What you keep

Sparse adaptive checkpoints (replay floors), the sealed final when a span goes dormant, and served
receipts. **Unserved interstitial checkpoints are garbage-collectable** once a later one subsumes them
and no receipt pins them — they were only ever floors, never records.

The record is the ledger. Everything else is scaffolding you are allowed to throw away.
