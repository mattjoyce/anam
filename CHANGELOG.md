# Change log

Anam has no releases: master is what runs, and a build names itself `v0.<commits>-<hash>`
(`anam version`). This file lists the changes that alter what an operator, an importer or a reader
of the API must do or may expect, newest first, by the day they merged. Each names its card on the
kanban board, which holds the reasoning. The full history is `git log`; the story behind the
decisions is in `journal/`.

## 2026-10-22

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
