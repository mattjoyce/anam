---
type: component
title: Selection (internal/selection)
description: "The shared filter: fact-time from and to, type or prefix, source, standing, and facets ticked or unticked by hand, as one Go value that compiles to a Datalog pick/1 program and parses back only when the program is one it writes."
timestamp: 2026-10-21T00:00:00Z
resource: internal/selection/program.go#Program
anchor: "func (f Filter) Program() (string, error) {"
basis: code
tier: detail
group: derived
depends_on:
  - /components/facet.md
  - /components/fold.md
  - /components/read.md
---

Card 199, 2026-10-21. `selection.Filter` holds the criteria a person adds in any order and the picks they make by hand: `From` and `To` as fact-time lexemes (`From` included, `To` not, `Days` turns two dates into the bounds of whole UTC days), `Types` as type patterns (an exact type or a prefix ending in `.`, card 193), `Sources`, `Standing` (`live` by default, or `superseded`), `Ticks` and `Exclusions` by facet id. It is a set: `Canonical` sorts each list, drops repeats and drops an exclusion that is also a tick.

The filter has two readings, and `TestEquality_ProgramIsThePredicate` holds them equal over a real ledger, through the canonical and through the `bp` projection:

- **The direct predicate.** `Matches(Row)` asks the criteria alone; `Picks(rows)` is the selection, the rows the criteria match minus the exclusions, plus every tick. A filter with no criteria picks its ticks and nothing else, not the whole record.
- **The program.** `Program` writes Datalog over the query surface's `entry/7` that defines `pick/1`. Each pair of a type and a source the filter names is one rule, a conjunction of the criteria, so the rules' union is "any of these types, any of these sources". A prefix is a pair of lexical bounds, `T > "activity.exercise-session."` and `T < "activity.exercise-session/"`, which is `facet.Matches`'s own rule, as the bundled edge rules bound a pointer. The standing `superseded` is `St != "admitted", St != "endorsed"`; `live` writes nothing, since a served value holds only versions in force. A tick is the fact `pick("<id>")`; an exclusion is the fact `unpick("<id>")`, which each rule reads as `not unpick(E)`. The program reads no `attr`, so it never names a pointer.

`Parse` reads a program back to the filter only when it is one `Program` writes, rule for rule: layout, comments and the order of the rules do not matter, anything else does. It extracts a candidate filter, writes that filter's program, and compares the two sets of rules; a join, a recursion, an `attr`, a constant in an `entry` slot or a bare date is `ErrNotChips`, and the error names the line. `Parse(f.Program())` is `f.Canonical()` for every filter `Validate` accepts (`TestRoundTrip`).

The consumers resolve the program differently. The inspector's selection page draws the criteria from it and runs it through `read.Evaluate`, the evaluation `POST /v1/query` makes, without a receipt. The projection page's rows view (card 201) selects its type, window and standing by `Matches`. The link form (card 202) will resolve it to ids once, when an edge is saved, and migrate (card 200) will keep a query receipt beside its run.
