# Anam API — operating the record and reading it

Design proposal · 2026-10-07 · documentation only; these APIs are not implemented.
Split 2026-10-10 (card 72): the MCP mapping moved to the `anam-mcp` repository (§8).

**The shapes are in the spec (2026-10-19, card 49).** `internal/daemon/openapi.yaml` describes every
route the daemon serves, with its roles, parameters, bodies and status codes, and a test holds it to
the mux: a route added without a path there, or a path without a route, fails `scripts/check.sh`,
and every example decodes into its `internal/api` type. A running daemon serves it at
`GET /v1/openapi.yaml` without a token. This document stays the prose: why the routes are what they
are, and what is still proposed.

**What runs today (2026-10-08, cards 42–45).** `anam daemon` serves an interim `/v1` surface
that carries the CLI's existing reads, not the `ReadRecord` contract below: `status`, `ingest`
and `operations/{id}`, `timeline`, `changes`, `receipts` (list, show, verify) and `checkpoints`
(list, mint). Each answers with a value from `internal/api`, the same value the CLI computes
from local files, so the CLI renders one type either way. With `server` set (`ANAM_SERVER`,
`--server`) the CLI asks the daemon and opens no local file. Reads are whole, bounded by the
daemon's timeout; paging and receipted reads remain this proposal's work. The route list is
in `anam daemon --help`. An ingest operation that moved W carries `mint`, the mint the daemon ran
after it (card 61): `w`, `full`, `from`, `minted`, `buckets`, or `error` when it failed, which
fails nothing. `POST /v1/backups` (card 69) copies the record into `backup_dir` on the write
queue and answers where, at what W, and how big. `POST /v1/facets/{id}/retract` (card 188, slice 3)
tombstones one facet by hand, as the operator: the body is `{"source": "workbench"}`, the source the
facet must belong to, and the answer is the tombstone's `seq`, the `artifact_version` it marks and the
new `w`. It plans inside the write queue's one write, with the plan `anam retract <source> --facet <id>`
applies, and refuses with 409 a facet of another source, a facet that is not live, and a source an
importer declares and has completed a snapshot for (its next resend reconciles it); 404 for a facet the
ledger does not hold. Like the other writes it is 403 to a browser on another site, and 503 while another
write holds the queue. The artifact stays, and a read as of before the tombstone still shows the facet.
`POST /v1/entry` (card 188, slice 2) enters one fact by hand, as the operator, for the Workbench. The
body is `{"type", "definition_version", "payload"}` and nothing else: no source, injector or id. The daemon
submits the one envelope as the `workbench` principal the importers file declares (a source with no token and
`types` as its allow-list; [IMPORTERS.md](IMPORTERS.md) §8.2), through the same write as `POST /v1/envelopes`,
so the gate, the write queue, the index catch-up, `/v1/status` and the mint are the same. The gate fills the
identity field from the Definition's `identity_template`, or refuses a payload that lacks it when the type
declares none. The answer is `outcome` (`admitted`, `superseded` for a new version of a facet already held, or
`no-op`), the `facet_id`, the `artifact_version`, the `seq` of the event it wrote (none for a no-op) and `w`.
404 when the importers file declares no workbench principal; 409 when `definition_version` is not the type's
version in force; 422 with the gate's reasons, verbatim, for a type outside the workbench's types or a payload
the Definition refuses; 403 to any role but the operator and to a browser on another site; 503 while another
write holds the queue. A fact entered this way is under source `workbench`; `source`, not `source_schema`, is
where a fact came from. `POST /v1/envelopes` and the `/v1/definitions`
routes (card 53) are the importers' side, under importers' own tokens; their contract is
[IMPORTERS.md](IMPORTERS.md) §8.

**Receipted reads (2026-10-10, card 87).** `POST /v1/receipts` takes a timeline query that names
a projection, reads it as `GET /v1/timeline` would, and files what it shows with a receipt. The
receipt records W and the resolved narrowing (`query`: `at`, `from`, `before`, `type`), and
`/v1/receipts/{hash}/verify` replays it. Reads also take `w`, a watermark, in place of `as_of`, so a
follow-up stands on a cited read's knowledge. This is the first piece of §5's receipted reads;
paging and evidence by facet are cards 92 and 91.

**Bounded reads (2026-10-11, card 88).** The daemon answers a read within `max_read_bytes`
(`ANAM_MAX_READ_BYTES`, default 512 KiB, §3's budget; 0 is no bound), measured over the entries it
would answer with, encoded as it sends them, whole facets only. `GET /v1/timeline`, `POST
/v1/receipts` and `GET /v1/receipts/{hash}` refuse a larger answer with 413 and a body a program can
act on: `code: result_too_large`, and `too_large` with `matched` (how many entries), `bytes` (counted
up to the bound), `max_bytes`, and `narrow`, the query parameters still unused (`type`, `from`, `to`,
`at`, `projection`). A refused publish files nothing. A receipt too large to show still verifies,
and shows locally. A local read, the operator at their own files, has no bound. The fold still runs
whole before the refusal: the bound protects the answer and the client's context, not the daemon's
work.

**Fields (2026-10-12, card 138; built the same day).** A read may name its **fields**: a
temporary subset of the type's schema, as JSON pointers, and nothing else of the payload comes
back. The envelope (type, fact-time, status, provenance, hash) is always whole. Fields only narrow:
they are applied after the projection and the narrowing above, to what those already admitted, so a
reader sees no facet and no field a projection without them would not have served. The answer is a
**frame**: `columns`, the envelope's columns then the pointers, and `rows`, one array per facet in
timeline order, in place of `entries`. A pointer a row lacks is null. Measured on the live daemon, a
month of `activity.exercise-session` is 184 KB whole and 2.7 KB as a frame of distance, duration and erg
type; the largest row, 752 strokes, is 72 KB whole and under 500 bytes in the frame. The receipt pins
the fields beside `query`, the served hash is over the frame's bytes, and `narrow` names `fields`
when a read is refused as too large. This amends the sentence below, §5's "cannot request selected
payload fields", and ARCHITECTURE.md §9. Computing over fields (joins, selection by a payload
value, aggregates, windows) is not this either: it is the query, `anam query` and `/v1/query`, a
Datalog program run through a projection (decided 2026-10-17, card 173, ARCHITECTURE.md §9; built
2026-10-19, below). Blending projections stays a later contract.

**The query (2026-10-19, card 173).** `POST /v1/query` takes a JSON body: `projection` (required;
the projection decides which whole facets the query may see), `text`, a Datalog program in Anam's
own small syntax built on the bundled rules (`current/2`, `displaced/2`), `relation`, the derived
relation to answer, `as_of` or `w` as a timeline read takes them, and `dry_run`. The base relations
are `entry/7` (the envelope with the Definition version), `attr/3` (every scalar leaf of each served
payload by its JSON pointer, read from the served copy) and `closes/1`; `instant/1` is the parameter
the program defines, as facts with the whole fact-time lexeme or as a rule. The answer is one sorted
relation that hashes, filed with a receipt whose `program` names the rule set, its hash, the text
and the relation, and whose `template` is the projection; `/v1/receipts/{hash}/verify` re-runs it
and must get the same hash, and refuses (409) under a binary whose rules moved. `GET /v1/query` is
the introspection a program is written against: the relations, the fact-time lexeme, each type's
pointers at each Definition version held, and the rules with their text. A program that cannot run
is 400 with a message that says what to change; a derivation or an answer over `max_read_bytes` is
413 with `instant`, `projection` and `program` as `narrow`. `warnings` names a pointer some versions
of a type declare and others do not. `anam query run` and `anam query describe` are the CLI, local
or over the daemon.

This document proposes the application API that every client shares: the CLI, the inspector,
importers, and conversational clients (§8). The two conversational features are **chat with my
record** and **log this**. The API supplies record operations and evidence; the conversational
client supplies language understanding and dialogue. It must also support an operator keeping
Anam current, inspecting failures, and recovering derived state.

The governing contracts are [ARCHITECTURE.md](ARCHITECTURE.md),
[SELF-LOG.md](SELF-LOG.md), and the current records workflow in [INBOX.md](INBOX.md).
Where this proposal needs a new core capability, it names it rather than silently assigning domain
logic to a client. Whole-facet projection is retained; a reader may name the fields it wants of
it (above, 2026-10-12). Before that date element masking was out of scope.

## 1. Start with the conversations

| Question or instruction | What the application must provide |
|---|---|
| What do you have about me? | Available types, projections, observed coverage, source health, and limitations |
| Show my blood pressure during June | A governed timeline window, bounded pages, and stable evidence references |
| What medication was I on when that reading was taken? | A snapshot at the reading's fact-time, sharing the original read's knowledge watermark |
| What did we believe then, before I corrected the date? | The same query at an earlier anam-time, with exact historical versions |
| What changed since we last looked? | Governed changes between ledger positions, including correction and retraction |
| Where did that number come from? | Exact facet version, source provenance, supporting references, and receipt |
| Log this: I donated blood yesterday | Resolve the local date, validate a supported record type, persist a source assertion, admit it |
| That was Tuesday, not Monday | Identify the source record, conditionally correct it, and retain ledger history |
| Did you actually save it? | Durable submission status, separating source persistence from ledger admission |
| Why is this missing? | An explanation of selection, coverage, or availability without inventing facts |

An empty result means no matching recorded evidence under this query. It does not mean an event did
not happen, a medication was never taken, or a source has complete coverage. A returned medication
snapshot describes the recorded regimen; it does not establish adherence.

## 2. Boundaries and ownership

```text
Conversational client -------+
CLI -------------------------+--> Anam service API (Docker on Unraid)
Inspector -------------------+              |
                              Application operations
                               /         |         \
                         reads       authoring    operations
                           |             |             |
                    fold/projection   records -> capture -> gate
                           |                               |
                    value + receipt                     ledger
```

**Target deployment, agreed 2026-10-07:** Anam runs as a service in Docker on Unraid, exposing a
comprehensive application API. SQLite remains the database substrate, on safe host-local storage.
The service owns the ledger, operational store, artifacts, and managed source-writing operations.
CLI, inspector, conversational clients, and scheduled ingestion use the service API in normal operation; they do not
independently open its database files. A separately controlled offline maintenance mode may access
them only with the service stopped.

The domain operations remain transport-independent Go code inside the service. Operation names below
identify contracts, not mandatory package names or HTTP routes. A versioned network API exposes these
contracts to clients; its concrete routes and authentication scheme remain implementation design work.
Define small interfaces at consumers, grouped by concern; do not build a single store/service interface
containing every operation. Every client reaches the record through this API and needs no database
volume mount.

### Storage and process ownership

- Bind persistent host-local storage into the container for both SQLite files and their WAL/shared
  memory companions. Never place the live databases on an SMB-mounted path or in the disposable
  container layer. Select and verify the actual Unraid disk/pool path during deployment.
- A local array-disk path can retain configured array parity protection; a redundant pool is another
  placement option. Local filesystem access and parity protection are separate choices. Keep live
  database directories stationary while the service runs; do not move them between storage tiers.
- One service instance owns the stores. Serialize application writes through bounded scheduling,
  permit bounded concurrent reads, and retain SQLite transactions, busy handling, and gate invariants.
  No database transaction spans a conversation, model call, or wait for client input.
- Distinguish SQLite WAL checkpoint maintenance from Anam's semantic bucket checkpoints. The service
  coordinates both, plus backup and derived-store repair. Do not copy a live database file alone as
  a backup; use a database-consistent backup procedure. Parity does not replace backups.
- Docker supervises the service with an explicit restart policy, readiness checks, and graceful
  shutdown. Shutdown stops accepting work and drains or cancels bounded work; durable submissions
  reconcile after restart. A service outage returns unavailable, never a fallback to direct DB access.

This target replaces the earlier suggestion that each interface could host the domain packages and
open the stores itself. It changes deployment ownership, not Anam's immutable record model.

- **Core:** temporal selection, series resolution, projection verdicts, canonical serialization,
  receipt creation, admission, and immutable ledger invariants.
- **Source authoring:** persistence of user assertions into the records source, source revisions,
  deduplication of submissions, and recovery after interrupted admission.
- **Adapters:** typed request decoding, caller context, cancellation, pagination presentation,
  and translation of structured application results. No SQL, second fold, or medication-specific rules.
- **Conversational client:** interpretation of the user's words, clarification, analysis, and citation.
  It does not manufacture anam-time, endorsement, source identity, or consent.

The server is configured for one person. Caller requests cannot choose a different person, a ledger
path, a source directory, or an arbitrary executable. Operation context carries the authenticated
actor, allowed capabilities, configured zone, deadline, and request ID; those are not model claims.

### Principals and roles (card 89)

Every caller is a principal: a name, a role, and the SHA-256 of its token. The daemon holds the
hashes and never a token. The operator is the principal whose token is in `token_file`, named
`operator`. Everyone else is listed in `importers_file`, under `principals` with a name and a role,
or under `importers`, the file's first shape, where each entry is an importer named for its source.

| Role | Opens |
|---|---|
| `reader` | Every read: status, timeline, changes, receipts and their verification, projections, checkpoints, Definitions, snapshots. Publishing a read (`POST /v1/receipts`), which writes artifacts and never the ledger |
| `importer` | A reader's routes, and envelopes, documents and Definition registration, as its one source and for its types only. With `any_type` in place of `types` it submits every type the daemon knows, registers none and stores no document (card 177). It sees only its own source's snapshots |
| `operator` | A reader's routes, and minting, backups, projection templates and the inspector. It submits no envelopes, since it is no source; it enters facts by hand at `POST /v1/entry`, which the daemon admits as the `workbench` source (card 188) |

A route names the roles it admits, in one table (`internal/daemon/api.go`), and a test sends a
token of each role to every route. No token, or one the daemon does not know, is 401; a token whose
role the route does not admit is 403, and the refusal names the principal, its role and the roles
the route admits. Without a `token_file` the daemon listens on loopback only, and a request that
presents no principal's token is the operator's. Roles are the floor of access control: a reader
narrowed to some projections is later work, not a role.

## 3. Common contracts

### Identity and time

| Value | Contract |
|---|---|
| `record_position` | `{ledger_id, watermark}`; a watermark alone is never a portable position |
| `facet_ref` | `{ledger_id, facet_id, artifact_version}`; identifies immutable evidence |
| `source_record_ref` | Opaque source-owned record identity; distinct from a facet version |
| `source_revision` | Opaque revision token used for conditional corrections |
| `receipt_ref` | Content address of a published receipt, with ledger identity in its contents |
| `submission_id` | Client-generated retry identity, stable across retries of one write intent |
| `operation_id` | Server identity for durable work and progress, not a watermark |

All wire keys use snake_case. Instants are RFC 3339 with an offset; returned instants are UTC.
Date expressions use a tagged shape: either `{instant: "..."}` or
`{date: "2026-10-06", zone: "Australia/Sydney"}`. The two forms are mutually exclusive.
Date-only input retains its precision; it must not become a claim that the event happened at midnight.
The core must define its mapping into fact-time before a type accepting date-only assertions is
offered for conversational writes. Existing legacy timestamp interpretations must be disclosed,
not silently corrected by the adapter.

The application accepts resolved dates, not strings such as `yesterday`. Discovery returns the
server's current instant and configured zone. The client resolves relative expressions against the
user's utterance time, preserving that anchor in the submission. A retry tomorrow still logs the
same day. Unknown or ambiguous time is a validation issue, never a default to now.

### Errors and bounds

Every operation is bounded by context. A normal failure has `code`, `message`, `retryable`, optional
`field_errors` (JSON pointer, reason, acceptable values), and, after any durable write,
`operation_id` plus `durable_effects`. It never exposes sensitive payloads in routine logs.

Required error codes include `invalid_argument`, `unsupported_capability`, `definition_changed`,
`not_found_or_not_visible`, `ledger_mismatch`, `revision_conflict`, `idempotency_conflict`,
`source_unavailable`, `deadline_exceeded`, `result_too_large`, `cursor_invalid`,
`artifact_unavailable`, and `corrupt_record`. Only transient failures invite retries. A cancellation
or lost response is not evidence of rollback; writes are reconciled by submission ID.

Start with a default page size of 50 facets, a maximum of 200, and a serialized response budget of
512 KiB. These are advertised deployment limits, subject to measurement. Whole facets are never
cut to fit. An oversized single facet returns a typed size error and an authorized artifact reference
where supported. Publication also has a configured total byte/work limit; paging does not license
unbounded computation or storage. Large operator work uses an operation handle.

## 4. Discovery: let the agent learn the record's vocabulary

| Operation | Inputs | Result |
|---|---|---|
| `DescribeRecord` | Optional governed scope | API version, person label, current time/zone, ledger position, enabled capabilities, limits, known limitations |
| `ListProjections` | Cursor | Authorized template IDs, versions, descriptions, supported read modes and parameters. Built 2026-10-13 as `GET /v1/projections` (card 125): name, origin, newest version, every version held, description, types, shape, hash |
| `GetProjection` | Template ID/version | Immutable definition, type coverage, restrictions, examples, supported narrowing. Built as `GET /v1/projections/{name}[/{version}]`, the manifest as canonical JSON; `POST /v1/projections` registers one under the operator's token, 201 new, 200 the same body again, 409 another body at a held version or a version behind, 422 a bad manifest or a type with no Definition |
| `ListFacetTypes` | Optional projection, cursor | Available types, human descriptions, read/write eligibility, definition versions |
| `GetFacetDefinition` | Type/version | Identity and time contracts, classification, validation rules, payload schema, examples, series semantics if supported, and the `handling` block verbatim when the type declares one (card 166) |
| `GetCoverage` | Projection, optional window and pinned position | Observed counts and date ranges by type/source, freshness observations, gaps only where justified |

Definition discovery is important because payloads vary. An agent needs to know that a lab result
may carry a unit and reference range, or that a medication change needs a course reference. Today's
closed JSON Schema definitions (card 52) give field names and types, which is not yet a complete
authoring interface. Authorable types need
descriptions, field types, examples, precision rules, and writable-field guidance. Missing schema
knowledge must be reported rather than guessed into an invented normalized payload.

Coverage distinguishes `observed_fact_time_range`, `last_successful_capture`, `last_admission`, and
`checked_at`. A quiet source may be current without recent facts. Completeness is `unknown` unless
the source declares a coverage contract. The receipt watermark proves a ledger position, not source
freshness. Discovery itself is scoped: disallowed types and counts must not leak through a catalog.

## 5. Reading: one explicit query, one stable evidence set

### Query shape

`ReadRecord` takes a projection reference, one read mode, one knowledge selector, optional allowed
narrowing, and pagination bounds. The operator can configure a default whole-person projection;
the client may omit a projection only when that default is advertised. The client selects templates
and supplies permitted parameters; it never sends a new projection or changes classification
ceilings. It may name fields, which only take away.

```json
{
  "projection": {"id": "meds", "version": 1},
  "selection": {
    "mode": "snapshot",
    "at": {"date": "2026-06-15", "zone": "Australia/Sydney"}
  },
  "knowledge": {"position": {"ledger_id": "ledger-example", "watermark": 11381}},
  "fields": ["/name", "/dose"],
  "page_size": 50
}
```

This is a proposed medication query, not an assertion that the template exists today.

- **Timeline:** `selection.mode = timeline`, optional `from` inclusive and `before` exclusive.
  No window means the whole timeline, delivered in pages. Calendar-day windows expand using their
  stated zone, including daylight-saving boundaries; discovery/examples show this explicitly.
- **Snapshot:** `selection.mode = snapshot`, required `at`. Keep the latest state per declared
  series at or before that instant. A date means end of that local calendar day. No lower bound:
  a course started years earlier may still be relevant. Non-series types retain the documented
  prefix behaviour and are identified as such; the result is not universally one row per type.
- **Knowledge:** exactly one of `head`, `as_of` (anam-time instant), or `position`.
  Omitted means head. Resolve it once to a ledger position and return that position.

Fact-time `at` and anam-time `as_of` are separate fields, never synonyms. Related queries should
reuse the returned position, so a medication answer and a blood-pressure answer concern the same
state of Anam's knowledge. A request for a position in another ledger fails explicitly.

Permitted narrowing is an intersection with the template: `types`, `sources`, and `series_refs`
where the template advertises them, plus the temporal selection above. Unknown filters fail. A
client cannot broaden a template through a filter or supply arbitrary payload predicates.
- **Fields** (2026-10-12): a client may request selected payload fields, as JSON pointers into the
  type's schema, the same pointer vocabulary a Definition uses for its fact-time and identity. Each
  pointer must resolve in the schema of every type the read can serve, or the read fails explicitly,
  as an unknown filter does. Fields take away and never add: no field outside the verbatim payload,
  no computed value, no field of a facet the projection would not have served. The envelope is
  always whole. A type whose Definition declares no schema (verbatim mode) has no fields to name
  and says so. No fields means the whole facet, as before. On the wire: `fields`, repeated, on
  `GET /v1/timeline` and `POST /v1/receipts`; `anam timeline --field`, repeated. The answer carries
  `frame` (`columns`, `rows`) in place of `entries`; a 413's `narrow` names `fields`.

Type descriptions and projection descriptions support discovery by topic;
full-text or semantic payload search is a separate future capability with its own evidence and
governance contract. Never describe a metadata query as having searched every word in the record.

### Selection order is a domain contract

1. Resolve the knowledge position and reconstruct the canonical at that position.
2. For a snapshot, resolve series over the necessary fact-time prefix, including closing events.
3. Apply the authorized projection and permitted narrowing to the resulting whole facets.
4. Cut each admitted facet's payload to the named fields, if any, into a row of the frame. Nothing
   after this step sees a whole payload, and nothing before it sees the fields: fields can neither
   resurrect nor exclude a facet.
5. Publish the selected value and receipt; deliver bounded pages of that value.

A filter that removes `health.medication-stopped` before series resolution could resurrect a stopped course.
The core therefore owns this order. A snapshot template must be validated as compatible with the
series families it serves. Hidden evidence can affect core state resolution without being exposed
in explanations. Equal fact-times within a series need an explicit core tie policy; until defined,
return `ambiguous_series` rather than letting map order or UUID order imply a clinical sequence.

Checkpoint selection is an implementation choice beneath this API. A usable checkpoint covers no
anam-events beyond the requested position. Replay must account for late writes to older buckets,
retractions, and versions moving between buckets. Snapshot prefix reads include earlier buckets.
The equality oracle is the full fold at the same position, followed by the same selection.

### Result and continuation

`ReadRecord` is **read-only with respect to the ledger**, but publishes immutable artifacts. This
proposal chooses durable evidence by default for conversational reads. It does not persist the chat.
Publication is reported as a side effect; a client must not present the read as free of effects.
An operator-only `PreviewRecord` can derive an ephemeral result without a receipt, clearly labelled
as a preview. It shares the same selection implementation.

Every first-page response contains:

- `record_position`, resolved query and its digest, template and definition-set versions;
- `receipt_ref`, `served_hash`, `served_at`, `total_count`, `returned_count`, and `next_cursor`;
- facet entries with exact `facet_ref`, fact-time/precision, folded status, classification,
  source identity, content hash, definition version, payload kind, media type, and the verbatim
  payload; or, when fields were named, a frame whose columns are those envelope attributes then the
  pointers, one row per facet, and nothing else. The content hash is always the whole payload's, so
  a row still names the fact it came from;
- evidence links available to this caller and explicit limitations such as legacy timezone handling;
- an unambiguous empty-result description when `total_count` is zero.

Citation metadata is a companion envelope. It must not change canonical bytes merely to add an
artifact version, URI, or page number. The publication contract must pin the correspondence between
canonical entries and exact evidence references.

`ReadPage(cursor)` reads the immutable published result, never head again. The opaque cursor binds
the receipt, query, stable ordering, and next offset; it is tamper-evident or validated against stored
state. Recheck authorization on every page. Invalid or missing artifacts produce explicit errors,
not a silent new query. The page is a portion of the receipted value: the receipt hashes the complete
published value, not a client's response wrapper or that page alone.

Receipts must grow to pin ledger identity, the resolved query (including snapshot date and zone),
the fields, selection-semantics version, exact template version, and definition provenance needed for historical
series interpretation. Preserve older receipts under their declared format; never reinterpret one
using whatever template or definition happens to be current. Template history must be available for
reproduction, or verification must report it unavailable. These are core extensions.

### Evidence, history, and changes

| Operation | Purpose and boundary |
|---|---|
| `GetEvidence(receipt_ref, facet_ref)` | Retrieve an exact member of an authorized published result, with provenance; no arbitrary facet-ID bypass |
| `ExplainSelection(query_ref, candidate_ref)` | Use the core's verdict and folded standing; disclose candidate details only if separately authorized |
| `ReadFacetHistory(facet_id, bounds)` | Explicit forensic capability for versions/status events; ordinary projection access does not grant it |
| `ReadChanges(from_position, to_position, projection)` | Pin both ends, return authorized changes with a bounded cursor and reasons |
| `GetReceipt(receipt_ref)` | Inspect the published claim and its query without needing a live ledger |
| `VerifyReceipt(receipt_ref)` | Report artifact integrity and ledger reproduction independently, including wrong-ledger and unavailable-version outcomes |

Projected changes mean changes to the selected view, not a raw event dump. They must represent
entries leaving a view as well as entering or changing within it. Snapshot changes are not offered
until the core can account for a new series event displacing another facet. Raw anam-events remain
an operator/forensic operation. A change cursor carries ledger identity and both pinned endpoints.

Supporting document references expose identity, media type, and hash where available. They do not
grant arbitrary filesystem reads. In today's records workflow, document/extraction provenance stays
in the records file rather than facet payloads. An evidence-link resolver is therefore a real missing
capability: it must bind links to source revisions, and distinguish verified links from merely current
source-file associations. Do not imply every facet already has a ledger-attested document link.

## 6. Writing: make “log this” reliable

**Decided since (card 93, 2026-10-15); the records-file route below is overtaken.** A conversation is
an importer like any other, posting envelopes under its own token (§8 of `docs/IMPORTERS.md`); no
records file sits between it and the gate. The person's words are saved at once and verbatim as a
`record.note` under source `conversation`, which may name no facet yet (v3, card 157). A model's
reading of those words is a second source, `interpretation`, whose facts link back to the exact note
version and carry `standing: draft | endorsed | rejected`; the person endorses or rejects a draft
with a new version, and nothing is deleted (cards 158 to 161). The operations and the concurrency
protocol below are kept for their reasoning; the envelope path gives idempotency and correction by
itself.

### The source is durable before admission

A user's assertion is source material. The application writes it into the configured records source,
then capture submits an Envelope through the ordinary gate. No client operation inserts a facet
directly.
This is a source-authoring capability alongside Anam, not permission for importers to write the
ledger or for the ledger to become the only home of the assertion.

Use a managed records file per submission (or per deliberately grouped assertion), within the
declared records directory. Its row IDs remain stable through corrections. An external human-edited
file is not automatically writable through the API. Source targets are configured identities, never
caller-supplied paths. The existing scanner's flat directory and duplicate-ID rules must be respected.

| Operation | Contract |
|---|---|
| `ValidateAssertion` | Validate a typed candidate against a pinned Definition and authoring rules; return normalized interpretation, missing fields, and candidate matches; write nothing |
| `SubmitAssertion` | Persist one resolved user assertion using a required submission ID, then attempt bounded admission |
| `GetSubmission` | Find durable outcome by submission ID, including source identity/revision and admitted facet refs |
| `CorrectAssertion` | Replace a managed source record conditionally on its expected revision; same identity, new content; then admit |
| `RetractAssertion` | Future explicit retraction contract, coordinating source state and gate tombstone; not file deletion |

Validation is optional for a fully specified request; it is useful when the client needs guidance.
`SubmitAssertion` always validates again. No mandatory prepare/confirm ceremony is added: “log this”
is a write instruction. Clarification is needed when the intended record cannot be determined, such
as an unknown medication course or an ambiguous date. A successful validation is not a promise that
the definition, source revision, or authorization cannot change before submission.

### Submission shape

```json
{
  "submission_id": "client-generated-stable-id",
  "type": "health.medication-missed-dose",
  "definition_version": 1,
  "assertion": {
    "text": "I missed yesterday's dose",
    "uttered_at": "2026-10-07T09:00:00+11:00",
    "zone": "Australia/Sydney"
  },
  "record": {
    "date": "2026-10-06",
    "course_ref": "previously-resolved-course"
  }
}
```

The example illustrates the proposed authoring contract. `course_ref` needs an agreed Definition;
it is not a claim about today's `health.medication-missed-dose` schema. Discovery must advertise exactly
what the installed version accepts.

The source writer assigns a stable row ID on the user's behalf and returns it. It retains the original
assertion and interpretation provenance in the source wrapper, separate from the facet payload.
The writer renders only the agreed record fields and identity into the captured record. All generated
identity must follow the chosen type's identity contract; an `/id` convention cannot override types
whose definitions use natural keys. Initial conversational authoring can restrict itself to supported
managed `/id` types and say so in discovery.

The authenticated actor is attached by the application. The model cannot mark itself as the user,
claim human endorsement, set classification, or supply anam-time. It may translate an explicit user
assertion into the declared type. Unsupported types fail with alternatives; a schema's optional
fields are not permission to invent a medication schema or discard uncertain meaning.

### Idempotency, concurrency, and recovery

Scope submission IDs to the configured person/source and authenticated caller. Store a digest of the
resolved write intent. Same ID and same intent returns the existing outcome; same ID and different
intent is `idempotency_conflict`. Rephrasing the same event under a fresh ID is not mechanically
identifiable as a duplicate: return candidate matches for the client to resolve, never merge by a
similarity score.

Source publication and ledger admission are separate commits. The durable states are:

```text
validated (ephemeral) -> source_saved -> admitted
                              |
                              +-> admission_failed (source retained, retry/reconcile)
```

Use complete-file atomic publication and appropriate durability before reporting `source_saved`;
temporary files are outside the scanner's accepted extensions. Persist submission identity and
recoverable intent with the source before admission. An operational journal may index progress, but
it cannot be the only home of the newly authored assertion. Submission/source revision history is
authored data requiring backup, not a disposable fold index.

After a crash, a reconciler scans durable submissions and compares gate/ledger outcomes. If admission
completed before the response was lost, return the existing result. If only the source was saved,
resume admission. Capture and retry use the same idempotent gate. Do not reapply an older revision
after a newer correction: serialize admission per source record and reconcile to its current revision.
The implementation must coordinate with scheduled records ingest, not merely other clients.

`CorrectAssertion` requires `source_record_ref`, `expected_revision`, replacement record, correction
reason, and a new submission ID. It returns a conflict if a human or another client changed the source.
Identity/type changes require a separate deliberate operation; do not disguise them as corrections.
The source writer must have enforceable concurrency semantics. For files writable outside the service,
atomic rename alone is not compare-and-swap; use a shared edit protocol or refuse managed corrections
while unmanaged writers can race. This is a prerequisite, not an implementation detail to gloss over.

Status results expose source persistence and admission separately, with per-record outcomes for any
future multi-record submission. The first API submits one record per call. “Saved, awaiting admission”
is truthful; “logged in your record” is reserved for confirmed admission. Cancellation leaves the
submission queryable. Durable pending work is resumed by a named application worker or operator
reconciliation command; a client process's memory is never the recovery mechanism.

### Retraction and derived findings

Removing a file does not retract a facet. Do not advertise `RetractAssertion` until the source can
represent withdrawal durably and the gate can apply it without a later scan resurrecting the record.
Retraction is distinct from correction and returns an anam-event position, never a claim of erasure.

Agent conclusions and document extraction are separate from user assertions. A future
`ProposeFinding` must carry method/model identity, evidence refs and record position, and enter as
`proposed`. `ReviewFinding` records a real reviewer decision. Neither is enabled until core proposed
admission and review semantics work. The current manually reviewed records workflow does not justify
an automated route that admits model guesses as user assertions.

## 7. Operating Anam

These are application operations, mostly exposed through an operator CLI or a separately enabled
operator profile of a client. A conversational client does not offer them by default.

Over the service API these are the `operator` role's (§2, Principals and roles). A `reader` or an
`importer` token reaches the reads in this table (status, checkpoints, receipts) and nothing that
runs the daemon.

| Concern | Operations | Required result/constraint |
|---|---|---|
| Health | `GetStatus`, `ListSources`, `GetSourceStatus` | Ledger/index positions, pending submissions, source availability and freshness with observation times |
| Capture | `ValidateImport`, `RunIngest` | Configured sources only; preflight read failures distinguished from partial committed batches; per-source tallies |
| Work tracking | `GetOperation`, `CancelOperation` | Progress, committed effects, retry guidance; cancel is best effort and never undo |
| Checkpoints | `ListCheckpoints`, `GetCheckpoint`, `PreviewMint`, `RunMint` | Bucket/version/hash/covers position; hard vs ephemeral soft; no automatic remint required for a fresh read |
| Derived recovery | `CheckDerivedState`, `RebuildDerivedState` | Correct ledger pin; replace only replayable tiers; preserve provisioned and authored data |
| Publication | `ListReceipts`, `GetReceipt`, `VerifyReceipt` | Stable artifacts, distinct integrity/reproduction results; paginated listing |
| Governance | `ValidateDefinition`, `InstallDefinition`, `ValidateProjection`, `InstallProjection` | Operator capability; immutable versions, historical availability, identity/time compatibility checks |
| Source writes | `ListPendingSubmissions`, `ReconcileSubmissions` | Recover saved assertions without creating duplicates or restoring obsolete revisions |

Initialization, configuration, backup/restore, and destructive maintenance remain explicit operator
workflows. They need safe application contracts if automated, but not generic shell or file access for clients.
Backups cover the ledger, authored operational state, records source, and published artifacts as
appropriate; rebuilding an index is not recovery of all of Anam.

Long operations return an `operation_id` promptly and persist progress where the operation requires
restart recovery. An ingest may have committed earlier batches before a later one fails. Results must
say which work landed. Validation does not promise run-wide atomicity. Authored operation records have
an explicit retention/backup policy; they cannot be called derived merely because they are operational.

## 8. Clients

Every client is a translator over the operations above. It decodes typed requests, carries caller
context and deadlines, and presents structured results. It holds no SQL, no second fold, and no
domain rule, and the record's correctness never depends on its process state.

| Client | Where | Contract |
|---|---|---|
| CLI | `anam` with `ANAM_SERVER` set | The daemon's `/v1` routes, rendering the same values as local reads |
| Inspector | the daemon's `/` pages | A diagnostic view, and the Anam Workbench's forms to enter, correct and retract a fact by hand (`/entry`, `/retract`, over the same writes as `POST /v1/entry` and the retract route; card 188); it yields to this design |
| Importers | the `anam-importers` repository | [IMPORTERS.md](IMPORTERS.md) §8 |
| MCP adapter | the `anam-mcp` repository | Its tool mapping, model-facing descriptions, resources and scenarios live there |

A client that needs an operation this document does not offer asks for it here. It does not
assemble one from smaller reads, and it reports an unsupported operation as unsupported.

## 9. Acceptance scenarios

1. **Discovery to evidence:** an agent finds the available BP projection, reads a month, follows one
   citation, and verifies its receipt. Payload, version, and position agree at every step.
2. **Stable conversation:** ingest occurs between pages and between related queries. Pages remain
   pinned; a second query using the same position excludes the newly arrived knowledge.
3. **Medication at a reading:** a start before the queried month, dose change, stop, and new course
   yield the hand-computed snapshot. A late stop is absent under an earlier `as_of`.
4. **Closing event and exclusion:** excluding a closing type from visible output never resurrects its
   series. Unauthorized history and supporting documents cannot be fetched through evidence links.
5. **Log and retry:** the connection drops after source publication, then after ledger admission.
   Retrying each with the same ID creates one source identity and one admitted version.
6. **Correct safely:** two clients correct the same revision. One succeeds; one conflicts. A scheduled
   scan cannot subsequently admit an older source revision over the correction.
7. **Time honesty:** “yesterday” across midnight and daylight-saving changes retains its original
   resolution. A type that cannot represent the known date precision refuses rather than inventing time.
8. **Missing evidence:** unavailable source, no matching facets, incomplete coverage, wrong ledger,
   missing template history, and expired/missing artifact produce distinct truthful results.
9. **Uncertain extraction:** a model-read lab value cannot use `SubmitAssertion` to impersonate a user
   statement. Unsupported proposed admission returns an explicit capability limitation.
10. **Recovery:** rebuilding derived indexes preserves authored submissions and historical receipts;
    restarting a client process loses no admitted fact or durable pending assertion.
11. **Service ownership:** every client and scheduled ingest works without database mounts.
    Concurrent ingest and conversational writes are bounded and recoverable. Restarting the service
    reconciles pending submissions; an outage never causes a client to open shared database files.

Exercise these against a real service over real temporary ledger, operational, source, and artifact
files. Core query tests compare checkpoint-derived answers to full folds. Each client owns its own
scenarios over these operations; the MCP adapter's are in its repository.

## 10. Delivery boundaries and remaining decisions

As inspected on 2026-10-07, Anam has records-file capture, bundled Definitions, whole-facet
projections, ledger folds/as-of reads, receipt publication/verification, and bucket minting. That is
substantial reuse, but not an existing application API with all contracts above.

| Stage | Deliverable | Prerequisites owned by the core and the service |
|---|---|---|
| 0 | Anam service in Docker on Unraid, SQLite on verified local persistent storage | Single store owner, versioned service API, authentication, bounded work scheduling, lifecycle and backup procedures; CLI/scheduler routed through the service |
| 1 | Discovery and timeline conversations with exact evidence | Callable core query/publication seam, ledger-bound receipts, durable query metadata, stable paging, sufficient authorized projections |
| 2 | Log supported user assertions and check their outcomes | Managed records writer, durable submission identity, precision rules, admission reconciliation and concurrency with scheduled ingest |
| 3 | Snapshot conversations and corrections | Series/closure semantics, snapshot projection and receipt support; source revision protocol for corrections |
| 4 | Richer provenance, changes, proposals, operator profile | Versioned source evidence resolver, governed change semantics, real proposed/review lifecycle, operational recovery APIs |

Stages describe dependency boundaries, not a requirement to postpone useful reads until every
operational capability exists. A client adopts each operation as the service implements it and
reports the rest as unsupported.

Decisions still needing agreement before their dependent implementation:

- Which projections provide useful default personal context, and which query parameters each allows.
- How date-only precision is carried in core metadata and how legacy zoneless sources are handled.
- The series tie rule and whether all change events carry enough state for latest-facet snapshots.
  A dose-change facet must carry the state the snapshot promises, or the core needs an explicit
  reducer; no client may fill missing regimen fields by an undocumented merge of payloads.
- The source writer's ownership/concurrency protocol, revision history, and scheduled-ingest integration.
- Receipt format/versioning for query parameters, ledger identity, exact evidence references, and
  historical interpretation; publication retention and limits for frequent conversational reads.
- How document/extraction provenance is linked durably without injecting document metadata into payloads.

## 11. Hickey × Armstrong — Before review

**Hickey, structure:** keep the query a value; keep knowledge position separate from fact-time selection;
keep assertion identity separate from transport retry identity; keep the source assertion separate from
its admission outcome. Definitions and templates remain versioned data. The strongest risk of accidental
complexity is rebuilding a domain model inside a client to compensate for a missing core operation. Refuse
that shortcut. The source writer is a substantive new responsibility and must be named as such.

**Armstrong, runtime:** the ledger/gate remains the error kernel. A client process can die at any
request boundary and be restarted by whatever launched it.
The Anam service is independently supervised by Docker and is the sole normal store owner; losing
it makes API operations unavailable until restart, while durable work remains recoverable.
Read failure produces an error or an unreferenced published value, never a fabricated partial answer.
Source writes recover through durable submission identity and reconciliation; operational indexes
recover through replay. Corruption and divergent identity stop the affected operation rather than
triggering blind retries. Every long-running operation has a deadline and a named recovery path.

**The tension:** durable conversational evidence creates artifact volume; bounded, ephemeral reads are
cheaper but harder to cite after the fact. This proposal chooses receipted reads for conversation and
an explicitly ephemeral operator preview. Similarly, source-first writes introduce a pending state,
but removing that state would hide the lack of a transaction across source and ledger. Make these costs
visible and measurable rather than hiding them in adapter process state.
