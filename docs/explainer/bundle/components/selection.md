---
type: component
title: Selection (internal/selection)
description: "The shared filter: fact-time from and to, type or prefix, source, standing, and facets ticked or unticked by hand, as one Go value that compiles to a Datalog pick/1 program and parses back only when the program is one it writes; and the store that saves a program under a short code."
timestamp: 2026-10-22T00:00:00Z
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

Card 199, 2026-10-21. `selection.Filter` holds the criteria a person adds in any order and the picks they make by hand: `From` and `To` as fact-time lexemes (`From` included, `To` not, `Days(from, to, loc)` turns two dates into the bounds of whole days in the reader's zone `loc`, UTC when it is nil, by `clock.Day`, the rule every bare date follows, card 236), `Types` as type patterns (an exact type or a prefix ending in `.`, card 193), `Sources`, `Standing` (`live` by default, or `superseded`), `Ticks` and `Exclusions` by facet id. It is a set: `Canonical` sorts each list, drops repeats and drops an exclusion that is also a tick.

The filter has two readings, and `TestEquality_ProgramIsThePredicate` holds them equal over a real ledger, through the canonical and through the `bp` projection:

- **The direct predicate.** `Matches(Row)` asks the criteria alone; `Picks(rows)` is the selection, the rows the criteria match minus the exclusions, plus every tick. A filter with no criteria picks its ticks and nothing else, not the whole record.
- **The program.** `Program` writes Datalog over the query surface's `entry/7` that defines `pick/1`. Each pair of a type and a source the filter names is one rule, a conjunction of the criteria, so the rules' union is "any of these types, any of these sources". A prefix is a pair of lexical bounds, `T > "activity.exercise-session."` and `T < "activity.exercise-session/"`, which is `facet.Matches`'s own rule, as the bundled edge rules bound a pointer. The standing `superseded` is `St != "admitted", St != "endorsed"`; `live` writes nothing, since a served value holds only versions in force. A tick is the fact `pick("<id>")`; an exclusion is the fact `unpick("<id>")`, which each rule reads as `not unpick(E)`. The program reads no `attr`, so it never names a pointer.

`ProgramIn(zone)` is `Program` with one comment line under the header, `% dates read in Australia/Sydney`, when the filter has a bound and the zone is an IANA name (card 236). The bounds are instants and mean the same in any zone; the line records what the dates meant to the person who chose them, and `ZoneOf` reads it back (empty for a program that names none, or a zone this machine cannot load, or `Local`). `Program` writes none.

`Parse` reads a program back to the filter only when it is one `Program` writes, rule for rule: layout, comments and the order of the rules do not matter, anything else does. It extracts a candidate filter, writes that filter's program, and compares the two sets of rules; a join, a recursion, an `attr`, a constant in an `entry` slot or a bare date is `ErrNotChips`, and the error names the line. `Parse(f.Program())` is `f.Canonical()` for every filter `Validate` accepts (`TestRoundTrip`).

The consumers resolve the program differently. The inspector's selection page draws the criteria from it and runs it through `read.Evaluate`, the evaluation `POST /v1/query` makes, without a receipt. The projection page's rows view (card 201) selects its type, window and standing by `Matches`. The link form (card 202) resolves it to ids once, when it opens, and migrate (card 200) will keep a query receipt beside its run.

## Saved selections and their codes

Since 2026-10-22 (card 206) `selection.Store` (`saved.go`) saves a program under a short code, over the operational store's `Put`, `Get` and `List`. `CanonicalText` is what a program is saved and hashed as: `ProgramIn` of what `Parse` reads off it and the zone `ZoneOf` reads off it, when it is within the grammar (so the zone line survives a save), so another layout or rule order is the same selection, and otherwise the text trimmed. `Save` refuses text that is empty or does not parse (`ErrNotProgram`) and files the canonical text as an artifact of kind `selection`, addressed by its SHA-256. The code is the first `CodeLen` (8) hex digits of that hash, written `sel:<code>`.

When another saved selection already shares those 8 digits, the new code lengthens to the shortest prefix of its hash no other saved selection shares, and the lengthened code is filed first, as a small JSON row of kind `selection-code` (`{"code", "hash"}`): which of two colliding selections came first is a fact the hashes cannot say, so it is written down once. The first keeps its 8 digits. Saving again answers the code already given and writes nothing new. `Open` resolves a code: a filed code names its selection; 8 digits name the one saved selection starting with them that did not lengthen; a longer code that was never filed is read as a prefix of a hash. No match is `ErrNoCode`, two is `ErrAmbiguous`, and a string that cannot be a code is `ErrBadCode`. The test finds two tick programs whose hashes share `f7c90470` and holds that the later one's code is `f7c90470d` and both still open.

A code names the program, not its results: as the record grows the same code may select more. The rows are authored, not derived, so a backup carries them with the receipts; a selection's bytes are its Datalog text.
