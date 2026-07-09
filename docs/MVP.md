# Anam — MVP

> The first slice, made concrete: what we build to prove the core idea, what we deliberately leave out,
> and how we know it worked. Companion to `ARCHITECTURE.md` and `GLOSSARY.md`.

---

## 1. The question the MVP answers

One falsifiable question:

> **Can Anam publish a useful, chronological, evidence-backed account of *one person* — composed from
> fragmented personal sources — such that the person reads one whole-life timeline instead of querying
> each source, and can prove exactly what they were shown?**

Everything in scope serves that sentence. Everything out of scope waits until it's needed.

---

## 2. The vertical slice (walking skeleton)

One thin end-to-end path must work before anything is widened:

```text
capture (health-db / bp-csv) → ingest gate (resolve → commit, three pillars)
       → append-only ledger (DynamoDB Local) → mint (checkpoint over the whole timeline)
       → canonical fold → one projection + CLI timeline viewer → receipt
```

Build it for **one facet type first**, then add the rest along the same path. **No step is stubbed —
depth before breadth.**

---

## 3. In scope

- **Two real sources:**
  - **`health-db`** — the existing `healthdata.db` (Garmin + Withings unified). Facet types
    `health.sleep`, `health.body-composition`, `health.heart-rate`, and a generic
    `health.metric-group` for the remaining groups.
  - **`bp-csv`** — a blood-pressure export. Facet type `health.blood-pressure`, one facet per reading.
- Each facet type has a **Facet Definition** (bundled JSON), coarse-grained and verbatim; decomposed
  later only where earned.
- The **three-pillar admission gate** (`resolve` → `commit`) with a static `source → types` token map.
- The **append-only ledger** as Anam's record of what it knew; **tombstone** for retraction;
  **idempotent, atomic commit** (re-ingest is a no-op; a changed row supersedes).
- **Mint:** a manual checkpoint over the whole timeline; canonical account folded on read.
- **`timeline`:** the whole-person read (the inversion) — default, no span required — with optional
  `--from/--to`, `--type`, and `--span` narrowing.
- **`changes_since`** between two record-times (GSI2).
- **At least one prune-only projection** (e.g. `bp`, `sleep`) plus the **CLI timeline viewer** (the
  diagnostic harness — proves the model, not a shipped UI).
- **Receipts:** prove exactly what a projection served at a watermark.
- **One derived-facet path:** an **insight** admitted as `proposed`, visible on the timeline, no
  write-back.
- **`init`** creates the table + GSIs idempotently; **`version`** reports the ldflags build.
- **Read-only** throughout — Anam never writes back to a source.

---

## 4. Out of scope — deferred, not forgotten

- **Binary / native-reference facets** — reserved; the early sources are all structured.
- **Adaptive mint cadence, a freshness SLO, alarms** — manual `checkpoint` only.
- **Sealed projections / a seal point** — receipts suffice.
- **Multiple device/role projections** — one or two templates plus the viewer.
- **Consent policy + classification-driven gating** — the attribute and a pass-through `Evaluate` exist;
  policy does not (`ARCHITECTURE.md` §10).
- **Token rotation; multi-scope tokens; identity graph; merge** — not applicable / deferred.
- **Non-health facets** (`calendar.*`, `mail.*`, `note.*`) — the model admits them; not built yet.
- **Storage tiering / archival** — Anam is append-only and grows; deferred, tractable because it owns
  no facts.

---

## 5. Sources and first Facets

| Source | Facet type(s) | Payload (verbatim) | Notes |
|---|---|---|---|
| `health-db` | `health.sleep`, `health.body-composition`, `health.heart-rate`, `health.metric-group` | a period's metric rows as JSON | grain = one group per period; `event_time = period_start`; `bp_*` excluded from `health.heart-rate` |
| `bp-csv` | `health.blood-pressure` | one reading row as JSON | one facet per row; `event_time` from `Date (Excel)`; values kept as source strings |
| *(agent)* | `health.insight` *(illustrative)* | the finding value | `proposed`; provenance carries method + the watermark it was computed from |

Granularity is coarse on purpose; decompose only where real event-times or finer access control earn the
cut.

---

## 6. The projections

At least one prune-only projection over the canonical account — e.g. **`bp`** (allow-list
`health.blood-pressure`, tombstones hidden) and **`sleep`** — pinned to `(canonical watermark, template
version)`. Alongside them, the **CLI timeline viewer** switching projections to demonstrate governance —
a harness for proving the model, not a shipped product.

---

## 7. Success criteria — how we know it worked

- A person opens **one** chronological whole-life timeline, each entry linked to its source evidence,
  and does **not** query the source DB/CSV directly.
- **Re-ingesting** the same source produces **no duplicate facets** (idempotency); a **changed** source
  row appears as a *superseding* artifact plus a `superseded` record-event, the original retained
  (append-only).
- **`changes_since`** correctly reports what was admitted after a given time.
- A **checkpoint** is content-addressed: **stable** when nothing changed, and its hash **moves** after
  new ingest.
- A projection **receipt** reproduces exactly the facet set served at its watermark.
- Switching Anam off loses **no** record — **rebuildable-from-source** demonstrated: drop the table,
  `init`, re-ingest → identical canonical fold.
- `scripts/check.sh` passes (fmt, vet, golangci-lint, gosec, `-race` tests).

---

## 8. Deliberate simplifications (crude on purpose, so no one mistakes a stub for a gap)

- Mint is a manual trigger, not adaptive.
- One static ingest token map; no rotation.
- One or two projection templates; the viewer is diagnostic.
- Consent is default-allow; classification is carried but not gated.
- The source database is **`/home/matt/admin/data/healthdata.db`** — `day` periods across 2026-03-07 to
  2026-06-06, plus `week`, `month` and `year` back to 2024-01. (The `/mnt/Projects/healthdata` copy holds
  a single `month` and a single `year` period and cannot demonstrate a timeline at all.) The mapping is
  period-agnostic, so whichever periods a copy holds slot in unchanged.

---

## 9. Demo timeline (illustrative, over the real data)

A whole-person read that exercises every moving part:

1. `health.body-composition` (Withings, monthly) — weight/fat/muscle trend enters the timeline.
2. `health.heart-rate` (Garmin) — resting HR / stress snapshot.
3. `health.sleep` (Garmin) — sleep-stage breakdown.
4. `health.blood-pressure` (bp-csv) — a morning triplet on 2026-01-24 with comment `"No filodipine"`
   (medication-adherence signal), placed at its reading time.
5. `health.insight` (derived, `proposed`) — "resting HR ran higher across the week you skipped
   medication" — visible, endorsable, no write-back.

The `bp` projection prunes to blood-pressure facets; `changes_since` shows the latest ingest and the
insight; a receipt pins exactly what was served at the checkpoint watermark. Re-running `ingest bp`
changes nothing — proof the ledger is idempotent and Anam owns no facts.
