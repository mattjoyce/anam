---
id: 202610110930
type: decision
title: "Retiring a source is a separate verb from reconciling it"
created: 2026-10-11
status: evergreen
tags: [anam, decision, retraction, cli, importers, health-db]
project: anam
related: ["[[retraction-by-resend-reports-and-never-tombstones-itself]]", "[[tombstone-is-a-record-event]]", "[[sources-reach-anam-only-through-importers]]"]
---

# Retiring a source is a separate verb from reconciling it

**Decision (2026-10-11, card 120).** `anam retract <source> --retired` retires a source whole. It
plans every live facet of the source, reports the count by type, refuses a source that any import or
importer still declares, and tombstones only with `--yes --max-fraction 1`. The reconciling form of
`anam retract` keeps its guard against an empty scan unchanged. The importers stream cut the card
while rehearsing the Garmin cut-over, from Matt's "let's push for separation", and the dispatcher
built it on Matt's word.

## The tension

`anam retract` reconciles. It rescans a source, compares what it holds with the live record, and
tombstones what the source has dropped. It refuses a scan that comes back empty, on purpose, because
an empty source and an unreadable one look the same, and neither is a reason to forget a record.

That guard makes reconciling safe and retirement impossible. When the Garmin importer replaced
`health-db` under the source name `garmin`, all of `health-db`'s facets had to go, and by design no
scan could ever say so. The quick fix was a flag that let an empty scan through. That would have
weakened the reconciling path's one safety for a case it was never meant for, and the next unreadable
share would have been one flag away from wiping a source.

Retirement is a decision, not an observation. So it got its own form, which reuses the same plan with
"holds now" declared empty by the operator instead of read from a scan. The only new parts are the
declaration check and the whole-fraction guard. Each tombstone is still one anam-event through
`gate.Retract`.

## The rule

- Reconcile reads the source and refuses an empty read.
- Retire names the source, refuses while anything still declares it, and asks for the whole fraction.
  The declaration comes out first, so a live source cannot be retired by mistake.
- The artifacts stay. A read as of before the retirement still shows the retired facts.

Rehearsed on a copy of the pre-move ledger, it tombstoned 5,808 facets across five types in 20
seconds, and a rerun had nothing to plan. The live retirement of `health-db` followed the same day and tombstoned 5,818.
Rehearsing each half of a cut-over on a copy is what found the gap before the live run.
