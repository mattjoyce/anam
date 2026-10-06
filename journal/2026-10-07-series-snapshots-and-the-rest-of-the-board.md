---
type: journal
date: 2026-10-07
session: 13
title: "Series, snapshots, and the rest of the board"
tags: [anam, journal, series-key, snapshot, projection, reference, retract, receipt, inspect]
---

# Series, snapshots, and the rest of the board

Matt's brief for the day was four words long: work autonomously, commit often, work the cards, use
luminaries. Nine cards went from todo or backlog to done in eleven commits, each behind
`scripts/check.sh`, each with a design pass written down before the code where the change was not
trivial ([[series-key]], [[reference-payloads]]).

## The series key (card 15)

The card had been open since July and everything medication-shaped leaned on it. The Hickey pass
found a third thing hiding in "one declaration or two": the *family*. Without a name for it,
`med.started` and `med.stopped` could stamp the same string by accident or fail to by accident, and
nothing could check them. So a Definition says `series: {name, key, closes}`, the registry refuses a
family keyed two ways, and the gate stamps `<family>:<values>` beside classification. Stamping at
capture, not deriving at read, keeps an artifact interpretable under the definition version that
admitted it.

The ledger schema moved to 2 for the column. `ledger_meta` is write-once by trigger, so the version
is not a pin that moves: the created-at version stays and `schema_migration:2` accretes beside it.
`anam init` migrated the live ledger in one DDL transaction, and `anam checkpoint --dry-run` minted
0 of 143 buckets afterwards, because a fold entry without a series serialises exactly as before. The
migration is value-preserving, and the hash said so.

## The snapshot (card 23)

`fold.Snapshot` is the last-known-value read §4 had named as unbuilt since July. The July decision
that *shape goes in the template* held: a projection has a fifth knob, `shape: log | snapshot`, and
the relation is decided once in `fold.Fates`, so a facet not served can say "displaced by" or
"closed by" which one. The closing rule reaches the projection as a parameter at the point of use,
like the consent gate, and a snapshot template without it is refused. `timeline --at` is the
person's own view of the same read; `serve meds` publishes it with a receipt. The equality oracle
passed: the snapshot from composed hard checkpoints equals the snapshot from a full fold equals the
regimen worked by hand, and the bitemporal case (the answer at D before the stop row arrived) too.

## The rest

- **Life events** (27): illness and travel are series; `travel.returned` closes, not an arrival that
  happens to be home. The type, not the payload.
- **Compliance estimates** (26): the estimate is a fact bounded to (0, 1] at the gate; the schedule is
  `regimen.Expected`, a number, and a test holds the watermark still while it runs.
- **Reference payloads** (24): the descriptor describes the payload it holds, the payload describes
  the file. One integrity invariant for both kinds. The `documents` worker reads a sidecar for the
  date and refuses a file without one, naming the sidecar to write. Not yet in the live imports.
- **Receipts name their ledger** (18): `ledger_id` last, omitted when empty, so the July receipts kept
  their hashes and still verify, with a note.
- **Required means filled** (13): null and "" are refused; all 13,600 live rows still admit.
- **Band pan and zoom** (20): four server-derived links, arrows and wheel bound to them. Round trip
  first, as the card asked.
- **Retraction** (17): `anam retract <source>`, dry run by default, three guards against a bad read.
  Over the live sources today: nothing to retract.

## What the gate caught

`scripts/check.sh` failed once and a pipe hid its exit code, so one commit landed red: the documents
test imported the store from a capture package. The fix was the precedent the records test already
set, a gate round-trip in `internal/app`. The lesson is in the shell, not the code: capture the exit
status before the pipe.

## Left on the board

Cards 11 and 12 need an empty ledger and are Matt's call to schedule. Card 14 needs somewhere to hold
a Definition's earlier version before it can guard one. Card 21 is to be discussed. Card 10 lives in
the ETL repo. Two untracked files from someone else sit in the tree: `docs/API.md`, an application
API proposal, and `journal/notes/mcp-go-sdk.md`.
