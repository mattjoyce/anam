# Anam — Glossary

> The shared language. Each term has **one** meaning. Where a word is deliberately *not* an Anam
> concept, it says so, so it doesn't creep back in. Companion to `ARCHITECTURE.md`.

---

**Admission gate** — The check every Facet must clear to enter the ledger, on three pillars:
*facet-compliant*, *authenticated*, *associated*. Exposed as two calls, `resolve` then `commit`. The
**only ingress**. See *Binding*, *Envelope*.

**Anam** — This system: a personal, longitudinal episodic-context store over one person's own sources.
*anam* is Irish for *soul*. Its default read is the whole person (see *Whole-person read*).

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
that is corrected. See *Ledger*, *Fold*, *Checkpoint*.

**Capture worker** — The MVP's name for what is now an *importer*: an external, pluggable client that
reads a source and submits envelopes to the ingest gate. In the MVP: `health-db` and `bp-csv`, compiled
in; since 2026-10-11 every vendor source is an importer outside the binary, and now every
source is (`docs/IMPORTERS.md`).
**Out of the kernel.** See *Source*, *Envelope*.

**Checkpoint** — An immutable, content-addressed value of the canonical (live-only) fold over **one
minted span's** members at an anam-time *watermark*. Produced by the mint solely to bound replay depth.
Never whole-life. Comes in two provenances, *hard* and *soft*, below; only a hard one is a replay
floor. See *Mint*, *Watermark*, *Bucket span*.

**Checkpoint, hard** — A checkpoint that has been given a place: a row in the checkpoint log
(`span_id`, `v`, `covers_w`, `value_hash`) and a row under its hash in `checkpoint_value`. A checkpoint is a cache that may go missing: a later checkpoint of the span replaces it, a mint then removes it, and a read that needs it folds the ledger. Minted,
citable, a replay floor. The mint promotes a soft checkpoint to a hard one. See *Checkpoint, soft*.

**Checkpoint, soft** — A bucket's live-only value at a watermark, computed on demand and **ephemeral**:
the same type and hash a hard checkpoint would have, with no row and no file. Shown, compared, dropped;
never kept between requests, or it becomes a stale place. Soft equals hard by hash means the bucket is
*sealed*; soft differs means the bucket is *behind* (still a correct floor, costing replay); no hard
means *never minted*. A soft checkpoint bounds no replay and is not what the doctrine means by
"checkpoint". Distinct from a *Derived span*, which composes either kind over a range the person
names. The inspector's strip under the timeline band draws every leaf bucket of the mint's layout in
one of the three states, computed for the page and dropped (2026-10-13); its header line
counts them and names the last mint run. "Sealed" is also said of the *mint log* as a whole: every
checkpoint belongs to a recorded run; an unsealed log means a mint did not finish, and reads fold in
full until the next one. See *Checkpoint, hard*, *Mint*, *Span*.

**Classification** — A governance level on the Facet Definition and each Facet:
`Normal` / `Confidential` / `Sensitive`. Present and carried; **gating on it is deferred** (§10). See
*Consent*.

**Consent** — In Anam, single-user and **default-allow**; the gate *shape* is kept (`Evaluate` returns
allow) so the seam exists for future third-party serving. Not a policy engine yet. See *Classification*.

**Content hash** — `sha256` of the canonicalised payload; recorded in the descriptor and used for
idempotency and supersession (a changed source row hashes differently). See *Idempotency*.

**Derived facet** — A Facet produced by computation over Anam's own content (an **Insight**) rather than
captured from a source. Re-enters through the ordinary gate as `proposed`. See *Endorsement*, *Insight*.

**Domain** — The first segment of a *Facet type*, and a closed set of ten: `health`, `activity`, `place`, `record`,
`mind` (self-reported inner states, where attention is the first concept), `org` (relationships with
organisations, where employment is the first concept) and `money` (what the person paid, where
purchases are the first concept) are in use; `social`, `media` and
`work` are reserved for the next dense sources (`social.message`, `media.play`). A new kind of
fact joins one of the ten. A head is added only by a logged decision, as `mind`, `org` and `money` were, so growth is
in the concept, in a specialisation after it, and in the Definition's `family` (a finer grouping, such as `meds` or `trips`) and `kind`
(`measure`, `reading` or `event`). Name the common concept and code the open-ended tail: lab results are one type
with the test code in the payload, not a type per test.

**Endorsement** — The review status of a derived facet (`proposed` → `endorsed` / `rejected`), recorded
as an anam-event. **Never writes back** into source systems. See *Derived facet*.

**Envelope** — The message an importer hands the gate: `{type, source, injector, source_ref?,
payload, asserted_person?, span_hint?}`. Left out, `source_ref` is composed by the gate from the
Definition's `identity_key`. The **only ingress**; importers share nothing with the
ledger except through it. See *Admission gate*.

**Episode** — *Not a first-class Anam concept.* Anam has **no required read span** and inherits no
boundary from any System of Record. A trip, an illness or a training block is an optional derived *Span*
bookmark. Anam still mints per span, a *Bucket span*, to bound replay. See *Span*, *Bucket span*,
*Whole-person read*.

**Error kernel** — Armstrong's term, adopted: Anam's small trusted core that must not corrupt — the
**append-only ledger plus the admission-gate invariants** (three pillars, idempotency, immutability).
`commit` writes artifact, anam-event and watermark in **one transaction in one substrate**; there is no
transaction across substrates. Everything above it — fold, projections, importers, CLI — is
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

A wall clock the zone skips or repeats at a daylight-saving change names no single instant, and the
gate refuses it rather than choose. How the source's clock was read is the payload's
*Source time*.

**Series** — Which facets are repeated statements about one thing over time, so that a *Snapshot*
can keep the latest per thing. Neither identity (every change is its own row) nor type (`health.medication.started`
and `health.medication.stopped` are one regimen). A Definition declares a series **family** (`med.regimen`, named like
a type), the ordered key pointers that name one series within it (the drug and the course), and
whether the type **closes** the series (`health.medication.stopped`). The gate composes `<family>:<values>` and
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
semantics, linking elements, validation bar, classification. Bundled as embedded JSON; an importer registers
its own through `POST /v1/definitions`. See *Facet*.

**Facet type** — The kind of a Facet, named **lower-case dotted** as `domain.concept[.specialisation…]`: `health.blood-pressure`,
`health.sleep-episode`, `health.heart-rate-alert`, `activity.steps`, `place.stay`, `record.edge`. The first word is a
*Domain*; the second names the concept; a third or later names a specialisation, either a kind within the
concept or the vendor whose own payload shape it is. Dotted on purpose; payload keys are snake_case. Where a
distinction goes is [where a distinction is encoded](explainer/bundle/decisions/where-a-distinction-is-encoded.md).

**Fold** — The computation that reads a span's anam-events up to a watermark and derives each Facet's
**current status**, keeping the live ones — producing the *Canonical*. The serve path is *nearest
checkpoint + replay-since*, so a read is **as fresh as ingest**. The whole-person view **composes** bucket
checkpoints; it never re-folds the person. See *Canonical*, *Checkpoint*.

**Handling** — The optional third block of a Facet Definition, beside `meta` and `schema`: what every
writer and reader of the type must agree on for its facts to mean the same thing, and the schema cannot
say (a naming convention, an id's shape, what a disappearance means). Anam holds it as part of the
version, serves it verbatim and checks only that it is an object; it enforces nothing inside it,
but reads one key, `label`, a short template of literal text and `{/pointer}` fields that a page draws a
facet of the type as ("melatonin 5 mg"). By convention a `guidance` key carries prose. **Not** a rule set for the gate, and **not** importer
tactics (retry, cadence), which stay in the importer's config. See *Facet Definition*.

**Idempotency** — Re-ingesting the same source row is a **no-op** (same `source_ref` + same
`content_hash`). Guaranteed by a deterministic `facet_id = uuidv5(source_ref)` and an atomic conditional
write. What makes ingest **crash-clean**. See *Content hash*, *Supersession*.

**Insight** — An Anam-authored *derived facet*: a finding computed over the *Canonical* (e.g. a trend),
admitted `proposed`, isolated from world-facts. See *Derived facet*, *Endorsement*.

**Whole-person read** — Anam's default and normal mode: the whole-person timeline, read without a span.
A person is unbounded and "longitudinal is system-of-record gravity"; **Anam makes the Person the
substrate** and accepts that gravity, at the scale of one life where the cost is affordable. Note what
it does *not* license: the **read default** is unbounded, the **mint unit** is not. See *Span*.

**Ledger** — Anam's **system of record**: the append-only log of everything it admitted, every Facet
retained and **marked at its current status**, including superseded and tombstoned ones. Facts are
**added** and re-statused — never modified in place, never deleted. Forensic reads, `as-of` reads and
rebuild go here; the *Canonical* is derived from it. Authoritative about what Anam **knew**, never about
the world (the sources are). See *Canonical*, *Tombstone*.

**Live-set** — The Facets whose current status is live at a watermark. The content of a *Checkpoint*.

**Mint** — Adaptive checkpointing, **per minted span**, whose only job is to **bound read-time replay
depth** — not freshness (the Ledger is always current, so a read replays to the head). Cadence is
time-OR-size-OR-rate; a quiet bucket mints nothing. Automatic after every ingest that moves W since
2026-10-09, and `POST /v1/checkpoints` by hand. See *Checkpoint*, *Bucket span*.

**Library** — The folder beside the ledger (`documents/`, `library_dir`) that holds document bytes.
Only Anam writes it (`POST /v1/documents`), and it names each file by its sha256,
in subfolders of the sha256's first characters once a folder holds `library_folder_limit` files. A
`record.document` Facet's payload is the document's **name** (its `path`, the identity), sha256, size,
media type and the date its sidecar gives it; the sha256 finds the bytes. The folder never enters a
fact. The share's private folder is the **inbox**, not a library. See *Native reference*.

**Native reference** — A reference Facet's payload: a pointer to a document in its *Library*, resolved
live by the inspector from the library root. Built as `record.document`.

**Operational store** — The SQLite database holding Anam's own state: Facet Definitions, registered
projection templates, the facet, edge and anam-event indexes, the checkpoint log (per-span `V`/`Wᵥ`) and
its values, receipts, served values, saved selections, importers' snapshot records, admission holds and
sync cursors. It is a
real store, not a cache — authoritative for what Anam authors. Each table records **where it comes back
from**: the *derived* tier from replaying the *Ledger*, the *provisioned* tier from embedded assets and
config, the *authored* tier from a backup and nowhere else. Only the derived tier is disposable, and it
must stay provably derived (`ARCHITECTURE.md` §8). See *Ledger*, *Sync cursor*.

**Person** — The storage **substrate**: all facets partitioned onto one **timeline**, in a ledger
file pinned to that person's id. In Anam there is exactly one person and **no identity graph, no merge**. See *Timeline*.

**Fields** — A temporary subset of a type's schema that a read names (decided 2026-10-12): JSON
pointers into the payload, so only those come back. Fields only **take away**: they are applied after the
projection and narrowing, to whole facets those already admitted, so they can neither resurrect nor exclude
a facet, and never show a field a projection would not have served. The envelope is always whole; the
stored payload stays opaque bytes, read at serve time. The answer is a *Frame*. The *Receipt* pins the
fields. Computing over fields is a *Query*, not fields; blending projections is neither. See *Frame*,
*Projection*, *Query*.

**Frame** — The shape a read with *Fields* answers in: `columns`, the envelope's attributes then the
pointers, and `rows`, one array per facet in timeline order, a null where a row lacks a pointer. A
relation, as a query returns one, rather than a list of facets; it reads straight into a DataFrame. A read
without fields answers in whole facets as before. See *Fields*.

**Projection** *(variant)* — A versioned template that **prunes** the *Canonical* for a purpose: metadata
filters only (type allow-list, datetime, endorsement status, max classification); whole-Facet
include/exclude; **never reshapes, never generates**. Materialised as a content-addressed row (`served_value`). A consumer
*selects*, never defines; it may name the *Fields* it wants of what the projection serves, which only
narrows, or run a *Query* over it, which relates what it served and never widens it. The operator
defines: templates are **bundled** with the binary (the floor) or **registered**
over the API (2026-10-13), each version immutable and every version held readable, since a
receipt names the version and its hash. Tombstone visibility is **not** a knob — the Canonical is live-only; a reader wanting tombstones
queries the *Ledger*. See *Fields*, *Receipt*, *Canonical*.

**Provenance** — The recorded origin of a Facet: source and injector identity, plus (for derived facets)
the method/model and the canonical watermark it was computed from.

**Published time** — When a Facet first appeared in a served account: the **earliest `served_at` over
the receipts naming it** (an Anam clock). Deliberately *not* an anam-event — being shown changes
nothing Anam knows, so it lives in the operational store's authored tier and is lost with it. See *Receipt*,
*Anam-event*.

**Query** — The third read, after the timeline and *Fields* (decided 2026-10-17, built 2026-10-19;
reference in [QUERY.md](QUERY.md)): a Datalog program evaluated over the *Fold* at a watermark, **through a *Projection***. The
projection decides which whole facets the query may see; the query relates them: joins across types
along *Fact-time*, selection by any payload field the served Definition's schema names, aggregates,
windows. Base relations are the *Envelope* (`entry/7`, with `definition_version`) and the payload
fields read from the served copy (`attr/3`, any schema pointer). Bundled rules such as `current/2` ship
as a versioned rule set; `current/2` is held equal to *Snapshot read* by a standing test. The answer
is a derived value that hashes, with a *Receipt* naming the projection, the rule-set hash and the
query text, so `receipt verify` re-runs it. A query reads payload fields from the served copy and
never from the store, moves no *Watermark*, and writes nothing. The store never selects by payload; a
read may. See *Fields*, *Projection*, *Receipt*.

**Receipt** — Proof of exactly what a projection served at a watermark: `served_hash`, the facet-id set,
watermark, template version, definition-set version, `served_at`, optional span, and since 2026-10-07
the `ledger_id` the watermark counts in (a verify against another ledger says so instead of failing), and
since 2026-10-10 an optional `query`: the snapshot, window and type a read narrowed the projection by, as
resolved instants, which a verify replays. A Facet's
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

**Source** — A capture origin, named **lower-case hyphenated**: `garmin`, `bp-csv`. The identity that
appears in provenance and cursors. See *Importer*.

**Source time** — The payload's `source_time`: the fact time as the source supplied it
(`value`), the zone that value is read in (`zone`, an IANA name, `UTC` or a fixed offset), and who
decided that zone (`basis`: `source`, `configuration` or `assumption`). It sits beside *Fact-time*,
which stays the one UTC instant the gate derives, so a reader can tell an offset the source supplied
from an importer's guess. Its shape is Anam's (`internal/def/sourcetime/`), copied into each
Definition that carries it and checked at load; a type that needs it lists it in `required`. A
daily aggregate's interval is not here: it is the Open mHealth `effective_time_frame`, each end at
its own offset.

**Source ref** — The deterministic natural key identifying **which** source row this is — composed from
the Definition's declared `identity_key` and **never from the row's values**: `bp-csv:19:<excel_datetime>:1:<reading>`,
*not* `bp-csv:…:<systolic>/<diastolic>/<pulse>`. The shape is `facet.SourceRef`: the source, then each
identity value length-prefixed, so distinct tuples never share a ref whatever their values contain; the
gate and every worker compose it through that one function. Where the identity is one payload field
whose text a writer must build (a dose's `id`), the Definition declares an `identity_template` and the gate
fills or checks that field from it, so a form, a chat and an importer cannot spell it three ways.
Basis for the `facet_id`. Braiding the
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
artifact is written: an Anam artifact has nowhere to carry a status (a tombstone is not a status-bearing
artifact version). It advances the watermark like any mutation. The Facet stays in the
*Ledger* forever, marked; it is absent from the *Canonical* value by construction. See *Ledger*,
*Canonical*, *Status*.

**Vendor leaf** — A third segment that names the vendor whose own payload shape a type is:
`activity.exercise-session.garmin`, `.concept2`, `.technogym` and `.strong`, `activity.day.garmin`,
`health.measurement.withings`, `health.cpap-session.dreamstation`. A vendor's shape takes the leaf even
with one vendor; a standard shape (Open mHealth, IEEE 1752) takes none. A concept split into leaves keeps a
shared core, the fields every leaf carries with one meaning, which `internal/def/shared-core.json` names and a test
holds identical. A reader asks across the leaves by the prefix (`activity.exercise-session.`). See
*Facet type*.

**Watermark** — An anam-time marker pinning "what was known" at a point — the basis for checkpoints and
receipts. `W` is the **stream watermark**: the `seq` of the newest anam-event, advanced by *every*
anam-event that can change the live-set (including a pure status transition). It is read from the
events, not stored beside them. `Wᵥ` is the watermark the last checkpoint
actually folded; `W > Wᵥ` is the whole re-mint test. See *Checkpoint*, *Receipt*.
