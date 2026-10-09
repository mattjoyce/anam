# Prior art: making JSON and JSON Schema readable

Survey for the Definition page (meta block, closed draft 2020-12 payload schema, and a facet instance beside it). Researched 2026-10-10. The real shapes matter here. `health.blood-pressure` is fifteen flat string fields, six carrying the same numeric `pattern`. `exercise.session` is about forty fields, a 120-value `enum`, a `{unit, value}` object repeated six times, two arrays of objects, and a fact-time pointer three levels deep.

## Editor tools

VS Code's built-in JSON support does the most with the least. Hovering a key shows the schema's `description`, and a VS Code-only `markdownDescription` allows links. Every object and array gets a fold region, and Go to Symbol jumps to properties (https://code.visualstudio.com/docs/languages/json). Breadcrumbs show "the symbol path up to the cursor position", drawn from the same symbols as the Outline view (https://code.visualstudio.com/docs/editing/editingevolved). The idea worth taking is that the instance stays as text and the schema's prose and path ride along with it. Hover does nothing on a phone, though, and validation squiggles need a live validator.

Tree-view extensions (https://github.com/sagarmetaverse/json-tree-viewer-vscode, https://dev.to/hugohbrt/i-built-a-vs-code-extension-to-visualize-json-as-tree-diagrams-5g7b) add a collapsible, colour-coded tree and copy a node's path. Formatters like Prettify JSON and Better JSON only indent; I found no primary source for them, and they teach nothing `json.MarshalIndent` does not. The JSON Hero extension just opens jsonhero.io (https://github.com/triggerdotdev/vscode-extension).

## Browser viewers

Firefox's JSON viewer is the closest model for a diagnostic page. It has collapsible objects and arrays, a filter box, Raw and Pretty Print views, and response headers (https://firefox-source-docs.mozilla.org/devtools-user/json_viewer/index.html). The filter is the useful part, and it is twenty lines of vanilla JS over server-rendered rows. Opening nodes collapsed hides a flat object behind a click for no gain.

JSON Hero (https://github.com/triggerdotdev/jsonhero-web) offers a Finder-style column view with a path bar, content previews that recognise dates, URLs and colours inside strings, and "related values" listing every value one key takes. Previews would make `date_excel` or `start_date_time` readable at a glance. Column view fails at phone width, and the app is a heavy React SPA.

JSON Crack (https://github.com/AykutSarac/jsoncrack.com) draws JSON as a node graph with a configurable node limit. Forty sibling scalars become a fan of boxes that says less than a list. It looks good in a screenshot and gets in the way of reading.

## Schema documentation renderers

json-schema-for-humans (https://github.com/coveooss/json-schema-for-humans) is the closest prior art to an HTML schema page. Each property is an accordion header with yellow "Required", red "Deprecated" and blue "Pattern Property" badges. Each section carries a clickable breadcrumb from `root`, restrictions are spelled out in prose ("Must be at least `1` characters long"), and an optional Expand all / Collapse all pair sits on top. Its `flat` template drops collapsing and JavaScript entirely. The README warns that this "may make it hard to understand the schema structure". The default `js` template loads jQuery, Bootstrap, Font Awesome and Google Fonts from CDNs (https://github.com/coveooss/json-schema-for-humans/blob/main/json_schema_for_humans/templates/js/base.html); `js_offline` bundles them. One card per property makes a 40-field schema very tall, and a repeated `pattern` spelled out six times is noise.

adobe/jsonschema2md (https://github.com/adobe/jsonschema2md) emits Markdown. A summary table with columns Property, Type, Required, Nullable and Defined by sits above one section per property (https://github.com/adobe/jsonschema2md/blob/main/examples/docs/complex.md). The summary table is the right density. The per-property sections repeat everything in the table, and the header block (Abstract, Extensible, Status, Identifiable, Can be instantiated) is mostly "Unknown" or "No". That is metadata about the schema language, not the data.

Stoplight's json-schema-viewer (https://github.com/stoplightio/json-schema-viewer), used by Stoplight Elements, renders nested objects "to any depth", collapsible, with validations, Markdown descriptions and a `defaultExpandedDepth`. Redoc (https://github.com/Redocly/redoc) uses three panels (navigation and search, schema, examples) and can emit one static HTML file. Swagger UI shows each model as either "Example Value" or "Model", toggled by links, with `defaultModelRendering` choosing the start (https://swagger.io/docs/open-source-tools/swagger-ui/usage/configuration/). Pairing schema with instance is the right instinct, but a toggle hides one behind the other and defeats comparison. Redoc's panels stack on narrow screens, losing side-by-side anyway. All three are React bundles.

Docson (https://github.com/lbovet/docson) is draft-4 only, and its README admits `patternProperties`, `additionalItems` and non-primitive enums are unimplemented. Atlassian's json-schema-viewer (https://github.com/atlassian-labs/json-schema-viewer) is a draft-07 React SPA whose distinctive idea is "examples describing the JSON you should expect at every level of the hierarchy". json-schema.app returned only a title to my fetch (a client-rendered shell), so I draw nothing from it.

## Health standards

FHIR's StructureDefinition pages, for example Observation (https://hl7.org/fhir/R4/observation.html), are the best prior art here for a dense schema. The Structure tab is one table that is also a tree. Its columns are Name, Flags, Card., Type, and Description & Constraints. Nesting is drawn with small vertical-line and join images in the Name cell (`tbl_vline`, `tbl_vjoin`, `tbl_vjoin_end`), so the hierarchy reads without any collapsing. Cardinality is `0..1`, `1..1` or `0..*`, which says required and repeating in one token. Coded elements show their value set with binding strength in parentheses (required, extensible, preferred, example). Flags are single glyphs (`Σ`, `?!`, `I`) explained in a legend on another page (https://hl7.org/fhir/R4/formats.html). The page also offers UML, XML, JSON and Turtle templates, since the table gives "a logical tree" and pseudo-instances give "a visual sense of what instances will look like". The faults are opaque glyphs without the legend, and five views where a diagnostic page wants one.

Open mHealth keeps every field's unit in the data as a `{unit, value}` pair. Each measure narrows that pair with an `enum`, so `systolic-blood-pressure-1.1.json` is `allOf` a shared `unit-value` plus `"unit": {"enum": ["mmHg"]}`. Each schema also links a SNOMED code through a `references` array (https://github.com/openmhealth/schemas, see `schema/omh/`). Example instances sit next to the schemas as `test-data/<schema>/<version>/shouldPass` and `shouldFail` files. An IEEE P1752 deck says the library shows "schemas and sample data on the same page" (https://sagroups.ieee.org/1752/wp-content/uploads/sites/277/2020/01/2020-01-14-1752-schema-slides.pdf), and the OMH explainer reads schema first, then sample data (https://www.openmhealth.org/mhealth-schema/). The live library is a hash-routed SPA I could not fetch, so that layout claim is secondhand. The weakness is that "mmHg" sits two `$ref` hops away, invisible unless a renderer resolves it.

## What works without a framework, and on a phone

Static HTML handles a table-tree with CSS connector lines, badges, breadcrumb paths, row anchors, `<details>` collapsing with no script, a vanilla-JS filter, and CSS-grid panes that stack below a breakpoint. Hover-only descriptions, Finder columns, node graphs and three-panel layouts need a framework or fail on a phone. A table past four columns overflows a phone; FHIR scrolls sideways, and stacking each row into a card under about 600px reads better.

## Ideas to borrow, ranked

1. **One table that is also a tree** (FHIR Structure tab). Columns: field, cardinality, type, description and constraints. Indent nested rows under `effective_time_frame` and `sets[]` with CSS connector lines. No collapsing is needed for these sizes.
2. **Cardinality instead of a "required" column** (FHIR). `1..1`, `0..1`, `0..*` fold `required` and `type: array` into one token. Show `additionalProperties: false` once, as "closed".
3. **Schema and instance side by side, aligned by path** (Swagger UI's example/model pair, OMH's "schemas and sample data on the same page"). Put the instance value in its own column of the same table so a row reads field, rule, actual value. Stack it under the rule on a phone.
4. **Mark the meta pointers on the rows they point at** (VS Code breadcrumbs, json-schema-for-humans breadcrumbs). Badge the row that `/effective_time_frame/time_interval/start_date_time` resolves to as "fact time", and the `identity_key` rows as "identity". The meta block then reads as annotations, not a separate puzzle.
5. **Units in their own visible slot** (Open mHealth `unit-value` with an `enum` of units). When units arrive, render them beside the type (`number · mmHg`), and render the six repeated `{unit, value}` objects as one named shape, not six subtrees.
6. **Name a repeated constraint once** (FHIR's "see [name]" reuse). The `^-?[0-9]+(\.[0-9]+)?$` pattern on six blood-pressure fields becomes a "numeric string" badge with the regex in a footnote.
7. **Collapse long enums with `<details>`** (json-schema-for-humans' collapsed long descriptions, FHIR's value-set link). Show the count ("enum, 120 values") with the list behind a summary, and highlight the instance's value.
8. **A filter box over row paths and descriptions** (Firefox JSON viewer, JSON Hero search).
9. **Content previews for time strings** (JSON Hero). Next to the fact-time value, show the parsed instant in the declared zone using the declared layout.
10. **Raw JSON one click away** (Firefox Raw tab, FHIR's JSON template). A `<details>` with the exact Definition bytes, for copy and for trust.

## Ideas to avoid

1. **Graph or node-diagram views** (JSON Crack, JSON Diagram Viewer). Flat objects turn into a fan of boxes, and they need a canvas library.
2. **One expandable card per property with everything repeated in prose** (json-schema-for-humans `js`, jsonschema2md per-property sections). Too tall for 40 fields, and it duplicates the summary table. jsonschema2md's schema-language metadata block (Abstract, Extensible, Identifiable) is the same mistake.
3. **Hover-only or collapsed-by-default information** (VS Code hover, Firefox's collapsed nodes, Swagger's model/example toggle). It fails on a phone and hides the comparison the page exists for. Keep descriptions inline and keep both schema and instance visible.
