# Anam API — operating the record and conversing with it

Design proposal · 2026-10-07 · documentation only; these APIs are not implemented.

This document proposes the application API that the CLI, inspector, and MCP adapter can share.
The two conversational features are **chat with my record** and **log this**. The API supplies
record operations and evidence; the conversational client supplies language understanding and
dialogue. It must also support an operator keeping Anam current, inspecting failures, and recovering
derived state.

The governing contracts are [ARCHITECTURE.md](ARCHITECTURE.md),
[SELF-LOG.md](SELF-LOG.md), and the current records workflow in [INBOX.md](INBOX.md).
Where this proposal needs a new core capability, it names it rather than silently assigning domain
logic to the MCP adapter. Whole-facet projection is retained. Element masking is out of scope.

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
Conversation / CLI / inspector
              |
       MCP or other adapter
              |
       Application operations
        /         |          \
  record reads  source authoring  operator operations
       |           |                  |
 fold + projection records source -> capture -> gate -> ledger
       |
 published value + receipt
```

The application API is transport-independent, initially a Go API. Operation names below identify
contracts, not mandatory package names or HTTP routes. No HTTP service or separate public Go module
is needed to establish these contracts. Define small interfaces at consumers, grouped by concern;
do not build a single store/service interface containing every operation.

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
| `ListProjections` | Cursor | Authorized template IDs, versions, descriptions, supported read modes and parameters |
| `GetProjection` | Template ID/version | Immutable definition, type coverage, restrictions, examples, supported narrowing |
| `ListFacetTypes` | Optional projection, cursor | Available types, human descriptions, read/write eligibility, definition versions |
| `GetFacetDefinition` | Type/version | Identity and time contracts, classification, validation rules, payload schema, examples, series semantics if supported |
| `GetCoverage` | Projection, optional window and pinned position | Observed counts and date ranges by type/source, freshness observations, gaps only where justified |

Definition discovery is important because payloads vary. An agent needs to know that a lab result
may carry a unit and reference range, or that a medication change needs a course reference. Today’s
lenient required-pointer definitions are not a complete authoring interface. Authorable types need
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
and supplies permitted parameters; it never sends a new mask or changes classification ceilings.

```json
{
  "projection": {"id": "meds", "version": 1},
  "selection": {
    "mode": "snapshot",
    "at": {"date": "2026-06-15", "zone": "Australia/Sydney"}
  },
  "knowledge": {"position": {"ledger_id": "ledger-example", "watermark": 11381}},
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
client cannot broaden a template through a filter, supply arbitrary payload predicates, or request
selected payload fields. Type descriptions and projection descriptions support discovery by topic;
full-text or semantic payload search is a separate future capability with its own evidence and
governance contract. Never describe a metadata query as having searched every word in the record.

### Selection order is a domain contract

1. Resolve the knowledge position and reconstruct the canonical at that position.
2. For a snapshot, resolve series over the necessary fact-time prefix, including closing events.
3. Apply the authorized projection and permitted narrowing to the resulting whole facets.
4. Publish the selected value and receipt; deliver bounded pages of that value.

A filter that removes `med.stopped` before series resolution could resurrect a stopped course.
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
Publication is reported as a side effect; do not label it universally read-only in MCP annotations.
An operator-only `PreviewRecord` can derive an ephemeral result without a receipt, clearly labelled
as a preview. It shares the same selection implementation.

Every first-page response contains:

- `record_position`, resolved query and its digest, template and definition-set versions;
- `receipt_ref`, `served_hash`, `served_at`, `total_count`, `returned_count`, and `next_cursor`;
- whole facet entries with exact `facet_ref`, fact-time/precision, folded status, classification,
  source identity, content hash, definition version, payload kind, media type, and verbatim payload;
- evidence links available to this caller and explicit limitations such as legacy timezone handling;
- an unambiguous empty-result description when `total_count` is zero.

Citation metadata is a companion envelope. It must not change canonical bytes merely to add an
artifact version, URI, or page number. The publication contract must pin the correspondence between
canonical entries and exact evidence references.

`ReadPage(cursor)` reads the immutable published result, never head again. The opaque cursor binds
the receipt, query, stable ordering, and next offset; it is tamper-evident or validated against stored
state. Recheck authorization on every page. Invalid or missing artifacts produce explicit errors,
not a silent new query. The page is a portion of the receipted value: the receipt hashes the complete
published value, not the MCP response wrapper or that page alone.

Receipts must grow to pin ledger identity, the resolved query (including snapshot date and zone),
selection-semantics version, exact template version, and definition provenance needed for historical
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

### The source is durable before admission

A user's assertion is source material. The application writes it into the configured records source,
then capture submits an Envelope through the ordinary gate. No MCP operation inserts a facet directly.
This is a source-authoring capability alongside Anam, not permission for capture workers to write the
ledger or for the ledger to become the only home of the assertion.

Use a managed records file per submission (or per deliberately grouped assertion), within the
declared records directory. Its row IDs remain stable through corrections. An external human-edited
file is not automatically writable through MCP. Source targets are configured identities, never
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
  "type": "med.missed-dose",
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
it is not a claim about today's lenient `med.missed-dose` schema. Discovery must advertise exactly
what the installed version accepts.

The source writer assigns a stable row ID on the user's behalf and returns it. It retains the original
assertion and interpretation provenance in the source wrapper, separate from the facet payload.
The writer renders only the agreed record fields and identity into the captured record. All generated
identity must follow the chosen type's identity contract; an `/id` convention cannot override types
whose definitions use natural keys. Initial conversational authoring can restrict itself to supported
managed `/id` types and say so in discovery.

The authenticated actor is attached by the application. The model cannot mark itself as the user,
claim human endorsement, set classification, or supply anam-time. It may translate an explicit user
assertion into the declared type. Unsupported types fail with alternatives; lenient validation is
not permission to invent a medication schema or discard uncertain meaning.

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
The implementation must coordinate with scheduled records ingest, not merely other MCP clients.

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
reconciliation command; an MCP process's memory is never the recovery mechanism.

### Retraction and derived findings

Removing a file does not retract a facet. Do not advertise `RetractAssertion` until the source can
represent withdrawal durably and the gate can apply it without a later scan resurrecting the record.
Retraction is distinct from correction and returns an anam-event position, never a claim of erasure.

Agent conclusions and document extraction are separate from user assertions. A future
`ProposeFinding` must carry method/model identity, evidence refs and record position, and enter as
`proposed`. `ReviewFinding` records a real reviewer decision. Neither is enabled until core proposed
admission and review semantics work. The current manually reviewed records workflow does not justify
an automated MCP route that admits model guesses as user assertions.

## 7. Operating Anam

These are application operations, mostly exposed through an operator CLI or a separately enabled
MCP profile. The everyday conversation tool list should stay small.

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
workflows. They need safe application contracts if automated, but not generic MCP shell/file tools.
Backups cover the ledger, authored operational state, records source, and published artifacts as
appropriate; rebuilding an index is not recovery of all of Anam.

Long operations return an `operation_id` promptly and persist progress where the operation requires
restart recovery. An ingest may have committed earlier batches before a later one fails. Results must
say which work landed. Validation does not promise run-wide atomicity. Authored operation records have
an explicit retention/backup policy; they cannot be called derived merely because they are operational.

## 8. MCP mapping

Recommended initial conversational tools:

| MCP tool | Application operation | Model-facing description emphasis |
|---|---|---|
| `describe_record` | Discovery overview | Learn coverage, capabilities, projections, and limitations before choosing evidence |
| `describe_type` | `GetFacetDefinition` | Learn the exact record shape and authoring requirements |
| `read_record` | `ReadRecord` | Timeline versus snapshot; fact-time versus knowledge time; publishes evidence |
| `read_page` | `ReadPage` | Continue the same immutable result |
| `get_evidence` | `GetEvidence` | Inspect an exact cited facet and available provenance |
| `validate_assertion` | `ValidateAssertion` | Resolve missing/invalid fields without writing |
| `log_assertion` | `SubmitAssertion` | Record an explicit user assertion; retry with the same submission ID |
| `get_submission` | `GetSubmission` | Determine what persisted after success, delay, or interruption |
| `correct_assertion` | `CorrectAssertion` | Correct an identified managed record using its expected revision |

Discovery returns bounded summaries and references for further catalog pages. Add projection catalog,
changes, explanation, and receipt verification tools as the conversation scenarios require them.
Only advertise implemented operations and modes; unsupported snapshot reads must not silently become
timeline reads. Keep operator tools separately enabled by configuration and authorization.

Resources can identify immutable receipts and their evidence, for example
`anam://receipts/{hash}` and `anam://receipts/{hash}/facets/{facet_id}/{version}`. The receipt binds
ledger identity. Reading a resource enforces the same authorization and size constraints as tools;
URI knowledge grants no extra access. Tools remain sufficient for clients that do not browse resources.

Optional prompts can teach “compare two periods” and “record an event,” but correctness belongs in
operation contracts. No `chat(message)` endpoint is needed: it would move model choice, dialogue state,
and interpretation into Anam. No `log(text)` endpoint that silently guesses type/time is needed either.

The official [Go MCP SDK](https://github.com/modelcontextprotocol/go-sdk) supplies typed tool
registration, structured results, transports, and in-memory client/server testing. Keep SDK types in
the adapter. Start with local stdio, protocol traffic alone on stdout and diagnostics on stderr.
Remote transport and authentication follow a concrete client/deployment requirement. Tool annotations
describe effects accurately but do not enforce permissions. See the [SDK research note](../journal/notes/mcp-go-sdk.md).

Treat facet payloads and source documents as evidence, including any text that looks like instructions.
They cannot authorize tool use or change the user's request. Carry provenance and evidence kind so
the client can distinguish a user assertion, a device observation, an extraction, and a proposed finding.

## 9. Acceptance scenarios

1. **Discovery to evidence:** an agent finds the available BP projection, reads a month, follows one
   citation, and verifies its receipt. Payload, version, and position agree at every step.
2. **Stable conversation:** ingest occurs between pages and between related queries. Pages remain
   pinned; a second query using the same position excludes the newly arrived knowledge.
3. **Medication at a reading:** a start before the queried month, dose change, stop, and new course
   yield the hand-computed snapshot. A late stop is absent under an earlier `as_of`.
4. **Closing event and mask:** excluding a closing type from visible output never resurrects its
   series. Unauthorized history and supporting documents cannot be fetched through evidence links.
5. **Log and retry:** the connection drops after source publication, then after ledger admission.
   Retrying each with the same ID creates one source identity and one admitted version.
6. **Correct safely:** two clients correct the same revision. One succeeds; one conflicts. A scheduled
   scan cannot subsequently admit an older source revision over the correction.
7. **Time honesty:** “yesterday” across midnight and daylight-saving changes retains its original
   resolution. A type that cannot represent the known date precision refuses rather than inventing time.
8. **Missing evidence:** unavailable source, no matching facets, incomplete coverage, wrong ledger,
   missing template history, and expired/missing artifact produce distinct truthful results.
9. **Uncertain extraction:** a model-read lab value cannot use `log_assertion` to impersonate a user
   statement. Unsupported proposed admission returns an explicit capability limitation.
10. **Recovery:** rebuilding derived indexes preserves authored submissions and historical receipts;
    restarting the MCP process loses no admitted fact or durable pending assertion.

Exercise these through a real SDK client/server pair and real temporary ledger, operational, source,
and artifact files. Core query tests compare checkpoint-derived answers to full folds. Adapter tests
verify contracts, cancellation, discovery, and effects rather than reproducing domain algorithms.

## 10. Delivery boundaries and remaining decisions

As inspected on 2026-10-07, Anam has records-file capture, bundled Definitions, whole-facet
projections, ledger folds/as-of reads, receipt publication/verification, and bucket minting. That is
substantial reuse, but not an existing application API with all contracts above.

| Stage | Deliverable | Prerequisites owned outside the MCP adapter |
|---|---|---|
| 1 | Discovery and timeline conversations with exact evidence | Callable core query/publication seam, ledger-bound receipts, durable query metadata, stable paging, sufficient authorized projections |
| 2 | Log supported user assertions and check their outcomes | Managed records writer, durable submission identity, precision rules, admission reconciliation and concurrency with scheduled ingest |
| 3 | Snapshot conversations and corrections | Series/closure semantics, snapshot projection and receipt support; source revision protocol for corrections |
| 4 | Richer provenance, changes, proposals, operator profile | Versioned source evidence resolver, governed change semantics, real proposed/review lifecycle, operational recovery APIs |

Stages describe dependency boundaries, not a requirement to postpone useful reads until every
operational capability exists. The MCP adapter can be developed against narrow contract fixtures while
core agents implement the application seams. It must report unsupported features honestly.

Decisions still needing agreement before their dependent implementation:

- Which projections provide useful default personal context, and which query parameters each allows.
- How date-only precision is carried in core metadata and how legacy zoneless sources are handled.
- The series tie rule and whether all change events carry enough state for latest-facet snapshots.
  A dose-change facet must carry the state the snapshot promises, or the core needs an explicit
  reducer; MCP must not fill missing regimen fields by an undocumented merge of payloads.
- The source writer's ownership/concurrency protocol, revision history, and scheduled-ingest integration.
- Receipt format/versioning for query parameters, ledger identity, exact evidence references, and
  historical interpretation; publication retention and limits for frequent conversational reads.
- How document/extraction provenance is linked durably without injecting document metadata into payloads.

## 11. Hickey × Armstrong — Before review

**Hickey, structure:** keep the query a value; keep knowledge position separate from fact-time selection;
keep assertion identity separate from transport retry identity; keep the source assertion separate from
its admission outcome. Definitions and templates remain versioned data. The strongest risk of accidental
complexity is rebuilding a domain model inside MCP to compensate for a missing core operation. Refuse
that shortcut. The source writer is a substantive new responsibility and must be named as such.

**Armstrong, runtime:** the ledger/gate remains the error kernel. The MCP process can die at any request
boundary and be restarted by its local client (or by a service supervisor if deployed remotely).
Read failure produces an error or an unreferenced published value, never a fabricated partial answer.
Source writes recover through durable submission identity and reconciliation; operational indexes
recover through replay. Corruption and divergent identity stop the affected operation rather than
triggering blind retries. Every long-running operation has a deadline and a named recovery path.

**The tension:** durable conversational evidence creates artifact volume; bounded, ephemeral reads are
cheaper but harder to cite after the fact. This proposal chooses receipted reads for conversation and
an explicitly ephemeral operator preview. Similarly, source-first writes introduce a pending state,
but removing that state would hide the lack of a transaction across source and ledger. Make these costs
visible and measurable rather than hiding them in adapter process state.
