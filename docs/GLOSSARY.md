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

**Bucket span** — Anam's default **mintable** Span: a calendar window over **event-time** (default one
month). Every Facet is a member of exactly one, by arithmetic on its event-time. Invisible to the reader.
The unit of checkpointing. See *Span*, *Checkpoint*.

**Canonical** — The **live-only** fold at a watermark: every Facet whose current status is live.
Superseded and tombstoned Facets are **excluded from the value** (they remain in the *Ledger*, marked).
*Canonical* names this value and nothing else. Earlier Anam drafts used the word for the Ledger surface;
that is corrected (Arch ADR 0032, MC5). See *Ledger*, *Fold*, *Checkpoint*.

**Capture worker** — An external, pluggable client that reads a source and submits envelopes to the
ingest gate. In the MVP: `health-db` and `bp-csv`. **Out of the kernel.** See *Source*, *Envelope*.

**Checkpoint** — An immutable, content-addressed value of the canonical (live-only) fold over **one
minted span's** members at a record-time *watermark*. Produced by the mint solely to bound replay depth.
Never whole-life. See *Mint*, *Watermark*, *Bucket span*.

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

**Episode** — *Not a first-class Anam concept.* In Arch the Episode is the unit of storage, cost and
checkpointing (ADR 0035); Anam has **no required read span** and inherits no boundary from any System of
Record. Anam still mints per span — a *Bucket span* — because bounded minting was never what the Episode
was for. What Arch calls an Episode, Anam expresses as an optional derived *Span* bookmark. See *Span*,
*Bucket span*, *Inversion*.

**Error kernel** — Armstrong's term, adopted: Anam's small trusted core that must not corrupt — the
**append-only ledger plus the admission-gate invariants** (three pillars, idempotency, immutability).
`commit` writes artifact, record-event and watermark in **one transaction in one substrate**; there is no
transaction across substrates. Everything above it — fold, projections, capture workers, CLI — is
disposable and rebuildable. See *Operational store*.

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

**Fold** — The computation that reads a span's record-events up to a watermark and derives each Facet's
**current status**, keeping the live ones — producing the *Canonical*. The serve path is *nearest
checkpoint + replay-since*, so a read is **as fresh as ingest**. The whole-person view **composes** bucket
checkpoints; it never re-folds the person. See *Canonical*, *Checkpoint*.

**Idempotency** — Re-ingesting the same source row is a **no-op** (same `source_ref` + same
`content_hash`). Guaranteed by a deterministic `facet_id = uuidv5(source_ref)` and an atomic conditional
write. What makes ingest **crash-clean**. See *Content hash*, *Supersession*.

**Ingested time** — When Anam received a Facet (an Anam clock).

**Insight** — An Anam-authored *derived facet*: a finding computed over the *Canonical* (e.g. a trend),
admitted `proposed`, isolated from world-facts. See *Derived facet*, *Endorsement*.

**Inversion, the** — The single decision that distinguishes Anam from Arch. Arch's keystone (ADR 0035)
**demotes the Person from substrate to a thin identity index**, because a person is unbounded and
"longitudinal is system-of-record gravity". **Anam re-promotes the Person to substrate** and accepts that
gravity, at the scale of one life where the cost is affordable: the whole-person timeline read is Anam's
default and normal mode. Anam wants to be the longitudinal record Arch refuses to be. Note what the
inversion does *not* license: the **read default** is unbounded, the **mint unit** is not. See *Span*.

**Ledger** — Anam's **system of record**: the append-only log of everything it admitted, every Facet
retained and **marked at its current status**, including superseded and tombstoned ones. Facts are
**added** and re-statused — never modified in place, never deleted. Forensic reads, `as-of` reads and
rebuild go here; the *Canonical* is derived from it. Authoritative about what Anam **knew**, never about
the world (the sources are). See *Canonical*, *Tombstone*.

**Live-set** — The Facets whose current status is live at a watermark. The content of a *Checkpoint*.

**Mint** — Adaptive checkpointing, **per minted span**, whose only job is to **bound read-time replay
depth** — not freshness (the Ledger is always current, so a read replays to the head). Cadence is
time-OR-size-OR-rate; a quiet bucket mints nothing. Manual in the MVP. See *Checkpoint*, *Bucket span*.

**Native reference** — A binary Facet's payload: a pointer to a document in its source, resolved live.
**Reserved**, not built in the first slice (Anam's early sources are all structured/verbatim).

**Operational store** — The SQLite database holding Anam's own state: Facet Definitions, projection
templates, token scopes, the facet and record-event indexes, the span registry and per-span `V`/`Wᵥ`, the
checkpoint and receipt index, saved span bookmarks, sync cursors, GC state, the operational log. It is a
real store, not a cache — authoritative for what Anam authors. Each table records **where it comes back
from**: the *derived* tier from replaying the *Ledger*, the *provisioned* tier from embedded assets and
config, the *authored* tier from a backup and nowhere else. Only the derived tier is disposable, and it
must stay provably derived (`ARCHITECTURE.md` §8). See *Ledger*, *Sync cursor*.

**Person** — The storage **substrate**: all facets partitioned onto one **timeline** under
`person#<id>`. In Anam there is exactly one person and **no identity graph, no merge**. See *Timeline*.

**Projection** *(variant)* — A versioned template that **prunes** the *Canonical* for a purpose: metadata
filters only (type allow-list, datetime, endorsement status, max classification); whole-Facet
include/exclude; **never reshapes, never generates**. Materialised as a content-addressed file. A consumer
*selects*, never defines. Tombstone visibility is **not** a knob — the Canonical is live-only; a reader
wanting tombstones queries the *Ledger*. See *Receipt*, *Canonical*.

**Provenance** — The recorded origin of a Facet: source and injector identity, plus (for derived facets)
the method/model and the canonical watermark it was computed from.

**Receipt** — Proof of exactly what a projection served at a watermark: `served_hash`, the facet-id set,
watermark, template version, definition-set version, `served_at`, and optional span. A Facet's
*published time* is the earliest `served_at` over the receipts naming it — publication is recorded here,
never as a record-event. See *Projection*, *Record-event*.

**Record-event** — An assertion that Anam's *knowledge* changed. There are exactly five kinds —
`admitted` / `superseded` / `tombstoned` / `endorsed` / `rejected` — each carrying **record-time** and a
monotonic `seq`. Every kind can change the live-set, so `W ≡ seq` (one counter). Append-only.
`changes_since` is built from these. A Facet never references a record-event.
**Publication is not one of them**: serving a facet changes nothing Anam knows, and a *Receipt* already
records what was shown. See *Record-time*, *Watermark*, *Receipt*.

**Record-time** — When Anam's knowledge changed (an Anam clock). Orders record-events. Contrast
*Event-time*.

**Source** — A capture origin, named **lower-case hyphenated**: `health-db`, `bp-csv`. The identity that
appears in provenance and cursors. See *Capture worker*.

**Source ref** — The deterministic natural key identifying **which** source row this is — composed from
the Definition's declared `identity_key` and **never from the row's values**: `bp-csv:<excel_datetime>`,
*not* `bp-csv:<excel_datetime>:<systolic>/<diastolic>/<pulse>`. Basis for the `facet_id`. Braiding the
values in would give a corrected reading a new `facet_id`, so it would enter as a new Facet and orphan
the original rather than supersede it. **Identity says which fact; `content_hash` says whether it
changed.** See *Content hash*, *Idempotency*, *Supersession*.

**Span** — Two tiers, and conflating them is the mistake Anam already made once:
a **Bucket span** (minted, invisible, the checkpoint unit) and a **Derived span** (a lens the person
names: `[from, to]`, a saved bookmark, or "everything"). A derived span is **composed on demand, never
stored as a unit, never minted, never pulsed**; it materialises only when a receipt is served. Removing
the span as a *required read filter* never required removing it as a *mint unit*. See *Bucket span*,
*Inversion*, *Episode*.

**Status** — A Facet's **current** state — `admitted` / `superseded` / `tombstoned` / `endorsed` /
`rejected` — **derived by the fold** from record-events, **never stored** on the artifact. Contrast
*Birth status*.

**Supersession** — When a source row changes: a **new artifact version** is written and a `superseded`
record-event appended against the prior. Append-only; the old artifact remains. See *Idempotency*.

**Sync cursor** — A per-source resumption marker (`sync#<source>`). A **cache, not truth** — safe to
lose, because ingest is idempotent; a lost cursor is a re-scan, not data loss.

**Timeline** — A person's facts in chronological (event-time) order — the storage substrate. A Span
selects an interval of it. See *Span*, *Event*.

**Tombstone** — A `tombstoned` **record-event** marking a Facet retracted **without** deleting it. No new
artifact is written: an Anam artifact has nowhere to carry a status (contrast Arch, whose tombstone *is* a
status-bearing artifact version). It advances the watermark like any mutation. The Facet stays in the
*Ledger* forever, marked; it is absent from the *Canonical* value by construction. See *Ledger*,
*Canonical*, *Status*.

**Watermark** — A record-time marker pinning "what was known" at a point — the basis for checkpoints and
receipts. `W` is the **stream watermark**, a monotonic counter advanced by *every* record-event that can
change the live-set (including a pure status transition). `Wᵥ` is the watermark the last checkpoint
actually folded; `W > Wᵥ` is the whole re-mint test. See *Checkpoint*, *Receipt*.
