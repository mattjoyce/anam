# Card 71 design panel: collation

Card 71 asks for a Definitions page in the inspector, field-level documentation in the Definitions
themselves, and a written review of the inspector. Matt asked for the design to channel the
luminaries and to borrow from established JSON viewers. Two pairs ran a Before pass: Naur × Procida
(theory × need, the authoring pass) and Tufte × Majors (display × inquiry). A third agent surveyed
JSON and JSON Schema viewers and how FHIR and Open mHealth present schemas. The question: what
should a person see when they ask "what does this field mean", and where do the words live.
Matt's standing rule frames all of it: the inspector is a diagnostic tool and yields to the design.

## Verdicts

| Lens | Axis | One-line verdict |
|---|---|---|
| **Naur × Procida** | theory × need | A description is part of the interpretation and versions with it; it describes what the source wrote, never a claim about the world. |
| **Tufte × Majors** | display × inquiry | One row per field keyed by its JSON Pointer, with identity, fact-time and series as roles on the row, and the facet page answering the question where it is asked. |
| **Prior art** | survey | FHIR's structure table, a table that is also a tree, with cardinality, type and description columns and the instance value beside it, is the model; graphs and hover are not. |

## The shared meta-finding

**The field row is the unit of the design.** Tufte reaches it from data-ink (the meta block is
mostly pointers into the field list, so draw the pointers as roles on the rows). The survey reaches
it from prior art (FHIR's Structure tab, Swagger and Open mHealth pairing schema with instance).
Majors reaches it from the job story (the question starts on a facet, so its payload should render
as the same rows with the value beside the description). Naur reaches the same place from the other
side: the words belong to the field and travel with the Definition. One table, keyed by full
pointer, serves the Definition page, the facet page and the version comparison.

## Where the lenses converge

**C1 🔴 The field table.** Tufte × Majors, prior art. Columns: pointer (indented under its parent
with CSS connector lines, no collapsing), cardinality (`1..1`, `0..1`, `0..*`, after FHIR), type
with unit (`number · mmHg`), role badges (identity, fact time, series), description, example. On a
facet page an extra column holds the facet's value. Under about 40rem a row stacks to pointer, type
and description with the rest folded beneath. Fix: one template partial in
`internal/inspect/templates/`, used by the Definition and facet pages.

**C2 🔴 Words are versioned, and the admitted version is reachable.** Naur, Majors. Verified in the
code: registered versions are immutable by canonical body (`internal/def/register.go`), and every
bundled version Anam has seen is kept in the operational store's `definition` table, which the
daemon already serves at `GET /v1/definitions/{type}/{version}` (`internal/daemon/definitions.go`,
`heldDefinitions`). Naur's worry that an old bundled version is gone is answered by that table. His
rule stands for a different reason: the table is keyed `(type, version)`, so new words at an
unchanged version would hide the old ones. Fix: every change to a description is a version bump,
batched; the inspector is handed the existing held-definitions lookup in place of the current-only
registry (no new store, no new read path, within the card 65 ruling).

**C3 🟠 Say a thing once.** Naur (theory once), Tufte (ink once), prior art (FHIR's named reuse).
Concepts every type shares (fact time, identity, series, verbatim, absent is not zero) live once in
`docs/GLOSSARY.md`, and the page links to them. Rules every field repeats (`minLength: 1`,
`additionalProperties: false`, the numeric-string pattern, the `{unit, value}` shape) are stated
once per Definition or shown as a named badge.

**C4 🟠 Descriptions and examples have rules.** Naur × Procida, prior art. A description says what
the source called the field, its unit or precision as written, whether the value is the source's
lexeme or derived (and by whom), and what the schema cannot express. Examples are synthetic, in
the source's exact type and layout (`"128"`, not `128`, for bp-csv), never drawn from the ledger.
Units sit in their own slot.

**C5 🔴 Phone.** Tufte × Majors, prior art. No inspector template sets
`<meta name="viewport">`, so every page renders at desktop width. Fix: add it to `style.html`,
then stack wide tables under a breakpoint.

## The genuine tensions

**T1. Which words does a facet show: the ones in force when it was admitted, or today's best?**
Naur holds the first: a facet was read under an interpretation, and that is what it means.
Procida holds the second: Matt, debugging now, wants the clearest words. C2 makes both cheap,
because both versions are held. The choice is which one the facet page leads with. Recommendation:
lead with the admitted version and, when a newer one exists, say so on one line with a link
("admitted under v2; v3 is current").

**T2. Is the Definitions page a document or a read?** Tufte's pole is a dense, stable reference that
could also be generated as markdown under `docs/`. Majors' pole is a read: counts and first and last
fact time per type, and every value a jump point into the timeline and facet lists. Naur × Procida
side with the document and say counts belong on the facet index. Recommendation: a reference page
with jump links (type to the timeline filter and a facet list, version to its facets) and no counts.
Links cost nothing and need no read; counts make it a report. Generated markdown waits for a reader
outside the inspector and the API.

## Single-lens signals

- **Naur.** The verbatim line. `health.blood-pressure` holds `systolic` as text because the export
  wrote text; "systolic pressure in mmHg" would read as Anam's claim. `lab.result` has `value` and
  `value_num` side by side and nothing says which is the lexeme.
- **Procida.** The "write and register a Definition" how-to belongs in `docs/IMPORTERS.md` §8.3, not
  on the page.
- **Tufte.** Versions as small multiples: fields down, versions across, changed cells in the
  inspector's existing superseded colour. The same grid across types is the Definitions index. A
  120-value enum shows its count and the values in use.
- **Majors.** `/facet` cannot show an older artifact version, so "why did this supersede" dead-ends;
  add `?v=` and a prior-and-current payload view. `/facets` lists every facet unpaged and answers
  nothing at 14,000 rows; make it a filtered list. Nothing records which importer registered a
  Definition; show "bundled" or "registered at T".
- **Prior art.** Show the fact-time value parsed in its declared zone next to the raw string. Keep
  the raw Definition JSON one click away in a `<details>`. A small vanilla filter box over paths
  and descriptions. Avoid node graphs, one card per property, and hover-only information.

## Found outside the panel

The Withings importer ships `health.measurement.json` identical to Anam's bundled copy. Documenting
that type is a version bump on both sides at once, coordinated with the importers stream. Card 54
already plans to move the Definition to the importer's folder; documenting it there, after the move,
avoids the double edit.

The lint walks every key under a schema, `examples` included. An example payload that itself holds
a key named `properties` or a `type: object` could be misread as a subschema. A test should pin
that annotations pass the lint before any manifest gains them.

## Build verdict

Ship in slices, each leaving the inspector working:

1. **Read and render.** Viewport meta. The inspector gets the held-definitions lookup. A Definitions
   index (the type-by-attribute grid) and a Definition page (the field table, roles, raw JSON, the
   undocumented count, version columns). The facet page links to the admitted version. No manifest
   changes yet, so every count reads "0 of N described", which is the gap made visible.
2. **Write the words.** Descriptions, units and synthetic examples in the bundled manifests, one
   version bump per type, batched; the lint test first. `health.measurement` waits for card 54's
   move. The how-to goes into IMPORTERS.md.
3. **The facet page.** The payload as field rows with descriptions, `?v=` for any artifact version,
   and the prior-and-current view.
4. **The inspector review**, written from the page-by-page findings above. The `/facets` and
   `/events` changes it implies become their own card.

Decisions for Matt before slice 1: T1 and T2.
