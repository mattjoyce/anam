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

If Anam were deleted tomorrow, no fact would be lost: it rebuilds from the sources. What Anam *authors*
— governing metadata, the ledger's order, checkpoints, receipts, and its own derived findings — is
operational record, backed up, never authoritative. This is what keeps Anam a *context product*, not a
second copy of your life pretending to be the original.

### The one inversion (read this before anything else)

Arch person-partitions storage but makes every read **filtered by an Episode span, and that filter is
the default and required**; a whole-person read is privileged, consent-gated, and denied by default —
deliberately, so Arch can never accrete into a longitudinal record.

**Anam inverts exactly this.** A personal soul-record *wants* to be longitudinal. So:

> **The whole-person timeline read is Anam's default and normal mode. A Span (a trip, an illness, a
> training block) is an *optional* derived lens you impose to focus — never a required filter, never a
> minted or sealed unit.**

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

- A **Span** is an inclusive interval over the timeline — a **derived** filter with query-time
  endpoints (`[from, to]`, optionally narrowed to some facet types). It is **composed on demand, never
  stored as a unit of consistency, never minted, never sealed.** Spans may be saved as **bookmarks**
  (a label + endpoints) for convenience — a `SPAN#` item is a saved query, nothing more.
- Because there is one person, there is **no identity graph and no merge** — Arch's linkage machinery is
  dropped entirely.

What keeps a person-partitioned, longitudinally-read store honest is **not** a span filter (Anam
removed that) but the invariant of §1: **Anam owns no source fact; every payload is verbatim-from-source
and rebuildable.** Anam is a longitudinal *lens over your sources*, not a second system of record.

---

## 5. Time — the two clocks

| Time | Clock | Meaning |
|---|---|---|
| **Event time** | Facet | When the thing happened (the reading, the night's sleep) — timeline placement |
| **Recorded time** | Facet | When a person or system documented it |
| **Ingested time** | Anam | When Anam received it |
| **Published time** | Anam | When it first appeared in a served account |

A **Facet event** is the domain fact — immutable, carrying its own event-time, true independently of
Anam; placed on the timeline at its **event-time**. An **Anam record-event** asserts that Anam's
*knowledge* changed (`admitted`, `superseded`, `tombstoned`, `endorsed`, `published`); it carries
**record-time**.

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
- `commit` writes the artifact and its `admitted` record-event in **one transaction**. Idempotency is a
  `UNIQUE (source_ref, content_hash)` constraint, so the marker is not a separate row to be kept
  consistent — it *is* the artifact's own key (§8).
  - Same `source_ref` **and** same hash → **no-op** (safe re-ingest); the insert conflicts and is ignored.
  - Same `source_ref`, **different** hash → the source row changed: write a **new artifact version** and
    append a `superseded` record-event against the prior. Nothing is mutated; the old artifact stays.
- **Retraction is a tombstone** — a `tombstoned` record-event, never a delete. The `facet` table's
  `BEFORE DELETE` trigger makes this structural rather than aspirational.

> **Design note (Armstrong).** Because `commit` is idempotent and atomic, a capture worker may **crash
> mid-batch and simply be re-run to known-good**. There is no partial facet and no corruption. This — the
> ledger plus these gate invariants — is Anam's **error kernel**: the small trusted core that must not
> fail. Everything above it is disposable and rebuildable.

---

## 7. Ledger, mint, and checkpoints

- The **append-only ledger is Anam's record of what it knew** — its own log, never authoritative (the
  sources are). Everything admitted stays; supersession and tombstoning are status transitions, never
  deletes.
- The **canonical account** of the person is the **fold over the ledger to the latest known state**,
  produced on read — so a read is **as fresh as ingest** (§9).
- The **mint** produces **checkpoints to bound replay**. In Anam the mint unit is the **whole person
  timeline** (there is no required span, so no per-episode fold — the clean resolution of Arch's
  "inherited span = mint unit" under the inversion). A checkpoint is an **immutable, content-addressed**
  value of the canonical fold at a record-time **watermark**: `hash = sha256(canonical bytes)`.
- The first slice mints on a **manual trigger** (`anam checkpoint`); adaptive cadence is deferred.
- **Receipts** prove exactly what was served at a watermark (§9).

---

## 8. Storage — an embedded SQLite ledger

Anam stores in a single **SQLite** file (`db_path`), opened in WAL mode through `database/sql`. There is
no server, no network, and no daemon: the store is a library call, and the ledger is a file you can copy.

**The substrate is accidental, not essential.** Anam is a single-user, single-partition, local store; the
problem is a personal longitudinal record, not a key-value service. SQLite is chosen because it is the
*simplest thing that satisfies the invariants* — and because it can enforce two of them directly, which
no remote key-value store can. Only `internal/store` may name SQLite or `database/sql`; the domain does
not know what it is stored in, and `scripts/check.sh` enforces that. Because Anam owns no source fact,
**changing substrate is a re-ingest, not a migration.**

The seven domain values get seven tables. There is no marshalling envelope and no `kind` discriminator —
they were artefacts of single-table key-value modelling, and they are gone.

```sql
facet(facet_id, artifact_version, type, event_time, recorded_time, ingested_time,
      person_id, classification, confidence, definition_version,
      binding_assurance, span_hint, source, injector, source_ref,
      method, model, watermark, birth_status,
      source_schema, schema_version, content_hash, payload_kind, media_type,
      payload BLOB NOT NULL,                          -- opaque; never a structured column
      PRIMARY KEY (facet_id, artifact_version),
      UNIQUE (source_ref, content_hash))              -- the idempotency marker (§6a)

record_event(seq INTEGER PRIMARY KEY AUTOINCREMENT,   -- monotonic, allocated in-transaction
      kind, facet_id, artifact_version, prior_artifact_version,
      record_time, person_id)

definition(type, definition_version, ...)   checkpoint(watermark, ...)
span(span_id, ...)                          cursor(source, ...)   receipt(id, ...)

CREATE INDEX timeline    ON facet(person_id, event_time);          -- was GSI1
CREATE INDEX by_type     ON facet(person_id, type, event_time);    -- was GSI1
CREATE INDEX changes     ON record_event(person_id, record_time, seq);  -- was GSI2
```

Times are stored as **fixed-width UTC RFC3339Nano text**, so lexical order still equals chronological
order and `BETWEEN` still gives the timeline window (§5). That decision survives the substrate change
untouched: any ordered text timestamp needs it.

> **Design note (Hickey).** Storing a `Facet` no longer requires braiding seven values into one item
> shape. Each domain value maps to its own table, and the **payload is a `BLOB`, never a set of typed
> columns** — this is where metadata/payload separation (§3) is enforced. Current status remains absent
> from `facet`: it is folded from `record_event` (§9).

> **Design note (Armstrong).** Two invariants are enforced **by the substrate rather than by our
> vigilance**, which is the strongest form the error kernel can take:
>
> - **Append-only** — `BEFORE UPDATE` and `BEFORE DELETE` triggers on `facet` and `record_event`
>   `RAISE(ABORT)`. A future refactor *cannot* overwrite a fact; the database refuses.
> - **Idempotency** — `UNIQUE(source_ref, content_hash)`. Re-ingesting an unchanged row is
>   `INSERT … ON CONFLICT DO NOTHING`; there is no marker to keep atomically consistent with the artifact,
>   because the constraint *is* the marker.
>
> `commit` is therefore one ordinary transaction: insert the artifact, insert the `admitted`
> record-event, commit. A worker may crash mid-batch and be re-run to known-good. `SQLITE_CONSTRAINT`
> surfaces as a typed conflict.

Every store method still takes a `context` with a deadline: SQLite can block on a write lock, and a hung
call is a hung call whatever its cause.

---

## 9. Canonical and projections

The **canonical account** is append-only and complete: every facet ever admitted, at its **current
status derived from record-events** (including tombstoned items, with provenance). *What is admitted
stays; it is superseded or re-statused, never pruned by retraction.*

- **Fold** reads facet artifacts in a window (default: **the whole timeline** — the inversion) plus the
  record-events up to a watermark, and derives each facet's current status (`admitted` / `superseded` /
  `tombstoned` / `endorsed` / `rejected`).
- **`changes_since`** reads `record_event` by record-time and reports what changed between two points.

From the canonical account, Anam publishes governed **projections** (variants): **versioned templates
that prune — never reshape, never generate.** The only knobs are **metadata filters**: a facet-type
allow-list (deny-by-default), a datetime filter, tombstone visibility, an endorsement-status filter, and
a maximum classification. Filtering is on **metadata only, never the payload**; a projection includes or
excludes **whole Facets**. A consumer **selects** a projection; it never defines one.

- **Receipts** make a served projection reproducible: `served_hash = sha256(served bytes)` plus the
  exact facet-id set, the watermark, the template version, the definition-set version, and an optional
  span. (Arch's consent-version and linkage-version drop out — no consent gating, no identity graph.)

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
  and rebuildable.
- What is Anam-authored — governing metadata, the ledger order, watermarks, checkpoints, receipts, and
  provisional derived findings — is **authored, never authoritative**; backed up, not rebuilt.
- **Storage is partitioned by the person (one timeline).** The **whole-person read is the default**; a
  Span is an optional derived filter, never a stored/minted/sealed unit.
- **Append-only**: facts are added and superseded by status events (tombstones) — **never modified,
  never deleted.** Enforced by the substrate: `BEFORE UPDATE`/`BEFORE DELETE` triggers abort (§8).
- **Current status is derived** by the fold from record-events — **never stored** on the artifact (no
  place-oriented programming).
- **Metadata / payload separation**: the payload is verbatim, immutable, stored opaque, never parsed
  into a canonical schema.
- **Three-pillar admission through the one gate**; **`commit` is idempotent and atomic** (crash-clean).
- Projections **prune, never reshape or generate**; a consumer selects, never defines.
- Agent findings are derived Facets admitted as `proposed`; **endorsement never writes back**.
- **The Envelope is the only ingress**; every external call is timed and fails fast.

---

## 13. MVP boundary and deferred items

**MVP** — see `MVP.md`: the vertical slice end-to-end over two real sources (`health-db`, `bp-csv`),
one or two projections plus a CLI timeline viewer, manual mint, and receipts. Read-only throughout.

**Deferred, not forgotten:**

- **Binary / native-reference facets** — reserved; no binary source in the first slice.
- **Adaptive mint cadence and a freshness SLO** — manual `checkpoint` only.
- **Sealed projections / a seal point** — receipts suffice for now.
- **Token rotation; multi-scope tokens** — one static `source → types` map.
- **Consent policy and classification-driven gating** — attribute + pass-through gate only (§10).
- **Storage management** — Anam is append-only and grows; an archive tier is deferred, tractable
  precisely because Anam owns no facts and can re-ingest.
- **Extensibility to non-health facets** (`calendar.*`, `mail.*`, `note.*`) — the model already admits
  them; they are simply not built yet.
