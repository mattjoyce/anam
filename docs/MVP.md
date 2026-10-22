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
ingest gate (resolve → commit, three pillars) → append-only ledger (SQLite)
       → canonical fold → CLI timeline viewer          ← the thesis, made observable
       → capture (health-db, then bp-csv) → changes_since
       → one projection + receipt → mint (adaptive checkpoint, per bucket-span)
```

Build it for **one facet type first**, then add the rest along the same path. **No step is stubbed —
depth before breadth.**

**Mint comes late, and deliberately.** A checkpoint is a *cache of the fold*: it exists to bound replay,
and at this scale there is no replay cost to bound. Building it before the fold would be solving an
unmeasured performance problem. The *seam* (an anam-time watermark) is present from the start; the
machinery arrives once it earns its place.

**`timeline` comes early, and deliberately.** The whole-person read is the one claim Anam exists to make
(§1). It is the first thing that should be observable, not the sixth.

### The rungs

Each rung ends somewhere you can stop, look, and learn something. The rule for sizing them follows the
**error kernel** (`ARCHITECTURE.md` §6a): the ledger and the gate must be complete and un-provisional,
because nothing above them can repair a corrupt core. Everything above is disposable and rebuildable, so
it is built in the smallest steps that yield information.

> **Depth-first below the gate. Smallest-step above it.**

| Rung | Delivers | What it teaches |
|---|---|---|
| R0 ✅ | `facet`, `clock`, `config` | the domain values |
| R1 ✅ | substrate spikes, thrice, then thrown away | DynamoDB does atomic conditional `TransactWriteItems` *and* a monotonic counter; SQLite enforces append-only and idempotency itself. Split by role (§8) |
| R2 ✅ | `store` — idempotent `init` (table + seeded counter); the atomic `Append`; one facet round-tripped through the ledger; the operational schema, incl. the **bucket-span seam** | the key layout is real |
| R3 ✅ | the gate — `resolve` + `commit`, atomic, idempotent, supersession, tombstone | **the error kernel.** Complete, or not at all |
| R4 ✅ | `fold` + **`anam timeline`** | the inversion, visible. Plus the rebuild test (§7) as an automated check |
| R5 ✅ | capture worker `health-db`, one `health.metric-group` definition | real payloads, real time semantics |
| R6 ✅ | `changes_since` over `anam_event` | anam-time queries |
| R7 ✅ | one projection + receipt, and the viewer that switches between them | governance made real — and *legible*: every facet can say why it is in or out, at any point in Anam's own clock |
| R8 ✅ | mint / `checkpoint` per bucket-span → files | replay bounded, once measurable |
| R9 | derived insight, admitted `proposed` | the agentic path, isolated |

`bp-csv` attaches at R5 whenever the export is available; it gates nothing.

R1 and R2 record what was built at the time, on DynamoDB. The ledger has since moved to a SQLite file of
its own (journal, 2026-09-28), behind the same interface.

**The atomic write is R2's, not R3's.** `commit`'s single transaction *is* the error kernel, and
"depth-first below the gate" forbids stubbing it. So R2 ships the ledger's `Append` complete — the
transaction and its typed outcomes (`ARCHITECTURE.md` §8) — and R3 is then pure gate logic composed on
top: three pillars, hash comparison, supersede-or-no-op. R3 adds no substrate semantics. The alternative,
a plain write at R2 replaced by a transaction at R3, builds the kernel twice and trusts the second one.

Because a tombstone is an anam-event with no artifact (§6a), the ledger's primitive is **two calls, not
one with an optional facet**: admit an artifact with its event, or mark an existing facet with an event.
An optional pointer would make `{no artifact, admitted}` representable.

**Two substrates and the documents** (`ARCHITECTURE.md` §8): a **SQLite file** is the ledger (facet
artifacts and anam-events, atomic in one transaction; the watermark `W` is the newest event's `seq`); a
**second SQLite file** is the **operational store**: definitions, indexes, the checkpoint log and its
values, receipts, saved selections, cursors; the **filesystem** holds the library of source documents.
Config is YAML; definitions are JSON.

There is no transaction across two files, so everything that must be atomic lives in one. Only
`internal/store` may name any of them, and `scripts/check.sh` enforces that — because Anam owns no source
fact, a substrate swap is a re-ingest, not a migration.

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
- **Mint:** a manual checkpoint **per bucket-span**; the canonical (live-only) served as *nearest
  checkpoint + replay-since*. The whole-person view **composes** buckets, never re-folds the person.
- **`timeline`:** the whole-person read (the inversion) — default, no span required — with optional
  `--from/--to`, `--type`, and `--span` narrowing.
- **`changes_since`** between two anam-times.
- **At least one prune-only projection** (e.g. `bp`, `sleep`), **materialised as a content-addressed
  file**, plus the **CLI timeline viewer** (the diagnostic harness — proves the model, not a shipped UI).
  The viewer **switches projections** (`timeline --projection`) and can be wound back to an anam-time
  (`--as-of`); `--explain` shows every facet's verdict and the versions since displaced. The web
  inspector carries the same two controls at `/projection`.
- **Receipts:** prove exactly what a projection served at a watermark.
- **One derived-facet path:** an **insight** admitted as `proposed`, visible on the timeline, no
  write-back.
- **`init`** idempotently creates the ledger file, pins its person, and creates the operational
  schema. **Append-only triggers** on every ledger table refuse `UPDATE`, `DELETE` and any overwriting
  `INSERT`, so append-only is enforced by the database (`ARCHITECTURE.md` §8). **`version`** reports
  the ldflags build.
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
- **The reserved domains** (`social.*`, `money.*`, `media.*`, `work.*`) — the model admits them; not built yet.
- **Storage tiering / archival** — Anam is append-only and grows; deferred, tractable because it owns
  no facts.

---

## 5. Sources and first Facets

| Source | Facet type(s) | Payload (verbatim) | Notes |
|---|---|---|---|
| `health-db` | `health.sleep`, `health.body-composition`, `health.heart-rate`, `health.metric-group` | a period's metric rows as JSON | grain = one group per period; `fact_time = period_start`; `bp_*` excluded from `health.heart-rate` |
| `health-db` | `health.heart-rate-alert` | one alert row from the `events` table as JSON | one facet per alert the watch raised; `fact_time` is the alert's UTC time; the ETL reads it from the FIT files because GarminDB drops it |
| `bp-csv` | `health.blood-pressure` | one reading row as JSON | one facet per row; `fact_time` from `Date (Excel)`; values kept as source strings |
| *(agent)* | `health.insight` *(illustrative)* | the finding value | `proposed`; provenance carries method + the watermark it was computed from |

Granularity is coarse on purpose; decompose only where real fact-times or finer access control earn the
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
  row appears as a *superseding* artifact plus a `superseded` anam-event, the original retained
  (append-only).
- **`changes_since`** correctly reports what was admitted after a given time.
- A **checkpoint** is content-addressed: **stable** when nothing changed, and its hash **moves** after
  new ingest.
- A projection **receipt** reproduces exactly the facet set served at its watermark.
- Switching Anam off loses **no** record — **rebuildable-from-source**: drop the table, `init`,
  re-ingest → identical canonical fold. This is the executable specification of "Anam owns no fact", so
  it is an **automated test from R4 onward**, not a closing demo; the fold's content hash is the
  equality oracle.
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
4. `health.blood-pressure` (bp-csv) — a morning triplet on 2026-01-24 with a free-text comment
   (`"Before breakfast"`), placed at its reading time.
5. `health.insight` (derived, `proposed`) — "resting HR ran higher across the week of the short sleeps" — visible, endorsable, no write-back.

The `bp` projection prunes to blood-pressure facets; `changes_since` shows the latest ingest and the
insight; a receipt pins exactly what was served at the checkpoint watermark. Re-running `ingest bp`
changes nothing — proof the ledger is idempotent and Anam owns no facts.
