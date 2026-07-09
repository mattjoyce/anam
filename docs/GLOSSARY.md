# Anam — Glossary

> The shared language. Each term has **one** meaning. Where an Anam term differs from its Arch
> ancestor, the entry says how. Where a word is deliberately *not* an Anam concept, it says so, so it
> doesn't creep back in. Companion to `ARCHITECTURE.md`.

---

**Admission gate** — The check every Facet must clear to enter the ledger, on three pillars:
*facet-compliant*, *authenticated*, *associated*. Exposed as two calls, `resolve` then `commit`. The
**only ingress**. See *Binding*, *Envelope*.

**Anam** — This system: a personal, longitudinal episodic-context store over one person's own sources.
*anam* is Irish for *soul*. It re-instantiates *Arch* with one decision inverted (see *Inversion*).

**Arch** — The parent architecture (`/mnt/Projects/Arch`): a person-centric, episode-scoped clinical
context layer. Anam borrows its machinery wholesale and inverts its access default. See *Inversion*.

**Artifact** — A specific version of a Facet's captured body. Re-versioned on a whole re-capture or a
maturity change. Immutable once written. See *Facet*, *Supersession*.

**Binding** — The resolution of a Facet to the person via its Definition's linking elements. In Anam,
with one person, binding is near-trivial but still attested; it **never requires a Span**. See *Binding
assurance*.

**Binding assurance** — A derived attestation of how confidently a Facet bound to the person: *high*
(source explicitly scoped), *medium* (defaulted), *review* (a carried identity element contradicts).

**Birth status** — The **immutable** state a Facet was born in (`admitted` or `proposed`), stored on the
artifact. It is **not** the facet's current status — current status is *derived* by the fold. One truth
per fact. See *Status*, *Fold*.

**Canonical account** — The append-only, complete account of the person: every Facet ever admitted, at
its **current status derived from record-events**, with provenance. Produced as a **fold over the
ledger** (§9). See *Fold*, *Projection*.

**Capture worker** — An external, pluggable client that reads a source and submits envelopes to the
ingest gate. In the MVP: `health-db` and `bp-csv`. **Out of the kernel.** See *Source*, *Envelope*.

**Checkpoint** — An immutable, content-addressed value of the canonical fold at a record-time
*watermark*, produced by the mint to bound replay. In Anam the checkpoint is over the **whole person
timeline** (not per-span). See *Mint*, *Watermark*.

**Classification** — A governance level on the Facet Definition and each Facet:
`Normal` / `Confidential` / `Sensitive`. Present and carried; **gating on it is deferred** (§10). See
*Consent*.

**Consent** — In Anam, single-user and **default-allow**; the gate *shape* is kept (`Evaluate` returns
allow) so the seam exists for future third-party serving. Not a policy engine yet. See *Classification*.

**Content hash** — `sha256` of the canonicalised payload; recorded in the descriptor and used for
idempotency and supersession (a changed source row hashes differently). See *Idempotency*.

**Derived facet** — A Facet produced by computation over Anam's own content (an **Insight**) rather than
captured from a source. Re-enters through the ordinary gate as `proposed`. See *Endorsement*, *Insight*.

**Endorsement** — The review status of a derived facet (`proposed` → `endorsed` / `rejected`), recorded
as a record-event. **Never writes back** into source systems. See *Derived facet*.

**Envelope** — The message a capture worker hands the gate: `{type, source, injector, source_ref,
payload, asserted_person?, span_hint?}`. The **only ingress**; capture workers share nothing with the
ledger except through it. See *Admission gate*.

**Episode** — *Not a first-class Anam concept.* In Arch the Episode is the required, minted span; Anam
has **no required span and mints nothing per-span**. What Arch calls an Episode, Anam expresses as an
optional *Span* bookmark. See *Span*, *Inversion*.

**Error kernel** — Armstrong's term, adopted: Anam's small trusted core that must not corrupt — the
**append-only ledger plus the admission-gate invariants** (three pillars, idempotency, immutability).
Everything else is disposable and rebuildable from source.

**Event** — A meaningful occurrence on the person's timeline, placed at its Facet *event-time* and
carried by one or more Facets.

**Event-time** — When a fact happened (a Facet clock) — determines *where* it sits on the timeline.
Contrast *Record-time*.

**Facet** — Anam's atomic unit: a captured source fragment **held verbatim** plus its governing
metadata (attributes, a payload descriptor, an opaque payload). **Not** a normalised entity. See *Facet
Definition*.

**Facet Definition** — The independently-versioned contract for a Facet *type*: schema, envelope, time
semantics, linking elements, validation bar, classification. Bundled as embedded JSON; loadable via
`anam def load`. See *Facet*.

**Facet type** — The kind of a Facet, named **lower-case dotted and namespaced**: `health.sleep`,
`health.body-composition`, `health.heart-rate`, `health.blood-pressure`, `health.metric-group` (future:
`calendar.event`, `mail.thread`, `note.zettel`). Deviates from Arch's snake_case on purpose.

**Fold** — The computation that reads Facet artifacts plus record-events up to a watermark and derives
each Facet's **current status**, producing the canonical account. Default window is the **whole
timeline**. A read is *as fresh as ingest*. See *Canonical account*.

**Idempotency** — Re-ingesting the same source row is a **no-op** (same `source_ref` + same
`content_hash`). Guaranteed by a deterministic `facet_id = uuidv5(source_ref)` and an atomic conditional
write. What makes ingest **crash-clean**. See *Content hash*, *Supersession*.

**Ingested time** — When Anam received a Facet (an Anam clock).

**Insight** — An Anam-authored *derived facet*: a finding computed over the canonical account (e.g. a
trend), admitted `proposed`, isolated from world-facts. See *Derived facet*, *Endorsement*.

**Inversion, the** — The single decision that distinguishes Anam from Arch: **the whole-person timeline
read is Anam's default and normal mode**, whereas Arch requires an Episode-span filter and denies the
whole-person read by default. Anam wants to be the longitudinal record Arch refuses to be.

**Ledger** — Anam's append-only record of everything it admitted; facts are **added** and superseded by
status events (tombstones) — **never modified in place, never deleted.** Anam's own log, not an
authoritative record (the sources are). See *Tombstone*.

**Mint** — Checkpointing to bound replay. Manual in the MVP; over the **whole timeline**. See
*Checkpoint*.

**Native reference** — A binary Facet's payload: a pointer to a document in its source, resolved live.
**Reserved**, not built in the first slice (Anam's early sources are all structured/verbatim).

**Person** — The storage **substrate**: all facets partitioned onto one **timeline** under
`person#<id>`. In Anam there is exactly one person and **no identity graph, no merge**. See *Timeline*.

**Projection** *(variant)* — A versioned template that **prunes** the canonical account for a purpose:
metadata filters only (type allow-list, datetime, tombstone/endorsement visibility, max classification);
whole-Facet include/exclude; **never reshapes, never generates**. A consumer *selects*, never defines.
See *Receipt*.

**Provenance** — The recorded origin of a Facet: source and injector identity, plus (for derived facets)
the method/model and the canonical watermark it was computed from.

**Receipt** — Proof of exactly what a projection served at a watermark: `served_hash`, the facet-id set,
watermark, template version, definition-set version, and optional span. See *Snapshot*.

**Record-event** — An assertion that Anam's *knowledge* changed (`admitted` / `superseded` /
`tombstoned` / `endorsed` / `published`), carrying **record-time** and a monotonic `seq`. Append-only.
`changes_since` is built from these. A Facet never references a record-event. See *Record-time*.

**Record-time** — When Anam's knowledge changed (an Anam clock). Orders record-events. Contrast
*Event-time*.

**Source** — A capture origin, named **lower-case hyphenated**: `health-db`, `bp-csv`. The identity that
appears in provenance and cursors. See *Capture worker*.

**Source ref** — The deterministic natural key of a source row (e.g.
`bp-csv:<excel_datetime>:<systolic>/<diastolic>/<pulse>`), the basis for the `facet_id` and idempotency.

**Span** — An inclusive interval over the person timeline, with **query-time endpoints** — a **derived
filter**, composed on demand, **never stored as a unit, never minted, never sealed.** May be saved as a
**bookmark** (`span#<id>`: a label + endpoints — a stored query). Anam's answer to Arch's Episode, minus
the aggregate. See *Inversion*, *Episode*.

**Status** — A Facet's **current** state — `admitted` / `superseded` / `tombstoned` / `endorsed` /
`rejected` — **derived by the fold** from record-events, **never stored** on the artifact. Contrast
*Birth status*.

**Supersession** — When a source row changes: a **new artifact version** is written and a `superseded`
record-event appended against the prior. Append-only; the old artifact remains. See *Idempotency*.

**Sync cursor** — A per-source resumption marker (`sync#<source>`). A **cache, not truth** — safe to
lose, because ingest is idempotent; a lost cursor is a re-scan, not data loss.

**Timeline** — A person's facts in chronological (event-time) order — the storage substrate. A Span
selects an interval of it. See *Span*, *Event*.

**Tombstone** — A `tombstoned` record-event marking a Facet retracted/superseded **without** deleting
it; the canonical account stays append-only, and projections may strip it. See *Ledger*.

**Watermark** — A record-time marker pinning "what was known" at a point — the basis for checkpoints and
receipts. See *Checkpoint*, *Receipt*.
