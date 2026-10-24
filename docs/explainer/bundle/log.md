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

## 2026-10-21, cards 203 and 189

- Code: `query.ReadsAttr` decides whether a query needs `attr/3` by walking the rules from the relation it answers, and `NewBase` builds the relation only then (card 203). `docs/QUERY.md` is the query reference with worked questions that `internal/query/docs_test.go` runs against a real ledger (card 189).
- Bundle: the gap `the-query-surface-is-decided-and-not-built` removed, false since card 173 was built; the `query` term says what is built and points at the decision and `docs/QUERY.md`.

## 2026-10-21, card 198 (the writers)

- Code: the fact names its zone. `time_semantics.fact_time` may carry a `zone_pointer`; `source_time` gains shape v2 with a required `offset`; an id template placeholder may end in `?`. `record.edge` v2 and `health.medication.dose` v2 require `source_time` and are read at its offset; `health.weight` v5 declares it, since records sends weight without it. The dose's template appends `{/source_time/offset?}`. `def.Guard` and `def.Register` hold `identity_template` too, and accept two widenings proved from the stored versions. The Workbench form sends the browser's zone and an offset field the page fills. History holds 74. `record.edge` and `health.medication.dose` leave the source-time list.
- Bundle: the def and gate component pages, the add-a-definition-version change path and the Definition manifest page say so.

## 2026-10-22, card 202

- Code: the Workbench's link form. `GET /link` draws one `record.edge` from a basket (`tick`, `fromside`, or the selection page's `program` resolved to ids when it opens, or `edit` of a Workbench link); `POST /link` saves it through the Workbench's entry, with the id `workbench:edge:` and 16 hex of the SHA-256 of the kind, the sorted sides and the moment with its offset, kept on every edit. The facet page offers "link this…" and "edit this link"; the selection page's "Use this selection" now opens the form and carries an edit through. The zone script is shared by both forms.
- Bundle: the inspector component has a section on the link form and its id rule, and the selection page's handoff names it.

## 2026-10-22, card 205

- Code: `health.heart-rate.day` v1 is bundled, one fact a day with `resting`, `minimum` and `maximum` as optional `{value, unit}` fields, keyed on the day and placed at the start of the interval Garmin counted, with `source_time` in card 197's shape. `health.heart-rate-day` stays bundled for the facts admitted under it. `projections/heart.v3.json` names the new type. The Garmin importer's poll names both types in its snapshot, so `anam retract garmin` retires the old facts; `TestRetract_ASnapshotThatNamesAnOldTypeRetiresItsFacts` proves it over a real ledger.
- Bundle: the placement decision says the hyphenated name left by a new type, not a rename.

## 2026-10-22, card 206

- Code: a selection has a short code. `selection.Store` files a program as an artifact of kind `selection`, addressed by the SHA-256 of its canonical text, under the code `sel:` and its first 8 hex digits, lengthened at save to the shortest unique prefix when another saved selection shares them, the lengthened code filed as a `selection-code` artifact. The select page saves (`POST /select/save`); `/select`, `/` and `/link` take `?sel=<code>`, the timeline drawing the selection alone or highlighted (`selmode`) within the window and `max_read_bytes`, and an unknown code is a 404 page. `POST /v1/selections` and `GET /v1/selections/{code}` for any role. anam-mcp has `save_selection` and `open_selection`.
- Bundle: the selection component has a section on saved selections and their codes; the inspector component has one on the save and on `?sel=` across its pages.

## 2026-10-22, card 200

- Code: `anam migrate` carries the live facets of one ledger into a fresh one. The old ledger and operational store are opened read-only; the new pair must not exist. The selection is every live facet at its head, folded by the one fold, or those a program's `pick/1` selects over the canonical. Each is admitted through the gate by `CarryBatch`, held to the Definition version it was admitted under (`Holding`, `def.Versions`), so the id, hash and fact time are the old ones; the registered Definitions and projections are copied first (`ops.ReadRegistered`, `CarryRegistered`). A dry run reports per type and source and writes nothing; a refusal anywhere stops a real run. A `record.edge` end the selection does not carry is listed. After `--yes` the command proves the new ledger's live facets equal the selection and that a dry run against it selects the same set. `catchup.EdgeEnds` is exported for the links. `long_write_timeout` also bounds each phase.
- Bundle: the workflow `migrate-to-a-fresh-ledger`, a `migrate` row on the CLI page, the gate and store-ops pages.

## 2026-10-22, card 200 (current version)

- Code: `anam migrate` carries a facet under the type's current Definition version when its payload passes it and the facet built has the same id, content hash, fact time, series, classification, descriptor and binding as the one read; otherwise under the version it was admitted under, as before; a facet neither carries is refused. The dry run prints MOVED and OWN per type and source and a line per group moved or kept; the proof reports the moves apart from the identity check. Matt's decision of 2026-10-22.
- Bundle: the migrate workflow and the IMPORTERS section 8.9 describe the rule.

## 2026-10-22, card 215

- Code: checkpoint values, receipts, served values and saved selections are rows of the operational store, not files under `artifact_dir`. `checkpoint_value` is derived and refuses an update; `receipt`, `served_value`, `saved_selection` and `saved_selection_code` are authored and immutable by trigger. The schema is 9, so a version-8 store is refused and carried forward by rebuilding it and running `anam move-artifacts --from <folder> [--registered-from <old tracking.db>] --yes`, which hashes every file against its name before it writes and loads the checkpoint values the log names. Matt's decision: a checkpoint is a cache that is allowed to go missing, and a missing one means fold past it and mint it again. `mint_run` gains `picks`, a digest of the checkpoints a read from the run takes, and `mint_seal` is gone; `RunCheckpoints` takes them in one transaction and says when the log no longer yields the ones the run saw, and then `ReadAt` and `ReadWindow` decline and the caller folds in full, as they do for a value that is not in the store. A mint writes back a value the log names and the store lost, and ends by deleting the checkpoint rows, sizes and values the newest run does not read (`CollectCheckpoints`). `internal/store/artifact` and the config key `artifact_dir` are gone; a backup is the ledger, the operational store and the library.
- Bundle: the artifact store component and the atomic-writes decision are replaced by the stored-content decision; the checkpoint value, checkpoint log, store-ops, config keys, mint and read workflows follow.

## 2026-10-22, card 216

- Code: a receipt can name its reader. `receipt.Receipt.Principal` is last and omitted when empty, so every receipt cut before it keeps its bytes and hash and still verifies (`TestPrincipal_OldReceiptKeepsItsBytesAndHash`). A principal in the importers file may set `receipts: always` (reader or importer only; validated at load). The daemon then files that token's `GET /v1/timeline` as `POST /v1/receipts` files a read, with status 200, naming the principal; `POST /v1/query` names it on the receipt it already files, and `dry_run` still files nothing. Such a token must name a projection and cannot use `explain` (400), opens only the receipts that name it (403), and its read fails if the receipt cannot be filed. `serve.Deps.Reader` carries the name; `serve.List` takes a `ListFilter`, `GET /v1/receipts` takes `principal`, `since`, `until` and `zone`, and `anam receipt list` takes `--principal`, `--since` and `--until` and shows a reader column.
- Bundle: the receipt, importers-file, principals, daemon and serve pages.

## 2026-10-22, the ledger's one index is named

- Docs: ARCHITECTURE §8 and §12 said the ledger carries no secondary indexes while `anam_event_by_facet` has existed since card 65. They now say no derived indexes, and name that one as a lookup on the ledger's own rows for the gate's fold of one facet (anam 9417fe2).
- Bundle: the invariant page is reworded and re-anchored; the gap page is closed.
## 2026-10-22, card 211

- Code: `POST /v1/query`'s `projection` is a name or an inline template object (`api.ProjectionRef`). `projection.ParseInline` is the manifest loader less a name, a version and a description, and the daemon also refuses a type with no Definition (`Template.CheckDefined`, shared with registration). An inline template is known by its content hash. `PublishQuery` files its body in the new authored table `inline_projection` before the receipt, and the receipt is marked `template_inline` with `template` `inline`, version 1, and the hash in `template_hash`. `serve.Verify` loads the body by that hash. The field is last and omitted when false, so earlier receipts keep their bytes (`TestInline_EarlierReceiptsKeepTheirBytes`). The schema stays 9: a version-9 store gains the empty table when it opens. `ReadRegistered` and `CarryRegistered` carry the bodies, and `move-artifacts` reports their count.
- Bundle: the query term, the receipt and store-ops pages, the stored-content decision and the reader gap.
## 2026-10-22, the operator admission hold

- Code: an operator can hold a source's admission at the daemon's door. `POST /v1/hold` (operator) takes `{"sources":[...]}` or `{"all":true}`, `DELETE /v1/hold` releases, and `GET /v1/status` reports the held sources in `held`. A held source's `POST /v1/envelopes` and `POST /v1/definitions` are refused with 503 and a `Retry-After`, and the body names the hold; the check is inside the write, so a hold and an admission are one order. Reads, the operator's own writes and `POST /v1/entry` (the workbench) are never held. The hold is authored state in the operational store (`admission_hold`, schema version 9), so it survives a restart of the daemon and is carried into a new store by `anam migrate`. `anam hold <source>...`, `anam hold --all`, `anam hold --release [<source>...]` and `anam hold` to list.
- Bundle: the daemon component's route table, the CLI component's command table, the store-ops component's table and the operational-store term.

## 2026-10-23, card 222

- Code: `mind` is the ninth domain head, decided by Matt on 2026-10-22. Three Definitions are bundled, `mind.attention` (a reading with a topic and an intensity from 1 to 5), `mind.attention.started` and `mind.attention.ended` (a declared pair that closes a series keyed by topic), each timed by a wall clock that names its own offset. A bundled `attention` projection serves them as a log, and the timeline draws them in an attention row.
- Bundle: the attention term is new, and the definition-manifest and where-a-distinction-is-encoded pages say nine domains and that a head is added only by a logged decision. Counts: 39 bundled Definitions, 33 terms.

## 2026-10-23, card 223

- Code: `internal/policy` holds every who-may rule as pure functions over the request and the importers file's principals: `Allow` (the route table's roles, handed in), `ScopesOf` and `MaySubmitType` (one rule for the gate's scopes, a snapshot's types and the Workbench's allow-list), `MayRegisterType`, `MayStoreDocument`, `Records`, `ReaderOf`, `TimelineRead`, `MayOpenReceipt`, `ListScope`, `Declared`, `MayRetract`, `MayRetire`, and `Held`, `Holdable` and `MayHold` for the admission hold, whose set the daemon reads inside the write and passes in on the request. The daemon, the inspector (`Workbench.Enters` replaces `Types`) and the CLI's retract and retire ask it; `retract.PlanFacet` takes the policy's answer for the source. Matt's decisions of 2026-10-22: a recording token lists only the receipts that name it (403 for another's) and verifies only those; dry_run stays open to every token with no rule. `TestPolicyReadsNoPayload` holds the package to `config` and `facet`.
- Bundle: the decision `who-may-is-one-static-policy-package` and the component `policy` added; the principals component and the route-roles invariant re-anchored on the policy.
## 2026-10-23, card 224

- Code: fixes from the 22 October audit. `ReadRegistered` and `CarryRegistered` carry the whole authored tier: receipts, served values, saved selections and selection codes join the registered rows, inline bodies and holds (`ops.Content`), each checked against its key, in the same transactions; `anam move-artifacts --registered-from` and `anam migrate` gain them, and the version refusal's remedy names them. The exact-decimal parser refuses an exponent past nine digits either way (`maxExponent`) before any arithmetic, where `1e9223372036854775807` panicked. A Put that finds the store locked past its busy timeout is `ops.ErrBusy`, and a read that files a receipt answers 503 with `Retry-After` for it. `checkpoint_value`'s comment no longer promises a delete guard; the schema pin moved, the version did not.
- Bundle: the store-ops component and the migrate workflow say what the carry takes.

## 2026-10-24, card 232

- Code: `health.intake` v1 is bundled, an event in a new `intake` family, Sensitive and strict, timed by a wall clock that names its own offset. `kind` is caffeine, alcohol or meal, and the schema's `if` and `then` per kind require a drink's `drink` and `count` or a meal's `text`, `rich` and `out` and refuse the other kind's fields. `part_of_day` makes `at` a nominal minute. A bundled `intake` projection serves it as a log up to Sensitive, the timeline gives the family a grey row, and docs/QUERY.md has a worked question, alcohol before a sleep episode.
- Bundle: the intake term is new, and the definition-manifest page says 40 bundled Definitions. Counts: 40 bundled Definitions, 34 terms.

## 2026-10-24, card 199 slice 3

- Code: a Definition may declare `handling.label`, a short template of literal text and `{/pointer}` fields (`def/label.go`); `Parse` checks it and `Definition.Label` renders it, never failing. The selection page's rows and basket and the link form's basket read as the label, falling back to the type and the time. `health.weight` v6, `health.medication.dose` v3, `mind.attention` v2, `health.intake` v2 and `record.edge` v3 declare one, and the earlier versions are in the history. The timeline's own selection gains a "link these…" anchor into `/select`, beside the facet page's "link this…".
- Bundle: the definition and inspector components say so.

## 2026-10-24, card 212

- Code: the bundled rule set is v2. It adds `in_force(E, F)`, and `within(A, B, Days)`, a builtin in a new table (`internal/query/builtin.go`) that the plan check holds to bound arguments and the evaluator answers in Go. Every earlier rule-set text is kept under `rules/history/`: version 1 was served as three different texts, since `anam.dl` changed on 2026-10-21 without a bump. Verify picks the text by the receipt's rule-set hash, and a hash no held text has answers 409, as a moved template does, where it had answered 500.
- Bundle: the query term says so.
