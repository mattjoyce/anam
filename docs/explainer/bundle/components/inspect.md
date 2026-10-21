---
type: component
title: Inspector (internal/inspect)
description: "The web harness over the full fold, the Anam Workbench: timeline, facets with the edges touching them, events, projections, the selection builder and definitions, the default library's bytes confined to its root, forms that enter, correct and retract a fact by hand, and the link form that joins facets with words."
timestamp: 2026-10-22T00:00:00Z
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
  - /components/selection.md
  - /components/read.md
  - /components/catchup.md
---

A diagnostic tool that also writes (see "Entering a fact by hand" below), and it keeps the full fold rather than reading from checkpoints. Routes: `/`, `/facets`, `/facet`, `/events`, `/projection`, `/select`, `/document?sha256=<sum>&name=<file name>`, `/definitions`, `/definition`. `/document` finds the file by its sha256 with `library.Find` and opens it under an `os.Root` on the library, so a symlink that points out gets the same 404 as a sum that is not a sha256 or a file that is not there. The `name`, the base of the reference's `path`, only names the download in `Content-Disposition` (card 150). Mounted inside the daemon as the operator role's route (the inspector shows the whole record unprojected, so no reader or importer token opens it), or standalone with `anam inspect`. The number of historical as-of folds it keeps is `inspect_cached_reads` (4 by default, 0 keeps none), a bound on its memory however many instants a person clicks through (card 151).

Since 2026-10-13 (card 21) the timeline carries a checkpoint strip under the band: one segment per leaf bucket of the mint's layout, sealed, behind or never minted by the one hash comparison of the bucket's value at W against the log's head, computed for the page and dropped. The daemon hands the inspector its read-only ops handle for that; `anam inspect` over a bare ledger draws no strip.

Since 2026-10-13 (card 106) every library document is served in a CSP sandbox with neither scripts nor the origin, and HTML, XHTML, SVG and XML files are downloads rather than inline previews: a document is untrusted material, and the inspector shares its origin with the API, so an active file shown inline could have read the record as the operator.

## Links on the facet page

Since 2026-10-21 (card 104, slice 3) the facet page carries a links section: the live edges touching the facet, each with its kind, which side the facet is on (from, to or both), the moment it was asserted, every facet id on each side as a link, the edge itself as a link, and for a note (kind `about`) its text. The page reads them through the same code the API's `GET /v1/receipts/{hash}/facets/{facet_id}` answers from, `read.LinksOf` over the edge index (`facet_edge` in the operational store), so the page and the API cannot drift. The daemon hands the inspector its read-only ops handle as `Edges`, the way it hands `Checkpoints`, wrapped so that an index a catch-up has found wrong refuses with `inspect.ErrIndexWrong`, the one word of "wrong" the inspector knows: the facet page then refuses as the API's facet route does, and the timeline, which is the ledger's and stays right, keeps the page and draws the checkpoint strip as one red band carrying the refusal, since none of the log can be trusted until the operator rebuilds the store; `anam inspect` over a bare ledger is given none and the page says there is no index to read, which is not the same as no edges. The edges carry their own watermark, the index's cursor when they were read, shown beside the heading, as in the API's answer. The text is the edge facet's own, read from the ledger by the edge's id, since the index holds ends and not words. Links are never written into a facet: the payload stays the entirety of the facet, and the page draws the derived part beside it.

## The projection page

Since 2026-10-21 (card 201) `GET /projection?p=<name>` opens on a summary, not on every row. It draws the header (watermark, the canonical and served hashes, the projection and as-of controls) and one line per facet type with three counts, served, excluded and superseded. Served and excluded come from `projection.Explain` over the live value the page already holds, so the verdict words are the ones `anam timeline --explain` gives. Superseded comes from the fold's per-facet state alone: a facet at version n has n-1 earlier versions, and a facet that is no longer live has its current version out of force as well, which is the count `fold.Displaced` lists, reached without reading a version from the ledger. A renamed facet's earlier versions count under its current type, because the fold does not hold what type an earlier version had. A facet that is no longer live has no type in the fold, so those are counted together on a line named `(not live)`. Each count is a link that opens its rows.

The rows view is `?p=<name>&type=<type>&show=served|excluded|superseded`, with `from` and `to` as dates on fact time (both ends included) and `asof` as before. It draws that type's rows of that standing, earliest first, at most 500, and when more match it says "Showing the earliest 500 of N" and asks for a narrower window. Only the superseded standing reads versions from the ledger, through `fold.DisplacedWhere` limited to that type's facets; the page before card 201 read all of them for every projection and drew about 45,000 rows. The as-of control works in both views and the links keep it. An unknown `show` or a date that is not YYYY-MM-DD is a 400.

## The selection page

Since 2026-10-21 (card 199) `GET /select` builds a Datalog selection with the shared filter (the [selection component](/components/selection.md)). The program is the value; the criteria and the boxes are a view of it. Every state is an address, so back and reload keep it:

- `p`, the projection the selection reads through, `canonical` by default, chosen as on the projection page; the value is the one `serve.Derive` serves, the query read's own.
- `from` and `to`, each a date (a `to` date is the whole day) or a whole fact-time, which is how a hand-edited bound off midnight stays a criterion; `type` and `source`, each repeated; `standing`, `live` unless `superseded`.
- `tick`, the basket, repeated; `fromside`, the ticks on the from side (to is the default); `untick`, the exclusions.
- `program`, a hand-edited program, in place of all of the criteria and the basket.

The criteria form adds any criterion in any order, each criterion is a chip with the count the criteria up to it match and a link that removes it, and the matches are listed by type, the readings of one type, source and fact time folded into one row with a box per facet. A box is ticked while the program picks the facet. The boxes post back to the page, which answers with a redirect to the address of the new state: a match left unticked becomes an exclusion, and "keep the ticked in the basket" makes each ticked match a tick, so it stays when the criteria change. With more than 500 rows the page lists none, states the count and asks for another criterion; a derivation over `max_read_bytes` is refused the same way. The program shows below in a textarea. A submitted program that `selection.Parse` reads redirects to the address of its criteria; any other runs as text through `read.Evaluate`, the same evaluation `POST /v1/query` makes without its receipt, within the same `max_read_bytes`, and the page turns the criteria and boxes off and says why. The page only reads. "Use this selection" links to `/link` with the program, the projection, the basket with its sides, and `edit`, the link being edited when the selection was opened from one, which every address of the page carries.

The projection page's rows view selects its type, window and standing through the same `selection.Filter`, so the two pages admit by one predicate.

## Entering a fact by hand

Since 2026-10-20 (card 188, slice 4) the inspector is also the Anam Workbench: it can enter a fact, correct one and retract one, through the gate. The daemon hands it a `Workbench` (declared in `internal/inspect/workbench.go`) only when the importers file declares a `workbench` principal; a standalone `anam inspect` is given none and has no entry routes at all. The interface answers the source the daemon enters as, the types it may enter (the principal's `types`, so the pages ask the daemon and never read the file), and the two writes: `Enter`, which ends in the same function `POST /v1/entry` calls, and `Retract`, the same as `POST /v1/facets/{id}/retract`. The read pages gain no write branch; the three routes below are their own handlers.

- `GET /entry?type=<type>` draws the form from the type's Definition in force, at request time. A type off the list is 403. Each schema property becomes one control: a string a text box, a number or integer a number box, a boolean or an `enum` a select, the fact-time field a date or date-and-time box that the server reads in the Definition's zone and writes in its layout. `description` and `examples` are the hint and the placeholder, `minLength` and `exclusiveMinimum` and the like are hints only (the gate checks again), and `handling.guidance` is shown word for word. The field the `identity_template` composes is left out: the gate fills it. A schema with a nested object or a list, such as blood pressure's, gets a plain message and no form.
- `POST /entry` reads the posted fields back through the same schema walk that drew them (`entryform.go`), so a field drawn is a field read, and sends the typed payload with the Definition version the form was drawn from. The answer is the gate's: admitted, superseded, no-op (each linked to the facet), or its refusal words beside the form with everything typed kept. A form drawn from an old version is refused by the gate and drawn again from the version in force.
- `GET /entry?type=<type>&edit=<facet id>` opens the same form filled from a live facet the Workbench entered, with the identity fields (`identity_key` and what the `identity_template` reads) read-only: changing one would name a different fact, so a wrong time is fixed by retracting the entry and entering a new one. Saving writes a new version of the same facet.
- `POST /retract` tombstones one facet of the workbench source after the page's confirm. The facet page shows Edit and Retract on the newest version of a live facet of that source and on no other.

The allow-list is the principal's `types`; the pages draw no form and offer no link from the type's kind, family or classification. The browser guard wraps every one of these routes, since a form post is same-origin and a foreign page's is not.

## The link form

Since 2026-10-22 (card 202) the Workbench writes a link (`record.edge`) by hand. The entry form cannot draw one, since its sides are lists, so the link form is its own pair of routes, mounted with the entry routes and like them only when the daemon hands over a Workbench, and refused (403) unless the workbench principal's `types` list `record.edge`.

- `GET /link` draws the form. Its query string carries the basket: `tick`, the facet ids, repeated; `fromside`, the ones on the from side; and optionally `program` with `p`, the selection page's program and projection, which the form resolves to facet ids once, when it opens, through `read.Evaluate` within `max_read_bytes`. `kind` chooses the kind of a new link. `edit=<facet id>` opens a live link the Workbench entered, its sides, kind, words, pins and moment filled in; with a basket as well (the selection page handing an edit back) the basket replaces its sides. Each facet shows its type, fact time and a link to its page, a box that keeps it in the link, its side, and "link to this version", which pins the content hash of its current version, read through the ledger's `CurrentVersion`. Pins start off for kind `about` and on for any other. The kind is a select of the Definition's `enum`. The moment, `at`, starts at the browser's now, and the zone and offset are filled from the browser by the same script the dose form uses. A basket that puts a facet on the from side of kind `about` gets a plain message; more than 20 facets draw a confirmation box.
- `POST /link` saves one `record.edge` through `Workbench.Enter`, the dose form's write, so it lands as source `workbench` with the Definition version the form was drawn from. Before the gate, the form refuses (422, nothing written, everything posted kept) a kind off the list, a from side on kind `about`, more than 20 facets unconfirmed, no moment or offset, and a pin whose version is no longer current (it moves the pin to the current version for the person to check). A new link's id is `workbench:edge:` and the first 16 hex digits of the SHA-256 of `kind=<kind>\nfrom=<sorted from ids, comma-joined>\nto=<sorted to ids>\nat=<at><offset>`, computed once at the first save (`edgeID` in `linkform.go`); the words are not in it. Each side is written sorted, so the same link saved twice is the same content and the gate's no-op. An edited link keeps the id it was first saved with, so saving it is a new version of the same facet even when its sides change, and a facet removed from it no longer lists it. `record.edge` declares no `identity_template`, which would compose the id from the edited sides again and make an edit a second link.

The facet page offers "link this…", which opens the selection page with that facet ticked, whenever the Workbench may enter `record.edge`; on a live link the Workbench entered, Edit reads "edit this link" and opens `/link?edit=`; Retract is the same as for any Workbench facet. The Definitions pages' Enter for `record.edge` opens `/link`.
