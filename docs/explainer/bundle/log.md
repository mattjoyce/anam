# Log

## 2026-10-11

First build of the bundle, at anam commit 38aa3dad70a7d1eda90193675073ffe75b602073 (master) and anam-importers commit 02a4907a980ff186bc21c85bdea966bd714c3938 (branch card-58-one-off).

- Code: nothing changed; this is the baseline. The repo at this commit has the daemon owning the ledger on unraid, importers for Withings, blood pressure, DreamStation and records in the sibling repo, retraction by complete resend and by retirement, composed reads from the last mint run, and bounded reads over the daemon.
- Bundle: one purpose, 29 terms, 29 components, 6 workflows, 17 data structures, 21 decisions, 27 invariants, 5 change paths, 16 gaps.

## 2026-10-13

Card 50 closes the importers move, at anam master after 3b97178.

- Code: `internal/capture/healthdb` deleted, with its `scanImport` case, scope entry and example import; health-db itself had left the live imports and its facets were retired on 2026-10-11 (card 55). `records` and `documents` remain in process until card 119.
- Bundle: the component `capture-healthdb` and the gap `healthdb-worker-still-built-in` removed, and the four pages that depended on the component no longer list it. `docs/ARCHITECTURE.md` §2 now draws importers, not capture workers.

## 2026-10-13, card 21

- Code: the inspector's timeline has a checkpoint strip and a header line counting sealed, behind and never-minted buckets; `inspect.Handler` takes a `Checkpoints` log.
- Bundle: the inspector component names the strip and depends on mint and the ops store.

## 2026-10-13, card 125

- Code: projection templates are bundled or registered. `projection.Registry` holds every version by name; `Register` and `WithRegistered` mirror the Definitions' rules; the daemon keeps a `registered_projection` table (authored tier, immutable) and serves `GET`/`POST /v1/projections`; receipts carry `template_hash` and verify re-prunes through the exact version named; `anam projection list|show|register`.
- Bundle: the projection component and the daemon's route table say so. The term and invariant pages still read "a consumer selects, never defines", which stays true.

## 2026-10-13, cards 105, 106, 136

- Code: the gate refuses a facet changing type (`ErrTypeChanged`); library documents are sandboxed and active kinds are downloads; `record.note` v2 times a note by its writing.
- Bundle: the inspector component notes the sandbox.

## 2026-10-13, card 108

- Code: `browserGuard` refuses cross-site unsafe requests before the token; the daemon's help and the service skill say so.
- Bundle: the daemon component notes the guard.

## 2026-10-13, cards 100 and 111

- Code: the day audit's low findings cleared (inspect.go split into facet, document and events files; read.Absent bounded over the API); the Definition history is provisioned from `manifests/history`.
- Bundle: the def component names the history; the inspector component's anchor is unchanged.


## 2026-10-14, card 119

- Code: `internal/capture` deleted; `records` and `documents` reach Anam only through `anam_records` and `anam_documents`, which keep those source names. `anam ingest`, `POST /v1/ingest`, `GET /v1/operations/{id}`, the imports file and its example, the library's auto-import, `mvpScopes` and the `ingest` field of `GET /v1/status` are gone. `check.sh` has no `internal/capture` carve-out. The daemon runs `guardBundled` at `Open`; its ingest file became `minting.go` (`mintAtHead`, `mintOnRequest`, `mintIfBehind`, `catchUp`, the `sources` view behind `last_scan_at`). `anam retract` plans only from a snapshot or `--retired`, with a gate of empty scopes. `internal/app/ingest.go` became `init.go`.
- Bundle: the components `capture-records` and `capture-documents` removed; the component `importer-documents` and the workflow `hand-anam-a-document` added, which also puts `POST /v1/documents` (card 102) in the daemon's route table. The ingress and substrate invariants, the mint cadence decision and the mint-failure, token_scope and API-doc gaps say what the code does now; pages that named the built-in workers say so as history.

## 2026-10-14, card 150

- Code: `POST /v1/documents?ext=` replaces `?path=`; Anam names the stored file `<sha256>.<ext>`, so there is no 409 and nothing is overwritten. A record.document's `path` is its name and identity and its `sha256` finds the bytes; `checkReferences` refuses a sha256 the library does not hold. The new package `internal/library` (`Store`, `Find`, `Index`, `IsSHA256`) places a file by `library_folder_limit` and finds it by a walk; `storeDocument` became `library.Store`. The inspector serves `/document?sha256=&name=`. On anam-importers branch `card-150-sha256-library`, `Client.store_document(name, data)` posts `?ext=`.
- Bundle: the component `library` added; the workflow `hand-anam-a-document`, the daemon's route table, the inspector and `importer-documents` say what the code does now.

## 2026-10-14, card 110

- Code: queue wait counts toward the caller's deadline. `tryWriteWithin` takes the caller's context and the request's budget (the shorter of `timeout` and `Anam-Timeout`) as an end time counted from arrival; the wait to be accepted is `min(wait, budget)`; `run` re-checks the caller and the budget as the worker takes a job, so a write whose caller has gone or whose budget is spent never starts; once accepted a write runs to its own end and is reported as it happened. `Write` keeps a deadline that starts at execution, for backups and the mint.
- Bundle: the invariant `queue-wait-spends-the-callers-budget` added; the decision `one-write-queue-in-the-daemon`, the invariant `external-calls-fail-fast` and the daemon, admit-a-batch and mint-checkpoints pages say so, and their stale queue anchors point at `run` and `tryWriteWithin`.

## 2026-10-14, card 151

- Code: `config.Limits` squashed into `Config`: 21 bounds and waits (HTTP timeouts, `accept_wait`, `importer_wait`, `retry_after`, `mint_quiet`, payload, manifest, document and response bounds, snapshot retention and limits, `network_margin`, `inspect_cached_reads`), each with a `Default…` constant and a range check at load and at `daemon.Open`. `gate.New` takes `gate.Limits`, `client.New` takes the margin and the response bound, `inspect.Serve` takes its `Deps`. `timeout`, `ingest_timeout` and `backup_keep` became `Default…` constants. What stayed a constant (`gate.MaxBatch`, `Anam-Timeout`, the snapshot id's shape, `clock.Width`, the token floor) says why beside it. `config.example.yaml` lists every key. AGENTS.md gained "Defaults and invariants".
- Bundle: the data structure `limits` and the decision `a-hard-coded-value-is-a-default-or-an-invariant` added; the gate, client, config, inspector, config-keys, admission-response, batch-submission, envelope, snapshot-part and resend pages name the keys; the gap `config-example-is-stale` narrowed to the two comments that remain.

## 2026-10-15, card 157

- Code: `record.note` v3 makes `about` optional, so a note may name no facet yet; v1 and v2 now ship in `manifests/history` (v2 had been bumped without copying v1). Card 93's decision is recorded in docs/API.md §6: a conversation is an importer and its words a note, a model's reading a second source of drafts.
- Bundle: the decision `a-persons-words-are-a-note-and-a-models-reading-is-a-draft` and the gap `the-interpretation-source-is-decided-and-not-built` added (only the note half is built); the Definition manifest and def pages say so; the API-doc gap points at API.md §2 and §6.

## 2026-10-15, card 89

- Code: tokens carry roles. Every caller is a `config.Principal` (name, role, token hash; an importer also its source and types); roles are `reader`, `importer` and `operator`; `token_file` is the operator, held as a hash. One route table in `api.go` names the roles each route admits and `admit` answers 401 for an unknown token and 403, naming the principal, for a role the route does not admit. `asImporter`, `asReader`, `authorize` and `internal/config/importers.go` are gone; the importers file keeps `importers` and gains `principals`. An importer now reads as a reader does. `internal/app/retire.go` and the anam-service skill follow.
- Bundle: the component `principals`, the invariant `each-route-names-the-roles-it-admits`, the decision `tokens-carry-roles`, the terms `principal` and `role` and the gap `reader-is-not-narrowed-to-projections` added; the daemon's route table now has a roles column; the importers file, the importer-token decision, the ingress and type-in-one-place invariants and the admit-a-batch and register-a-definition steps read from `auth.go` and `principals.go`. The ingress invariant no longer says an importer cannot read the record.

## 2026-10-15, card 141

- Code: docs only. docs/IMPORTERS.md §8.8 is the rebuild runbook, with `scripts/rebuild-drill.py` in anam-importers as the drill. The first drill, 2026-10-14, matched 2,939 facets in 18 types against the live record; the live rebuild has not been run.
- Bundle: the workflow `rebuild-the-record` and the gap `the-drill-is-the-only-rebuild-so-far` added.

## 2026-10-17, a sweep to master

Anam at 154f190, after the README, NOTICE and lineage edits (cards 142 and the history move to docs/HISTORY.md).

- Code: nothing new beyond the cards above. The README and ARCHITECTURE no longer name where the idea came from; ARCHITECTURE's "one inversion" heading is "The whole-person read".
- Bundle: 19 stale anchors, symbols and paths re-pointed (client and gate constructors, the importers file, the importer-token, mint-cadence and write-queue anchors, the ingress and deadline invariants, the type-in-one-place invariant, the admit-a-batch, hand-anam-a-document and mint-checkpoints steps). The two lineage sentences in `importers-shape-only` and `importers-live-outside-the-binary` rewritten without it. The gap `importers-checkout-behind-origin` restated at 3451d3a, 34 commits behind origin/master. The purpose page's commit is 154f190. Counts: 1 purpose, 31 terms, 28 components, 9 workflows, 18 data structures, 24 decisions, 29 invariants, 5 change paths, 18 gaps.

## 2026-10-19, card 173

- Code: docs only. ARCHITECTURE §8 no longer says fields "do not select by payload"; the store never selects by payload, a read may, over the served copy, through a projection. §9 gains the query as the third read, §12's prune invariant says a query generates only over what a projection served, §13 lists the surface as decided and not built. GLOSSARY, API.md, PRFAQ and SELF-LOG brought into line.
- Bundle: the decision `a-read-may-select-by-payload-the-store-never-does`, the term `query` and the gap `the-query-surface-is-decided-and-not-built` added; `projections-prune-never-reshape`, `metadata-payload-separation` and the `projection` term amended so opacity reads as a storage rule. Counts: 1 purpose, 32 terms, 28 components, 9 workflows, 18 data structures, 25 decisions, 29 invariants, 5 change paths, 19 gaps.

## 2026-10-20, cards 139, 140, 147, 165, 168, 176, 179

Anam at c08bfb5, the merge of the taxonomy (card 147). The purpose page's commit is c08bfb5.

- Code: every type is named `domain.concept` under eight closed domains, and the 24 renamed bundled types start again at v1 (card 147). `anam rename` carries a record from the old names to the new as new versions of the same facets (card 140). A Definition may declare `kind` and `family` (card 176). The OMH types `sleep.episode`, `health.heart-rate-daily` and `health.step-count` are bundled, so an importer may carry the same body (card 139 piece 1). `record.link` is registered by the links importer and not bundled (card 165), `location.stay` by the stays importer (card 168), and `health.medication-dose` is bundled with a `doses` projection (card 179).
- Bundle: the Definition manifest page states the eight domains, the counts now true (32 bundled, 43 in history) and which types importers carry. `record.note` is v1, so the pages that said v3 say `note.annotation` v3 or name no version. `health.blood-pressure` is v6 in the bp_csv, admit-a-batch and index pages, `health.measurement` v4 in the importer base. The gap `importers-checkout-behind-origin` is closed, since the importers checkout is on master. Counts: 1 purpose, 32 terms, 28 components, 10 workflows, 18 data structures, 25 decisions, 29 invariants, 5 change paths, 18 gaps.

## 2026-10-21, cards 104 and 91

- Code: `record.note` v2 is an edge facet: `kind` from a closed list (`about`, `interpreted_from`), `from` and `to` as lists of facet ids, `about` kept as the v1 name of `to`, `pins` for an exact version; every v1 value is a v2 value unchanged. The operational store gains `facet_edge` (schema version 8, so a pre-edge file refuses and is replayed), written by catch-up from each live edge's payload. The bundled rule set gains `edge_to/2`, `edge_from/2` and `linked/2`. `GET /v1/receipts/{hash}/facets/{facet_id}` answers one facet as the receipt served it beside its links at the index's watermark.
- Bundle: the edge index row page is new; the store, catch-up, daemon, query and decision pages say so; the Definition manifest counts move.

## 2026-10-21, card 92

- Code: a published read over `max_read_bytes` is filed once and answered a page at a time: `POST /v1/receipts` answers the first page with `page` and a `next_cursor`, and `GET /v1/receipts/{hash}?cursor=` reads each later page from the served artifact. The cursor is checked against the receipt (400 `cursor_invalid`). 413 stays for one facet over the bound alone. The timeline, explain, publish and verify narrow through one predicate, `receipt.Query.Admits`; `bounds.admits` is gone.
- Bundle: the decision "published reads page by a checked cursor" is new; the bounded-reads decision, the read component and the daemon route table say so.

## 2026-10-21, card 191

- Code: `record.note` is renamed `record.edge`, the edge with commentary card 104 made it. `record.edge` v1 is `record.note` v2 under the new name, which joins its v1 in `manifests/history`; `renames/record-edge.json` is the table `anam rename` runs; the edge rules, `catchup.EdgeType`, the inspector's note draft and `recent-life` v3 name the new type.
- Bundle: the edge index row, decision and gap pages say `record.edge`; the Definition manifest page counts 46 in history.
- Bundle: every page that tells the type's history gives its lineage in one clause, `note.annotation` (v1 to v3), then `record.note` (v1, v2), now `record.edge`; the def component page now says it was `note.annotation` v2 that card 157 found bumped without its v1.

## 2026-10-21, card 192

- Code: none. A type is `domain.concept[.specialisation…]`: a third or later segment is allowed for a specialisation, and the old "two segments" sentence from card 147 is dropped, since nobody reasoned it and `facet.typePattern` never enforced it. The closed set of eight domains stands.
- Bundle: the decision "a distinction goes in the cheapest place that lets its reader ask cleanly" is new, with the table of six places and their cost to change; the Definition manifest page says the shape and points to it. AGENTS.md, ARCHITECTURE, GLOSSARY and README say the same.

## 2026-10-21, card 193

- Code: a projection template's `types` entry, and the timeline's `type` parameter, may be a prefix ending in `.`; `facet.Matches` is the one predicate for the projection, the timeline and a receipt's replay.
- Bundle: the projection component and the projection template page say so.

## 2026-10-21, card 194

- Code: a rename table's row may carry `select` (`source`, a `pointer` and `prefix`, or both), so one table splits a type into leaves; every live facet of a split type must match exactly one row, or the dry run and the run refuse before writing. `internal/def/shared-core.json` and its test hold a split's shared fields identical across its leaves.
- Bundle: the rename workflow gains the placing step, the loader's select rules and the report's SELECT column.

## 2026-10-21, card 196

- Code: twelve types take a third segment, one to one: `health.medication.started`, `.stopped`, `.dose`, `.dose-changed`, `.dose-missed` (was `-missed-dose`) and `.compliance`; `health.illness.started` and `.ended`; `place.trip.departed`, `.arrived`, `.returned` and `.flight`. Each starts at v1 and the old name's versions join `manifests/history`; the series are unchanged. `renames/kinds-segment.json` is the table. The bundled `meds` is v3 and `doses` v2, and `recent-life` v4 is registered from `projections/`. The projection loader accepts more than one history version of a template.
- Bundle: the series and projection-template pages and the encoding decision name the new types.

## 2026-10-21, card 195

- Code: vendor-shaped types take the vendor leaf (card 192). `activity.exercise-session` (v2, 58 fields) splits into four bundled leaves at v1, `.garmin` (21 fields), `.concept2` (28), `.technogym` (29) and `.strong` (10), each the shared core (`/id`, `/activity_name`, `/effective_time_frame`, `/source`, named in `internal/def/shared-core.json`) and the fields its vendor writes; the old name's v1 and v2 stay in `manifests/history`. The importers rename `activity.day`, `health.measurement` and `health.cpap-session` to `activity.day.garmin`, `health.measurement.withings` and `health.cpap-session.dreamstation`. `renames/vendor-leaves.json` is the table: four rows select on `/id`'s prefix, three are plain. A rename's keying check compares the identity key only: `activity.day.garmin` reads its fact time at the start of the day Garmin counted, and the dry run names the rows whose rule moves facets. `recent-life` v5 names the prefix `activity.exercise-session.`.
- Bundle: the rename workflow, the def component, the Definition manifest page (35 bundled, 64 in history), the register workflow, the one-place invariant, the Open mHealth and encoding decisions and the importer pages name the leaves.

## 2026-10-21, card 201

- Code: the inspector's projection page opens on a per-type summary (served, excluded, superseded) and draws rows only on request, `?type=&show=` with `from`, `to` and `asof`, at most 500. The superseded count is derived from the fold's states and reads no version; `fold.DisplacedWhere` reads the versions of one type's facets for the rows view.
- Bundle: the inspector component gains a section on the projection page.

## 2026-10-21, card 199

- Code: `internal/selection` is the shared filter, fact-time from and to, type or prefix, source, standing, and ticks and exclusions by facet id. It compiles to a Datalog program defining `pick/1` (a tick is `pick("<id>")`, an exclusion `unpick("<id>")`, a prefix two lexical bounds, no pointer named) and parses back only when the program is one it writes. A real-ledger test holds the program's answer equal to the filter's own predicate. The inspector's `GET /select` builds a selection with it: criteria in any order with their counts, matches by type with readings of one moment folded into a row, a basket that travels in the address with each item's side, the program live in a textarea, and a program outside the grammar run as text with the criteria off. `read.Evaluate` is the query read's evaluation without its receipt, and the page runs through it within `max_read_bytes`. The projection page's rows view selects by the same filter.
- Bundle: a selection component page; the inspector component gains a section on the selection page; the read component names `Evaluate`.

## 2026-10-21, card 198

- Code: the importers say how their clocks were read. Seven bundled types move a version, identity and fact time unchanged: `activity.exercise-session.concept2`, `activity.exercise-session.strong` and `record.document` v2 require `source_time`; `health.blood-pressure` v7, `activity.exercise-session.technogym`, `health.lab-result` and `health.lab-panel` v2 declare it. History holds 71. `record.document` leaves the source-time list.
- Bundle: the importer base says `load` sends every version oldest first; the bp_csv page and index name v7; the Definition manifest page counts 71 in history.
