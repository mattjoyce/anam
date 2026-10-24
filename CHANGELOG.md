# Change log

Anam has no releases: master is what runs, and a build names itself `v0.<commits>-<hash>`
(`anam version`). This file lists the changes that alter what an operator, an importer or a reader
of the API must do or may expect, newest first, by the day they merged. Each names its card on the
kanban board, which holds the reasoning. The full history is `git log`. The story behind the
decisions is a journal kept apart from this repository since 2026-10-24, because it is the
account of one instance and carries its data.

## Unreleased

- A bare date in the Workbench is a day in the reader's zone (card 236), the rule the API, the CLI
  and anam-mcp already kept. The select page, the projection page's rows view, the timeline and the
  location page read `from` and `to` in a `zone` parameter (an IANA name), shown in a field beside
  the dates. A request without one reads in the daemon's configured zone (`zone`, `ANAM_ZONE`; UTC
  when that is `Local` with no name), and the page's script fills the field with the browser's zone
  while nothing on the page has been read yet. An unknown zone, and `Local`, are a 400 naming the
  zone. Before, these pages read a date as a UTC day, so a selection "from 12 Oct" built in Sydney
  started at 11:00 on the 12th and missed a 07:30 blood draw. A selection program carries one
  comment line, `% dates read in Australia/Sydney`, which `Parse` ignores and a save keeps; a saved
  selection keeps its instants, and its page names the zone its dates were read in, UTC when it has no
  comment. A bound is written back as a date only when it is midnight in the zone. A selection saved
  from a page now carries the zone line, so it has a different code from the same instants saved
  without it. The projection page's as-of stays UTC. A caller of `selection.Days` now passes the zone.
- The timeline and the select page hand a range to each other (card 237). A zoomed timeline offers
  "select in this range" and the select page offers "show on timeline"; each is a link to the
  existing route carrying the window, the type filter and the zone (dates when the window lies on
  the zone's midnights, instants otherwise), so a Sydney day stays a Sydney day both ways. An
  unzoomed timeline offers no range, a saved selection is drawn as before, and the source filter does
  not travel. No new route.
- The Workbench has a copy button beside every facet id it prints (card 235). The facet page's
  heading, the facet list, the anam-events list, the link and selection pages, a projection's rows
  and the receipt after a save or a retraction each show a small clipboard button that copies the
  whole id and reads "copied" for a second. It is a real button with the label "copy facet id". It
  uses the clipboard API where the browser has one and otherwise selects the id in a hidden input
  and copies that, since the Workbench is often reached over plain HTTP on the tailnet. A page that
  prints an id uses the `facetid` template fragment and gets the button. No new route.
- The selection page reads a facet as a thing (card 199). A Definition may declare
  `handling.label`, a short template of literal text and `{/pointer}` fields read from the payload
  (`{/drug} {/amount} {/unit}`), and the picker's rows, its basket and the link form's basket show
  it ("melatonin 5 mg", "80.4 kg"); a type without one keeps its type and time. A label never fails:
  a field the payload lacks is nothing, a text field is its first line, and a label with every field
  empty is no label. `Parse` refuses a label that reads no field, names none the schema declares or
  uses anything but `{/pointer}`. Because `handling` is part of the version, five bundled types
  moved a version to add one: `health.weight` v6, `health.medication.dose` v3, `mind.attention` v2,
  `health.intake` v2 and `record.edge` v3. Writers that read the Definition in force (the Workbench
  forms, anam-mcp, records files that name no version) follow; an importer that pins a version of
  one of these must move with it. Blood pressure and the Garmin and Withings types have no label
  yet, since their importers pin the version. The timeline's own selection (ctrl-click or shift-drag
  marks) gains "link these…", beside the facet page's "link this…", which opens the selection page
  with those facets in the basket.
- Anam can hold what a person takes in, said in a few words (card 232). `health.intake` v1 is
  bundled: `kind` is `caffeine`, `alcohol` or `meal`, and `at` is the minute at the offset
  `source_time` names. A drink says `drink` (one spelling, lower case with hyphens, `flat-white`,
  `red-wine`) and `count`, with `size`, `volume_ml` and, for alcohol, `abv` when known; a meal says
  `text`, `rich` and `out`. The gate refuses a field of the other shape. `part_of_day` makes `at`
  that part's nominal minute (08:00, 14:00, 19:00, 22:00). Two intakes of one kind in one minute are
  one fact, so `count` carries the number. Sensitive and strict, no nutrition or calories, and
  standard drinks are worked out on read. To enter them by hand, list `health.intake` under the
  `workbench` entry's `types` in the importers file and restart; an agent saves one through
  anam-mcp's `save_fact`. A bundled `intake` projection serves it as a log, up to Sensitive.
- The operator can name the projection an agent should read first (card 180). The new config key
  `default_projection` (`ANAM_DEFAULT_PROJECTION`, empty by default) is advertised as
  `default_projection` on `GET /v1/status` and as `default` on `GET /v1/projections`. It is advice,
  not applied: a read that names no projection is unchanged, and a receipt still names the version
  that served it. The daemon will not start on a name that is neither a bundled nor a registered
  projection. anam-mcp's `describe_record` names it.
- Anam can hold what a person says is absorbing them (card 222). `mind` is a ninth domain head, for
  self-reported inner states, so mood or energy can join later; a head is added only by a logged
  decision. Three Definitions are bundled at v1, `mind.attention` (a reading: `at`, `topic`,
  `intensity` from 1 to 5, an optional `note`), and `mind.attention.started` and
  `mind.attention.ended` (a pair the person declares, which closes a series keyed by `topic`; a
  later started opens a new one). `topic` has one spelling, lower case with hyphens (`deskcam`).
  `at` is a wall clock read at the offset `source_time` names, so one minute said in two zones is
  two readings. All three are Confidential and strict. A bundled `attention` projection serves them
  as a log. To enter them by hand, list `mind.attention`, `mind.attention.started` and
  `mind.attention.ended` under the `workbench` entry's `types` in the importers file and restart;
  an agent saves one through anam-mcp's `save_fact` with no change to either program. Nothing is
  inferred: readings never open or close an episode, and `mind.attention.reflection` is not built.

## 2026-10-22

- A token can always leave receipts, so a receipt says who read what (card 216). A principal in the
  importers file may set `receipts: always` (a reader or an importer; off when absent). Its
  `GET /v1/timeline` is then answered as `POST /v1/receipts` is, with status 200, and the receipt
  names the principal in a new `principal` field; its `POST /v1/query` names it on the receipt it
  already files, and `dry_run` still files nothing. Such a token must name a projection (a read
  without one, or with `explain`, is a 400), opens only the receipts that name it, and has the read
  fail if the receipt cannot be filed. A receipt cut before the field has the same bytes and hash
  and still verifies, but an older binary cannot parse a receipt that names a principal, so
  `GET /v1/receipts` fails on one after a rollback. `GET /v1/receipts` takes `principal`, `since`,
  `until` and `zone`, and `anam receipt list` takes `--principal`, `--since` and `--until` and shows
  a `READER` column.

- Receipts, served values, saved selections and checkpoint values are rows of the operational store
  (card 215), not files in a folder beside it. The schema is 9, so a store from before is refused and
  is carried forward by moving it aside, running `anam init` for a fresh one, and then
  `anam move-artifacts --from <the old artifacts folder> --registered-from <the old tracking.db>`.
  Without `--yes` that reads and checks every file against its name and writes nothing; with it, it
  loads the rows. `artifact_dir` is gone from the config, and a config file that names it is refused
  at start. Checkpoints are a cache that may go missing: a mint ends by removing the ones the newest
  run does not read, and a read that needs one that is gone, as a read at an older watermark does,
  folds the ledger in full, which gives the same value more slowly. A mint writes back a checkpoint
  value the log names and the store has lost. `GET /v1/receipts` names the store in `store` where it
  named a folder in `dir`, and a backup is the ledger, the operational store and the library.
- The operator can hold a source's admission. `POST /v1/hold` (operator) takes `{"sources": ["garmin"]}`
  or `{"all": true}` and answers the sources held now; `DELETE /v1/hold` releases the ones it names,
  or every hold when it names none. `GET /v1/status` reports them in `held`, where `*` is the global
  hold. A held source's `POST /v1/envelopes` and `POST /v1/definitions` answer 503 with `Retry-After`
  and a body naming the hold; reads, the operator's own writes and `POST /v1/entry` are never held,
  and the workbench is never held. The hold is authored state in the operational store (schema
  version 9, table `admission_hold`), so it survives a restart; a store at version 9 is refused by an
  older binary, so roll back to the previous store, not the previous build. `anam migrate` carries
  the holds into the new store. `anam hold <source>...`, `anam hold --all`,
  `anam hold --release [<source>...]`, and `anam hold` to list. Ductile retries a 503 on its own
  backoff and does not read `Retry-After`, so a held source's importer is retried later, not sooner.

- The Workbench writes links by hand (card 202). `GET /link` on the inspector draws one `record.edge`
  from a basket of facets, each on its side, with a kind, the person's words, a "link to this
  version" pin per facet and the moment in the browser's zone; `POST /link` saves it as source
  `workbench`. A new link's id is `workbench:edge:` and 16 hex of the SHA-256 of its kind, sorted
  sides and moment with offset, so the same link saved twice is one fact; an edit keeps the id. The
  selection page's "Use this selection" opens it, and a facet page offers "link this…" and, on a
  Workbench link, "edit this link". The form is refused until the importers file lists the type for
  the workbench principal:

  ```yaml
  importers:
    - source: workbench
      types: [health.medication.dose, record.edge]
  ```

## 2026-10-21

- The importers say how their clocks were read (card 198, the importer half). New bundled versions,
  each identity and fact-time rule unchanged: `activity.exercise-session.concept2` v2,
  `activity.exercise-session.strong` v2 and `record.document` v2 require `source_time`;
  `health.blood-pressure` v7, `activity.exercise-session.technogym` v2, `health.lab-result` v2 and
  `health.lab-panel` v2 declare it without requiring it, since records files written by hand, and
  Technogym's, which has no export left, still send none. The old bodies join the Definition
  history. The importers carry `health.blood-pressure` v7 and the Concept2 leaf v2 byte for byte,
  and register every version of the types they emit, oldest first. Deploy anam first, with the
  records, documents, Concept2 and bp_csv importers paused across it, and regenerate the Strong
  records file with the new `strong_csv.py` before records resumes: the Strong leaf now refuses a
  record without `source_time`. `record.document` leaves the list in `internal/def/sourcetime_test.go`.

- Vendor-shaped types take the vendor leaf (card 195). `activity.exercise-session` splits into four
  bundled leaves, `activity.exercise-session.garmin`, `.concept2`, `.technogym` and `.strong`, each v1
  with the shared core (`id`, `activity_name`, `effective_time_frame`, `source`) and only the fields
  its vendor writes; the old name's v1 and v2 stay in the Definition history. The importers rename
  `activity.day`, `health.measurement` and `health.cpap-session` to `activity.day.garmin`,
  `health.measurement.withings` and `health.cpap-session.dreamstation`, registered before the run.
  `activity.day.garmin` reads its fact time at the start of the day Garmin counted, so a day the
  watch spent at another offset moves to its own midnight (card 197). `anam rename` now compares
  the identity key alone: a type arrived at may place facets by another fact-time rule, and the
  dry run names each such row and how many facets move. The operator runs `anam rename --table
  renames/vendor-leaves.json` (dry, then `--yes`, the daemon stopped), renames the types in the
  importers file's `types` lists, registers `projections/recent-life.v5.json` (exercise by the
  prefix `activity.exercise-session.`), and retypes the records files with `retype_records.py`.
  `scripts/pipeline-staleness.sh` reads the Garmin day and Withings leaves.

- The medication, illness and trip types take a third segment (card 196). Twelve types, one to one:
  `health.medication-started`, `-stopped`, `-dose`, `-dose-changed`, `-compliance` become
  `health.medication.started`, `.stopped`, `.dose`, `.dose-changed`, `.compliance`;
  `health.medication-missed-dose` becomes `health.medication.dose-missed`; `health.illness-started`
  and `-ended` become `health.illness.started` and `.ended`; `place.departed`, `.arrived`,
  `.returned` and `.flight` become `place.trip.departed`, `.arrived`, `.returned` and `.flight`.
  Each new type is v1, and the old name's versions stay in the Definition history, so every facet
  admitted under an old name still reads. The series (`med.regimen`, `illness.episode`,
  `travel.trip`) are unchanged. The operator runs `anam rename --table renames/kinds-segment.json`
  (dry, then `--yes`, the daemon stopped), renames the types in the importers file's `types`
  lists, registers `projections/recent-life.v4.json`, and retypes the records files with
  `retype_records.py`. The bundled `meds` is v3 and `doses` is v2, with the same type lists under
  the new names. A projection's bundled history may now hold more than one earlier version.

- `record.note` is now `record.edge` (card 191): since card 104 the type joins facets, and a note is
  an edge of kind `about`. `record.edge` v1 is `record.note` v2 under the new name, and the old name's
  versions stay in the Definition history, so every facet admitted as `record.note` still reads.
  The operator runs `anam rename --table renames/record-edge.json` (dry, then `--yes`, the daemon
  stopped) to carry the live notes, and registers `projections/recent-life.v3.json`. A writer sends
  `record.edge` and `to`; `about` stays accepted as the old name of `to`.

## 2026-10-20

- The inspector can enter, correct and retract a fact by hand (card 188, slice 4). When the importers
  file declares a `workbench` principal, the Definitions page shows an Enter link on each type in its
  `types` and none on the rest, and `/entry?type=<type>` draws a form from that type's Definition at
  request time: one control per schema property (an `enum` a select, the fact-time field a date and
  time read in the Definition's zone and written in its layout), `description`, `examples` and
  `handling.guidance` as help, and the Definition version it was drawn from sent back so a changed
  Definition is refused. The id is not a field: the gate composes it. A save shows the gate's
  answer (admitted, superseded, no-op, or its refusal words beside what was typed); a type whose
  schema needs a nested object or a list, blood pressure's, gets a plain message and no form. On a
  facet the workbench entered, Edit reopens the form filled, with the fields that decide which facet
  it is locked, and Retract tombstones it after a confirm; a facet of any other source has neither
  button. Nothing new to configure: the pages ask the daemon for the workbench's types. A standalone
  `anam inspect` has no Workbench and no entry routes.
- The operator can enter a fact by hand through `POST /v1/entry` (card 188, slice 2). The importers
  file may declare a `workbench` principal (`source: workbench`, `types: [...]`, no token; a
  `token_sha256` or `any_type` on it is refused at start), and the daemon admits the operator's entry
  as that source, so no importer token reaches a browser. The body is `type`, `definition_version`
  and `payload`, with no source or id: the gate fills the identity field from the Definition's
  `identity_template`. The answer is `admitted`, `superseded` or `no-op` with the facet id, `seq`
  and `w`; a stale `definition_version` is 409, a refusal is 422 with the gate's words, and a daemon
  whose file declares no workbench answers 404. The route and `POST /v1/envelopes` now share one
  write (`admitBatch`), so both end in the same queue, catch-up, status and mint. To switch it on,
  add `- source: workbench` with its `types` to the importers file; there is no form yet.
- A type's id rule can be declared in its Definition (card 188 S1). `meta.identity_template` is a
  string such as `{source}:med/{/drug|slug}/dose/{/taken_at|2006-01-02T15:04}`: literal text, the
  source, and payload fields by JSON pointer with an optional `slug` or time layout. `def.ComposeID`
  builds the id from it for any writer. The gate fills the identity field when an envelope leaves it
  out and refuses one whose field disagrees, naming both. `health.medication-dose` is v2 and declares
  the rule its guidance used to state in words; the three melatonin doses on the record already match
  it. A type with no template is unchanged.
- One facet of a source nobody resends can be retracted by hand (card 188, slice 3).
  `anam retract <source> --facet <id>` shows the facet (id, type, version, fact-time, status) and
  what the tombstone would do, and with `--yes` appends one tombstone through the gate.
  `POST /v1/facets/{id}/retract` with `{"source": "..."}` does the same for the operator, behind
  the same browser-origin guard as the other writes, and answers the tombstone's `seq` and the new `w`. Both
  apply one plan (`internal/retract`) and refuse, by name, a facet of another source, a facet that is
  not live, and a source an importer declares and has completed a snapshot for, which its next
  resend reconciles. It serves `workbench` and `conversation`, which no importer resends. The button
  on the facet page comes with the form slice.
- The gate refuses to carry a facet that is not live (card 185). `Rehearse` and `RenameBatch` fold
  the facet's events and answer `ErrFacetNotLive` for a tombstoned facet, naming its id, type and
  status, and write nothing. Until now only the CLI's plan kept the 6,218 tombstoned facets dead on
  2026-10-20; any other caller of `RenameBatch` would have brought them back. A dead facet stays
  under the type it died in.
- The Soufflé oracle names the renamed medication types (card 182). `scripts/souffle-diff.py` still asked about `med.started` and `med.dose-changed`, which no facet holds since the rename, so its regimen questions would have answered empty on both engines and agreed for the wrong reason. They now ask about `health.medication-started` and `health.medication-dose-changed`.
- Verify reproduces a read served under an old type name (card 187). After the rename a receipt
  served before it, such as the two `meds` v1 receipts, failed to verify with one extra facet: the
  replay no longer knew that `med.stopped` closes a course, because the Definitions in force name
  only the new types, so the stopped course read as open. A type with no Definition in force now
  answers `closes` as its newest bundled history version did. The gate still admits nothing under
  a retired type. Verify also says in its notes when it replayed an earlier version of the
  template than the current one.
- The taxonomy (card 147). Every bundled type is named `domain.concept` under one of eight closed
  domains: `health`, `activity`, `place` and `record` in use, `social`, `money`, `media` and `work`
  reserved. Twenty-four types are renamed (`med.started` is `health.medication-started`,
  `sleep.episode` is `health.sleep-episode`, `travel.flight` is `place.flight`, `note.annotation` is
  `record.note`, `doc.reference` is `record.document`, and so on, per `renames/taxonomy.json`) and
  start again at v1 under the new name; the four kept (`health.blood-pressure`, `health.weight`,
  `health.body-composition`, `health.heart-rate-alert`) are one version up. Every one declares
  `kind` and `family`. The old names' Definitions stay in the bundled history, so an earlier version
  of a facet still reads against its own words, and the three retired health-db types are left as
  written. `health.medication-dose` is new (card 179): one dose taken at a moment, outside any
  course, with a `doses` log projection; `meds` is v2 under the new names and its v1 is held from a
  bundled history of its own, so a receipt that names it still verifies. The inspector's rows are
  the declared families. Nothing is live until the rename run (card 140) carries the record over;
  until then the daemon admits the new names only, and the importers and anam-mcp move with it.
- `anam rename --table <json> [--yes]` carries every live facet of a type into another type as the
  next version of itself, through the gate: the same id, source, identity values, fact-time and
  payload, with the new Definition's classification and series. The old version stays, so
  anam-time is kept and a read as-of before the run is unchanged. The two types must key alike,
  the dry run reports what the gate would refuse, and a refusal writes nothing. The gate
  supersedes on a type change only under this run's mark; the API's refusal of a changed type
  (card 105) stands. The daemon is stopped while it runs (card 140).
- `record.link`, a file Anam remembers but does not hold (card 165), is registered by the
  `anam_links` importer in anam-importers, not bundled. The inspector draws it in the documents
  row and its facet page says the bytes are not held and offers no download; `docs/INBOX.md`
  says when to use it and when `doc.reference`. No daemon change: the held-bytes refusal was
  already keyed on `doc.reference`.
- A Definition's `meta` may declare `kind` (`measure`, `reading` or `event`) and `family` (a
  lower-case word such as `meds`): what the type is, for a reader. The gate ignores both. The
  inspector draws a type that declares them on its family's row with its kind's marker before
  anyone edits core, and the definitions page shows them (card 176).
- `sleep.episode`, `health.heart-rate-daily` and `health.step-count` are bundled, so another source
  may name them. An importer re-sending the body the binary bundles, at its version, is answered
  200 unchanged rather than 409; another body of a bundled type is still refused (card 139 piece 1).
- `scripts/pipeline-staleness.sh` reads the Garmin days as `activity.day`. It reported "no rows"
  against a record that holds only the new name, so run it after the rename run, not before
  (card 147).
- The explainer bundle says what the taxonomy made true: the eight domains, 32 bundled Definitions
  and 43 in history, `health.blood-pressure` at v6, `record.note` at v1. Its log has an entry for
  cards 139, 140, 147, 165, 168, 176 and 179, its anchor is c08bfb5, and the gap about the importers
  checkout is closed. The tracked `.pyc` under `docs/explainer` is removed (card 147).
- The prose says the same: the GLOSSARY and README state the eight domains, AGENTS.md's naming table
  and the ARCHITECTURE and MVP extension notes name the reserved ones in place of `calendar.*` and
  `mail.*`, SELF-LOG's type table leads with each type's family, and `projections/README.md`
  registers `heart.v2`, `sleep.v3` and `recent-life.v2` (card 147).
- `scripts/check.sh` runs the explainer check (`docs/explainer/check.py`) after the link check. It
  reads the importers checkout beside this one, or at `EXPLAINER_IMPORTERS_DIR`, which a worktree
  under another folder must set; with no such checkout the gate says so and skips it (card 147).

## 2026-10-19

- `scripts/souffle-diff.py` runs the bundled rules and the card's questions through Anam's
  evaluator and through Soufflé over the same facts and compares the answers: the oracle for the
  engine's general machinery, run by hand until it has earned trust, never part of the gate
  (card 173).
- The query surface (card 173, second slice). `POST /v1/query` runs a Datalog program through a
  projection at the head, `as_of` or `w`, built on the bundled rules, and answers one relation with
  a receipt that names the projection, the rule set's hash and the program; `anam receipt verify`
  replays it. `GET /v1/query` and `anam query describe` are the introspection: relations, each
  type's pointers at each Definition version, the fact-time lexeme, the rules. `anam query run` is
  the CLI, local or over the daemon. A constant in `instant/1` must be the whole fact-time lexeme;
  a derivation or an answer over `max_read_bytes` is 413 with `instant`, `projection` and `program`
  as the narrowings. A query moves no watermark and writes artifacts only.
- `anam query check` holds the bundled Datalog rule `current/2` equal to `fold.Snapshot` over a
  real ledger, read-only, one instant at a time; `--ledger` and `--person` point it at a copy,
  `--projection` serves the log through a template first, `--every` samples the fact-times. The
  evaluator itself (`internal/query`) has no other surface yet (card 173).
- `long_write_timeout` names the daemon's long-write deadline. `ingest_timeout` and
  `ANAM_INGEST_TIMEOUT` are read as an alias with a warning at load; the two set differently are
  refused (card 156).
- A principal may have `any_type: true` in place of `types`: it submits every type the daemon
  knows, owns no Definition and stores no document. The conversation principal uses it, so an
  assistant saves a facet of any type as the person's own, bound with high assurance (card 177).
  anam-mcp gained `save_fact`, and `save_note` now binds the person the same way.

## 2026-10-18

- A Definition may carry a `handling` block, served verbatim, for what every writer of the type
  agrees on; anam-mcp's `describe_type` shows its guidance (card 166).

## 2026-10-17

- The repositories are public. The README is written for a stranger, and the lineage moved to
  `docs/HISTORY.md` (card 142).
- A snapshot projection takes its snapshot at `--at`, not at the head (card 172).
- `location.stay`, from source `osm`, names the places the phone stops, a sibling facet of
  `location.track` (card 168, in anam-importers).

## 2026-10-15

- Every token names a principal with a role, reader, importer or operator, and a route names the
  roles it opens (card 89).
- `note.annotation` v3: a note may name no facet. anam-mcp's `save_note` writes the person's words
  (card 157).

## 2026-10-14

- Anam reads no source itself: records and documents arrive through their importers over the
  envelope API, and the library stores a document by its sha256 (cards 119, 150, 102).
- Every opinionated default is a config key with an `ANAM_` variable; `config.example.yaml` lists
  them all (card 151).
- Queue wait counts toward the caller's deadline (card 110). The Definition history survives a
  rebuild (card 111).
