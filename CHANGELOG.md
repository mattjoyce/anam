# Change log

Anam has no releases: master is what runs, and a build names itself `v0.<commits>-<hash>`
(`anam version`). This file lists the changes that alter what an operator, an importer or a reader
of the API must do or may expect, newest first, by the day they merged. Each names its card on the
kanban board, which holds the reasoning. The full history is `git log`; the story behind the
decisions is in `journal/`.

## 2026-10-20

- A Definition's `meta` may declare `kind` (`measure`, `reading` or `event`) and `family` (a
  lower-case word such as `meds`): what the type is, for a reader. The gate ignores both. The
  inspector draws a type that declares them on its family's row with its kind's marker before
  anyone edits core, and the definitions page shows them (card 176).
- `sleep.episode`, `health.heart-rate-daily` and `health.step-count` are bundled, so another source
  may name them. An importer re-sending the body the binary bundles, at its version, is answered
  200 unchanged rather than 409; another body of a bundled type is still refused (card 139 piece 1).

## 2026-10-19

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
