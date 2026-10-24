---
type: journal
date: 2026-10-21
tags: [anam, journal, time, zone, types, edges, picker, rename, deploy]
---

# 21 October: the fact names its zone

The longest day of the week: seventy-eight board lines, five deploys, and the rule for time that
the record had been missing since the first import.

## An edge, not a note

Card 104's note became an edge facet: `kind`, `from` and `to` as lists, optional text, a version
pin. Links are derived beside a facet's payload in its response, never inside it, and the links
part states its own watermark. Built with card 91, deployed as v0.437 with the operational store
at schema 8. By the evening Matt had decided the name follows the thing: `record.note` became
`record.edge` across the three repositories (card 191). Coverage, paged reads with the MCP
reading the page, and the overview diagram rode the same deploy.

## Where a distinction is encoded

A type review with Matt, and his conversation with Codex, produced cards 192 to 197. The
sentence that came out of it: a type is `domain.concept[.specialisation]`, and a vendor's own
shape takes the vendor leaf. So `activity.exercise-session` split into four leaves, one per
source, and Garmin's day became `activity.day.garmin`; the medication, illness and trip types
took a third segment; a projection may name a prefix with a trailing dot; and the rename table
learned to split a type by a selector. Three live runs followed. Two of them met the automated
operating session's refusal of `rename --yes` as "blind apply" after a dry run that met Matt's
condition, and the runs finished with Matt present: 4,456 Garmin versions, 71 facets, then 8,742
facets at W 54129. The lesson went on the runbook: run `--yes` alone, never chained to a second
dry run, and plan for it to need Matt in the session.

## How a clock was read

Card 197 started from Garmin's days: the 17th was a 34-hour flight day and the days after it
were London days, and the record had been reading every wall clock as Sydney. The design is
`source_time` beside the fact time: the value as the source supplied it, the zone it is read in,
the offset it had at that moment, and who decided the zone, `source`, `configuration` or
`assumption`. The fact time stays the one UTC instant the gate derives. Card 198 carried it to
every writer, and Matt made the decision the self-report types needed: every writer sends an
offset, dates stay local dates, and a type whose fact can name its zone does so through a
`zone_pointer`, with the offset an optional segment of the id so one minute in two zones is two
facts. Deployed as v0.482 with the importers and anam-mcp together, 7,650 facets resent, the
hand-entered doses re-entered at their London clocks, W 61823.
[[every-writer-says-how-its-clock-was-read]] holds the decision.

## The picker becomes a program

Card 199 began as a picker of additive criteria for linking facets by hand. By the end of the
day Matt had made it something else: the filter is Datalog underneath, a unary relation over the
query surface; the program is the value and the chips and ticks are a view of it; migrate (card
200, redirected from a fresh ledger to moving what a filter matches) takes the same filter.
Slices one and two merged. Card 201 opened the projection page on a summary, 13 seconds and
18 MB down to 0.1 seconds and 20 KB, measured live on v0.462.

## Hickey × Armstrong

**Hickey.** `source_time` keeps the source's clock as a value beside the derived instant rather
than rewriting either. The id carrying the offset is the same thought: identity names what the
person said, including where they said it.

**Armstrong.** Five deploys in a day held because each was a plain code deploy or a rename the
gate could rehearse, and a refusal mid-run left the record behind, never wrong. The admission
hold (card 204) was proposed in the window as the kernel's own way to pause a source, in place
of pausing importers in the gateway.

## Left open

- Card 198's ratchet list: the types that still read a local clock without saying how.
- Card 199's third slice, labels and the entry points.
- Card 200, the cleansing migration, once the shapes settle.
