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

**As-of** — A read at a **anam-time**: what Anam knew at that instant, obtained by folding the ledger
to the watermark in force then. Unqualified and unprefixed — there is only one as-of, and it is always
Anam's clock, never the world's. (This matches Datomic's `as-of` and SQL:2011's `AS OF SYSTEM TIME`.)
Narrowing by **fact-time** is not an as-of: it is a *derived span*, and the two compose. See
*Bitemporal*, *Anam-time*, *Span*, *Watermark*.

**Binding** — The resolution of a Facet to the person via its Definition's linking elements. In Anam,
with one person, binding is near-trivial but still attested; it **never requires a Span**. See *Binding
assurance*.

**Binding assurance** — A derived attestation of how confidently a Facet bound to the person: *high*
(source explicitly scoped), *medium* (defaulted), *review* (a carried identity element contradicts).

**Birth status** — The **immutable** state a Facet was born in (`admitted` or `proposed`), stored on the
artifact. It is **not** the facet's current status — current status is *derived* by the fold. One truth
per fact. See *Status*, *Fold*.

**Bitemporal** — Anam carries two independent clocks, and a read may name either or both: **fact-time**
(when it happened in the world) and **anam-time** (when Anam learned it). They are not two views of
one axis — an as-of picks *which version of the record* you are reading, and a span filters *within*
that version, so they compose rather than compete. The industry's words for the pair are *valid time*
and *transaction time*; Anam's are fact-time and anam-time. See *As-of*, *Fact-time*, *Anam-time*.

**Bucket span** — Anam's default **mintable** Span: a calendar window over **fact-time**, **one month**
by default (`span.Default`; weekly is also implemented, and the id says which). Every Facet is a member
of exactly one, by arithmetic on its fact-time. Invisible to the reader. The unit of checkpointing.
Width is a tuning knob — it trades facets-per-mint against values-per-read — and is not a semantic
choice; the operator sets it with `bucket_width` in config. It is stamped into every bucket id the
facet index holds, so changing it is a rebuild of the derived tier rather than a setting that takes
effect quietly. See *Span*, *Checkpoint*.

**Canonical** — The **live-only** fold at a watermark: every Facet whose current status is live.
Superseded and tombstoned Facets are **excluded from the value** (they remain in the *Ledger*, marked).
*Canonical* names this value and nothing else. Earlier Anam drafts used the word for the Ledger surface;
that is corrected (Arch ADR 0032, MC5). See *Ledger*, *Fold*, *Checkpoint*.

**Capture worker** — An external, pluggable client that reads a source and submits envelopes to the
ingest gate. In the MVP: `health-db` and `bp-csv`. **Out of the kernel.** See *Source*, *Envelope*.

**Checkpoint** — An immutable, content-addressed value of the canonical (live-only) fold over **one
minted span's** members at an anam-time *watermark*. Produced by the mint solely to bound replay depth.
Never whole-life. Comes in two provenances, *hard* and *soft*, below; only a hard one is a replay
floor. See *Mint*, *Watermark*, *Bucket span*.

**Checkpoint, hard** — A checkpoint that has been given a place: a row in the checkpoint log
(`span_id`, `v`, `covers_w`, `value_hash`) and a file under its hash in the artifact tier. Minted,
citable, a replay floor. The mint promotes a soft checkpoint to a hard one. See *Checkpoint, soft*.

**Checkpoint, soft** — A bucket's live-only value at a watermark, computed on demand and **ephemeral**:
the same type and hash a hard checkpoint would have, with no row and no file. Shown, compared, dropped;
never kept between requests, or it becomes a stale place. Soft equals hard by hash means the bucket is
*sealed*; soft differs means the bucket is *behind* (still a correct floor, costing replay); no hard
means *never minted*. A soft checkpoint bounds no replay and is not what the doctrine means by
"checkpoint". Distinct from a *Derived span*, which composes either kind over a range the person
names. See *Checkpoint, hard*, *Mint*, *Span*.

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
as an anam-event. **Never writes back** into source systems. See *Derived facet*.

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
`commit` writes artifact, anam-event and watermark in **one transaction in one substrate**; there is no
transaction across substrates. Everything above it — fold, projections, capture workers, CLI — is
disposable and rebuildable. See *Operational store*.

**Event** — A meaningful occurrence on the person's timeline, placed at its Facet *fact-time* and
carried by one or more Facets.

**Fact-time** — When a fact happened (a Facet clock) — determines *where* it sits on the timeline, and
the axis the timeline is ordered by.

It is **extracted from the payload at the gate**, by the Definition's `time_semantics.fact_time` JSON
Pointer, read in the Definition's declared **zone** (a zoneless source clock is the wall clock where the
person stood: `Australia/Sydney` for every local export, `UTC` for Withings, which writes UTC), and then
promoted to a first-class governed field on the Facet and on every canonical entry. It
does not stay buried in the payload, and that promotion is load-bearing: it is why the timeline can be
ordered and a span filtered without anything ever parsing a payload. Narrowed with `--from`/`--to`, which
is a *derived span* and **not** an as-of. Contrast *Anam-time*; see *Bitemporal*, *Span*.

**Series** — Which facets are repeated statements about one thing over time, so that a *Snapshot*
can keep the latest per thing. Neither identity (every change is its own row) nor type (`med.started`
and `med.stopped` are one regimen). A Definition declares a series **family** (`med.regimen`, named like
a type), the ordered key pointers that name one series within it (the drug and the course), and
whether the type **closes** the series (`med.stopped`). The gate composes `<family>:<values>` and
stamps it on the Facet as metadata, beside *fact-time* and *classification*, so no read opens a
payload to group. Every type in a family declares the same key; the registry refuses one keyed two ways.

**Snapshot** — The last-known-value read: for a fact-time `D`, the latest facet in each *Series* at or
before `D`, with a series whose latest is a closing type dropped and facets without a series left
alone. A pure function of the value it is taken over, so the same answer from the canonical, from an
*as-of* read (bitemporal), or from the composed prefix of hard checkpoints up to `D`'s bucket. The
result is a value; serving it through a projection makes it a *Checkpoint, soft* a receipt can pin.

**Facet** — Anam's atomic unit: a captured source fragment **held verbatim** plus its governing
metadata (attributes, a payload descriptor, an opaque payload). **Not** a normalised entity. See *Facet
Definition*.

**Facet Definition** — The independently-versioned contract for a Facet *type*: schema, envelope, time
semantics, linking elements, validation bar, classification. Bundled as embedded JSON; loadable via
`anam def load`. See *Facet*.

**Facet type** — The kind of a Facet, named **lower-case dotted and namespaced**: `health.sleep`,
`health.body-composition`, `health.heart-rate`, `health.heart-rate-alert`, `health.blood-pressure`, `health.metric-group` (future:
`calendar.event`, `mail.thread`, `note.zettel`). Deviates from Arch's snake_case on purpose.

**Fold** — The computation that reads a span's anam-events up to a watermark and derives each Facet's
**current status**, keeping the live ones — producing the *Canonical*. The serve path is *nearest
checkpoint + replay-since*, so a read is **as fresh as ingest**. The whole-person view **composes** bucket
checkpoints; it never re-folds the person. See *Canonical*, *Checkpoint*.

**Idempotency** — Re-ingesting the same source row is a **no-op** (same `source_ref` + same
`content_hash`). Guaranteed by a deterministic `facet_id = uuidv5(source_ref)` and an atomic conditional
write. What makes ingest **crash-clean**. See *Content hash*, *Supersession*.

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

**Library** — A folder root whose rows are files, read by the `documents` worker: each file is a
`doc.reference` Facet whose payload is its **library-relative path**, sha256, size, media type and the
date its sidecar gives it. The **default library** is `documents/` beside the ledger and needs no
declaration; a named library is a `documents` import with its own root. The root is a place and never
enters a fact. The share's private folder is the **inbox**, not a library: documents move from it into
the library once their records are written. See *Native reference*.

**Native reference** — A reference Facet's payload: a pointer to a document in its *Library*, resolved
live by the inspector from the library root. Built in card 24 as `doc.reference`.

**Operational store** — The SQLite database holding Anam's own state: Facet Definitions, projection
templates, token scopes, the facet and anam-event indexes, the span registry and per-span `V`/`Wᵥ`, the
checkpoint and receipt index, saved span bookmarks, sync cursors, GC state, the operational log. It is a
real store, not a cache — authoritative for what Anam authors. Each table records **where it comes back
from**: the *derived* tier from replaying the *Ledger*, the *provisioned* tier from embedded assets and
config, the *authored* tier from a backup and nowhere else. Only the derived tier is disposable, and it
must stay provably derived (`ARCHITECTURE.md` §8). See *Ledger*, *Sync cursor*.

**Person** — The storage **substrate**: all facets partitioned onto one **timeline**, in a ledger
file pinned to that person's id. In Anam there is exactly one person and **no identity graph, no merge**. See *Timeline*.

**Projection** *(variant)* — A versioned template that **prunes** the *Canonical* for a purpose: metadata
filters only (type allow-list, datetime, endorsement status, max classification); whole-Facet
include/exclude; **never reshapes, never generates**. Materialised as a content-addressed file. A consumer
*selects*, never defines. Tombstone visibility is **not** a knob — the Canonical is live-only; a reader
wanting tombstones queries the *Ledger*. See *Receipt*, *Canonical*.

**Provenance** — The recorded origin of a Facet: source and injector identity, plus (for derived facets)
the method/model and the canonical watermark it was computed from.

**Published time** — When a Facet first appeared in a served account: the **earliest `served_at` over
the receipts naming it** (an Anam clock). Deliberately *not* an anam-event — being shown changes
nothing Anam knows — so it lives in the published-artifact tier and is lost with it. See *Receipt*,
*Anam-event*.

**Receipt** — Proof of exactly what a projection served at a watermark: `served_hash`, the facet-id set,
watermark, template version, definition-set version, `served_at`, optional span, and since 2026-10-07
the `ledger_id` the watermark counts in (a verify against another ledger says so instead of failing). A Facet's
*published time* is the earliest `served_at` over the receipts naming it — publication is recorded here,
never as an anam-event. See *Projection*, *Anam-event*.

**Anam-event** — An assertion that Anam's *knowledge* changed. There are exactly five kinds —
`admitted` / `superseded` / `tombstoned` / `endorsed` / `rejected` — each carrying **anam-time** and a
monotonic, gapless `seq`. Every kind can change the live-set, so `W ≡ seq` (one number). Append-only.
`changes_since` is built from these. A Facet never references an anam-event.
**Publication is not one of them**: serving a facet changes nothing Anam knows, and a *Receipt* already
records what was shown. See *Anam-time*, *Watermark*, *Receipt*.

**Anam-time** — When Anam's knowledge changed (an Anam clock). Orders anam-events, and `seq` ≡ `W`,
so an anam-time resolves to exactly one watermark. The clock an **as-of** reads.

It is **fine-grained** — one instant and one watermark per anam-event — but a bulk backfill compresses
its **span**: thirty months of fact-time can arrive in one afternoon of anam-time, and the two axes
then bear no relation to each other. An as-of *inside* such an import returns whatever prefix of the
backfill had landed by then, which is an artefact of read order rather than history. That is honest
rather than broken: Anam did not exist during the backfilled period, and "what did Anam know in 2024"
really is *nothing*. It makes anam-time an **audit** instrument (what did this receipt stand on; did
the ledger move under a published artifact) rather than a *history* one — history is fact-time's job.
Anam-time becomes historically meaningful only when ingest is incremental. Contrast *Fact-time*; see
*As-of*, *Bitemporal*, *Watermark*.

**Source** — A capture origin, named **lower-case hyphenated**: `health-db`, `bp-csv`. The identity that
appears in provenance and cursors. See *Capture worker*.

**Source ref** — The deterministic natural key identifying **which** source row this is — composed from
the Definition's declared `identity_key` and **never from the row's values**: `bp-csv:19:<excel_datetime>:1:<reading>`,
*not* `bp-csv:…:<systolic>/<diastolic>/<pulse>`. The shape is `facet.SourceRef`: the source, then each
identity value length-prefixed, so distinct tuples never share a ref whatever their values contain; the
gate and every worker compose it through that one function. Basis for the `facet_id`. Braiding the
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
`rejected` — **derived by the fold** from anam-events, **never stored** on the artifact. Contrast
*Birth status*.

**Supersession** — When a source row changes: a **new artifact version** is written and a `superseded`
anam-event appended against the prior. Append-only; the old artifact remains. See *Idempotency*.

**Sync cursor** — A per-source resumption marker (`sync#<source>`). A **cache, not truth** — safe to
lose, because ingest is idempotent; a lost cursor is a re-scan, not data loss.

**Timeline** — A person's facts in chronological (fact-time) order — the storage substrate. A Span
selects an interval of it. See *Span*, *Event*.

**Tombstone** — A `tombstoned` **anam-event** marking a Facet retracted **without** deleting it. No new
artifact is written: an Anam artifact has nowhere to carry a status (contrast Arch, whose tombstone *is* a
status-bearing artifact version). It advances the watermark like any mutation. The Facet stays in the
*Ledger* forever, marked; it is absent from the *Canonical* value by construction. See *Ledger*,
*Canonical*, *Status*.

**Watermark** — An anam-time marker pinning "what was known" at a point — the basis for checkpoints and
receipts. `W` is the **stream watermark**: the `seq` of the newest anam-event, advanced by *every*
anam-event that can change the live-set (including a pure status transition). It is read from the
events, not stored beside them. `Wᵥ` is the watermark the last checkpoint
actually folded; `W > Wᵥ` is the whole re-mint test. See *Checkpoint*, *Receipt*.
