---
type: journal
date: 2026-07-10
session: 4
title: "R4 — the fold, the timeline, and the oracle"
tags: [anam, journal, r4, fold, timeline, rebuild]
---

# R4 — the fold, the timeline, and the oracle

One rung, three deliverables, exactly as R3's journal ordered them: the fold with its explicit
kind→status map, `anam timeline`, and the rebuild test that makes "Anam owns no fact" executable.

## The fold

`internal/fold`. Pure at its core — record-events in, per-facet `State` out — with `Read` composing
the ledger's two access paths around it (head → replay → fold → fetch each live artifact).

The warned-about cast earned its warning. `facet.Status(ev.Kind)` compiles and is wrong in the one
case that matters: a `superseded` event does not leave the facet superseded — it re-admits it at the
higher version; only the displaced *version* is superseded. The cast would silently drop every
corrected fact from the canonical. The explicit switch is the map, and a sixth kind now lands in a
default arm that stops the fold instead of flowing through a conversion unexamined. `StatusSuperseded`
therefore never appears as a logical facet's state — a sentence the `State` doc says out loud.

Two decisions worth recording:

- **The value hashes facts, not bookkeeping.** A canonical `Entry` carries only rebuild-stable fields
  — payload, content hash, event/recorded time, source identity. Artifact version, ingested time and
  every seq/watermark are deliberately absent, because drop-and-re-ingest reassigns them (a superseded
  chain collapses to v1). The one admitted exception is `Status`: endorsements are Anam-authored and
  come back from backup, not replay; the field is carried because projections will gate on it.
- **`Read` retries the benign race and stops on the impossible one.** A supersession landing between
  the replay and a `CurrentVersion` fetch shows up as an artifact *ahead* of the fold — re-run from a
  fresh head, bounded. An artifact *behind* the fold cannot happen (artifact and event are atomic), so
  it is `ErrCorruptStream`, not a retry.

The fold reads the ledger directly, not the operational indexes — those still have no reader, and
"smallest-step above the gate" says the `last_seq == W` catch-up assertion arrives when an index does
(R6). Also closed: the R3 gap where a double `Retract` appended a second tombstone. The gate now folds
the facet's events inside its retry loop and reports a no-op — the fold's first consumer.

## The timeline

`anam timeline` — the whole-person read, no span required, which is the sentence this project exists
to make true (§1). `--from/--to` and `--type` are lenses over the same read, never a different one; a
bare `--to` date includes its whole day. `--json` emits entries in the canonical serialisation with
bookkeeping on stderr. The header prints `W` and the canonical hash, so the fold's identity is
observable on every invocation — over the R3 demo table it read W=5, live=2, the tombstoned facet
correctly absent.

## The oracle

The rebuild test lives a history against a simulated source — three admissions, a byte-identical
re-ingest (no-op, `W` unmoved), a correction (hash moves), a source-side deletion plus retraction
(hash moves again) — then drops the table, inits, re-ingests what the source now holds. The canonical
comes back byte-identical at a *different* watermark, and the test asserts the difference: if the
watermarks matched, the oracle might be hashing bookkeeping. This is MVP §7's "switching Anam off
loses no record", automated from this rung onward.

One honest wrinkle: retraction is only rebuild-stable because the test deletes the source row before
retracting — a tombstone over a row the source still holds would resurrect on re-ingest. That is
doctrine, not accident (the sources are authoritative; Anam's tombstone is Anam-authored), but it is
worth remembering when a real capture worker meets a real deletion at R5.

## Carried forward

- `--span` (saved bookmarks) waits until bookmarks exist to be named.
- The operational store's derived indexes still have no writer or reader; catch-up and its
  `last_seq == W` assertion arrive with `changes_since` (R6).
- The inspector's test harness needed a wider request timeout: the rebuild test's table drops
  serialise DynamoDB Local under the full `-race` suite. Latency was never what those tests test.
- **R5 next**: the `health-db` capture worker and the first `health.metric-group` definition — real
  payloads, real time semantics. The BP CSV is still missing, still blocking `bp-csv` capture.
