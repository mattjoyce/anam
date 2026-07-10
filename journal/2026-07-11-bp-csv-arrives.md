---
type: journal
date: 2026-07-11
session: 5
title: "The BP CSV arrives — and its identity isn't what the tests assumed"
tags: [anam, journal, bp-csv, capture, identity]
---

# The BP CSV arrives — and its identity isn't what the tests assumed

The artifact lost since R3 turned up (`BloodPressure_11-July-2026`, 234 readings, 2025-05-29 →
this morning 05:54). Ingested the same day: `admitted=234`, re-ingest `no-op=234`, `W=893`. The
second source joined the path the first one proved — worker, bundled definition, one scope entry;
the gate, fold, timeline, changes and inspector needed nothing.

## What the real data corrected

Every test fixture since R3 had assumed identity was the timestamp (`/measured_at`). The real export
refuted that: **13 timestamps carry two distinct readings each** — the device takes several a minute
apart or faster, and the rows differ (102/72/68 then 101/72/66). Keyed on timestamp alone, the second
reading of a minute would *supersede* the first: 13 real readings silently swallowed by the
correction machinery. The fix is the definition's: `identity_key = [/date_excel, /reading]`, where
`reading` is the row's ordinal within its timestamp, composed into the payload by the worker —
because identity is definition-owned and the gate recomposes `source_ref` from the payload, so
everything identity needs must be *in* the payload. A natural key must identify, never describe; and
here the row's position within its minute is part of which reading it *is*.

## Verbatim, vindicated twice

- The remembered demo detail — a 2026-01-24 reading with comment `"No filodipine"` — exists, dated
  right, but the source actually says `"No filofopine"` (and `"No filodopine"` the next day: two
  different typos for felodipine). The payloads keep both, letter for letter. The
  medication-adherence signal survives its own misspellings; correcting them would have been
  normalisation.
- The export is a packaging mess: `,\t `-padded fields, CRLF, quotes *inside* the padding, a trailing
  empty column, unpadded month/day digits (`2026-1-24`). The worker strips packaging and touches no
  value — numbers stay strings (`"137.0"`), per the MVP's "values kept as source strings".

## Named costs

- **Event times are the device's local clock, stored as written.** The definition's layout asserts no
  zone, so `2026-1-24 06:36:00` is parsed as-if-UTC. Against the Garmin data (true UTC) BP facets sit
  ~10h early on the shared timeline. If cross-source alignment ever matters, definition v2 can assert
  `+10:00`; the payload's raw string is untouched either way.
- One `#nosec G304` (os.Open of the operator's own `--csv` path) — needs Matt's eyes per AGENTS.
- The `changes --since 2026-07-11` surprise that wasn't: this morning's ingest happened on
  2026-07-10 *UTC*. Record-time is Anam's clock, and Anam's clock is UTC.
- A ledger test flaked once under the full `-race` suite and passed on every re-run, detail lost to a
  grep. If it recurs, capture the failure before touching anything.

## Where this leaves the MVP

Both real sources are in: 893 facets, five types, one timeline from 2024-01-01 to this morning.
Remaining rungs: R7 (projection + receipt — `bp` is now a *meaningful* projection), R8 (mint),
R9 (the proposed insight). The demo scenario in MVP §9 is now fully stocked.

## Addendum, same day: safepath and declared imports

Two hardening moves, prompted by the `#nosec` needing eyes:

- **`internal/safepath`** — the one place operator-supplied paths are handled. `Expand` (~/$VAR,
  absolute, cleaned) for paths that will be created; `ResolveFile` (symlinks followed, must be a
  regular file) for paths about to be read; `Open` as the codebase's single audited `os.Open` of a
  variable path. The per-worker `#nosec` disappears; the justification lives once, where the
  guarantee is established. Config's `db_path`/`artifact_dir` expand through it too. healthdb
  resolves *before* assembling its SQLite DSN — a DSN is a little language, and an unresolved `?` in
  a path would parse as options, not filename.
- **Imports are declared, not improvised.** `anam ingest` reads `imports.yaml` (--imports →
  `$ANAM_IMPORTS` → user config dir) naming each source and its file, and ingests only what is
  declared — all of it with no argument, one source's declarations with one. The `--db`/`--csv`
  flags are gone. A path typed ad hoc is provenance that evaporates with the shell history; a
  declared import is reviewable and repeatable, the same reasoning that keeps config keys strict.

Verified live: one naked `anam ingest` ran both declared sources — `no-op=659`, `no-op=234`, `W`
unmoved — an undeclared source is refused, and a declared path that resolves to a directory is
refused by name.
