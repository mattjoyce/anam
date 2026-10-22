# Policy touch points in Anam, surveyed for card 223

Written 2026-10-22 from master at 17fb086, read only. Every row was read in context, not guessed.

"Policy" here is a decision of the form "may this caller do this to this resource", or "what must
happen when it does", where the answer turns on who the caller is (name, role, source, a flag on
the principal) or on an operator-set rule. A check that turns on the data's own validity is
validation and stays where it is.

## The table

| # | file:line | Decision | Depends on | Rule's data today | Kind | Policy function |
|---|---|---|---|---|---|---|
| 1 | internal/daemon/auth.go:70 | A token names a principal; without a token_file, a loopback request with no known token is the operator. | token hash, tokenless mode | importers file, token_file | identity, feeds policy | none; `who` stays, its result is the request's principal |
| 2 | internal/daemon/auth.go:85 | A request passes when the principal's role is in the route's set: 401 for no principal, 403 for the wrong role. | role, route | route table (api.go:37, :55) mirrored in openapi.yaml `x-anam-roles` | policy | `Allow(req)` |
| 3 | internal/daemon/api.go:37, :55 | Reads and row-writing publishes are every role's; submitting is an importer's; minting, backups, templates, retract, entry are the operator's. | role, route | route table, hard-coded | policy | the table `Allow` reads |
| 4 | internal/daemon/api.go:87, internal/inspect/document.go:66 | The inspector, the Workbench pages and the document bytes they serve open to the operator alone. | role | route table | policy | `Allow` |
| 5 | internal/daemon/importers.go:18, snapshots.go:54 | A snapshot part may name only types the importer may submit: its types, or any with any_type. | source, Types, AnyType | importers file | policy | `MaySubmitType(p, t)` |
| 6 | internal/daemon/importers.go:45, internal/gate/commit.go:318 | An envelope is admitted only when its source may submit its type (the gate's authenticated pillar, built from the principal as a `TokenScopes`). | source, Types, AnyType | importers file | policy inside the gate | `MaySubmitType`, handed to the gate as its scopes |
| 7 | internal/daemon/importers.go:24, definitions.go:35 | An importer may register a Definition only for a type it lists; any_type owns none (403). | Types, AnyType | importers file | policy | `MayRegisterType(p, t)` |
| 8 | internal/daemon/importers.go:31, documents.go:41 | An importer may post document bytes only if it lists record.document (403); any_type does not reach the library. | Types | importers file + hard-coded `docType` | policy | `MayStoreDocument(p)`, the same `lists` test as 7 |
| 9 | internal/daemon/entry.go:33, :68 | Entry by hand is 404 without a workbench principal; an entry is admitted as the workbench and held to its allow-list by row 6. | workbench present, its Types | importers file | policy | `MaySubmitType(workbench, t)` |
| 10 | internal/inspect/workbench.go:98, :139; linkform.go:195 | The Workbench draws a form, or the link form, only for a type the workbench principal lists (403). | workbench Types | importers file via `hand.Types()` | policy | `MaySubmitType`, page side of 9 |
| 11 | internal/daemon/api.go:291, query.go:81 | A filed receipt names the principal only when its token records every read. | Receipts flag | importers file | policy (what must happen) | `Records(p)` |
| 12 | internal/daemon/api.go:334 | A recording token's timeline read is filed as a publish, must name a projection and may not ask explain (400). | Receipts flag, request fields | importers file + hard-coded | policy | `Records(p)` plus `MustPublish(p, req)` |
| 13 | internal/daemon/api.go:302 (used at :182, :204) | A recording token opens only receipts that name it (403); every other token opens any. | Receipts flag, the receipt's principal | importers file + receipt row | policy | `MayOpenReceipt(p, reader)` |
| 14 | internal/daemon/api.go:63, read/receipt.go:134 | Any token verifies any receipt; verify returns the receipt (facet ids, query text), not facet values, and makes no owner check. | none today | hard-coded | policy by omission | `MayOpenReceipt`, if verify is counted as opening |
| 15 | internal/daemon/api.go:151, read/receipt.go:82 | Any role lists every reader's receipts; `principal` is a request filter, not a scope. Card 216 open call (b). | none today | hard-coded | policy by omission | `ListScope(p)` |
| 16 | internal/serve/query.go:69, api/datalog.go:22 | A dry_run query files nothing, for a recording token too, so it is the one unrecorded facet read such a token has. Card 216 open call (a). | request field, Receipts flag | hard-coded | policy by omission | `MayDryRun(p)`, or a clause of `Records` |
| 17 | internal/daemon/snapshots.go:158 | An importer sees only its own source's snapshot report (404 for another's); a reader or the operator sees any. | role, source | hard-coded | policy | `ListScope(p)` or `MaySeeSnapshot(p, source)` |
| 18 | internal/daemon/retract.go:26, internal/retract/retract.go:77 | A hand retraction is refused for a source an importer declares once that source has completed a snapshot (409). | source declared in importers file, store state | importers file + ops store | mix | `MayRetract(p, source)` for the declared part; the snapshot part stays in retract |
| 19 | internal/retract/retract.go:75, :102 | The facet must belong to the named source and be live. | the facet's own provenance and status | ledger | validation | none |
| 20 | internal/app/retractfacet.go:86, app/retract.go:63 | The CLI applies row 18's declared-source rule itself, and `--yes` and `--max-fraction` bound a retirement. | importers file, flags | importers file + config default | mix | `MayRetract`, CLI side of 18 |
| 21 | internal/app/retire.go:55 | A source still declared in the importers file cannot be retired. | importers file | importers file | policy | `MayRetire(source)`, one rule with 18 |
| 22 | internal/daemon/projections.go:125 | Every template is listed as selectable by every role; no role-to-projection grant exists. | none today | hard-coded | policy by omission | `MaySelect(p, projection)`, the doctrine's intended shape |
| 23 | internal/projection/projection.go:224, manifest.go:103 | A facet is served only if its classification is at or under the template's `max_classification`. | projection template, facet metadata | template (bundled or registered by the operator) | projection knob, not who-may | none; stays in projection |
| 24 | internal/consent/consent.go:46, projection.go:286 | Every candidate facet passes a consent gate that allows all; its Request carries type, classification, status, fact-time and no principal. | facet metadata | hard-coded allow | seam, no rule | none today; see below |
| 25 | internal/config/principals.go:205, :248; daemon/auth.go:53 | Shape rules on the rule data at load: only importers have sources, an operator or the workbench cannot record, workbench takes no any_type or token, no principal holds the operator's token. | the file | importers file | validation of policy data | none; stays in config |
| 26 | internal/daemon/origin.go:286 | A cross-site browser write is refused (403) before the token is read. | request headers | hard-coded | transport guard, not policy | none |
| 27 | internal/daemon/documents.go:118 | A record.document envelope must name bytes the library holds. | payload `sha256` | library | validation, reads payload | none; must stay out |
| 28 | anam-mcp internal/app/app.go:35, :117; adapter/adapter.go:66; describe.go:146 | The adapter reads with ANAM_TOKEN_FILE and offers save_note and save_fact only when a conversation importer token is configured. | the operator's environment | env | client-side token choice | none in Anam; two principals from Anam's side |
| 29 | anam-mcp internal/adapter/query.go:139, selection.go:124 | query_record passes dry_run through; open_selection always runs a dry-run query. | tool input | hard-coded | client behaviour behind row 16 | none; a decision on 16 changes these tools |
| 30 | HANDOFF-opencode.md item 3 (not built) | A held source's envelopes and Definition registrations answer 503; reads, operator writes and the workbench are never held; the hold is operator-authored state in the ops store and survives a restart. | source, role, hold table | ops store (new authored table) | policy, future | `Held(source)` or a clause of `Allow`, with the hold set passed in on the request value |

The importers themselves (anam-importers) decide nothing by source on the Anam side: each posts as
one token and reads 401 and 403 as the daemon's word.

## The function set

Six functions cover every policy row: `Allow(req)` (rows 2, 3, 4, and 30 later), `MaySubmitType(p, t)`
(5, 6, 9, 10), `MayRegisterType(p, t)` (7, with 8 as `MayStoreDocument` or the same test on
record.document), `Records(p)` (11, 12, and 16 if the answer is a clause of it), `MayOpenReceipt(p,
reader)` (13, 14), and `ListScope(p)` (15, 17, and 22 if a projection grant ever exists).
`MayRetract(p, source)` is a seventh for 18, 20, 21, where the declared-source part is policy and the
snapshot part stays in `internal/retract`. Row 1 feeds the package its principal and is not in it.

## Rows that read a payload

Only row 27 reads a payload (the `sha256` of a record.document, to check the library holds the
bytes). It is validation, and it must stay out of policy: the card's line is that the package reads
the importers file and the request and never a payload, which is what keeps access a mechanism
rather than a reading of content. Rows 23 and 24 read governing metadata (type, classification,
status, fact-time) and no payload. They are not who-may rules either: the ceiling is a projection
knob the operator set, and the consent Request has no principal by design (`TestRequest_CarriesNoPayload`
pins its fields). The memory note's rule holds here: nothing below the serve boundary knows who is
asking, so policy plugs in at the handlers and the projection stays blind to the caller. If a
projection grant (row 22) is ever built it belongs in `ListScope` or `MaySelect` at the handler,
not in the consent gate.

## The same rule written twice

- Rows 5 and 6: `maySubmit` for a snapshot's declared types, and `scopesOf` building the gate's
  scopes, are one rule in two shapes. One function should produce both the boolean and the
  `TokenScopes` value.
- Rows 9 and 10: the gate holds the workbench to its allow-list; the inspector's `enterable`
  repeats it for the forms. The page copy exists to answer 403 before drawing a form, so it stays
  as a call to the same function.
- Rows 7 and 8: `mayRegister` and `mayStore` are both `lists(imp, t)`.
- Rows 18, 20 and 21: the daemon's `importerSources`, the CLI's `declaredSources` and retire's
  `refuseDeclared` each walk the importers file for "is this source declared".
- Rows 2 and 3 against openapi.yaml: the role sets live in the route table and again in
  `x-anam-roles`, held equal by a test. The card already says the table stays the one source.

## Card 216's two open calls

Rows 15 and 16. (a) A recording token can run dry_run queries unrecorded, and anam-mcp's
query_record and open_selection send dry_run (row 29), so refusing it costs those tools. (b) Any
role lists every reader's receipts. The audit's smallest rule is: a recording token lists only its
own, the operator lists all. Both become one table-driven case each in `Records` or `MayDryRun`
and `ListScope`, once Matt decides.

## The admission hold

Row 30. It is the first rule whose data is not the importers file but operator-authored state in
the operational store, and it changes while the daemon runs. It still fits the static package if
the daemon reads the hold table and passes the held set in on the request value, so the package
keeps reading nothing but the request and the file. It is also the first rule with a time-varying
answer, which is the kind of thing the card's "reads more than the request" trigger is about.

## Count

Policy: 17 (rows 2 to 13, 17, 18 in part, 20 in part, 21, and 30 future). Policy by omission: 4
(14, 15, 16, 22). Validation: 4 (19, 25, 27, and the snapshot and store parts of 18 and 20).
Projection knob or seam, not who-may: 2 (23, 24). Transport guard: 1 (26). Client-side, outside
Anam's policy: 2 (28, 29). Identity: 1 (row 1).

## Judgement

The set is small enough to stay static Go, as Matt intends. Seventeen rows collapse to six or seven
functions over a request value of a dozen fields, every rule's data is the importers file, the route
table or a constant, and every rule is a one-line comparison a table-driven test can name. Nothing
today needs a join, a recursion, or a reading of history. Two things would justify the Datalog flip.
The first is a number: more than about a dozen distinct rules, or a third principal class (a second
person, a clinician's reader with a projection grant per role, row 22), at which point the rule
set is itself something a receipt should name a version of. The second is an event: the first rule
that must read beyond the request and the file to answer. The admission hold is the first step
toward that, and passing its state in on the request keeps it inside the static line; a rule that
needs to read receipts, snapshots or the ledger to decide (for example "a token may read a type
only after the operator endorsed its last read") is the flip. Until one of those arrives, the
package is a file of small functions and one test table, and the handlers branch on nothing else.
