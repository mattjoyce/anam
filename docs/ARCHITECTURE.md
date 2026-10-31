# Anam — Architecture

> The technical design of Anam: the model, the contracts, and the mechanics. Companion to
> `PRFAQ.md` (what and why) and `GLOSSARY.md` (the shared language).
>
> Where Anam came from is in `HISTORY.md`. Nothing here depends on it.

---

## 1. Purpose and boundary

Anam is a **personal, longitudinal context store** over one person's own source systems. It assembles a
versioned, chronological, evidence-backed account of that person from fragmented sources (a health
database, a blood-pressure export, and — later — anything else), and serves it back to the person and
to agents acting for them.

One invariant governs every decision below:

> **Anam owns no source fact, and no captured payload exists only inside it — every captured payload is
> held verbatim from its source (rebuildable). Facts accrete; nothing is ever overwritten.**

If Anam were deleted tomorrow, **no fact would be lost**: every payload rebuilds from its source. What
Anam *authors* — governing metadata, the ledger's order, checkpoints, receipts, and its own derived
findings — is operational record, backed up, never authoritative.

### Say the rest of it honestly

The invariant above is true, and it is not the whole truth. Every *fact* rebuilds; the **continuity
does not**. The composed, ordered, whole-life account is
assembled by Anam and exists nowhere upstream. Anam is therefore not weightless, and pretending otherwise
would be the lie that lets a context product quietly become a system of record.

So state it plainly: **Anam owns no fact, and Anam owns the continuity.** The continuity is
*re-derivable* (re-ingest every source and the timeline reassembles) but it is not *held* anywhere else
while Anam is switched off. That is the whole product, and it is why the ledger's order, its watermarks
and its checkpoints are backed up rather than assumed rebuildable.

### The whole-person read (read this before anything else)

A person is unbounded and a life has no edges, so a store of one is pulled toward being a system of
record: *longitudinal is system-of-record gravity.* A personal record *wants* to be longitudinal, so
Anam makes the Person the substrate and accepts the gravity, knowingly, at the scale of one life,
where the cost is affordable.

> **The whole-person timeline read is Anam's default and normal mode.**

Two things follow that are easy to conflate, and must not be:

- **The read default is unbounded.** No span is required to read; a Span is an *optional derived lens*
  (a trip, an illness, a training block) you impose to focus.
- **The mint unit is bounded.** Minting is **per span**, never whole-life. Anam's
  default mintable span is a **time bucket** (§7). The lifelong view **composes** bucket checkpoints; it
  never re-folds the person.

*"There is no required span, therefore the mint unit is the whole timeline"* is a conflation of the
two. Removing the span as a **required read filter** never required removing it as a **mint unit**.

The rest of this document is the machinery: the Facet, the ledger, the two clocks, the admission gate,
mint, fold, and projections.

---

## 2. Topology

Information flows **upward**: the person's source systems at the base, Anam composing context in the
middle, the person and their agents consuming it at the top.

```text
            You · agents acting for you · views · exports
                                  ▲
                                  │  governed projections + receipts
 ┌─────────────────────────────────────────────────────────────────┐
 │  ANAM — personal episodic context                                 │
 │    Canonical person account  →  governed projections    ▲ serve   │
 │    Mint          (checkpoints over the whole timeline)  │         │
 │    Append-only ledger        (Anam's log)               │         │
 │    Ingest gate   (the only ingress; resolve → commit)   ▲ ingest  │
 └─────────────────────────────────────────────────────────────────┘
                                  ▲
                                  │  envelopes, one token per source
                                  │  (via importers on the gateway; docs/IMPORTERS.md)
            Your source systems  —  garmin · withings · bp-csv · dreamstation · owntracks
                                    · records · documents · (later: calendar, mail, notes)
```

Anam's **upstream boundary is the ingest gate.** Everything between a source and that gate — SQLite
readers, CSV parsers, API pollers — is an **importer**: a pluggable client outside the binary, not part
of the kernel, posting envelopes under its own token. Anam reads no source itself (card 119). The gate owns the admission contract and
everything served upward.

---

## 3. The Facet — the atomic unit

The atomic unit Anam captures and governs is the **Facet**: a captured source fragment *plus its
governing metadata* — deliberately **not** a normalised domain entity. Three parts:

- **(a) Attributes about itself** — `id`; `type`; `confidence`; `classification`
  (`Normal`/`Confidential`/`Sensitive`); `artifact_version`; `definition_version`;
  `fact_time`; `series` (the Definition's series stamp, empty for a facet that stands alone;
  §5a); `binding` + a derived `binding_assurance`; and `provenance` — whose optional
  `derivation` is the *only* mark distinguishing an Anam-authored finding from a captured fact (§11).
- **(b) Payload descriptor** — the `source_schema` + `schema_version` (self-describing), a
  `content_hash`, the `payload_kind` (`verbatim` | `reference`), and `media_type`.
- **(c) Payload** — *either* the **verbatim structured body** (the source fragment, held opaque) *or* a
  **native reference** to a binary in its source. The descriptor says which. A reference is JSON like
  any other payload (`path`, `sha256`, `media_type`, `bytes`, `date`), so the descriptor describes the
  payload it holds — `application/json`, kind `reference`, hash over the reference — and the reference
  describes what it points at. One integrity invariant for both kinds. The first reference type is
  `record.document`, from the `documents` folder source (built 2026-10-07; the
  journal's decision note "reference payloads").

Two rules make this safe:

1. **Confidence, type, and provenance are Facet attributes, never payload fields.**
2. **Metadata / payload separation** — Anam governs and projections filter on (a) and (b); the payload
   (c) is **verbatim and immutable**, read as needed, never altered or parsed into a canonical schema.
   This is enforced at the storage layer: the payload is stored as opaque bytes, never as a structured
   map (§8).

> **Design note (Hickey).** The Facet is a *value* — an immutable fact. Its current *status* is not
> stored on it; status is a **fold** over anam-events (§6, §9). The artifact carries only the state
> it was born in. One truth per fact; no place is ever overwritten.

### 3a. Facet and Facet Definition — two independent planes

- A **Facet (artifact)** is re-versioned on a whole re-capture of the source fragment, or a maturity
  change (machine-derived → reviewed).
- A **Facet Definition** is versioned **independently** — the per-*type* contract: schema, envelope,
  time semantics, the declared linking elements, the validation bar, and the type's `classification`.
  Old artifacts stay interpretable as definitions evolve.
- A Definition's `schema` is standard **JSON Schema** (draft 2020-12) and it is **closed**: it declares
  every field a payload may hold, with its type, and refuses the rest, nested objects included. A
  required field refuses null and the empty string. A source that grows a field gets the field
  declared; a source with a second shape gets a second Definition. `validation_mode` is `strict`
  (the schema as written) or `verbatim` (no schema). Validation reads the payload and never rewrites
  it, so a number the source wrote as text stays text, checked by a pattern.
- A new version may not change what a facet **is**: its identity rule (`identity_key` and
  `identity_template`) and its fact-time rule (pointer, layout, zone). `def.Guard` refuses such a
  binary at start and `def.Register` such an importer, because admitted facets would keep the id
  and instant the old rule gave them. Two widenings are not changes (card 198): a fact time may gain
  a `zone_pointer`, so a fact names the zone its wall clock was said in, and an id template may gain
  optional placeholders at its end (`{/source_time/offset?}`). Each reads a field that no stored
  version of the type let a payload carry (a strict version's closed schema did not declare it, and
  no version was verbatim), so every admitted facet reads the same instant and composes the same id.

Granularity is a choice: start coarse (one Facet per source fragment) and decompose only where real
fact-times or finer access control earn the cut. Facet types are **dotted `domain.concept[.specialisation…]`** under a closed set of ten
domains (`health.blood-pressure`, `health.body-composition`, `activity.steps`, `place.stay`,
`record.edge`, `mind.attention`, `org.employment.started`, `money.purchase`), so new sources slot into a domain (`social.message`, `money.transaction`, `media.play`). Where a
distinction goes, in the name or elsewhere, is the decision [where a distinction is encoded](explainer/bundle/decisions/where-a-distinction-is-encoded.md).

---

## 4. The person timeline — substrate and default access

Storage is **partitioned by the person**: a ledger file is pinned to one `person_id`, and all their
facets are co-located on one **timeline**, in **fact-time** order. The person is the **substrate**.

**Access is the person timeline, read whole by default.** There is no required
span filter and no fail-closed on a whole-person read — a longitudinal record is the point.
A read is "my facts, in time order," optionally narrowed.

### Two kinds of Span, and they are not the same thing

Conflating these would produce a whole-timeline mint, which §1 rules out. Anam has **two span tiers**
(and no tier inherited from a System of Record, since it has none):

| Tier | What it is | Minted? |
|---|---|---|
| **Bucket span** | a calendar window over **fact-time**, **one month** by default (`bucket_width` in config, `span.Default`; weekly is also implemented). Every facet is a member of exactly one, by arithmetic on its `fact_time` — no lookup. Invisible to the reader; nobody asks for it. | **minted** — the unit of checkpointing (§7) |
| **Derived span** | an inclusive interval the person names — `[from, to]`, optionally narrowed to some facet types; a saved bookmark; or "everything" | **never minted.** Composed on demand from bucket values; materialised only when a receipt is served |

A derived span is a **lens**, not a unit of consistency: composed on demand, never stored as one, never
pulsed. A saved bookmark (`span#<id>`) is a stored *query*, nothing more. The whole-life span is
expressible and of limited use — a life is what you compose, not what you mint.

Because there is one person, there is **no identity graph and no merge**.

What keeps a person-partitioned, longitudinally-read store honest is **not** a span filter (Anam removed
that) but the invariant of §1: **Anam owns no source fact; every payload is verbatim-from-source and
rebuildable** — while owning, and taking responsibility for, the continuity.

---

## 5. Time — the two clocks

There are exactly two, and where each one lives is the whole of the design:

| Time | Lives on | Meaning |
|---|---|---|
| **Fact time** | the Facet | When it was true in the world — the reading, the night's sleep. Timeline placement |
| **Anam time** | the anam-event | When Anam's knowledge changed — admitted, superseded, tombstoned, endorsed, rejected |

Telling them apart is the difference between asking about the world and asking about Anam, and the
split is structural rather than conventional. A **Facet carries one clock and it is the world's**; a
source can assert when something was true and nothing else. An **anam-event carries one clock and it is
Anam's**; nothing outside Anam can assert when Anam learned something. Neither can borrow the other's,
because neither has a field for it.

That is why there are two and not four. Earlier drafts also carried a *recorded time* ("when the source
documented it") and an *ingested time* ("when Anam received it"). The first was a clock no source in the
first slice actually provided — all five Definitions defaulted it to fact time, so it was a duplicate
column across 893 rows. The second was the anam-time of the `admitted` event, copied onto the Facet, and
so a second place for the same answer to disagree from. Both were deleted. Where the facet page once
read `ingested_time` off the artifact, it now goes to the ledger and reads it off the event that
admitted that version — one fact, one home.

**Published time** is not a third clock. It is derived on demand: the earliest `served_at` over the
receipts naming a facet (§9). Serving changes nothing Anam knows, so it is deliberately not an
anam-event.

### 5a. Reading on two clocks

**Fact-time is not left in the payload.** The Definition's `time_semantics.fact_time` JSON Pointer
extracts it at the gate and it becomes a first-class governed field on the Facet and on every canonical
entry. That promotion is load-bearing: it is what lets the timeline be ordered and a window filtered
without anything ever opening a payload. If fact-time lived only inside the blob, every read would have
to parse every blob, and the metadata/payload separation of §3 would be a fiction.

So a read may name either clock, or both:

| Question | Clock | How it narrows | Resolution |
|---|---|---|---|
| What was true **in the world** up to January? | fact-time | a **derived span** (`--from`/`--to`) | full — it comes from the source |
| What did **Anam know** on 10 July? | anam-time | an **as-of** (`--as-of`) | one point per anam-event — but see §5b on span |
| Both, composed | — | `--as-of T` then the span | bitemporal |

They are different *kinds* of operation, which is why they compose rather than compete: an **as-of picks
which version of the record you are reading** — one watermark, one fold — and a **span filters inside**
that version. Naming both "as-of" would hide that.

**`as-of` is unqualified and always means anam-time.** It needs no prefix because as-of has only ever
meant *what was available to know* — this is Datomic's `as-of` and SQL:2011's `AS OF SYSTEM TIME`. A
fact-time point is a different question and must be spelled differently; anything one letter away from
`--as-of` is a trap, not a name.

### 5b. A bulk import compresses anam-time, and nothing can uncompress it

Anam-time is **fine-grained** — every anam-event gets its own instant and its own watermark, so the
first slice's 893 facets have 893 distinct anam-times. What a backfill compresses is not the
resolution but the **span**:

| | |
|---|---|
| fact-time span | 2024-01 → 2026-07, ~30 months |
| anam-time span | one afternoon, 2026-07-10 |

The two axes therefore bear no relation to each other, and an as-of *inside* the import returns
whatever prefix of the backfill had landed by then — an artefact of the order the rows were read, not of
history. **This is honest, not broken.** Anam did not exist in 2024, so "what did Anam know in 2024"
correctly answers *nothing*, and manufacturing an anam-time per fact would be inventing provenance Anam
does not have.

The consequence is a division of labour worth stating plainly:

- **Anam-time is an audit instrument.** What did this receipt stand on? Has the ledger moved under a
  published artifact? That is what `as-of`, `changes_since` and the watermark exist for, and their
  resolution is exactly right for it.
- **Fact-time is the history instrument.** It has full resolution because it comes from the source, and
  it is the axis the person actually experienced.

Anam-time gains real resolution only when ingest becomes incremental — worth knowing before minting
(§7), because checkpoints are cut against watermarks.

> **The last-known-value read is the snapshot** (`fold.Snapshot`, built 2026-10-07).
> "The facts as of January" most often means *the latest statement before January about each thing*,
> and "each thing" is neither an identity (every change is its own row) nor a type (`health.medication.started` and
> `health.medication.stopped` are one regimen). It is the **series**: a Definition declares a series family, the key
> pointers that name one series within it, and whether its type **closes** the series. The gate composes
> `<family>:<values>` and stamps it on the facet as metadata, beside `fact_time`, so the snapshot groups
> on it without opening a payload. The snapshot at `D` keeps the latest entry per series at or before
> `D`, drops a series whose latest is a closing type, and leaves entries without a series alone. It is
> a function of the value it is given: over the canonical, over an as-of read (bitemporal), or over the
> composed prefix of hard checkpoints up to `D`'s bucket, which yields the same value (§7). A derived span
> still gives every reading up to January, which is a different answer.

A **Facet event** is the domain fact — immutable, carrying its own fact-time, true independently of
Anam; placed on the timeline at its **fact-time**. An **anam-event** asserts that Anam's
*knowledge* changed — `admitted`, `superseded`, `tombstoned`, `endorsed`, `rejected` — and carries
**anam-time**. There are exactly these five.

> Publication is **not** among them. Serving a facet changes nothing Anam knows about it; it records what
> Anam *showed*. That is what a **receipt** already is (§9), pinning the served bytes, the facet-id set,
> the watermark, the template version and the span. Keeping `published` in the ledger would braid *what
> Anam knows* with *what Anam has shown* — the same complecting §7 untangles for *ledger* vs *canonical*.
>
> The cost, named: published-time moves out of the ledger and into the receipts, which are rows of the
> operational store's authored tier, backed up and not rebuilt (§1, §8). Lose them and published-time is
> gone. That is the trade, and it is worth it, because every remaining kind maps one-to-one onto a status.

The dependency is **one-directional**: an anam-event references a Facet; a Facet **never** references an
anam-event — the world does not know Anam exists. This one-way rule is what makes rebuild-from-source
sound. `changes_since` is built from anam-time (§9).

---

## 6. The admission gate — the only ingress

Nothing enters the ledger except through the gate, which enforces **three pillars**,
simplified for a single person but kept in shape:

1. **Facet-compliant** — the payload meets its Facet Definition (schema, envelope, time semantics,
   validation bar). *This is where "validate all external input" lives* — source rows are external
   input, checked before admission.
2. **Authenticated** — the importer's `source` identity and its token scope: a token may inject
   only the facet type(s) it is scoped for (least privilege); the injector identity becomes part of the
   facet's provenance. The scope is real credentials: each importer's own token, held by the daemon
   as a hash beside its source and types (`importers_file`, docs/IMPORTERS.md §8).
3. **Associated** — the facet **binds** to the person. With one person this is near-trivial, but the
   shape holds: assurance is *high* when the source explicitly scopes to the person, *medium* when
   defaulted, *review* when a carried identity element contradicts. A `span_hint` may be recorded if a
   source declares one, but **binding never requires a span**.

The gate is exposed as two calls:

- **`resolve(envelope) → binding + assurance`** — enforces association.
- **`commit(envelope, resolution) → admitted | rejected`** — enforces facet-compliance and token scope,
  computes the content hash, and appends the artifact plus an `admitted` anam-event.

**The operator's hold is not a pillar.** `POST /v1/hold` and `DELETE /v1/hold` (operator) let the
operator stop one source's admission, or every source's, without touching a token: a held source's
envelopes and Definition registrations are refused with 503 and a `Retry-After`, and nothing of it is
admitted. The hold is read *inside* the write, so a hold and an admission are one order: a hold set
before the batch's turn is seen, and one set after it is not. A hold the write cannot read refuses
the batch too: an admission that cannot see a hold must not admit. It is authored state in the
operational store, so it outlives a restart of the daemon. The `workbench` is never held: a hand
entry is the operator's own, not an importer's submission.

### 6a. Idempotency and supersession (the crash-clean property)

- The facet's id is **deterministic**: `facet_id = uuidv5(namespace, source_ref)`, so the same source
  row always maps to the same logical facet.
- **`source_ref` is a source row's identity, never its contents.** It composes from the Definition's
  declared `identity_key` — the fields that say *which* row this is — and nothing else:
  `bp-csv:20:2026-01-24T06:30:00`, not `bp-csv:…:118/76/61`. The composition is one function,
  `facet.SourceRef`: the source name, then each identity value **length-prefixed** (`<bytes>:<value>`),
  so that two different tuples can never compose one string however many colons the values carry (a
  timestamp does; a lab's id prefix does). The gate composes it from the Definition. An envelope
  may leave `source_ref` out; one that carries it is refused if it
  disagrees (`ErrIdentityMismatch`). Braiding the values into the
  natural key would give a corrected reading a *different* `facet_id`, so it would enter as a **new
  facet** and orphan the original instead of superseding it — silently disabling the entire
  idempotency-and-supersession mechanism for that source. Identity decides *which fact*; `content_hash`
  decides *whether it changed*. Keep them unbraided. The type is not in the identity either, so two
  types from one source whose identity values coincide are one `facet_id`; the gate refuses the second
  (`ErrTypeChanged`, card 105), naming both types and the `source_ref`, rather than writing a fact of
  another kind over the first. An importer that emits several types keeps their identities apart.
- `commit` writes the artifact, its `admitted` anam-event, and the watermark bump `W` in **one atomic
  transaction** (§8). Idempotency is enforced on `(source_ref, content_hash)`.
  - Same `source_ref` **and** same hash → **no-op** (safe re-ingest).
  - Same `source_ref`, **different** hash → the source row changed: write a **new artifact version** and
    append a `superseded` anam-event against the prior. Nothing is mutated; the old artifact stays.
- **Retraction is a tombstone** — a `tombstoned` **anam-event** against the current artifact version.
  Never a delete, and never a new artifact. It advances `W` like any other mutation (§7).

  > A tombstone is not a *status-bearing artifact version*: Anam folds status out of the artifact
  > (§3, §12), so there is nowhere to put one, and a Facet has no status field of any kind. The status
  > transition *is* the whole event.

> **Design note (Armstrong).** Because `commit` is idempotent and atomic, an importer may **crash
> mid-batch and simply be re-run to known-good**. There is no partial facet and no corruption. This — the
> ledger plus these gate invariants — is Anam's **error kernel**: the small trusted core that must not
> fail. Everything above it is disposable and rebuildable.

---

## 7. Ledger, mint, and checkpoints

### The ledger is the system of record

The **append-only ledger is Anam's record of what it knew** — its own log, never authoritative about the
world (the sources are). Everything admitted stays; supersession and tombstoning are status transitions,
never deletes.

> **Any state = nearest checkpoint + replay of the anam-events since that checkpoint's watermark.**
> A single indexed range-scan. The current canonical is the head case; a historical
> as-of is the same scan with an upper bound.

### Two surfaces, two names

The word *canonical* names exactly one of these. Anam previously used it for the other; that is corrected
here.

| Surface | Contains | Used for |
|---|---|---|
| **Ledger** | **every facet ever admitted, marked at its current status** — including superseded and tombstoned | forensic reads, `as-of`, `changes_since`, rebuild |
| **Canonical** | the **live-only** fold at a watermark — superseded and tombstoned facets are *excluded from the value* | the normal serve path; what projections prune |

A tombstoned facet stays in the ledger forever, marked. It is absent from the canonical *value*. Neither
surface is "the truth about the past" more than the other — they answer different questions.

### Mint = adaptive checkpointing, and its only job is to bound replay

A **checkpoint** is an immutable, **content-addressed** value of the live-only fold over one span's
members at an anam-time **watermark**: `hash = sha256(canonical bytes)`.

Minting exists **only to bound read-time replay depth** — *not* for freshness. The ledger is always
current, so a read replays to the head and is **as fresh as ingest**. Freshness is a property of the
read, paid by a range-scan; it is not bought by a periodic tick.

- **Minting is per span, never whole-life** (ADR 0033). Anam's default mintable span is a **time bucket**
  over **fact-time**. **Width is a knob, not a constant** — `internal/span` implements monthly and
  weekly, and the operator turns it with **`bucket_width`** in config; `span.Default` is **monthly**.
  (Width bounds the cost of *making* a checkpoint; cadence bounds the cost of *using* one. They are
  different knobs and were once wearing one word.) Bucket ids are **self-describing** — `bucket:2026-07`
  monthly, `bucket:2026-W28` weekly — so ids already written keep their meaning when the default changes.
  Every facet is a member of exactly one bucket, so the minted set is never empty and every facet sits
  inside a mintable scope.
- **Checkpoint buckets subdivide** (card 59, 2026-10-09). With `checkpoint_budget` set (default 1 MiB),
  the mint starts from one bucket per year and splits any bucket whose value is over budget along the
  calendar: a year into months, a month into days (`span.Layout`). Bytes decide when, the calendar
  decides where, so a fact-time is still in exactly one bucket and every version of a facet is still in
  one. The layout is derived from the checkpoint log, not stored: a bucket is split once a checkpoint of
  one of its children exists. Splits only add. A log minted at a fixed monthly width reads as years
  split into months; `checkpoint_budget: 0` keeps the fixed `bucket_width`. The budget prices a read: a
  bucket a late correction dirties is re-derived whole, at about 46 ms a MB on the NAS. `bucket_width`
  still sets the operational index's bucket ids below.
- **The width is checked once and then carried, never re-decided.** `span.New` is the only constructor of
  a `span.Buckets`, and it is where an unknown width is refused; `Buckets.Of` is arithmetic with nothing
  to fall back to. A width that defaulted quietly would be the worst kind of wrong — the ids it wrote
  would be well-formed, plausible, and afterwards indistinguishable from correct ones. So a typo dies at
  **config load**, not on the ingest path.
- **Changing the width is a rebuild, not a migration.** `facet_index.bucket_id` is persisted and catch-up
  only rewrites the rows new events touch, so the width is a property of the *built index*, not of the
  run. The operational store pins it beside `schema_version` and **refuses a database built at another
  width**; the remedy is the same one a schema bump gets — delete `tracking.db` and re-run `init`. The
  derived tier replays, so that costs time, never a fact.
- **Anam buckets on fact-time, not anam-time.** It can, because
  `facet_id = uuidv5(source_ref)` and `source_ref` is a source row's **identity, not its contents** (§6a):
  a superseding artifact is the same row re-read, so it carries the same `fact_time` and lands in the
  same bucket. **Every version of a logical facet lives in one bucket.** The lifelong view is therefore a
  *disjoint union* of bucket values needing no cross-bucket version resolution — and a checkpoint aligns
  with the time-bounded question a person actually asks.
  - The cost: a backfilled facet (an old `fact_time` ingested today) re-opens its sealed bucket and
    re-mints it. One bucket — but *how many facets* is a property of the corpus, not of the design, and
    the "a few hundred" this once said was measured against 893 facets in total. Measured at the
    current 6,181 facets: **31 monthly buckets, mean 199, worst 575, thinnest 7** — and it is the
    *worst* bucket that prices a re-mint, never the mean. Garmin's `monitoring_*` and `stress` tables
    would put ~73,000 in one month. A re-mint that is free at the first density is not free at the second, so
    **either the width narrows as density rises, or re-mint becomes incremental**. Decided 2026-10-09:
    the width narrows where density rises, by subdivision (above). Monthly is
    chosen for the density this record has now — 31 buckets to stitch rather than 135 — and is expected
    to be re-checked, not inherited. Measured again at 11,246 live facets (2026-10-06, the first
    mint): **34 monthly buckets, mean 330, worst 744, thinnest 19**; a dry run of every bucket took
    0.8 s wall.
- **The lifelong / derived view composes, it never re-folds.** A Span the person names (`--from/--to`, a
  saved bookmark, "everything") is a **derived span**: computed on demand by stitching bucket values,
  materialised only when a receipt is served. It is never pulsed and never minted.
  Built 2026-10-09 (card 59): a timeline read starts from the checkpoints of the latest mint run at or
  below its watermark and applies only the events after it (`mint.ReadAt`). A read bounded by fact-time
  decodes only the buckets it overlaps and those the tail dirtied, and streams every other bucket's
  bytes into the whole value's hash without decoding them (`mint.ReadWindow`): the whole value's bytes
  are its buckets' bytes in time order, joined. Projections, snapshots and explain still read the whole
  value; explain, the inspector and retract read the full fold. Whether the whole hash should instead
  be a hash of the bucket hashes is card 60.
- **Cadence follows the ledger** (card 61, 2026-10-09). The daemon mints once importer batches stop
  arriving for two seconds, as its own write on the write queue, so an importer's hundred batches cost
  one mint. A mint from the last run costs the buckets the batches touched (60 ms on the NAS at
  19,631 events), so no size or time trigger earns its complexity; a quiet bucket still mints nothing.
  The mint is not part of what a batch admitted: its failure is logged and fails nothing, and the next
  changing batch, a restart (which mints when the last run is behind the head), or `anam checkpoint`
  catches up.

### The watermark and the head

`W` is the **stream watermark**: the `seq` of the newest anam-event, advanced by *every* anam-event that
can change the live-set — `admitted`, `superseded`, `tombstoned`, `endorsed`, `rejected`. A pure status transition
advances `W` even though no facet is born; if it did not, a checkpoint could reuse a value that still
lists a retracted facet.

Because those five are *all* the anam-events there are (§5), **`W` is the anam-event sequence itself**:
`W ≡ seq`. One number, held once: the ledger stores no counter, and `W` is read as the highest `seq`
(§8). Nothing takes a `seq` without moving the watermark, and nothing moves the watermark without taking a
`seq`.

Per minted span, Anam records `Wᵥ` — the watermark the last checkpoint actually folded (`covers_w`) —
and `V`, the checkpoint version, one row per mint in the `checkpoint` log; the head of a span is a view
over the log, never a second copy. **Whether a span needs re-minting is decided by its value, not by
bookkeeping about it**: the mint builds the bucket's value at `W` and compares its hash with the head
checkpoint's. Equal means nothing a reader could see has changed, and nothing is written. `W > Wᵥ` is
only *sufficient* — `W` is global, so on that test alone every ingest would re-mint every bucket. The
pre-filter the mint uses instead is the tail (below): a bucket no event since the last run touched cannot
have changed. The hash is also what
makes the moved-fact-time case (above) need no second index: a facet that left one bucket for another
moves both hashes, and both buckets re-mint. A bucket that has emptied is found through its head and
re-mints as the empty value, or a composed read would still list the facets that left.

Each mint run that writes records its `W` in `mint_run`, after its checkpoints, with each checkpoint's
facet count in `checkpoint_size`. The values themselves are rows of `checkpoint_value`, keyed by their own
SHA-256 (card 215, §8). A run checks every bucket, minting or finding it unchanged, so the
checkpoints at or below a run's `W` are the value at that `W`, and a read starts there. A run in progress
has no `mint_run` row yet, so no reader starts from it.

**A mint starts from the last run** (card 62, 2026-10-09). The checkpoints at the last run `R` are the
value at `R`, so the only buckets whose value can differ at `W` are the ones the events in `(R, W]`
touched, found as a narrow read finds them: the bucket each touched facet's version at `R` sat in, and
the bucket its version at `W` sits in. Those are rebuilt from their checkpoint and the tail, split if now
over budget, and compared by hash as above. So is any bucket whose checkpoint file is over budget, read
from the file's size, because the budget was lowered or `R` minted at a fixed width. Every other bucket
stands without being decoded, and the run is still a run that checked every bucket. A mint costs its tail,
the buckets the tail touched, and a stat per checkpoint: at 600k facets, 132 ms for a tail touching 32
daily buckets against 19.8 s for the full fold, on the ThinkPad. The full mint (`anam checkpoint --full`)
remains, and is taken without a budget, without a run, and when the log is not sealed. Recording a run
also records a digest of the checkpoints a read from it takes: each span's latest checkpoint at or below
its `W`, by span, version and value hash (`mint_run.picks`). A log is sealed when the newest run's digest
still matches the log and no checkpoint was written after that run. A mint that crashed leaves rows the
digest does not match or rows past the run, which `covers_w` cannot show, since a run at the last run's own
`W` (a lowered budget, no new event) writes rows at that `W` too. Until a mint records a run again, the
mint folds in full. The full mint settles every bucket against the fold whatever the log holds, so it is
both the oracle the incremental mint is tested against and the repair.

**Receipts** prove exactly what was served at a watermark (§9). A receipt names a watermark and not a
checkpoint, and verifying one re-derives it from the ledger, so no receipt holds a checkpoint in place.

**A checkpoint is a cache that is allowed to go missing** (card 215, 2026-10-22, Matt's decision). A
missing one means fold past it and mint it again. The checkpoints bound replay; they are never what a
read's answer depends on, because the ledger can always give the same value, only more slowly.

- *Reads decline rather than guess.* A read at `W` starts from the latest mint run at or below `W` and
  takes that run's checkpoints (`RunCheckpoints`), in one transaction with the run's digest. If the log no
  longer yields exactly the checkpoints the run saw, because a row is gone, has been replaced or another
  has joined it, or if a value is not in the store, the read reports no run to start from and its caller
  folds the ledger in full. The digest is what makes a lost row noticeable: without it a span whose row
  was lost would be absent from the composed value, or present at an older version, and nothing would say
  so. A value whose bytes do not hash to their key is damage and stays an error.
- *A mint heals the cache.* A bucket whose head checkpoint is unchanged but whose value the store no longer
  holds gets the value written again; the incremental mint, meeting a leaf whose value is gone, takes the
  full path, which does the same. No new version is cut.
- *A mint ends by clearing what the newest run does not read.* One transaction deletes the checkpoint
  rows, sizes and values that are not each span's latest checkpoint at or below the newest run (that is,
  the heads). Nothing else needs keeping: an older run whose checkpoints were deleted declines, and one
  whose checkpoints all still stand (nothing changed since) reads as before. A failure to clear is
  returned after the mint, which stands; the next mint clears.
- *The cost* is that a read at a watermark before the newest run folds the ledger in full: 19.8 s at 600k
  facets on the ThinkPad against 132 ms from a run. The live record's artifact folder, never collected,
  held 1,329 checkpoint files and 347 MB against a 57 MB ledger.

---

## 8. Storage: two substrates and the documents

Anam stores in **two** databases and a folder of documents, because different things want different shapes.

| Substrate | Holds | Nature |
|---|---|---|
| **SQLite, the ledger file** | facet artifacts (metadata + opaque payload) and anam-events | the **ledger** — Anam's record of what it knew |
| **SQLite, the tracking file** | Facet Definitions, projection templates, token scopes, the facet, edge and anam-event indexes, the checkpoint log and the checkpoint values it names, receipts, served values, saved selections, sync cursors | Anam's **operational store** |
| **Files, the library** | the primary source documents a `record.document` facet names, each file named by its sha256 | **documents**, written by Anam alone, content-addressed, never overwritten (card 150) |

Until 2026-10-22 a third substrate held checkpoint values, receipts, served values and saved selections as
files under an artifact directory. The split dated from when the ledger was in DynamoDB and SQLite was a
cache that could hold "nothing that cannot be rebuilt by replaying the ledger", and a content-addressed
value fitted neither. Since the ledger moved to SQLite (card 46) the operational store is authoritative for
what Anam authors, so those became rows (card 215), and a backup is the ledger, the operational store and
the library folder. A row is keyed by the SHA-256 of its bytes and every read hashes what it returns, so
what the files did for integrity the rows still do.

Config is **YAML**; Facet Definitions are **JSON** (§3a).

An earlier draft said *"SQLite holds nothing that cannot be rebuilt by replaying the ledger."* That named
one recovery source for a store with several, and demoted a real database to a cache. §1's claim is about
**source facts** — no captured payload exists only inside Anam. It says nothing about Anam's own state,
and Anam has plenty: definitions, indexes, registered projection templates, checkpoints, receipts, served
values, saved selections, importers' snapshot records and admission holds. The operational store is
authoritative for those, and losing it is not free.

What is worth knowing is not a rule but a fact per table — **where does this come back from?** — because a
rebuild has to know what it may drop:

| Tier | Tables | Recovery |
|---|---|---|
| **Derived** | facet index, edge index (card 104), anam-event index, checkpoint index and its values (card 215), `span_head`, sync cursors | replay the ledger, and mint again |
| **Provisioned** | Facet Definitions (catalog and registered), catalog projection templates, token scopes | the catalog (embedded, or `definitions_dir` / `projections_dir`), config, and each importer registering its Definitions again — i.e. git |
| **Authored** | registered projection templates (card 125), receipts, served values and saved selections (card 215), template bodies, the bytes a receipt's template hash names, filed with every receipt so it verifies after its template's file has left the catalog (card 211, card 257), `meta` | nowhere Anam can reach. Back it up; a registered template also comes back by registering it again from wherever the operator keeps the file |
| **Disposable** | importers' complete-resend snapshots (card 78): which parts of a run were admitted and which facet ids they carried | the next resend makes another; nothing is tombstoned from here without an operator's `anam retract --yes`, and an incomplete snapshot is dropped after `snapshot_ttl`, a day by default |

The tier belongs in the schema beside each table, because it is the only thing that tells a later reader
whether dropping it is survivable.

**Where the schema lives.** Each store's fresh-store schema is a `.sql` file beside the Go that opens it,
read with `//go:embed`, so the binary that checks the version carries the text it checks and nothing is
read from disk at run time. `internal/store/ledger/ledger.sql` holds the ledger's three tables and their
append-only triggers, and `internal/store/ledger/indexes.sql` holds its lookup indexes, which a writable
open installs as a separate step and which are not part of what the schema version pins.
`internal/store/ops/ops.sql` holds every operational table, view and trigger, each table under its
`-- tier:` comment. Reading one file shows the whole fresh store, and a schema change shows as a change to
that file. Each migration's SQL is a file of its own under `migrations/` beside the store, embedded the
same way. The ledger has `internal/store/ledger/migrations/0002.sql`, named for the version it moves to
(schema 3 has no file, because it changed no SQL and its step is a rebuild). The operational store has
`internal/store/ops/migrations/snapshot-part-digest.sql`, the in-place step for a store written before the
snapshot digest column, which is a compatibility step and moves no version. The Go keeps what is a
decision rather than a description: `schemaVersion` in each store, which file runs when and in what
transaction, the old-store refusal, the carry across a bump, `bindSnapshotParts` with its check for
whether the step is needed, and the teardown (`dropSchema`). Only `internal/store` names SQLite, and the
files sit inside it.

A test in each store pins the SHA-256 of each embedded file, the schema file and every migration, beside the schema version. It detects a change
and cannot judge it. A hash moving does not prove the stored layout moved (a comment edit changes it, and
several tables were added without a bump because an older store gains them on open), and a hash holding
does not prove the meaning held (ledger version 3 changed no SQL). The person making the change decides:
if an existing file would be misread, bump the version and write the migration or carry, and update both
pins; if the change is compatible, update only the hash, in the same commit, saying why. The ledger test
also lists the ten append-only triggers by name against the file and against a newly created ledger, and
the behavioural tests in `appendonly_test.go` are what show each trigger still refuses the write it
exists to refuse.

**Content-addressed rows** (card 215) are five tables of one shape, a hash and the bytes it names:
`checkpoint_value` in the derived tier, and `receipt`, `served_value`, `saved_selection` and
`saved_selection_code` in the authored tier. The authored four are immutable by trigger, like
`registered_projection`: an update and a delete are refused, for every writer. `checkpoint_value` refuses an
update, and lets a mint delete a value it no longer reads (§7): it is a cache. A receipt and the served value it names are two inserts, the value first, so a crash
leaves a value no receipt names and never a receipt without its value; the files they replace had the same
order. There is no transaction across the two database files, as before: the operational store catches up
by replay and by minting, and a crash leaves it behind and never wrong.

**Carrying a store across the change.** The tables bump the schema (9): a version-8 store has checkpoint
rows naming values it does not hold, and receipts and selections that exist only in the folder beside it,
and absent would read as none. The store refuses, as it does for any bump. It is carried forward by
rebuilding it and moving the folder in with `anam move-artifacts`, which checks every file's bytes against
its name before it writes anything, and which can copy the whole authored tier across from the old file
read-only (`--registered-from`): the registered Definitions and projections, the template bodies,
the admission holds, and the receipts, served values, saved selections and their codes, each row checked
against its key (card 224). `anam migrate` carries the same rows into the store it creates. The next bump
needs no folder: everything authored is already in the old file.

The **derived** tier must stay *provably* derived. The rebuild test (`internal/fold/rebuild_test.go`) drops it and re-derives
it from the ledger: if a facet payload, an anam-event or `W` ever acquired a second home there, the test
would pass on data it never rebuilt, and "Anam owns no source fact" would stop being falsifiable.

### The ledger — datom-shaped, not a datom engine

Adopt the datom **shape**, do not build a datom **engine**. Facts are
`[entity attribute value tx]` with `tx` = anam-time; the store supplies sorted indexes over the
envelope, Anam supplies the shape and a handful of fixed read forms. The general query language
Anam does have (Datalog, §9, card 173) is not an engine over the store: it evaluates over a served
**value**, the fold at a watermark, after a projection has decided what that value holds. The store
answers the fixed forms; everything richer is computed over what they return.

> **It is not EAVT, and calling it that invites a query Anam cannot answer.** EAVT implies you could ask
> the ledger for the `systolic` attribute. You cannot: there is no such attribute, because the payload
> is never decomposed into per-field triples. What *is* attribute-shaped and queryable is Anam's
> **envelope** — type, fact-time, classification, provenance, content-hash — which is exactly what
> projections filter on. The payload beneath it is one opaque, content-addressed value. A reader who
> wants a field names it as one of the read's **fields** (§9, 2026-10-12) and gets that field of each
> whole reading the projection served, cut from the verbatim payload at serve time; the store never
> decomposes it, and nothing is indexed by field. The **store** never selects by payload; a **read**
> may, over the served copy, through a projection (decided 2026-10-17, card 173). "Every reading with
> systolic over 140" is a query answered by opening each served payload the projection admitted,
> not an index lookup the ledger performs, and the distinction is what keeps opacity a storage rule
> rather than a ceiling on what a person may ask of their own record.
>
> The accurate description is **an entity, a governed envelope, an opaque payload, and two clocks**
> (§5) — a datom with a fat, opaque value. Bare "EAVT" is one letter-string standing in for a design it
> misdescribes, which is how it survives being wrong; prefer the sentence.

Grain is **hybrid at the verbatim line** (ADR 0031):

- **Verbatim substrate → coarse.** One item per facet artifact: the datoms cover only Anam's *envelope*
  (its governing metadata), and the payload is a single opaque, content-addressed value. Shredding a
  verbatim payload into per-field triples would impose a schema at ingest — crossing the line §3 draws.
- **Derived facets and consent → fine.** Per-attribute datoms, where field-level history and per-field
  provenance actually pay.

```text
ledger.db
  facet        PRIMARY KEY (facet_id, artifact_version)   one row per artifact version
  anam_event   PRIMARY KEY (seq)                          one row per anam-event
  ledger_meta  PRIMARY KEY (key)                          schema_version, person_id, ledger_id, schema_migration:<n>
```

There is no watermark row. `W` is `MAX(seq)`, and a trigger lets an anam-event take only the `seq` after
the newest one, so the sequence is gapless and the highest `seq` is the count of everything Anam learned.

**The ledger carries no derived indexes.** It serves three access paths, two on a primary key and
one on its own lookup index, `anam_event_by_facet` (`internal/store/ledger/indexes.sql`, installed as
its own step; card 65), which lets the gate fold one facet's events without replaying the ledger:

- one facet's versions, highest first: the current artifact version, whose `content_hash` `commit`
  compares against (§6a);
- the anam-events in a `seq` range: replay since a watermark.

Everything else a reader wants — the timeline in fact-time order, a type filter, `changes_since` — is an
**index**, and indexes live in the operational store, rebuilt by replaying the ledger. The ledger asks its
store for ordered keys and one atomic multi-row write, which any ordered store can give; that is what "the
substrate is accidental" has to mean if it means anything.

**The ledger is its own file, apart from the operational store.** Three things follow from the file being
the unit SQLite deletes, locks and opens read-only by:

- the operational store's repair is to delete its file and replay. That repair cannot reach a fact;
- a command that only reads the ledger opens it read-only, and SQLite refuses any write on that handle.
  `init`, the daemon, and `retract`, `rename` and `migrate` when given `--yes` are the only things that
  open it for writing;
- deleting the operational file and replaying proves the derived tier is derived, because nothing of the
  ledger could have been inside it.

The ledger's `person_id` is pinned in `ledger_meta` at `init`, and a ledger pinned to another person is
refused on every open. `ledger_meta` is write-once like every other ledger row, so the schema version is
not a pin that moves: `schema_version` is the version the file was **created** at, each migration applied
since adds a `schema_migration:<n>` row beside it, and the file's effective version is the highest of
them. A migration only ever adds (a column with a default, an index), never rewrites a row, and runs as
one transaction with its record, so a crash leaves the file at the version before it. `anam init` is the
one place a ledger's schema moves; every other opener refuses a file behind or ahead of its build. The operational store in turn pins the `ledger_id` it was derived from: a rebuilt
ledger gets a new id, so a tracking file left over from an earlier ledger is refused instead of being read
as an index of the new one.

**Backing the ledger up is not a plain file copy while anything has it open.** The ledger runs in WAL
mode, so recent commits can sit in `ledger.db-wal` until a checkpoint folds them in. Copy with
`sqlite3 ledger.db ".backup <dest>"`, or copy `ledger.db` together with its `-wal` file with no writer
running.

**The ledger's directory is the record, and the backup unit.** Beside `ledger.db` sit the operational
store (`tracking.db`, which holds the receipts) and the **default library** (`documents/`): the primary source documents the
facts were extracted from, each referenced by a `record.document` that names it and its sha256. Anam
alone writes the library (`POST /v1/documents`, card 102) and names each file by its sha256 (card 150),
so a stored file is a value: never overwritten, never renamed, found by its name. Where a file sits
is the library's business, and neither the root nor the folder enters a fact: the facet carries the
document's name, its identity, and the sha256, so a library that moves is a config change and a
document rescanned under the same name supersedes, its first bytes kept. The inspector serves the
library's bytes itself (`/document?sha256=`), confined to the root.

### Keys are identity-first, and integers are typed

- **`fact_time` must not appear in an artifact's key.** It is a *value*, read out of the payload;
  `facet_id` is *identity*. Key an artifact on its fact-time and a corrected source row whose fact-time
  moved lands at a fresh key: the prior-version lookup misses it, `commit` concludes "never seen",
  and writes `v1` again. Two live artifacts, one `facet_id`, **supersession silently disabled** — the
  `source_ref` mistake (§6a) wearing a key. Identity-first makes it unrepresentable.
- **`artifact_version` and `seq` are INTEGER columns**, so they order numerically. A key built as text
  orders lexically, where `v10 < v9` and the prior-version lookup reads the *ninth* artifact as current
  from the tenth onward. That defect is live in the `mint-cli` reference implementation's `AllSnapshots`.
  Typed columns remove it; zero-padding only guarded it.

Times are still text, so they are formatted **fixed-width UTC RFC3339Nano** and compare lexically in
chronological order (§5). The **payload is a BLOB, never a JSON column** — this is where
metadata/payload separation (§3) is enforced.

**Facet Definitions are not in the ledger.** A Definition is not something Anam *knew*; it is how Anam
*interprets* — governance, not record. It lives in the operational store, every version, so an
artifact's `definition_version` still resolves. It is provisioned from one of two places, never both
for one type: the JSON embedded in the binary, for the standard types Anam bundles, or the
importer that emits it, which registers it from its repo (`POST /v1/definitions`, docs/IMPORTERS.md
§8). A registered version is immutable, and the store's triggers hold it so. The error kernel is
smaller for it.

> **Design note (Hickey).** The row mapping is **quarantined to the store package**. A row is not a
> domain type. Current status has no column: it is folded from anam-events (§9).

### `commit` is one transaction, and it is the error kernel

The facet artifact and its `admitted` anam-event are written in **one SQLite transaction** that holds
the file's write lock from its first statement. Inside it the ledger checks, in this order:

| Check | What it means | Response |
|---|---|---|
| the head is not the watermark the caller read | another writer advanced `W` | `ErrStaleWatermark`: re-read, re-stamp `anam_time`, retry |
| this `(facet_id, version)` already exists | compare `content_hash` and payload bytes: **same** → we are seeing our own write; **different** → a lost update | `ErrAlreadyWritten`: idempotent no-op. `ErrDivergentArtifact`: stop |
| the database refuses an insert the checks passed | the checks and the constraints disagree | `ErrCorrupt`: stop |

Collapsing the first two into one "conflict" would make `commit` either retry a write that already
succeeded or reject a benign re-ingest — so this mapping *is* the idempotency, not error handling wrapped
around it. The stale check comes first because a writer holding an old head may also be re-presenting
artifacts that landed since, and "re-read and try again" is the answer that lets it sort those out.

Any refusal rolls the whole transaction back, so a refused batch writes nothing. This is what makes a
crash safe: the transaction committed or it did not, and a re-run reads its own earlier write back as a
no-op.

A writer that cannot get the write lock within the busy timeout gets `ErrBusy` and has written nothing.

`anam_time` is stamped **inside** the retry, after the successful `seq` read, so that `seq` order and
`anam_time` order agree by construction. A retry that reused its original timestamp could place an
earlier `anam_time` at a higher `seq`, and the ledger's order — the one thing §1 admits Anam owns —
would be ambiguous.

**The two clocks are not symmetric, and the asymmetry is deliberate.** `seq` is total: every anam-event
gets its own, unique and gapless. `anam_time` is not: a batch commit is *one* change in Anam's knowledge,
so all its events share one instant — up to `gate.MaxBatch` of them. The ledger sets no limit of its
own; the batch size is the gate's policy. "Agree by construction" above therefore
means non-decreasing, not strictly increasing, and anything ordering by anam-time must break the tie with
`seq` (`anam_event_by_time` is indexed `(anam_time, seq)` for exactly that).

The consequence worth naming: `changes_since` is a half-open window on a **non-unique** column, so it
returns its whole window and takes no row limit. A caller paging on the last `anam_time` it saw would
step over the rest of that instant's batch and be told nothing changed. A row limit here would mean
carrying `seq` in the cursor too — the absence of one is load-bearing, not incidental.

The retry loop belongs to the **gate**, not the store. An importer may already crash mid-batch and be
re-run to known-good, so the recovery is free; putting a loop, a bound and a give-up path inside the one
component that must not fail buys nothing.

> **Design note (Armstrong).** There is **no transaction across two files**, so everything that must be
> atomic lives in one: the artifact and the event. The operational store's **derived** tier is never
> written in the same breath as the ledger; it catches up afterwards by replaying anam-events since its
> last-seen `seq`. That is safe precisely because the ledger is append-only and `commit` is idempotent —
> **a crash leaves the derived tier behind, never wrong, and behind is fixable by replay.** Catch-up is
> not housekeeping: the fold reads its indexes, so it must assert `last_seq == W` before folding, or a
> checkpoint can be computed from a lagging index and be wrong at its own watermark.
>
> **Append-only is enforced by the database.** Triggers on every ledger table refuse `UPDATE` and
> `DELETE`, and refuse an `INSERT` over an existing key. Without that last one, `INSERT OR REPLACE`
> would overwrite a row without firing the delete trigger. They bind every writer, a `sqlite3` shell included. A trigger is
> not a security boundary: whoever holds the file can drop it. The threat it answers is a refactor or a
> hurried session, not an adversary.

Every store method takes a `context` with a deadline and **fails fast** — never a hang. A missing
ledger file is `ErrNotInitialised` on open; nothing is created by reading or ingesting.

**The substrate is accidental, not essential.** Only `internal/store` may name SQLite or
`database/sql`; `scripts/check.sh` enforces it. Because Anam owns no source fact, a change of
substrate can be a re-ingest: the payloads come back from their sources. The anam-times do not. A
re-ingest stamps new ones, so a ledger whose history of learning matters is moved by copying its rows.

There is no carve-out. A source that is a SQLite file is read by its importer, outside the binary, and
what reaches Anam is envelopes (card 119).

---

## 9. Ledger, canonical, and projections

Two surfaces (§7), and the reads that go to each.

**The ledger** is append-only and complete: every facet ever admitted, at its **current status derived
from anam-events**, with provenance — including superseded and tombstoned facets. *What is admitted
stays; it is re-statused, never pruned by retraction.* Forensic reads, `as-of` reads, and rebuild go here.

- **`changes_since`** reads anam-events by anam-time and reports what changed between two points.
- The **raw / forensic** view of the timeline reads the ledger directly, tombstones visible.

**The canonical** is the **live-only** fold at a watermark: every facet whose current status is live.
Superseded and tombstoned facets are *excluded from the value* — they remain in the ledger, marked.

- **Fold** reads the anam-events for a span's members up to a watermark and derives each facet's
  current status (`admitted` / `superseded` / `tombstoned` / `endorsed` / `rejected`), keeping the live
  ones. Liveness ⇔ the current artifact version's status is live.
- The **normal serve path** is *nearest checkpoint + replay-since* (§7), so a read is as fresh as ingest.
- The **whole-person timeline** — Anam's default read — is a **derived span composed** from bucket
  checkpoints plus the open bucket's replay. It never re-folds the person.

**Retraction is a command, never a side effect of ingest** (`anam retract <source>`, 2026-10-07). Ingest
admits and supersedes; a row's absence from one scan and a source's absence look alike from inside an
envelope, so no ingest tombstones. The command reports the live facets a source no longer holds, as a
set comparison, and tombstones them through the gate only with `--yes`. A scan that fails writes
nothing; a scan that holds nothing is refused; and a run may not retract more than a fraction of a
source (`--max-fraction`, a tenth by default) unless the operator raises it.

From the canonical, Anam publishes governed **projections** (variants): **versioned templates that
prune — never reshape, never generate.** The only knobs are **metadata filters**: a facet-type allow-list
(deny-by-default), a datetime filter, an endorsement-status filter, a maximum classification, and a
**shape**, `log` or `snapshot` (§5a; built 2026-10-07). The shape is the one knob whose verdict depends
on other entries: a snapshot keeps the latest per series within the window and drops a closed series,
and its explanation says *displaced by* or *closed by* which facet. It runs after the other four, over
what they admitted, so the window's `to` is the snapshot's "at". Filtering is on **metadata only, never
the payload**; a projection includes or excludes **whole Facets**. A consumer **selects** a projection;
it never defines one — which is why shape is a template knob and not a read-time flag. (`timeline --at`
is the person's own un-published view of the same read; `serve meds` is the published one.)

Defining is the **operator's** act, not a release's (decided 2026-10-11, built 2026-10-13, card 125).
Templates are **bundled or registered**: the bundled ones ship with the binary as the floor, and the
operator registers others over `POST /v1/projections`, kept in the operational store beside the
registered Definitions. A template version is a **value**: the same name and version is the same
canonical bytes or it is refused, versions only go forward, and every version ever held stays readable,
because a receipt names the version (and, since card 125, its content hash) that pruned it, and
`receipt verify` re-prunes through exactly that one. Every type a template names must have a Definition
in force when it is registered, so a template naming a typo is refused at the door rather than serving
nothing quietly; a type whose Definition is later retired stays nameable, since its past facets are
still servable.

A consumer may name the **fields** it wants of a projection (decided and built 2026-10-12, card
138): a temporary subset of the type's schema, as JSON pointers, validated against the Definition. Fields
run after every projection knob, over the whole facets they admitted, and only cut: they cannot resurrect
or exclude a facet, cannot show a field the projection would not have served whole, and never touch the
envelope. The stored payload stays opaque; the cut is made on the served copy. The answer is a **frame**,
columns and rows, a relation rather than a list of facets, which is why a model can ask about a year of
rowing without a stroke of it in its context. The receipt pins the fields and hashes the frame.

The third read is a **query** (decided 2026-10-17, card 173; Datalog, because Anam is Datomic-shaped
and a query is a pure function of a database value, and `fold.Value` at `W` is that value). A query
runs **through a projection**, never beside one: the projection decides which whole facets the
query may see, exactly as it does for a timeline or a frame, and the query then relates them: joins
across types along fact-time and selection by any payload field the served Definition's schema names. Aggregates (`count`, `sum`, `min`, `max`) are the engine's, in Soufflé's syntax, and every evaluation is bounded in steps as well as bytes. A query answer is a **derived value read off a projection**, which is why it
does not break the rule that projections prune. The projection still only prunes; the generating is
done by the query, over what the projection served, and the answer is a new value that hashes, not a
change to any facet. Base relations are the envelope (`entry/7` with the entity, type, fact-time, series,
status, source, and `definition_version`) and the payload fields read from the served copy
(`attr(E, Pointer, Value)` for any pointer the schema defines, validated as fields are). Bundled
rules (`current/2`, `displaced/2`, `in_force/2`, and `linked/2` over the edge facets, card 104) ship as a named, versioned rule set so an agent builds on
Anam's reading of its own time semantics rather than rewriting it. The set is at version 2; every earlier text a receipt may name is kept, and verify picks the text by the receipt's rule-set hash. `within/3`, a distance between two fact-times, is a builtin answered in Go, since the language has no arithmetic. The evaluator is the tarski module (`github.com/mattjoyce/tarski` v0.3.2, card 230); Anam supplies the base relations, the checks, `within/3` and the bundled rules, and refuses the string builtins and number spellings its language never had. The answer is a receipt like any
other served thing: it carries the watermark and `ledger_id`, the projection name, version and
content hash, the definition-set version, the rule-set hash, and the query text; `receipt verify`
re-runs the same query over the same value and must get the same hash.

A query may carry its projection inline (card 211), for a one-off question that no operator has
registered a projection for. The `projection` field is then a template object with a manifest's knobs,
without a name or a version, checked by the loader a registered manifest goes through. It is known by
its content hash. Its body is filed in the authored tier under that hash, and the receipt names it by
the hash with an inline marker, so verify finds the body and replays it after a restart. Every role may
already read every registered projection, so an inline one widens nothing; if projections ever become
a per-role ceiling, an inline one must sit under the caller's ceiling. Reading through a saved
selection's code in place of a projection is a possible later slice and is not built.

Who may read, list or open what is answered in one place, `internal/policy` (card 223): static Go over
the request and the importers file's principals, which never reads a payload, so access stays a
mechanism and never a judgement of content. The handlers branch on nothing else about the caller. The
stated later flip is the bundled Datalog engine over a policy base (`principal/2`, `flag/2`,
`request/3`, a versioned `policy.dl`, never `attr/3`), taken when the rules pass about a dozen, a
second class of principal arrives, or a rule must read beyond the request and the file.

A query moves no watermark and writes nothing to the ledger. The evaluator is read-side and disposable, outside the error kernel; a wrong
answer is fixed by fixing the rules and asking again, and the bundled `current/2` is held equal to
`fold.Snapshot` by a standing test, so there is one account of liveness even while there are two
implementations of it. Blending projections remains a later contract.

> *Tombstone visibility* is no longer a projection knob. It was one only while "canonical" named the
> append-everything surface. A projection over the canonical sees live facets by construction; a reader
> who wants tombstones is asking the **ledger** a forensic question, not asking a projection for a variant.

A projection is also **explainable**: the same verdict that decides whether a facet is served reports
*which* knob excluded it, so a governed variant can answer "why is this reading not here?" from the
filter itself rather than from a second account of it. Versions no longer in force — displaced by a
correction, or belonging to a facet no longer live — are not exclusions at all: they never reach a
template, because the canonical is live-only by construction. They are their own standing, derived from
the fold.

Projections are **materialised as rows**, content-addressed: the served bytes are a `served_value` row keyed by their SHA-256. **Receipts** make a served projection
reproducible: `served_hash = sha256(served bytes)` plus the exact facet-id set, the watermark, the
template version and its content hash (an inline template is marked as one, and its body is filed under that hash), the definition-set version, the `served_at` time, the span, and the `ledger_id` the
watermark counts in (since 2026-10-07; a watermark means nothing against another ledger, so `receipt
verify` on a rebuilt ledger reports *other ledger*, never a failed reproduction, and a receipt from
before the field is verified with a note).

Receipts are also where **publication** is recorded. A facet's published time (§5) is the earliest
`served_at` over the receipts naming it; there is no `published` anam-event, because being shown is not
something Anam learned about the fact.

---

## 10. Consent and classification (kept in shape, trivial for one person)

Anam is single-user, so consent is **default-allow** — but the *shape* is preserved so the seam exists
for a future where Anam serves facets to third-party agents:

- Every facet carries a **`classification`** (`Normal` / `Confidential` / `Sensitive`).
- A projection passes through a `consent.Evaluate()` gate that today returns *allow*; the hook is real
  even though the policy is open.

Full consent policy, context-of-use rules, and classification-driven gating are **deferred** — the
attribute and the gate exist; the policy does not.

---

## 11. Agents as consumers — derived facets

An agent finding (an Anam **insight** — "resting HR drifted up during that travel week") is a **derived
Facet**: it re-enters through the **same admission gate**, admitted while still `proposed`, its
provenance carrying the method/model and the canonical **watermark it was computed from**. It carries
confidence and an endorsement status.

**Admission is not endorsement.** Endorsement (`endorsed` / `rejected`) is itself an anam-event and
**never writes back** into source systems. Any real-world action a person takes on an insight re-enters
Anam later as an ordinary source fact.

> **Design note (Armstrong).** The agentic path is **isolated**: a `proposed` derived facet is a
> separate value that can never reach or mutate a world-fact. The fallible part is contained; the
> trusted core (world-facts in the ledger) is untouched by it.

---

## 12. Invariants — the do-not-break list

- Owns no source fact; **no captured payload exists only in Anam** — every one is verbatim-from-source
  and rebuildable. **Anam does own the continuity**, which is re-derivable but held nowhere else (§1).
- What is Anam-authored — governing metadata, the ledger order, watermarks, checkpoints, receipts, and
  provisional derived findings — is **authored, never authoritative**; backed up, not rebuilt.
- **Storage is partitioned by the person (one timeline).** The **whole-person read is the default**; a
  Span is an optional derived *read* filter. **Minting is per span, never whole-life** — the two are
  different concerns and must not be conflated (§1, §7).
- **The ledger is the system of record**; the library's files hold documents; SQLite is Anam's **operational
  store** — authoritative for what Anam authors, and annotated per table with where it comes back from.
  Its **derived** tier must stay provably rebuildable by replaying the ledger, or the rebuild test proves
  nothing (§8).
- **Append-only**: facts are added and superseded by status events (tombstones) — **never modified,
  never deleted.** The ledger's triggers refuse both, for every writer (§8).
- **Current status is derived** by the fold from anam-events — **never stored** on the artifact (no
  place-oriented programming). A tombstone is therefore a **anam-event, not an artifact version**: an
  Anam artifact has nowhere to carry a status (§6a).
- **`source_ref` is identity, `content_hash` is content.** The natural key never contains the row's
  values, or supersession silently becomes duplication (§6a).
- **Sort keys are identity-first and fixed-width.** An artifact's key carries its `facet_id`, never its
  `fact_time`; every integer in a sort key is zero-padded. Otherwise supersession silently duplicates, or
  lexical order stops being chronological (§8). The ledger carries **no derived indexes**: anything
  derived lives in the operational store. Its one index, `anam_event_by_facet`, is a lookup on its own
  rows for the gate's fold of one facet (§8), not a derivation.
- **Canonical means the live-only value; the ledger means append-everything-marked.** One word, one
  surface (§7, §9).
- **Metadata / payload separation**: the payload is verbatim, immutable, stored opaque, never parsed
  into a canonical schema.
- **Three-pillar admission through the one gate**; **`commit` is idempotent and atomic** — artifact,
  anam-event and watermark in one transaction, in one substrate (crash-clean).
- Projections **prune, never reshape or generate**; a consumer selects, never defines. The **fields** a
  consumer names only take away. A **query** generates, but only over what a projection served:
  every query runs through a projection, reads payload fields from the served copy and never from
  the store, moves no watermark, and answers with a receipt that names the projection, the
  rule set and the query (§9).
- Agent findings are derived Facets admitted as `proposed`; **endorsement never writes back**.
- **The Envelope is the only ingress**; every external call is timed and fails fast.

---

## 13. MVP boundary and deferred items

**MVP** was the vertical slice end-to-end over two real sources (`health-db`, `bp-csv`),
one or two projections plus a CLI timeline viewer, manual mint, and receipts. Read-only throughout. (The daemon has since
learned to mint on its own, §7.)

**Deferred, not forgotten:**

- **Binary / native-reference facets** — `record.document` is built over a folder source (2026-10-07).
  The inspector serves the bytes from the library (`GET /document`, confined to the library root). A removed file is retracted
  by `anam retract`, deliberately; EXIF is not read, a sidecar carries the date.
- **Adaptive mint cadence (time-OR-size-OR-rate)** — the daemon mints once batches stop arriving for
  `mint_quiet` (§7, card 61), and `anam checkpoint` mints by hand; a size or rate trigger is not built,
  since a mint from the last run is cheap.
- **Sealed projections / a seal point** — receipts suffice for now.
- **Token rotation; multi-scope tokens** — one static `source → types` map.
- **Consent policy and classification-driven gating** — attribute + pass-through gate only (§10).
- **Storage management** — Anam is append-only and grows; an archive tier is deferred, tractable
  precisely because Anam owns no facts and can re-ingest.
- **Extensibility to the reserved domains** (`social.*`, `media.*`, `work.*`) — the model already
  admits them; they are simply not built yet. `money.*` is open (card 241).
- **Last-known-value reads** (§5a) — built as the snapshot over the series stamp (2026-10-07), and exposed
  as `anam timeline --at` and as `shape: snapshot` projection templates (`meds`, `employment`). A Definition that changes its series
  key reaches only facets admitted after the change, because admission is idempotent on content.
- **The query surface** (§9, card 173) — decided 2026-10-17, built 2026-10-19: the evaluator, the
  bundled rule set and the `current/2` = `fold.Snapshot` equality test (`internal/query`, held on a
  copy of the real record by `anam query check`), then introspection (`GET /v1/query`), the size
  bound over the derivation, the receipt, `anam query run` and `POST /v1/query`; the anam-mcp
  tools; and `scripts/souffle-diff.py`, the differential run against Soufflé, an oracle run by
  hand until the engine has earned trust and then retired, never a dependency of the gate. Whether `fold.Fates` later becomes the bundled rules, so there is one
  decision procedure instead of two, is a separate decision for after the equality test has run for a
  while: it would move the evaluator onto the serve path, inside the trusted core, and that is not
  what this card admits.
