# The README, reviewed by a panel (2026-10-19)

Card 73. Three pairs read `README.md` on `card-73-readme` at b253db7 with the card's first three
items already in the working tree (the status section rewritten, a dead-link step in the gate, a
`CHANGELOG.md`): Ousterhout × Liskov (the README as the module's surface), Hickey × Armstrong (what
it braids, what it says about failure) and Naur × Procida (reconciled against the code, then read
as a stranger). Each ran alone and wrote its own file beside this one. This file collates them and
records what was fixed and what was declined.

## Where the lenses converged

Three of three found the same three things, which makes them the highest-confidence findings:

- **The bundled table had drifted.** `med.started` was listed at v4; the manifest is v5. Every
  lens asked for the machine to hold the table, not the prose.
- **"Reads are narrowed by role" is not what the daemon does.** Roles open routes; every role reads
  the whole record; the one narrowing is an importer seeing only its own source's snapshots.
- **"Nothing here needs a server", then "Run the daemon."** The daemon is the server; what was
  meant is no other machine and no token.

Two of three:

- The quick start's `./anam serve bp` is the one command that never routes to the daemon: under
  `ANAM_SERVER` it refuses, without it, it opens the files a running daemon owns.
- The two Definition tables sit between the ideas and the first command, and the importer table
  states versions that another repository owns.
- The README named no failure, and "rebuilt from the importers" was not the whole truth: what Anam
  authors, and what a person says straight into it, come back only from the backup.

## Where they pulled apart

Hickey and Procida would cut: the tables, the aside about defining a projection, the importer
versions. Liskov and Naur would keep and make true: a checked table, a theory paragraph before the
commands. Armstrong wanted a paragraph added that nobody else asked for, on what breaks and what
comes back. The settlement follows the reviews' own closing lines: keep and check what this
repository owns, drop what it does not, add the failure paragraph first, and give the stranger the
four words the commands print before the commands print them.

## Fixed

1. `med.started` to v5, and `internal/def/readme_test.go` now fails the gate when any bundled row's
   version, presence or the stated count disagrees with `internal/def/manifests/`.
2. `./anam serve bp` replaced by `./anam timeline --projection bp --publish`, which routes to the
   daemon, with `./anam projection list` before it; the comment says the operator registers one.
3. "Routes are opened by role, not narrowed by it", with the one narrowing named, and two sentences
   on a leaked token: hashes only, one source, retractable, revoked by restart.
4. "Nothing here needs another machine or a token."
5. The importer table loses its version column and says whose versions they are; `location.stay`
   is attributed to the `stays` importer, not `owntracks`.
6. "Anam reads no source itself" now ends: the facts can be resent from the importers; what Anam
   authors and what a person says straight into it come back from the backup. The second file's
   sentence says derived tables by replay, the operator's registrations from backup.
7. A paragraph on the error kernel under "How it fits together": idempotent commit, the second
   file behind and never wrong, deadlines and `Anam-Timeout`, facts waiting at their sources when
   the daemon is down, retraction as the remedy for a wrong fact. The diagram draws checkpoints as
   a cached branch off the fold, not a stage.
8. "Four words the commands print" before the quick start: ledger, W, the fold, the canonical.
9. "Letting an AI read it" moved below the quick start, and says a token exists only when the daemon
   was given a `token_file`.
10. "Never stored" became "derived from the facts, every cache of it rebuilt by replay".
11. Status names the backup's actor (a job on the same server, nightly, onto another disk) and says
    what a retraction is instead of "no delete".
12. The docs index is grouped by what the reader is doing (understand, look up, do, records of a
    slice), and indexes the two orphans, `compelling-reason-for-Anam.md` and `pipeline.html`.
13. "Every link above is checked" became "every path above exists at the last merge".
14. "Getting facts in" points the reader with a running daemon at the importers' README for how to
    run one by hand.

## Declined, with the reason

- **Generate the table in the gate** (Hickey × Armstrong 8, Naur × Procida 1). A test that fails on
  drift gives the same guarantee without a generator to maintain, and keeps the prose hand-written,
  which the card asked for.
- **Move the tables to `docs/DEFINITIONS.md`** (Naur × Procida 3). Moving them below the quick start
  answers the complaint; a reader who wants the list still finds it here, checked.
- **Draw the importer-registered versions from the daemon at build time** (Ousterhout × Liskov 5).
  Out of this repository's reach; the column is dropped instead.
