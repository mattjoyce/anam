---
id: 202610181900
type: decision
title: "The ledger is never purged; a forget is a new ledger built without the records"
created: 2026-10-18
status: evergreen
tags: [anam, decision, ledger, excision, privacy, immutability]
project: anam
related: ["[[tombstone-is-a-record-event]]", "[[retraction-by-resend-reports-and-never-tombstones-itself]]", "[[a-rebuild-from-source-restores-the-facts-not-anams-knowledge]]", "[[invariants-enforced-by-the-substrate]]"]
---

# The ledger is never purged; a forget is a new ledger built without the records

No verb edits the live ledger. Not for erroneous versions, not for tombstoned facets, not for
content the person wants gone. If the record must forget, the remedy is a new ledger, replayed
from the old one's anam-events with the forgotten records omitted and one event at the head
naming what was left out. The old ledger is succeeded, never mutated.

Matt decided this on 18 October 2026 (card 178), on an assessment that split "purge" three ways.
Erroneous versions have no case: each is what the source presented, and the honest record is how
two bugs were found that day. Tombstoned content has no current need. Content that must not
exist (a secret, a third party's data) has a real case, and it is met by the rebuild, not by a
delete. For now none is built: "live with the things I don't like, and trust the ledger."

## The tension

A lifelong personal record needs some way to forget, or the only erasure is the whole file. The
pull is toward a delete, scoped and audited. What pulls back is that the ledger is Anam's error
kernel: append-only held by the database's own triggers, every receipt a proof over its events,
every checkpoint a hash of them. One delete path, however guarded, makes every reader ask
whether the past it reads is the past that was. A new ledger keeps the question answerable: the
two files differ exactly by the excision event.

## Consequences

- The triggers stay, for every writer, with no exception path.
- Datomic's excision is the precedent, including its warning: not for routine use. Anam adds
  that the excision is a rebuild, so a crash mid-way leaves the old ledger whole and a half-built
  file to delete; nothing is ever half forgotten.
- A rebuild must replay the ledger, not the sources, so anam-time survives; receipts' `W` keeps
  its meaning if seq is carried with gaps.
- Idempotency means the source can present the content again; a durable forget needs the gate
  to refuse what the excision event names.
- The stub keeps a facet's id and version and drops its content hash, which would otherwise be a
  membership oracle on guessable content.

## Rejected

- **A delete endpoint, or an importer-callable purge.** Importers present; the person decides.
- **Excising versions for tidiness.** The flip-flop is diagnostic; the fix is upstream, and the
  read side can collapse it.
- **A "noise" mark on a version.** A status written onto a fact is the mutable place the
  doctrine forbids.
