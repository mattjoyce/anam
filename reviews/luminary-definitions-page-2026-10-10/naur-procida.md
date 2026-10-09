# Naur × Procida on card 71, the Definitions page

Authoring pass (docs → reader) on card 71, read against `internal/def/`, four manifests, `docs/API.md` §4, `docs/IMPORTERS.md` §8.3 and `docs/GLOSSARY.md`.

Severity: must-fix, should-fix, consider.

## Naur: the theory a reader needs before a field means anything

**must-fix. Words in the manifest are versioned words.** The card says the text lives in the schema's `description` and `examples` keywords and asks "whether that needs a schema change or only filling in the manifests." The code answers. A registered version is immutable by canonical-JSON body (`register.go` `sameBody`, IMPORTERS §8.3), so a corrected description is a 409 at the same version and must be a new version. A bundled edit changes `SetVersion()`, the hash a receipt records "so that a served value can be read back years later alongside the interpretation that was in force." Bundled types hold one version each (`Registry.byType`), so a facet admitted under `health.blood-pressure` v1 links to a version no binary holds. The theory to write down is that a description is part of the interpretation and moves with it. Fixing a typo is a version bump. The card's acceptance criterion "links to its Definition at the version the facet was admitted under" will 404 for every bundled type that has moved, and the card should say what the link does then.

**must-fix. The verbatim line is the theory a field description most often breaks.** ARCHITECTURE §3 says the payload is "verbatim, immutable, ... never parsed into a canonical schema". `health.blood-pressure` stores `systolic` as a string with a number pattern because the source wrote a string. A description of "systolic pressure in mmHg" reads as Anam's claim about the world. It must read as a description of what the source wrote: "the systolic value as the bp-csv export printed it; mmHg by the device, not checked by Anam." `lab.result` carries both `value` (string) and `value_num` (number) and `flag_as_printed` beside `flagged`. Nothing on the page today says which is the lexeme and which is derived, by whom, and that is exactly what a reader cannot work out from the schema.

**should-fix. Absent is not zero, and the schema cannot say it.** `filledSchema` enforces "a measurement the source left blank is not a measurement taken" for required fields. For the many optional fields (`pulse`, `weight_kg`, `avg_hr_bpm`) absence is silent. One sentence of theory, stated once, covers all 28 types. It must not be repeated in every field.

**should-fix. `meta` has no prose home, and should not get one per type.** Identity key, fact-time pointer and zone, series family and `closes` are the theory of how a facet is keyed and placed (GLOSSARY "Fact-time", "Series", identity at line 247). The page should render each type's values and link the concept to the glossary. Explaining "why identity never comes from the row's values" in 28 manifests means 28 drifting copies of one theory.

**consider.** The undocumented-field count is a fair residue test. It shows where nobody wrote the theory down, not where the words are wrong.

## Procida: four readers, four needs

**must-fix. The card has one page serving three needs.** Matt in the explorer, mid-diagnosis, wants **reference**: what this field is, its unit, an example, one screen. An importer author wants a **how-to**: write a Definition that registers, keys and places a facet. Anyone new wants **explanation**: why closed schemas, why verbatim, why a description bump is a new version. The acceptance criteria list per-type facts (identity key, fact-time rule, series, payload schema, where facets appear) as reference, which is right. The risk is the "To work out" item that adds facet counts and first and last fact-time. That turns a reference page into a read of the record, a report. Keep the Definitions page austere reference, and leave counts on the facets index where they already belong.

**should-fix. Field descriptions are reference, so keep them boring.** A good one says what the source called it, its unit or precision as written, whether it is the source lexeme or a derived value, and a constraint the schema cannot express. It does not teach the domain or tell an importer what to do.

**should-fix. Examples must be synthetic and shaped like the source.** `examples` sits inside a manifest that ships in the binary, is served by `GET /v1/definitions`, and is copied into importer repos. The value of a real reading, a drug name from `med.started` or a lab number is Matt's data leaking into an artifact with a different audience. Use plausible synthetic values in the source's exact format, `"128"` not `128` for bp-csv, and `"2025-5-3 07:12:00"` matching the layout in `time_semantics`.

**should-fix. The how-to belongs in IMPORTERS.md, not the page.** §8.3 is a reference table. A short "write and register a Definition" recipe (closed objects, required-means-filled, keyed fields required, descriptions and examples, bump the version to change words) belongs beside it. The explorer page can link to it.

**consider. Generated markdown under `docs/`.** Each reader already has a surface: the explorer, `GET /v1/definitions`, the importer's repo. A generated `docs/DEFINITIONS.md` adds a fourth surface that can go stale against registered types it never sees.

## Where they converge

- The manifest is the single source for field words, and both `/v1/definitions` and the page render it. Generated markdown is optional and derived, never edited by hand.
- Concepts shared by every type (fact-time, identity, series, verbatim, absent-not-zero) live once, in GLOSSARY and ARCHITECTURE, and the page links to them. They never go in a `description`.
- Examples are synthetic.

## Where they pull apart

Naur wants the version history visible: a facet links to the exact words in force when it was admitted, because that is the interpretation it was read under. Procida wants Matt, debugging today, to see the best current words. With one bundled version held, both cannot be served from the registry. Decide this, do not average it. One option keeps the link to the admitted version number, renders the current manifest, and flags the difference ("admitted under v1; showing v2"). No store of old manifests is added.

## Recommendations for the card

1. Answer the "schema change or fill in" question in the card. No lint change is needed (`closed` and `filledSchema` ignore annotations; confirm with a test), but every documented type takes a version bump and moves `SetVersion`. Say so, and batch the bumps.
2. Add the fallback for an admitted version that is not held: link the number, show current, mark the mismatch.
3. Add a field-description rule to the card: source name, unit or precision as written, lexeme or derived, and what the schema cannot express. One or two sentences. Domain teaching stays out.
4. Examples: synthetic, in the source's exact type and layout, and never drawn from the ledger.
5. Use the top-level schema `description` for the one-line "what one facet of this type is". Link meta concepts to GLOSSARY anchors.
6. Drop ledger counts from the Definitions page and keep it manifest-only, per the card 65 ruling.
7. Write the importer how-to in IMPORTERS.md §8.3 as part of this card, or as its own card.
8. Generated `docs/` markdown: defer it until a reader outside the explorer and the API asks for it.
