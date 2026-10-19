# Change log

Anam has no releases: master is what runs, and a build names itself `v0.<commits>-<hash>`
(`anam version`). This file lists the changes that alter what an operator, an importer or a reader
of the API must do or may expect, newest first, by the day they merged. Each names its card on the
kanban board, which holds the reasoning. The full history is `git log`; the story behind the
decisions is in `journal/`.

## 2026-10-19

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
