---
type: journal
date: 2026-07-10
session: 3 (continued)
title: "R3 — the gate, plus a window to watch it through"
tags: [anam, journal, r3, gate, def, inspect]
---

# R3 — the gate, plus a window to watch it through

The same session, after the R2 harvest. Two things landed: a diagnostic web inspector (out of order —
Matt wanted to *see* the data as it builds), and R3 complete.

## The inspector, first

`anam inspect` — the first `cmd/anam` binary and three read-only pages over the ledger: facets at their
current version in event-time order, a facet's metadata + verbatim payload + its events, and the raw
record-event stream. Two disciplines held under pressure to be helpful: the facet list shows **last
event**, not "status" — the fold is R4's, and the inspector must not pre-empt it with a plausible wrong
one; and it binds loopback by default, with LAN exposure a per-run flag rather than a baked-in address.
Live at :8787 over a seeded `anam_demo` table.

Also recovered: the capture→compose→use diagram, committed to `docs/assets/` this time — the second
artifact this project nearly lost to a transient channel (the BP CSV is still gone).

## R3 — complete, or not at all

Built bottom-up, each layer forced by the one above:

- **`facet.Derivation`** — capture and derivation provenance unbraided; a captured fact with a model is
  no longer representable. Parked at R0, cashed in here because the constructor needed the shape.
- **`facet.New`** — the only way to build a Facet. It computes the ID from the source ref and the hash
  from the payload *itself*, so neither can be asserted independently of what it describes — the
  [[canonicalisation-fixes-serialisation-not-values|Canonical]] move, one level up. Confidence pinned to
  (0, 1] after two sessions as a bare float.
- **`internal/def`** — [[facet-definition-format]] made real: meta DSL + the four borrowed JSON-Schema
  keywords, JSON Pointer addressing, three validation modes. The load-bearing test proves §6a's sentence
  directly: correct a value and the source ref does not move.
- **`internal/gate`** — Resolve (three assurance arms), Commit (the pillar pipeline, then
  admit/supersede/no-op decided by identity × content), Retract (a record-event, nothing else — the
  corrected doctrine's first consumer). The retry loop the ledger refused now exists, bounded, with
  record-time stamped inside each attempt.

One new decision earned a note: [[identity-is-definition-owned]] — the gate recomputes `source_ref`
from `identity_key` and rejects a worker that disagrees, settling the definition-format note's open
thread in the strongest form.

The choreography test is the MVP's §7 idempotency criterion running three rungs early: admit v1 →
byte-identical re-ingest is a no-op with `W` unmoved → corrected value supersedes at v2 naming its prior
→ retract appends only a tombstone — asserted against the exact event stream the ledger holds, on the
real endpoint. Both load-bearing guards (identity ownership, no-op-not-supersede) were broken on purpose
and watched to fail.

## Carried forward

- Double-`Retract` appends a second tombstone until the fold can refuse it (noted in the code).
- `internal/app` (cobra wiring) has no tests — exercised by running the binary, said plainly.
- **R4 next**: the fold with its *explicit* kind→status map (`Status(event.Kind)` compiles and must
  never be written), `anam timeline`, and the rebuild test that makes "Anam owns no fact" executable.
- The BP CSV: still missing, still blocking `bp-csv` capture at R5.
