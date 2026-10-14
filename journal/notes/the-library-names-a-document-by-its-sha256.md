---
id: 202610141800
type: decision
title: "The library names a document by its sha256; the facet keeps its name"
created: 2026-10-14
status: evergreen
tags: [anam, decision, documents, library, identity, api]
project: anam
related: ["[[source-ref-is-identity-not-content]]", "[[verbatim-payload-metadata-separation]]", "[[sqlite-is-the-operational-store]]"]
---

# The library names a document by its sha256; the facet keeps its name

**Decision (Matt, 2026-10-14, card 150).** A stored document's file is `<sha256>.<ext>`. The
`doc.reference` facet keeps the document's name and path as its identity, and names the bytes by
sha256. In Matt's words, "rename the file to its sha256, and store the original name in the facet,
when we retrieve it, we can rename it."

## The tension

Card 102 stored a posted document at the importer's path and answered 409 to different bytes at a
held path. That kept the library from ever overwriting bytes, but it meant a corrected document needed
a new name. It also contradicted the `doc.reference` Definition, which says changed bytes at the same
path supersede the facet.

The name and where the bytes live had been braided into one path. Pulling them apart settles both
demands. The facet's `path` is the name and the identity, so a rescan under the same name supersedes
the facet as the Definition says. The library holds bytes by sha256, so nothing is ever overwritten
and the old bytes stay for the old version. Two names for the same bytes share one file.

Where a file sits was the second question. A computed location would be a place to keep true. Matt:
"the sha should be unique, so just find it. don't use the path, we put the file where we need to, we
promise not to rename it." So no fact and no table records where a file is.

## The rule

- `POST /v1/documents?ext=` takes bytes with no path and answers sha256, bytes, media type and
  whether it was new. The same bytes again are 200. There is no conflict.
- A `doc.reference` is admitted when the library holds bytes with its sha256, under any name.
- A file goes into the deepest existing folder along its sha256 (`a/`, `a/ab/`, and so on). When a
  folder holds `library_folder_limit` files (default 1000), the next goes one level deeper. A stored
  file never moves.
- Lookup finds `<sha256>.*` anywhere in the library. Writes go through the write queue, so one writer
  counts and places.
- The inspector serves a document by sha256, with the facet's file name in `Content-Disposition`.
