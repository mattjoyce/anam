---
type: component
title: Inspector (internal/inspect)
description: "The web harness over the full fold, the Anam Workbench: timeline, facets with the edges touching them, events, projections and definitions, the default library's bytes confined to its root, and forms that enter, correct and retract a fact by hand."
timestamp: 2026-10-21T00:00:00Z
resource: internal/inspect/inspect.go#Handler
anchor: "func Handler(d Deps) http.Handler {"
basis: code
tier: detail
group: tools
depends_on:
  - /components/fold.md
  - /components/projection.md
  - /components/def.md
  - /components/span.md
  - /components/store-ledger.md
  - /components/mint.md
  - /components/store-ops.md
  - /components/library.md
---

A diagnostic tool that also writes (see "Entering a fact by hand" below), and it keeps the full fold rather than reading from checkpoints. Routes: `/`, `/facets`, `/facet`, `/events`, `/projection`, `/document?sha256=<sum>&name=<file name>`, `/definitions`, `/definition`. `/document` finds the file by its sha256 with `library.Find` and opens it under an `os.Root` on the library, so a symlink that points out gets the same 404 as a sum that is not a sha256 or a file that is not there. The `name`, the base of the reference's `path`, only names the download in `Content-Disposition` (card 150). Mounted inside the daemon as the operator role's route (the inspector shows the whole record unprojected, so no reader or importer token opens it), or standalone with `anam inspect`. The number of historical as-of folds it keeps is `inspect_cached_reads` (4 by default, 0 keeps none), a bound on its memory however many instants a person clicks through (card 151).

Since 2026-10-13 (card 21) the timeline carries a checkpoint strip under the band: one segment per leaf bucket of the mint's layout, sealed, behind or never minted by the one hash comparison of the bucket's value at W against the log's head, computed for the page and dropped. The daemon hands the inspector its read-only ops handle for that; `anam inspect` over a bare ledger draws no strip.

Since 2026-10-13 (card 106) every library document is served in a CSP sandbox with neither scripts nor the origin, and HTML, XHTML, SVG and XML files are downloads rather than inline previews: a document is untrusted material, and the inspector shares its origin with the API, so an active file shown inline could have read the record as the operator.

## Links on the facet page

Since 2026-10-21 (card 104, slice 3) the facet page carries a links section: the live edges touching the facet, each with its kind, which side the facet is on (from, to or both), the moment it was asserted, every facet id on each side as a link, the edge itself as a link, and for a note (kind `about`) its text. The page reads them through the same code the API's `GET /v1/receipts/{hash}/facets/{facet_id}` answers from, `read.LinksOf` over the edge index (`facet_edge` in the operational store), so the page and the API cannot drift. The daemon hands the inspector its read-only ops handle as `Edges`, the way it hands `Checkpoints`, wrapped so that an index a catch-up has found wrong refuses with `inspect.ErrIndexWrong`, the one word of "wrong" the inspector knows: the facet page then refuses as the API's facet route does, and the timeline, which is the ledger's and stays right, keeps the page and draws the checkpoint strip as one red band carrying the refusal, since none of the log can be trusted until the operator rebuilds the store; `anam inspect` over a bare ledger is given none and the page says there is no index to read, which is not the same as no edges. The edges carry their own watermark, the index's cursor when they were read, shown beside the heading, as in the API's answer. The text is the edge facet's own, read from the ledger by the edge's id, since the index holds ends and not words. Links are never written into a facet: the payload stays the entirety of the facet, and the page draws the derived part beside it.

## Entering a fact by hand

Since 2026-10-20 (card 188, slice 4) the inspector is also the Anam Workbench: it can enter a fact, correct one and retract one, through the gate. The daemon hands it a `Workbench` (declared in `internal/inspect/workbench.go`) only when the importers file declares a `workbench` principal; a standalone `anam inspect` is given none and has no entry routes at all. The interface answers the source the daemon enters as, the types it may enter (the principal's `types`, so the pages ask the daemon and never read the file), and the two writes: `Enter`, which ends in the same function `POST /v1/entry` calls, and `Retract`, the same as `POST /v1/facets/{id}/retract`. The read pages gain no write branch; the three routes below are their own handlers.

- `GET /entry?type=<type>` draws the form from the type's Definition in force, at request time. A type off the list is 403. Each schema property becomes one control: a string a text box, a number or integer a number box, a boolean or an `enum` a select, the fact-time field a date or date-and-time box that the server reads in the Definition's zone and writes in its layout. `description` and `examples` are the hint and the placeholder, `minLength` and `exclusiveMinimum` and the like are hints only (the gate checks again), and `handling.guidance` is shown word for word. The field the `identity_template` composes is left out: the gate fills it. A schema with a nested object or a list, such as blood pressure's, gets a plain message and no form.
- `POST /entry` reads the posted fields back through the same schema walk that drew them (`entryform.go`), so a field drawn is a field read, and sends the typed payload with the Definition version the form was drawn from. The answer is the gate's: admitted, superseded, no-op (each linked to the facet), or its refusal words beside the form with everything typed kept. A form drawn from an old version is refused by the gate and drawn again from the version in force.
- `GET /entry?type=<type>&edit=<facet id>` opens the same form filled from a live facet the Workbench entered, with the identity fields (`identity_key` and what the `identity_template` reads) read-only: changing one would name a different fact, so a wrong time is fixed by retracting the entry and entering a new one. Saving writes a new version of the same facet.
- `POST /retract` tombstones one facet of the workbench source after the page's confirm. The facet page shows Edit and Retract on the newest version of a live facet of that source and on no other.

The allow-list is the principal's `types`; the pages draw no form and offer no link from the type's kind, family or classification. The browser guard wraps every one of these routes, since a form post is same-origin and a foreign page's is not.
