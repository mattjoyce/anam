---
id: 202610101600
type: decision
title: "Retraction by complete resend reports absence and never tombstones by itself"
created: 2026-10-10
status: evergreen
tags: [anam, decision, retraction, tombstone, importers, snapshot]
project: anam
related: ["[[tombstone-is-a-record-event]]", "[[retiring-a-source-is-a-separate-verb-from-reconciling-it]]", "[[sources-reach-anam-only-through-importers]]"]
---

# Retraction by complete resend reports absence and never tombstones by itself

**Decision (Matt, 2026-10-10, card 78).** An importer tells Anam what its source no longer holds by
resending everything as one labelled snapshot, not through a retract route. Anam records the facet
ids each part carried and, once the snapshot is complete, reports the source's live facets that were
not carried (`GET /v1/snapshots/{id}`). It does not tombstone them. There is no `retract: auto` and
no `max_retract_fraction` key. Every importer's tombstones go through `anam retract --yes`, which
plans from the newest complete snapshot. A complete snapshot with an absent set raises an alert
through the gateway instead.

## The tension

Automatic retraction is what makes a mirror honest: a source that deletes a row should see the
record follow. An importer that only admits can never forget. Against that sits one fact about the
gate. It decides a no-op by content hash alone and ignores status, so when a wrongly tombstoned row
is sent again, the resend is a no-op and the facet stays dead. No event reverses a tombstone
([[tombstone-is-a-record-event]]).

That makes the two mistakes very unequal. An importer bug that labels a partial resend "complete"
would tombstone everything it forgot to send, permanently, with only a fraction guard between the
bug and the loss. A fact that stays live a few days until someone confirms the retraction costs
almost nothing. The importer is outside the error kernel and should not decide what the record
forgets.

## The rule

- A resend-everything run labels its batches as parts of one `snapshot`, and Anam binds each part to
  its contents (card 107), so a part resent with different contents is refused.
- Absence is judged from the snapshot's start (card 94). A facet admitted while the run was going is
  not reported absent.
- The report is data, the alert is the gateway's job, and the tombstone is an operator's act.
- Revisit only with evidence, months of report-only snapshots whose absent sets were right, not
  with optimism.

The general shape: before automating a destructive step, find out whether its worst mistake can be
reversed by the normal flow. Here it cannot, so a person confirms it.
