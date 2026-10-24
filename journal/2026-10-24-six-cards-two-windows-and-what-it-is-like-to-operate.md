---
type: journal
date: 2026-10-24
tags: [anam, journal, projections, intake, labels, ductile, docs, deploy]
---

# 24 October: six cards, two windows, and what it is like to operate

A new lead session took the handoff with Matt's one line of direction, work on cards. By the
end of the day six were closed and live, two deploy windows had run, and a conversation about
Ductile said something true about how agents design.

## The cards

Each went to a Sonnet agent in its own worktree, with the decision written on the card first.

- 227: the test fixtures open their daemon under an explicit thirty seconds, and the gate takes
  a lock so a second gate waits instead of failing.
- 231: bundled projections by concept, `body`, `activity` and `clinical`, `doses` v4, and
  registered versions that add the importer-owned types. Coverage now names the projections
  serving each type, so a gap reads as `none`. Only tracks, documents and links are unserved,
  on purpose.
- 208: the retired heart-rate type leaves the Garmin importer's snapshot and the principal, in
  that order. The proof had to go round the gateway, a hand poll ending with the snapshot
  complete and none absent.
- 232: `health.intake`, caffeine, alcohol and notable meals in the person's words, one type with
  three kinds and no nutrition. Matt asked for it quick; it was decided on card 222's pattern and
  built the same way, with the gate refusing a drink's fields on a meal and a meal's on a drink.
- 199's last slice: a Definition may declare a label, so a picker row reads as a thing, and
  "link these" from the timeline's selection. Five bundled Definitions moved a version to carry
  a label, because the handling block is part of the version.
- 235: a copy button beside every facet id the Workbench prints, asked for and closed within the
  day. The first id Matt pasted back came from it.

The doing column was read against the code too: 139 closed, and 141, 84, 182, 198 and 199 moved
to todo with what remains written down, so the board says who is on what.

## Two windows

The first shipped 231 and 232 as v0.591, registered the three projection files, added intake
to the Workbench's allow-list, swapped the importer tree for 208 and dropped the type from the
principal. The second shipped 199 and 235 as v0.601. Both were plain deploys. The hand poll's
driver takes a batch limit as its second argument, and I handed it a file name; the poll output
survived and the driver ran again. It is on the runbook.

## What it is like to operate

Matt's documents question exposed a README that said "not deployed" for every importer, some
live for two weeks. Fixed the same hour, and a recipe for saving a document went where agents
read: the file and its sidecar into the documents folder on the Library share, the watch does
the rest. Card 229's PR from opencode took the two-stage shape but partitioned by file
modification time, which on the live archive puts nearly everything in one window; the review
on the PR gives the partition that works.

Then the conversation. Ductile's attestation, service accounts and config model had grown
strict because every agent that worked on it, Claude and GPT alike, saw a gateway with secrets
and designed for the enterprise case. Matt's own words: it is a runner; someone changes a plugin
and it should run. The shape we landed on is a time-boxed clearpass per plugin root, the
operator's intent before the change rather than a ceremony after it, with refusals outside the
window made loud. The underlying lesson is the one the Anam memory already holds: size the risk,
or the agents will size it for you, and they size it as a hospital.

## Hickey × Armstrong

**Hickey.** A label in the handling block is presentation braided with the schema's version, and
the day showed the cost: importer-pinned types cannot take one without both repositories moving.
Whether handling should be versioned apart from the schema is the open question.

**Armstrong.** Two windows, nothing refused, the record's watermark unchanged by either deploy.
The gate lock from card 227 is a small supervisor: it turns a race into a queue.

## Left open

- Card 232's second acceptance: LifeOS re-enters the 23 October notes as intake facts.
- Card 229, after opencode's changes.
- Card 233, the schema lint that demands `additionalProperties` on a conditional branch.
- Labels for importer-pinned types, and where handling is versioned.
