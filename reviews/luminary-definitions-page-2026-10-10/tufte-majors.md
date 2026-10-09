# Tufte × Majors: card 71, a Definitions page and the inspector review

Before pass, on `~/kanban/anam/cards/document-the-schemas-in-the-explorer.md`, read against `internal/inspect/` and `internal/def/manifests/` as of 66421ea. Severity tags are [must], [should] and [consider].

## Tufte: display

**[must] The data-ink of a schema is one row per field, and the meta belongs on those rows.** Each leaf carries a JSON Pointer, a type, required or not, the constraint that narrows it, a unit, a description and an example. The meta block in `health.blood-pressure.json` is mostly pointers into that same field list. `identity_key: ["/date_excel", "/reading"]`, `fact_time.pointer: "/date_excel"` and `series.key: ["/drug", "/course"]` in `med.started.json` are roles a field plays. Draw them as a role column (id, time, series) on the field's row and the reader sees in one place that `date_excel` is both the clock and half the identity. What stays whole-definition (classification, validation mode, `min_binding_assurance`, `source_schema`, `closes`) fits one line under the type name.

**[must] Key rows by full pointer, flat and indented, not a collapsible tree.** `exercise.session` nests `/effective_time_frame/time_interval/start_date_time`, and the fact-time rule names it by that pointer. Rows keyed by the same string make the rule and the field visibly the same thing. Collapse-to-reveal hides data behind chrome, and 60 flat rows fit a screen.

**[should] Erase the redundant ink the manifests repeat.** Every string carries `"minLength": 1` and every object `"additionalProperties": false` (card 52's lint enforces both). Print each rule once per definition ("closed; strings non-empty"), not on every row. `exercise.session` repeats the `{unit, value}` object at six fields. Name it once as a quantity and show "quantity" in the type column. The `^-?[0-9]+(\.[0-9]+)?$` pattern on BP's `systolic` should read "number as text", which is what it means. The 120-value `activity_name` enum shows its count and the values actually used, the rest behind one link. Refuse the usual viewer chartjunk, which is brace glyphs, rainbow syntax colour, type badges, the `$schema` URI and an arrow on every node.

**[should] Versions as small multiples.** Fields as rows, versions as columns, each cell the type and required mark, a changed cell in the superseded ochre the inspector already uses (`.kind-superseded`). The same grid across types (rows are types, columns are versions, class, identity, fact-time pointer, series, field count, undocumented count) is the index page.

**[should] The undocumented count is the gap made visible.** A number beside each type, and the field rows without a description drawn faint. Write "0 of 15 described", not a red bar, so the lie factor stays at one.

## Majors: inquiry

**[must] The job story starts on a facet, so the answer has to land on the facet page.** "When I am reading a facet... and a field's meaning is not obvious." `facet.html` prints the payload as an indented `<pre>`. Render the payload as the same field rows, value beside description and unit, and the question is answered where it was asked.

**[must] The inspector cannot reach the admitted version today.** `Handler` takes `defs func() *def.Registry`, and `Registry.Lookup` returns one Definition per type, the current one. The facet page shows "definition v{{.Facet.DefinitionVersion}}" as text with nothing behind it. Every version is already held (`definition` and `registered_definition` tables) and already served at `GET /v1/definitions/{type}/{version}`. Hand the inspector that existing accessor. That adds no read path, so it holds to Matt's ruling.

**[must] "Why did this facet supersede" dead-ends.** `facetPage` reads `CurrentVersion` only, and the events table lists artifact versions with no link. `ledger.Version` already exists on the `Ledger` interface. Add `?v=` to `/facet` and a two-column payload view (prior, current) with changed fields marked.

**[should] Every value on the Definitions page is a jump point.** A type goes to `/?type=<type>` (the timeline filter exists in `typeFilter`) and to a filtered facet list. A version links each admitted facet. A field's pointer goes to the facets whose payload carries it. Counts and first and last fact-time per type come from the fold the inspector already caches at W (`s.read`), so the page is a read with no new store. Definitions number 28; facet id, source, source_ref and version are the high-cardinality dimensions, and the page must hand off to them, never summarise them away.

**[should] "Which importer registered this version" is not in the data.** `registered_definition` holds `registered_at` and no registrant. Show "bundled" or "registered at T" and stop. The importer's identity is visible per facet through `Provenance.Source`. Joining sources to types in the fold answers "who sends this type", the real question.

## The inspector review, page by page

- **Timeline `/`.** What is in the record, where in time, at what status. Stays as the landing page. Card 41 stands on its own. On a phone `touch-action: none` on the band swallows vertical scroll.
- **Facets `/facets`.** Every facet in one unpaged table, no filter. It answers no question at 14,000 rows. Merge into a filtered list (`?type=`, `?status=`, `?source=`) that the timeline, Definitions and projection pages link into.
- **Facet `/facet`.** What is this fact and what happened to it. Stays, gains `?v=`, the annotated payload, the version diff, and a link to its Definition at the admitted version.
- **Events `/events`.** The raw ledger in seq order. Keep it as the audit tail, newest first and bounded, with each row linking `/facet?id=&v=`.
- **Projection `/projection`.** What a role would see, as-of. The one page with a free time axis. Stays.
- **Document `/document`.** Plumbing for reference bytes. Stays, not in the nav.
- **Definitions (new).** Joins the nav. Card 21 (checkpoints) belongs on the timeline's compose strip, not here, so leave it standing.

**[must] Phone.** No template has `<meta name="viewport">`, so every page renders at desktop width and shrinks. Add it to `style.html` first. Then five- and six-column tables need a stacked layout under about 40rem, and the field table should drop to pointer, type and description with role and unit folded underneath.

## Convergence

Both lenses put the field row at the centre. Tufte puts roles, units and descriptions on it; Majors puts the payload value on it and links every cell out. Tufte's version comparison and Majors' "why superseded" fail on the same gap, the missing `?v=` and older Definitions.

## Tension

The card's "To work out" item is exactly the split. Tufte's surface is a document, dense, stable, and printable as generated markdown under `docs/`. Majors' surface is a read, with counts, first and last fact-time and jump points into the live fold, which a static file cannot carry. Decide which the page is. My reading of the job story is a read in the inspector, with a generated markdown reference as a separate artifact for readers outside it.

## Recommendations

1. Viewport meta in `style.html`.
2. Fill `description`, `examples` and a unit in the manifests (they are standard JSON Schema keywords, so check whether card 52's lint accepts them before assuming no schema change). The undocumented count follows.
3. Pass the inspector the existing all-versions definition accessor.
4. `/facet?id=&v=` with a prior-current payload diff.
5. Facet page payload as annotated field rows, linking to `/definitions/{type}/{version}`.
6. Definitions index as one type-by-attribute grid, detail page as the pointer-keyed field table with role column and version small multiples.
7. Replace `/facets` with a filtered list. Make `/events` newest first and bounded.
