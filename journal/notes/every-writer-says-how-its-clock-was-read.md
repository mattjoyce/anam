---
id: 202610211800
type: decision
title: "Every writer says how its clock was read; dates stay local dates"
created: 2026-10-21
status: evergreen
tags: [anam, decision, time, zone, source_time, identity]
project: anam
related: ["[[fixed-width-timestamps-lexical-order]]", "[[verbatim-payload-metadata-separation]]", "[[identity-is-definition-owned]]"]
---

# Every writer says how its clock was read; dates stay local dates

**Decision (cards 197 and 198, 2026-10-21).** A fact whose source gives a wall clock carries
`source_time` beside its fact time: `value`, the clock exactly as the source supplied it;
`zone`, the zone it is read in; `offset`, the offset from UTC it had at that moment; and
`basis`, who decided the zone, `source` when the source or the person's client named it,
`configuration` when a setting for that source did, `assumption` when the writer chose it. The
fact time stays the one UTC instant the gate derives. A self-report type requires `source_time`
with an offset, points its zone rule at that offset, and carries the offset as an optional
segment of its id, so one minute said in two zones is two facts. A date stays a local date; it
has no honest UTC day.

## The tension

The record had read every wall clock as Sydney since the first import. Garmin's days follow the
watch: 17 October was a 34-hour flight day and the days after it were London days. Two ways to
be right: store UTC in the payload, or keep the source's clock and say how it was read.

Storing UTC changes a dozen types' time rules, which the gate's guard refuses, and gives a date
a UTC day it never had. It also rewrites what the source said, and the payload is verbatim by
doctrine. Keeping the clock as said and adding how it was read changes nothing already stored,
lets a reader tell a supplied offset from a writer's guess, and moves the self-reports to the
right instant by the offset alone.

## Why the offset is in the id

Identity names what the person said. The same minute said in Sydney and in London are two
different moments, so they are two facts, and the id must say so or the second would supersede
the first. The segment is optional in the template so that types whose source fixes the zone
need not carry it.

## What it cost

Every importer sends `source_time` under its own basis; 7,650 facets were resent in the window
and the hand-entered doses re-entered at their London clocks. Twenty-three bundled types still
read a local clock without requiring it, held in a test list that only shrinks (card 198's
remaining slice); most are date-only and leave by rule, and the rest need their records files
regenerated.
