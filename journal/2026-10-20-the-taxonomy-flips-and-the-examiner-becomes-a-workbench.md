---
type: journal
date: 2026-10-20
tags: [anam, journal, taxonomy, rename, query, receipts, workbench, links]
---

# 20 October: the taxonomy flips, and the Examiner becomes a Workbench

The day the record changed its names. 6,868 live facets moved to `domain.concept` types in one
ledger, by supersession, with anam time kept, and the pages that read them never noticed.

## The gates and the run

Matt approved G1, the eight domains and the map, and G2, the scoped disposition that lets the
gate carry a facet from one type to another without a writer. Card 140's run was already built
and waiting: `Rehearse` and `RenameBatch` on the gate, `anam rename --table <json> [--yes]` with
a dry run, a keying check and per-facet refusals. Two prerequisites landed first: card 176, so
a manifest may declare its kind and family in `meta` rather than in the inspector's table, and
card 139's first piece, the three Garmin concepts bundled in core with a same-body registration
answered as unchanged.

The rehearsal on a copy carried 816 facets with none refused. The full run on a current copy
carried all 28 pairs, 6,867 facets, reads before and after identical but for type and version.
The live flip carried 6,868 at W 40842. Projections re-registered under the new names, the
records files retyped, importers resumed and polled clean. Two operator lessons went on the
pipeline page: `enabled: false` stops schedules and not routed jobs, and a tree swapped before
its lock dies on a fingerprint mismatch.

Then two of six old receipts stopped verifying. The first reading blamed the template; the
cause was `Registry.Closes` not knowing a retired type had closed its series. Card 187, fixed
and deployed as v0.401 the same evening, all six verifying. The after-audit of the two days cut
cards 184 to 186: a rename as a fact in the provisioned tier, the gate refusing to carry a dead
facet, and the run's own catch-up and rollback.

## Links and the query

Card 165 went live: `record.link`, a file Anam remembers but does not hold, from a links
importer over an Obsidian vault on the Library share. Card 173 finished: introspection, the
query receipt with verify, `POST /v1/query`, the anam-mcp tools, and a Soufflé differential that
agrees with the evaluator through two projections at every instant. The equality check on a
copy of the live ledger agreed at all 6,702 fact-times. What the oracle could not settle went to
card 182.

## The Workbench

Matt's direction: the Examiner becomes the Anam Workbench and enters facets from forms drawn
from the Definitions. "Diagnostic, never writes" is retired. The Before review raised thirteen
findings and three decisions, and Matt took all three: the entries are source `workbench`, a
single facet of a source nobody resends can be retracted from its page, and a type's id rule is
declared in its Definition as `identity_template`. Four slices built and merged through the day
and deployed as v0.415, with a standing instruction recorded on the way: merge gated slices
without asking.

## Hickey × Armstrong

**Hickey.** A type name is a value the facet carries, so a rename is a new version naming the
new value, never an edit of a place. That is what let the flip keep anam time and let old
receipts re-verify once the series rule was fixed.

**Armstrong.** The kernel took the run through its own gate, with a disposition scoped to one
operator act. The failure that did appear was in the derived tier, a registry that forgot a
closed series, and it was fixed and redeployed inside an hour with the record untouched.

## Left open

- Card 182, items 1 to 4.
- Card 188 stays in doing until Matt has entered a fact through the form.
- Cards 183 to 186.
