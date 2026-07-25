# Anam — Architecture

> The technical design of Anam: the model, the contracts, and the mechanics. Companion to
> `PRFAQ.md` (what and why) and `GLOSSARY.md` (the shared language).
>
> Anam is a re-instantiation of **Arch** (`/mnt/Projects/Arch/docs/ARCHITECTURE.md`) — the same
> machinery, **one decision inverted**. Where this document is silent, Arch governs; where it speaks,
> it speaks because Anam differs.

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

Arch's ADR 0035 aims that invariant back at us, and it lands:

> *"'Owns no facts, rebuildable from source' stays literally true under either grain — but under
> Person-grain it stops being the whole truth: the cross-episode continuity and identity linkage exist
> **nowhere else**."*

True. Every *fact* rebuilds; the **continuity does not**. The composed, ordered, whole-life account is
assembled by Anam and exists nowhere upstream. Anam is therefore not weightless, and pretending otherwise
would be the lie that lets a context product quietly become a system of record.

So state it plainly: **Anam owns no fact, and Anam owns the continuity.** The continuity is
*re-derivable* (re-ingest every source and the timeline reassembles) but it is not *held* anywhere else
while Anam is switched off. That is the whole product, and it is why the ledger's order, its watermarks
and its checkpoints are backed up rather than assumed rebuildable.

### The one inversion (read this before anything else)

Arch's keystone is **ADR 0035**: *"The Episode is the unit — of storage, cost, completeness, and the
checkpoint span. The Person is a thin identity index."* Arch deliberately **demoted Person from
substrate to index**, reversing its own earlier person-centric pivot, on the grounds that a person is
unbounded, a life has no edges, and *"longitudinal is system-of-record gravity."*

**Anam inverts exactly that.** A personal soul-record *wants* to be longitudinal, so Anam re-promotes
the Person to substrate and accepts the gravity — knowingly, at the scale of one life, where the cost
Arch could not pay across a jurisdiction is affordable.

> **The whole-person timeline read is Anam's default and normal mode.**

Two things follow that are easy to conflate, and must not be:

- **The read default is unbounded.** No span is required to read; a Span is an *optional derived lens*
  (a trip, an illness, a training block) you impose to focus.
- **The mint unit is bounded.** Minting is **per span**, never whole-life (Arch ADR 0033). Anam's
  default mintable span is a **time bucket** (§7). The lifelong view **composes** bucket checkpoints; it
  never re-folds the person.

Anam's earlier reasoning — *"there is no required span, therefore the mint unit is the whole timeline"* —
was a conflation of these two, and is retracted. Removing the span as a **required read filter** never
required removing it as a **mint unit**.

Everything else in Arch — the Facet, the ledger, the two clocks, the admission gate, mint, fold, and
projections — carries over. The rest of this document is that machinery, re-grained to the person.

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
                                  │  source fragments
                                  │  (via capture workers)
            Your source systems  —  health-db · bp-csv · (later: calendar, mail, notes)
```

Anam's **upstream boundary is the ingest gate.** Everything between a source and that gate — SQLite
readers, CSV parsers, future API pollers — is a **capture worker**: a pluggable client, not part of the
kernel. The gate owns the admission contract and everything served upward.

---

## 3. The Facet — the atomic unit

The atomic unit Anam captures and governs is the **Facet**: a captured source fragment *plus its
governing metadata* — deliberately **not** a normalised domain entity. Three parts (Arch §3):

- **(a) Attributes about itself** — `id`; `type`; `confidence`; `classification`
  (`Normal`/`Confidential`/`Sensitive`); `artifact_version`; `definition_version`;
  `event_time` / `recorded_time` / `ingested_time`; `binding` + a derived `binding_assurance`;
  `provenance`; and `birth_status` — the **immutable** state it was born in (`admitted` or `proposed`).
- **(b) Payload descriptor** — the `source_schema` + `schema_version` (self-describing), a
  `content_hash`, the `payload_kind` (`verbatim` | `reference`), and `media_type`.
- **(c) Payload** — *either* the **verbatim structured body** (the source fragment, held opaque) *or* a
  **native reference** to a binary in its source. The descriptor says which. (Anam's first slice is all
  `verbatim`; references are reserved.)

Two rules make this safe:

1. **Confidence, type, and provenance are Facet attributes, never payload fields.**
2. **Metadata / payload separation** — Anam governs and projections filter on (a) and (b); the payload
   (c) is **verbatim and immutable**, read as needed, never altered or parsed into a canonical schema.
   This is enforced at the storage layer: the payload is stored as opaque bytes, never as a structured
   map (§8).

> **Design note (Hickey).** The Facet is a *value* — an immutable fact. Its current *status* is not
> stored on it; status is a **fold** over record-events (§6, §9). The artifact carries only the state
> it was born in. One truth per fact; no place is ever overwritten.

### 3a. Facet and Facet Definition — two independent planes

- A **Facet (artifact)** is re-versioned on a whole re-capture of the source fragment, or a maturity
  change (machine-derived → reviewed).
- A **Facet Definition** is versioned **independently** — the per-*type* contract: schema, envelope,
  time semantics, the declared linking elements, the validation bar, and the type's `classification`.
  Old artifacts stay interpretable as definitions evolve.

Granularity is a choice: start coarse (one Facet per source fragment) and decompose only where real
event-times or finer access control earn the cut. Facet types are **dotted and namespaced** —
`health.sleep`, `health.body-composition`, `health.heart-rate`, `health.blood-pressure`,
`health.metric-group` — so new sources slot in cleanly (`calendar.event`, `mail.thread`, `note.zettel`).

---

## 4. The person timeline — substrate and default access

Storage is **partitioned by the person** (`person#<id>`): all their facets are co-located on one
**timeline**, in **event-time** order. The person is the **substrate**.

**Access is the person timeline, read whole by default.** This is the inversion. Unlike Arch, there is
no required span filter and no fail-closed on a whole-person read — a longitudinal record is the point.
A read is "my facts, in time order," optionally narrowed.

### Two kinds of Span, and they are not the same thing

Conflating these produced the whole-timeline mint that §1 retracts. Anam has **two span tiers**
(Arch ADR 0033, minus the inherited tier — Anam has no System of Record to inherit boundaries from):

| Tier | What it is | Minted? |
|---|---|---|
| **Bucket span** | a calendar window over **event-time** (default: one month). Every facet is a member of exactly one, by arithmetic on its `event_time` — no lookup. Invisible to the reader; nobody asks for it. | **minted** — the unit of checkpointing (§7) |
| **Derived span** | an inclusive interval the person names — `[from, to]`, optionally narrowed to some facet types; a saved bookmark; or "everything" | **never minted.** Composed on demand from bucket values; materialised only when a receipt is served |

A derived span is a **lens**, not a unit of consistency: composed on demand, never stored as one, never
pulsed. A saved bookmark (`span#<id>`) is a stored *query*, nothing more. The whole-life span is
expressible and of limited use — a life is what you compose, not what you mint.

Because there is one person, there is **no identity graph and no merge** — Arch's linkage machinery is
dropped entirely.

What keeps a person-partitioned, longitudinally-read store honest is **not** a span filter (Anam removed
that) but the invariant of §1: **Anam owns no source fact; every payload is verbatim-from-source and
rebuildable** — while owning, and taking responsibility for, the continuity.

---

## 5. Time — the two clocks

| Time | Clock | Meaning |
|---|---|---|
| **Event time** | Facet | When the thing happened (the reading, the night's sleep) — timeline placement |
| **Recorded time** | Facet | When a person or system documented it |
| **Ingested time** | Anam | When Anam received it |
| **Published time** | Anam | When it first appeared in a served account — the earliest `served_at` over the receipts naming it (§9). Not a record-event |

A **Facet event** is the domain fact — immutable, carrying its own event-time, true independently of
Anam; placed on the timeline at its **event-time**. An **Anam record-event** asserts that Anam's
*knowledge* changed — `admitted`, `superseded`, `tombstoned`, `endorsed`, `rejected` — and carries
**record-time**. There are exactly these five.

> Publication is **not** among them. Serving a facet changes nothing Anam knows about it; it records what
> Anam *showed*. That is what a **receipt** already is (§9), pinning the served bytes, the facet-id set,
> the watermark, the template version and the span. Keeping `published` in the ledger would braid *what
> Anam knows* with *what Anam has shown* — the same complecting §7 untangles for *ledger* vs *canonical*.
>
> The cost, named: published-time moves out of the ledger and into the receipts, which are published
> artifacts — authored, backed up, not rebuilt (§1). Lose the artifact directory and published-time is
> gone. That is the trade, and it is worth it, because every remaining kind maps one-to-one onto a status.

The dependency is **one-directional**: a record-event references a Facet; a Facet **never** references a
record-event — the world does not know Anam exists. This one-way rule is what makes rebuild-from-source
sound. `changes_since` is built from record-time (§9).

---

## 6. The admission gate — the only ingress

Nothing enters the ledger except through the gate, which enforces **three pillars** (Arch §7),
simplified for a single person but kept in shape:

1. **Facet-compliant** — the payload meets its Facet Definition (schema, envelope, time semantics,
   validation bar). *This is where "validate all external input" lives* — source rows are external
   input, checked before admission.
2. **Authenticated** — the capture worker's `source` identity and its token scope: a token may inject
   only the facet type(s) it is scoped for (least privilege); the injector identity becomes part of the
   facet's provenance.
3. **Associated** — the facet **binds** to the person. With one person this is near-trivial, but the
   shape holds: assurance is *high* when the source explicitly scopes to the person, *medium* when
   defaulted, *review* when a carried identity element contradicts. A `span_hint` may be recorded if a
   source declares one, but **binding never requires a span**.

The gate is exposed as two calls:

- **`resolve(envelope) → binding + assurance`** — enforces association.
- **`commit(envelope, resolution) → admitted | rejected`** — enforces facet-compliance and token scope,
  computes the content hash, and appends the artifact plus an `admitted` record-event.

### 6a. Idempotency and supersession (the crash-clean property)

- The facet's id is **deterministic**: `facet_id = uuidv5(namespace, source_ref)`, so the same source
  row always maps to the same logical facet.
- **`source_ref` is a source row's identity, never its contents.** It composes from the Definition's
  declared `identity_key` — the fields that say *which* row this is — and nothing else:
  `bp-csv:2026-01-24T06:30:00`, not `bp-csv:2026-01-24T06:30:00:118/76/61`. Braiding the values into the
  natural key would give a corrected reading a *different* `facet_id`, so it would enter as a **new
  facet** and orphan the original instead of superseding it — silently disabling the entire
  idempotency-and-supersession mechanism for that source. Identity decides *which fact*; `content_hash`
  decides *whether it changed*. Keep them unbraided.
- `commit` writes the artifact, its `admitted` record-event, and the watermark bump `W` in **one atomic
  transaction** (§8). Idempotency is enforced on `(source_ref, content_hash)`.
  - Same `source_ref` **and** same hash → **no-op** (safe re-ingest).
  - Same `source_ref`, **different** hash → the source row changed: write a **new artifact version** and
    append a `superseded` record-event against the prior. Nothing is mutated; the old artifact stays.
- **Retraction is a tombstone** — a `tombstoned` **record-event** against the current artifact version.
  Never a delete, and never a new artifact. It advances `W` like any other mutation (§7).

  > Arch writes a tombstone as a *status-bearing artifact version* (ADR 0030), and Anam inherited the
  > sentence. It cannot be true here: Anam folds status out of the artifact (§3, §12), so there is
  > nowhere to put one — `birth_status` admits only `admitted` and `proposed`. The status transition
  > *is* the whole event. Corrected once code had to write one.

> **Design note (Armstrong).** Because `commit` is idempotent and atomic, a capture worker may **crash
> mid-batch and simply be re-run to known-good**. There is no partial facet and no corruption. This — the
> ledger plus these gate invariants — is Anam's **error kernel**: the small trusted core that must not
> fail. Everything above it is disposable and rebuildable.

---

## 7. Ledger, mint, and checkpoints

### The ledger is the system of record

The **append-only ledger is Anam's record of what it knew** — its own log, never authoritative about the
world (the sources are). Everything admitted stays; supersession and tombstoning are status transitions,
never deletes.

> **Any state = nearest checkpoint + replay of the record-events since that checkpoint's watermark.**
> (Arch ADR 0032.) A single indexed range-scan. The current canonical is the head case; a historical
> as-of is the same scan with an upper bound.

### Two surfaces, two names (Arch ADR 0032, MC5)

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
members at a record-time **watermark**: `hash = sha256(canonical bytes)`.

Minting exists **only to bound read-time replay depth** — *not* for freshness. The ledger is always
current, so a read replays to the head and is **as fresh as ingest**. Freshness is a property of the
read, paid by a range-scan; it is not bought by a periodic tick.

- **Minting is per span, never whole-life** (ADR 0033). Anam's default mintable span is a **time bucket**
  over **event-time** (default: one calendar month). Every facet is a member of exactly one bucket, so
  the minted set is never empty and every facet sits inside a mintable scope.
- **Anam buckets on event-time, where Arch buckets on record-time.** Anam can, and Arch cannot, because
  `facet_id = uuidv5(source_ref)` and `source_ref` is a source row's **identity, not its contents** (§6a):
  a superseding artifact is the same row re-read, so it carries the same `event_time` and lands in the
  same bucket. **Every version of a logical facet lives in one bucket.** The lifelong view is therefore a
  *disjoint union* of bucket values needing no cross-bucket version resolution — and a checkpoint aligns
  with the time-bounded question a person actually asks.
  - The cost: a backfilled facet (an old `event_time` ingested today) re-opens its sealed bucket and
    re-mints it. One bucket, a few hundred facets. Garmin backfills routinely; this is cheap and correct.
- **The lifelong / derived view composes, it never re-folds.** A Span the person names (`--from/--to`, a
  saved bookmark, "everything") is a **derived span**: computed on demand by stitching bucket values,
  materialised only when a receipt is served. It is never pulsed and never minted.
- **Cadence is adaptive** — time-OR-size-OR-rate, whichever trips first. A quiet bucket mints nothing.
  The first slice mints on a **manual trigger** (`anam checkpoint`); the adaptive trigger is deferred, the
  seam is not.

### The watermark and the head

`W` is the **stream watermark**: a monotonic counter advanced by *every* record-event that can change the
live-set — `admitted`, `superseded`, `tombstoned`, `endorsed`, `rejected`. A pure status transition
advances `W` even though no facet is born; if it did not, a checkpoint could reuse a value that still
lists a retracted facet.

Because those five are *all* the record-events there are (§5), **`W` is the record-event sequence itself**:
`W ≡ seq`. One counter, not two. Nothing takes a `seq` without moving the watermark, and nothing moves the
watermark without taking a `seq`.

Per minted span, Anam tracks `Wᵥ` — the watermark the last checkpoint actually folded — and `V`, the
checkpoint version. Whether a span needs re-minting is then one comparison, `W > Wᵥ`.

**Receipts** prove exactly what was served at a watermark (§9). Unserved interstitial checkpoints are
garbage-collectable once a later checkpoint subsumes them and no receipt pins them; they were only ever
replay floors.

---

## 8. Storage — three substrates

Anam stores in **three** places, because three different things want three different shapes.

| Substrate | Holds | Nature |
|---|---|---|
| **DynamoDB** | facet artifacts (metadata + opaque payload), record-events, and the watermark `W` | the **ledger** — Anam's record of what it knew |
| **Filesystem** | checkpoint values (`VALUE#<hash>`) and served receipts | **published artifacts** — immutable, content-addressed |
| **SQLite** | Facet Definitions, projection templates, token scopes, the facet and record-event indexes, the span registry and per-span `V`/`Wᵥ`, the checkpoint and receipt index, saved span bookmarks, sync cursors, GC state, the operational log | Anam's **operational store** |

Config is **YAML**; Facet Definitions are **JSON** (§3a).

An earlier draft said *"SQLite holds nothing that cannot be rebuilt by replaying the ledger."* That named
one recovery source for a store with several, and demoted a real database to a cache. §1's claim is about
**source facts** — no captured payload exists only inside Anam. It says nothing about Anam's own state,
and Anam has plenty: definitions, indexes, projection templates, roles, logs, bookmarks. The operational
store is authoritative for those, and losing it is not free.

What is worth knowing is not a rule but a fact per table — **where does this come back from?** — because a
rebuild has to know what it may drop:

| Tier | Tables | Recovery |
|---|---|---|
| **Derived** | facet index, record-event index, checkpoint index, `span_head`, sync cursors | replay the ledger |
| **Provisioned** | Facet Definitions, projection templates, token scopes | the embedded assets and config — i.e. git |
| **Authored** | saved span bookmarks, GC state, the operational log | nowhere. Back it up |

The tier belongs in the schema beside each table, because it is the only thing that tells a later reader
whether dropping it is survivable.

The **derived** tier must stay *provably* derived. The rebuild test (`MVP.md` §7) drops it and re-derives
it from the ledger: if a facet payload, a record-event or `W` ever acquired a second home there, the test
would pass on data it never rebuilt, and "Anam owns no source fact" would stop being falsifiable.

### The ledger — datom-shaped, not a datom engine

Following Arch ADR 0031: adopt the datom **shape**, do not build a datom **engine**. Facts are
`[entity attribute value tx]` with `tx` = record-time; the store supplies sorted indexes, Anam supplies
the shape and a handful of fixed query forms.

Grain is **hybrid at the verbatim line** (ADR 0031):

- **Verbatim substrate → coarse.** One item per facet artifact: the datoms cover only Anam's *envelope*
  (its governing metadata), and the payload is a single opaque, content-addressed value. Shredding a
  verbatim payload into per-field triples would impose a schema at ingest — crossing the line §3 draws.
- **Derived facets and consent → fine.** Per-attribute datoms, where field-level history and per-field
  provenance actually pay.

```text
pk = person#<id>
sk:
  facet#<facet_id>#v<%06d>   facet artifacts, keyed by identity  (Query: begins_with)
  rec#<%020d>                record-events, keyed by seq         (Query: BETWEEN — replay-since)
  seq                        the watermark W — a counter item, seeded at 0 by init
```

**No secondary indexes.** The ledger serves exactly two access paths, and both are strongly-consistent
base-table range scans that must live in the same substrate as the conditional write:

- `begins_with("facet#<facet_id>#")`, descending, limit 1 — the current artifact version, whose
  `content_hash` `commit` compares against (§6a);
- `BETWEEN("rec#…")` — replay since a watermark.

Everything else a reader wants — the timeline in event-time order, a type filter, `changes_since` — is an
**index**, and indexes live in the operational store, rebuilt by replaying the ledger. A table with a
partition key, a sort key and a conditional counter is *any* ordered key-value store; that is what "the
substrate is accidental" has to mean if it means anything.

### Keys are identity-first, and every integer in them is zero-padded

Both halves were learned the hard way, and both are verified against DynamoDB Local rather than assumed.

- **`event_time` must not appear in an artifact's sort key.** It is a *value*, read out of the payload;
  `facet_id` is *identity*. Key an artifact on its event-time and a corrected source row whose event-time
  moved lands at a fresh sort key: the prior-version lookup misses it, `commit` concludes "never seen",
  and writes `v1` again. Two live artifacts, one `facet_id`, **supersession silently disabled** — the
  `source_ref` mistake (§6a) wearing a sort key. Identity-first makes it unrepresentable rather than
  guarded.
- **Sort keys compare lexically, so integers are fixed-width.** Unpadded, `rec#10 < rec#9` and `v10 < v9`:
  replay returns the wrong window, and the prior-version lookup reads the *ninth* artifact as current from
  the tenth onward, comparing against a stale hash. Spiked: `facet#RAW#v1..v12` queried descending returns
  `v9`; padded, `v000012`. This is the fixed-width timestamp rule (§5) again, in a different key — and it
  is live today in the `mint-cli` reference implementation's `AllSnapshots`.

Times are formatted **fixed-width UTC RFC3339Nano** for the same reason (§5). The **payload is a Binary
attribute, never a Map** — this is where metadata/payload separation (§3) is enforced.

**Facet Definitions are not in the ledger.** A Definition is not something Anam *knew*; it is how Anam
*interprets* — governance, not record. It lives in the operational store, provisioned from embedded JSON
(every version, so an artifact's `definition_version` still resolves). The error kernel is smaller for it.

> **Design note (Hickey).** The single-table `item` is a **marshalling envelope, quarantined to the store
> package** — not a domain type. Each domain value owns its `toItem`/`fromItem`; the `kind` discriminator
> exists only at the storage edge. Current status is absent from the artifact: it is folded from
> record-events (§9).

### `commit` is one transaction, and it is the error kernel

The facet artifact, its `admitted` record-event, and the bump of `W` are written in a **single
`TransactWriteItems`**: artifact and event each conditional on their sort key not already existing, and
`W` conditional on still holding the value it was read at.

`TransactionCanceledException` reports its reasons **positionally**, and the three positions mean three
different things. Collapsing them into one "conflict" would make `commit` either retry a write that
already succeeded or reject a benign re-ingest — so this mapping *is* the idempotency, not error handling
wrapped around it:

| Cancelled item | What it means | Response |
|---|---|---|
| the artifact | this `(facet_id, version)` already exists | compare `content_hash`: **same** → idempotent no-op, we are seeing our own write; **different** → a lost update. Stop |
| the record-event | that `seq` is taken *while `W` was not stale* | the counter is behind its own log. The ledger is corrupt. Stop |
| the watermark | another writer advanced `W` | re-read, re-stamp `record_time`, retry |

`ReturnValuesOnConditionCheckFailure: ALL_OLD` returns the existing artifact alongside the cancellation, so
the hash comparison costs no second read. This is what makes a dropped connection safe: the transaction
committed, the client saw an error, and the retry lands on the first row and reads its own write back as a
no-op.

`record_time` is stamped **inside** the retry, after the successful `seq` read, so that `seq` order and
`record_time` order agree by construction. A retry that reused its original timestamp could place an
earlier `record_time` at a higher `seq`, and the ledger's order — the one thing §1 admits Anam owns —
would be ambiguous.

The retry loop belongs to the **gate**, not the store. A capture worker may already crash mid-batch and be
re-run to known-good, so the recovery is free; putting a loop, a bound and a give-up path inside the one
component that must not fail buys nothing.

> **Design note (Armstrong).** There is **no transaction across substrates**, so everything that must be
> atomic lives in one: the artifact, the event, and the watermark. The operational store's **derived**
> tier is never written in the same breath as the ledger; it catches up afterwards by replaying
> record-events since its last-seen `seq`. That is safe precisely because the ledger is append-only and
> `commit` is idempotent — **a crash leaves the derived tier behind, never wrong, and behind is fixable by
> replay.** Catch-up is not housekeeping: the fold reads its indexes, so it must assert `last_seq == W`
> before folding, or a checkpoint can be computed from a lagging index and be wrong at its own watermark.
>
> The price of this substrate, paid knowingly: DynamoDB cannot refuse an `UpdateItem`, so **append-only
> drops from an invariant the substrate enforces to one our code must not violate.** It falls back to a
> test. SQLite would have held it with a trigger. This is the single largest cost of the split, and it
> lands on Anam's deepest promise.

Every store method takes a `context` with a deadline and **fails fast** on an unreachable endpoint —
never a hang. `ConditionalCheckFailed` and `TransactionCanceledException` surface as a typed conflict.

**The substrate is accidental, not essential.** Only `internal/store` may name DynamoDB, SQLite, or
`database/sql`; `scripts/check.sh` enforces it. Because Anam owns no source fact, **changing substrate is
a re-ingest, not a migration** — as this document has already demonstrated once.

The one carve-out: `internal/capture` may read a **source** in its native format — a source that happens
to be a SQLite file needs `database/sql` to be read at all. That is the *source's* substrate, not Anam's:
a capture worker sits outside the gate, is disposable, and speaks whatever its source speaks. It never
names the ledger's SDK, and `check.sh` enforces that too.

---

## 9. Ledger, canonical, and projections

Two surfaces (§7), and the reads that go to each.

**The ledger** is append-only and complete: every facet ever admitted, at its **current status derived
from record-events**, with provenance — including superseded and tombstoned facets. *What is admitted
stays; it is re-statused, never pruned by retraction.* Forensic reads, `as-of` reads, and rebuild go here.

- **`changes_since`** reads record-events by record-time and reports what changed between two points.
- The **raw / forensic** view of the timeline reads the ledger directly, tombstones visible.

**The canonical** is the **live-only** fold at a watermark: every facet whose current status is live.
Superseded and tombstoned facets are *excluded from the value* — they remain in the ledger, marked.

- **Fold** reads the record-events for a span's members up to a watermark and derives each facet's
  current status (`admitted` / `superseded` / `tombstoned` / `endorsed` / `rejected`), keeping the live
  ones. Liveness ⇔ the current artifact version's status is live.
- The **normal serve path** is *nearest checkpoint + replay-since* (§7), so a read is as fresh as ingest.
- The **whole-person timeline** — Anam's default read — is a **derived span composed** from bucket
  checkpoints plus the open bucket's replay. It never re-folds the person.

From the canonical, Anam publishes governed **projections** (variants): **versioned templates that
prune — never reshape, never generate.** The only knobs are **metadata filters**: a facet-type allow-list
(deny-by-default), a datetime filter, an endorsement-status filter, and a maximum classification.
Filtering is on **metadata only, never the payload**; a projection includes or excludes **whole Facets**.
A consumer **selects** a projection; it never defines one.

> *Tombstone visibility* is no longer a projection knob. It was one only while "canonical" named the
> append-everything surface. A projection over the canonical sees live facets by construction; a reader
> who wants tombstones is asking the **ledger** a forensic question, not asking a projection for a variant.

A projection is also **explainable**: the same verdict that decides whether a facet is served reports
*which* knob excluded it, so a governed variant can answer "why is this reading not here?" from the
filter itself rather than from a second account of it. Versions no longer in force — displaced by a
correction, or belonging to a facet no longer live — are not exclusions at all: they never reach a
template, because the canonical is live-only by construction. They are their own standing, derived from
the fold.

Projections are **materialised as files**, content-addressed. **Receipts** make a served projection
reproducible: `served_hash = sha256(served bytes)` plus the exact facet-id set, the watermark, the
template version, the definition-set version, the `served_at` time, and the span. (Arch's consent-version
and linkage-version drop out — no consent gating, no identity graph.)

Receipts are also where **publication** is recorded. A facet's published time (§5) is the earliest
`served_at` over the receipts naming it; there is no `published` record-event, because being shown is not
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

**Admission is not endorsement.** Endorsement (`endorsed` / `rejected`) is itself a record-event and
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
- **The ledger is the system of record**; files hold published artifacts; SQLite is Anam's **operational
  store** — authoritative for what Anam authors, and annotated per table with where it comes back from.
  Its **derived** tier must stay provably rebuildable by replaying the ledger, or the rebuild test proves
  nothing (§8).
- **Append-only**: facts are added and superseded by status events (tombstones) — **never modified,
  never deleted.** DynamoDB cannot enforce this; our code must not violate it, and a test guards it (§8).
- **Current status is derived** by the fold from record-events — **never stored** on the artifact (no
  place-oriented programming). A tombstone is therefore a **record-event, not an artifact version**: an
  Anam artifact has nowhere to carry a status (§6a).
- **`source_ref` is identity, `content_hash` is content.** The natural key never contains the row's
  values, or supersession silently becomes duplication (§6a).
- **Sort keys are identity-first and fixed-width.** An artifact's key carries its `facet_id`, never its
  `event_time`; every integer in a sort key is zero-padded. Otherwise supersession silently duplicates, or
  lexical order stops being chronological (§8). The ledger carries **no secondary indexes**: an index is
  derived, and derived things live in the operational store.
- **Canonical means the live-only value; the ledger means append-everything-marked.** One word, one
  surface (§7, §9).
- **Metadata / payload separation**: the payload is verbatim, immutable, stored opaque, never parsed
  into a canonical schema.
- **Three-pillar admission through the one gate**; **`commit` is idempotent and atomic** — artifact,
  record-event and watermark in one transaction, in one substrate (crash-clean).
- Projections **prune, never reshape or generate**; a consumer selects, never defines.
- Agent findings are derived Facets admitted as `proposed`; **endorsement never writes back**.
- **The Envelope is the only ingress**; every external call is timed and fails fast.

---

## 13. MVP boundary and deferred items

**MVP** — see `MVP.md`: the vertical slice end-to-end over two real sources (`health-db`, `bp-csv`),
one or two projections plus a CLI timeline viewer, manual mint, and receipts. Read-only throughout.

**Deferred, not forgotten:**

- **Binary / native-reference facets** — reserved; no binary source in the first slice.
- **Adaptive mint cadence (time-OR-size-OR-rate)** — manual `checkpoint` only; the bucket-span seam and
  the `W`/`Wᵥ` machinery exist from R2, the trigger does not.
- **Sealed projections / a seal point** — receipts suffice for now.
- **Token rotation; multi-scope tokens** — one static `source → types` map.
- **Consent policy and classification-driven gating** — attribute + pass-through gate only (§10).
- **Storage management** — Anam is append-only and grows; an archive tier is deferred, tractable
  precisely because Anam owns no facts and can re-ingest.
- **Extensibility to non-health facets** (`calendar.*`, `mail.*`, `note.*`) — the model already admits
  them; they are simply not built yet.
