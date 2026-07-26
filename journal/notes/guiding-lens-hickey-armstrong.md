---
id: 202607100008
type: decision
title: "Guiding design lens — Hickey × Armstrong"
created: 2026-07-10
status: evergreen
tags: [anam, decision, process, luminaries]
project: anam
related: ["[[verbatim-payload-metadata-separation]]", "[[reference-implementation-mint-cli]]"]
---

# Guiding design lens — Hickey × Armstrong

**Decision.** Anam's standing design lens is **Hickey × Armstrong** (*structure × runtime*), run via the
`luminaries` skill on every design review and code audit. Recorded in `AGENTS.md`.

## Why this pair

Anam is a *running software system* holding *immutable facts*. That is the pair's exact target:
Hickey watches the shape at rest (what's complected, where state lives, is it simple or just easy);
Armstrong watches behaviour under fault (what crashes, who supervises, what's the message contract).
And they converge on the thing Anam is built from — **immutability** (Hickey's *value of values*,
Armstrong's *share-nothing + crash-clean*) — so the lens reinforces the grain rather than fighting it.

## What the lens changed in the spec

Running the *Before — design review* pass on the draft produced binding rules:

- **Anam is Datomic-shaped (Hickey).** Append-only immutable facts, epochal time, current state as a
  fold. Never overwrite a fact; [[verbatim-payload-metadata-separation|hold payloads verbatim]].
- **Name the error kernel (Armstrong).** The append-only ledger + admission-gate invariants
  (three pillars, idempotency, immutability) are the small trusted core that must not corrupt.
  Everything else is disposable and rebuildable from source.
- **Quarantine the substrate (Hickey).** The stores are [[substrate-is-accidental|accidental
  complexity]]; only `internal/store` may name them. The single-table `item` is a marshalling envelope at
  the storage edge, not a domain type — the seven domain values stay un-braided in code.
- **One truth per fact (Hickey, no PLOP).** The artifact stores no status of any kind; current
  status/endorsement is always a fold output, never written back.
- **Cursor is a cache, not truth (Hickey).** Derivable, safe to lose; ingest idempotency makes a lost
  cursor a re-scan.
- **Crash-clean + restart (Armstrong).** Idempotent, atomic `commit` — a worker may die mid-batch and be
  re-run to known-good; no partial facet.
- **Envelope is the only ingress; every external call is timed and fails fast (Armstrong).**

## How to use it

On any non-trivial change: `luminaries` skill → *Before* (design) or *After* (audit). Surface the
tension between the two eyes; never average it.
