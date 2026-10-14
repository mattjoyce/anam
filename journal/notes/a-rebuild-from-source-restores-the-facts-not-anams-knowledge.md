---
id: 202610142320
type: decision
title: "A rebuild from source restores the facts, not Anam's knowledge of them"
created: 2026-10-14
status: open
tags: [anam, decision, rebuild, disaster-recovery, continuity, importers]
project: anam
related: ["[[anam-owns-the-continuity]]", "[[tombstone-is-a-record-event]]", "[[source-ref-is-identity-not-content]]"]
---

# A rebuild from source restores the facts, not Anam's knowledge of them

**Finding (card 141, 2026-10-14). Decision pending, Matt's (`docs/IMPORTERS.md` §6).** A rebuild
through the importers brings back every fact its source still holds, exactly. It does not bring back
what Anam itself authored about those facts.

## What the drill showed

`scripts/rebuild-drill.py` in anam-importers starts a scratch daemon on an empty ledger, runs each
importer's resend-everything `poll` and submit against it, and compares the result with the live
ledger by type, on `facet_id`, `content_hash` and status. It only reads live. For records and
documents from the Library share, 2,939 facets in 18 types matched exactly, with no differences, in
about 17 seconds of polls and submits. The six importers that run on unraid are the drill's other
half.

Identity and hash are pure functions of the source ([[source-ref-is-identity-not-content]]), so this
was expected. Now it is measured.

## What it cannot bring back

- **anam-time.** A rebuild stamps every fact with the day it was rebuilt, so "what did Anam know on
  1 October" has a different answer afterwards.
- **Superseded versions** the source no longer holds. Only the latest comes back.
- **Operator retractions** of a fact the source still holds. A retraction is an operator's act, not a
  source's fact, so the importer sends the fact again and it is live again.

## How it relates to the continuity

[[anam-owns-the-continuity]] said in July that "owns no facts" is true and evasive, and that the
ordering and the account of what was known when exist nowhere upstream. The drill is that note made
measurable. The payloads rebuild, and the continuity does not. A ledger backup is therefore not
redundant with the sources.

## The open decision

Two questions are Matt's, recorded in `docs/IMPORTERS.md` §6. Should a rebuild keep the old ledger's
anam-times and superseded versions? Card 140's migration importer, ledger to ledger, would. And should
an operator's retraction of a fact its source still holds replay? Until they are decided, the rebuild
route in §8.8 restores facts, and the backup remains the only copy of the history.
